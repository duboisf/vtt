package transcribe

import "math"

// ClientVAD is a simple energy-threshold voice activity detector that runs
// on raw PCM16 samples client-side. It mirrors Lemonade's SimpleVAD
// structure — hysteresis on both ends (min speech before a start is
// declared, min silence before a stop is declared) — but the absolute
// RMS threshold is adaptive, not fixed. A fixed threshold works on
// quiet lab mics but fails on consumer setups where the ambient noise
// floor can sit at 0.20+ RMS (fan, AC, mic preamp). We track a decaying
// noise-floor estimate from the lowest RMS we see and gate speech on
// RMS > floor * vadMargin. The configured `streaming.threshold` acts
// as a lower bound (an absolute hard floor) so the detector never
// declares every sub-whisper a valid utterance.
//
// Used when streaming.manual_commit is on (server-side VAD is off) and
// streaming.client_vad is on: the dictation pump calls Feed for each
// PCM chunk and sends input_audio_buffer.commit whenever it sees
// VADSpeechStopped, so the server transcribes each utterance the moment
// the user pauses instead of waiting for hotkey release.
type ClientVAD struct {
	sampleRate      int
	threshold       float64 // absolute lower bound on effective threshold
	minSilenceMs    int
	minSpeechMs     int
	minUtteranceMs  int     // emit SpeechStopped only if the episode had at least this much speech
	vadMargin       float64 // multiplier on noise floor to declare speech

	inSpeech  bool
	silenceMs int
	speechMs  int

	// Noise-floor tracker. floorInit gates the warm-up period: for the
	// first ~300ms we adopt the minimum RMS seen so far as the floor.
	// After warm-up, the floor drops instantly to any lower RMS (quick
	// adapt when the room gets quieter) and drifts up slowly during
	// louder stretches so a long held utterance doesn't re-train the
	// floor to mid-speech levels.
	noiseFloor    float64
	floorWarmupMs int
	floorInit     bool

	// Rolling stats for diagnostics: the last RMS we measured, and the
	// min/max over the current silence window. Callers can snapshot
	// these to log what ambient noise vs speech looks like in the
	// real environment, which is the only way to verify tuning.
	// lastMean exposes any DC offset — if it's far from zero, the
	// mic is biased and we'd be computing garbage without DC removal.
	lastRMS   float64
	lastMean  float64
	minRMS    float64
	maxRMS    float64
	sampleCnt int
}

// VADEvent is the state transition reported by Feed. VADNone is returned
// when Feed didn't change the in-speech state.
type VADEvent int

const (
	VADNone VADEvent = iota
	VADSpeechStarted
	VADSpeechStopped
)

// NewClientVAD builds a detector.
//   - threshold: absolute lower bound on the effective speech threshold
//     (0-1 RMS on normalized samples).
//   - minSilenceMs: silence hold before declaring SpeechStopped.
//   - minSpeechMs: speech hold before declaring SpeechStarted.
//   - minUtteranceMs: total speech an episode needs to accumulate before
//     a silence transition is allowed to emit SpeechStopped. Shorter
//     episodes reset silently and roll into the next utterance, which
//     is what you want for Whisper — it produces unreliable output on
//     segments under ~1s.
//   - vadMargin: multiplier on the learned noise floor (3.0 default:
//     speech needs to be ~3x louder than ambient).
func NewClientVAD(sampleRate int, threshold float64, minSilenceMs, minSpeechMs, minUtteranceMs int, vadMargin float64) *ClientVAD {
	if vadMargin <= 0 {
		vadMargin = 3.0
	}
	return &ClientVAD{
		sampleRate:     sampleRate,
		threshold:      threshold,
		minSilenceMs:   minSilenceMs,
		minSpeechMs:    minSpeechMs,
		minUtteranceMs: minUtteranceMs,
		vadMargin:      vadMargin,
	}
}

const (
	// floorWarmupTargetMs is how long we collect RMS minima before
	// trusting the noise floor — long enough to skip the button-click
	// transient, short enough that the first utterance isn't missed.
	floorWarmupTargetMs = 300

	// Floor drift rate during speech. At 0.0005 per chunk (~50ms) the
	// floor takes ~30 seconds to drift halfway to a higher value, which
	// is slower than any single utterance.
	floorDriftRate = 0.0005
)

// effectiveThreshold returns the current speech-gating RMS. It is the
// max of the configured absolute threshold and floor * margin. During
// warm-up the floor estimate isn't trusted yet, so we fall back to the
// configured threshold.
func (v *ClientVAD) effectiveThreshold() float64 {
	if !v.floorInit {
		return v.threshold
	}
	adaptive := v.noiseFloor * v.vadMargin
	if adaptive > v.threshold {
		return adaptive
	}
	return v.threshold
}

// Feed processes one PCM16 chunk and returns a VAD event if the in-speech
// state changed on this chunk. Chunks can be any length; timing is
// accumulated in milliseconds derived from chunk length and sample rate.
func (v *ClientVAD) Feed(samples []int16) VADEvent {
	if len(samples) == 0 || v.sampleRate <= 0 {
		return VADNone
	}
	stats := signalStats(samples)
	rms := stats.RMS
	durationMs := len(samples) * 1000 / v.sampleRate

	v.lastRMS = rms
	v.lastMean = stats.Mean
	if v.sampleCnt == 0 || rms < v.minRMS {
		v.minRMS = rms
	}
	if rms > v.maxRMS {
		v.maxRMS = rms
	}
	v.sampleCnt++

	v.updateNoiseFloor(rms, durationMs)

	// Don't run the state machine during warm-up: the floor estimate
	// isn't trusted yet, and the configured absolute threshold will
	// usually be below ambient on noisy setups, which would falsely
	// declare speech and then fire a stop on the first post-warm-up
	// chunk when the effective threshold jumps up. Warm-up takes
	// ~300ms — inconsequential for dictation latency.
	if !v.floorInit {
		return VADNone
	}

	effective := v.effectiveThreshold()
	if rms >= effective {
		v.speechMs += durationMs
		v.silenceMs = 0
		if !v.inSpeech && v.speechMs >= v.minSpeechMs {
			v.inSpeech = true
			return VADSpeechStarted
		}
		return VADNone
	}

	v.silenceMs += durationMs
	if v.inSpeech && v.silenceMs >= v.minSilenceMs {
		// Capture the episode duration before we reset — short
		// utterances (< minUtteranceMs) don't emit SpeechStopped, so
		// callers won't commit a stub segment that Whisper would
		// transcribe unreliably. The detector just goes quiet and
		// lets the next utterance take over.
		episodeMs := v.speechMs
		v.inSpeech = false
		v.speechMs = 0
		if episodeMs < v.minUtteranceMs {
			return VADNone
		}
		return VADSpeechStopped
	}
	// Decay the speech counter during sustained silence so a later blip
	// has to re-earn the minSpeechMs floor.
	if !v.inSpeech && v.silenceMs > 200 {
		v.speechMs = 0
	}
	return VADNone
}

// updateNoiseFloor maintains a live estimate of ambient RMS. During the
// warm-up window it tracks the running minimum (so a brief startup
// transient doesn't anchor the floor too high). After warm-up it snaps
// down to any lower RMS immediately (quick adapt when the room quiets)
// and drifts up very slowly during sustained higher RMS — the drift is
// slow enough that a single utterance won't retrain the floor to
// speech levels.
func (v *ClientVAD) updateNoiseFloor(rms float64, durationMs int) {
	if !v.floorInit {
		v.floorWarmupMs += durationMs
		if v.noiseFloor == 0 || rms < v.noiseFloor {
			v.noiseFloor = rms
		}
		if v.floorWarmupMs >= floorWarmupTargetMs {
			v.floorInit = true
		}
		return
	}
	if rms < v.noiseFloor {
		v.noiseFloor = rms
		return
	}
	v.noiseFloor += (rms - v.noiseFloor) * floorDriftRate
}

// InSpeech exposes the internal flag so callers can decide whether to
// suppress commits (no speech since last commit → skip).
func (v *ClientVAD) InSpeech() bool { return v.inSpeech }

// Snapshot returns current VAD internals for diagnostic logging. The
// dictation pump samples this periodically so the session log shows
// exactly what the detector is seeing — indispensable for tuning
// threshold against real-world mic noise.
type VADSnapshot struct {
	InSpeech     bool
	LastRMS      float64
	LastMean     float64 // DC component; large non-zero = mic has DC offset
	MinRMS       float64
	MaxRMS       float64
	NoiseFloor   float64
	EffectiveThr float64
	SilenceMs    int
	SpeechMs     int
}

func (v *ClientVAD) Snapshot() VADSnapshot {
	return VADSnapshot{
		InSpeech:     v.inSpeech,
		LastRMS:      v.lastRMS,
		LastMean:     v.lastMean,
		MinRMS:       v.minRMS,
		MaxRMS:       v.maxRMS,
		NoiseFloor:   v.noiseFloor,
		EffectiveThr: v.effectiveThreshold(),
		SilenceMs:    v.silenceMs,
		SpeechMs:     v.speechMs,
	}
}

// Reset clears accumulated state. Call after sending a commit so the
// next utterance starts fresh. The learned noise floor is intentionally
// preserved across resets — room conditions don't change because we
// committed.
func (v *ClientVAD) Reset() {
	v.inSpeech = false
	v.silenceMs = 0
	v.speechMs = 0
	v.minRMS = 0
	v.maxRMS = 0
	v.sampleCnt = 0
}

// rmsEnergy returns DC-corrected RMS of int16 samples normalized into the
// [0,1] range, matching Lemonade's server-side scale (float32 PCM /
// 32768). We subtract the chunk mean before summing squares — some
// consumer mics ship samples with a significant DC offset (a constant
// bias added to every sample by the preamp) that would otherwise
// dominate RMS and make it look identical whether the user is speaking
// or silent. See `signalStats` when diagnosing unexpected RMS values.
func rmsEnergy(samples []int16) float64 {
	stats := signalStats(samples)
	return stats.RMS
}

// SignalStats bundles mean (DC component) and DC-corrected RMS. Exposed
// for diagnostic logging when RMS behaves unexpectedly on a user's mic.
type SignalStats struct {
	Mean float64 // sample mean normalized to [-1,1]
	RMS  float64 // DC-corrected RMS, normalized to [0,1]
}

func signalStats(samples []int16) SignalStats {
	if len(samples) == 0 {
		return SignalStats{}
	}
	var meanSum float64
	for _, s := range samples {
		meanSum += float64(s) / 32768.0
	}
	mean := meanSum / float64(len(samples))

	var sqSum float64
	for _, s := range samples {
		v := float64(s)/32768.0 - mean
		sqSum += v * v
	}
	return SignalStats{
		Mean: mean,
		RMS:  math.Sqrt(sqSum / float64(len(samples))),
	}
}
