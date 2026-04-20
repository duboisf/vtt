package transcribe

import (
	"math"
	"testing"
)

// silenceChunk returns `ms` of 16kHz PCM16 zeros.
func silenceChunk(ms int) []int16 {
	return make([]int16, 16*ms)
}

// speechChunk returns `ms` of a 200Hz sine at the given amplitude (0-1),
// as 16kHz PCM16. Amplitude is normalized — 0.1 ≈ quiet speech on
// Lemonade's RMS scale.
func speechChunk(ms int, amplitude float64) []int16 {
	n := 16 * ms
	out := make([]int16, n)
	for i := 0; i < n; i++ {
		t := float64(i) / 16000.0
		v := amplitude * math.Sin(2*math.Pi*200*t)
		out[i] = int16(v * 32767)
	}
	return out
}

// primeQuietWarmup feeds near-silent chunks until the VAD warm-up
// completes, so follow-up tests operate in steady state with a
// trustworthy (~0) noise floor. Returns once Snapshot reports the
// warm-up is done.
func primeQuietWarmup(t *testing.T, vad *ClientVAD) {
	t.Helper()
	for i := 0; i < 10; i++ {
		vad.Feed(speechChunk(100, 0.001))
		if vad.Snapshot().EffectiveThr > 0 && vad.Snapshot().NoiseFloor < 0.01 {
			// Warm-up may still be running on early iterations; the
			// cheap check is whether effectiveThreshold reflects the
			// floor (floor > 0 means post-warmup; but floor can be 0
			// on absolute silence). Keep priming until at least
			// 400ms have elapsed.
			if i >= 3 {
				return
			}
		}
	}
}

func TestClientVAD_SpeechStartedAfterMinSpeech(t *testing.T) {
	t.Parallel()

	vad := NewClientVAD(16000, 0.02, 300, 100, 0, 3.0)
	primeQuietWarmup(t, vad)

	// 50ms of speech at amplitude 0.3 — above threshold but below the
	// 100ms minSpeechMs floor. Should not trigger yet.
	if evt := vad.Feed(speechChunk(50, 0.3)); evt != VADNone {
		t.Fatalf("first chunk: got %v, want VADNone", evt)
	}
	// Another 60ms — now past 100ms cumulative, should fire SpeechStarted.
	if evt := vad.Feed(speechChunk(60, 0.3)); evt != VADSpeechStarted {
		t.Fatalf("second chunk: got %v, want VADSpeechStarted", evt)
	}
	// Third chunk while still in speech: no event.
	if evt := vad.Feed(speechChunk(50, 0.3)); evt != VADNone {
		t.Fatalf("third chunk: got %v, want VADNone", evt)
	}
}

func TestClientVAD_SpeechStoppedAfterSilence(t *testing.T) {
	t.Parallel()

	vad := NewClientVAD(16000, 0.02, 300, 100, 0, 3.0)
	primeQuietWarmup(t, vad)

	// Get into speech first.
	vad.Feed(speechChunk(200, 0.3))
	if !vad.InSpeech() {
		t.Fatal("should be in speech after 200ms loud signal")
	}

	// 200ms of silence — not yet past 300ms floor.
	if evt := vad.Feed(silenceChunk(200)); evt != VADNone {
		t.Fatalf("short silence: got %v, want VADNone", evt)
	}
	// Another 150ms — cumulative 350ms, should fire SpeechStopped.
	if evt := vad.Feed(silenceChunk(150)); evt != VADSpeechStopped {
		t.Fatalf("long silence: got %v, want VADSpeechStopped", evt)
	}
	if vad.InSpeech() {
		t.Fatal("should be out of speech after SpeechStopped")
	}
}

func TestClientVAD_BriefPauseDoesNotStop(t *testing.T) {
	t.Parallel()

	vad := NewClientVAD(16000, 0.02, 300, 100, 0, 3.0)
	primeQuietWarmup(t, vad)

	// Get into speech.
	vad.Feed(speechChunk(200, 0.3))

	// Inter-word pause ~150ms, then speech resumes.
	if evt := vad.Feed(silenceChunk(150)); evt != VADNone {
		t.Fatalf("brief pause: got %v, want VADNone", evt)
	}
	if evt := vad.Feed(speechChunk(100, 0.3)); evt != VADNone {
		t.Fatalf("resumed speech: got %v, want VADNone", evt)
	}
	if !vad.InSpeech() {
		t.Fatal("brief pause should not exit speech")
	}
}

func TestClientVAD_BelowThresholdCountsAsSilence(t *testing.T) {
	t.Parallel()

	vad := NewClientVAD(16000, 0.05, 300, 100, 0, 3.0)

	// Amplitude 0.01 → RMS ~0.007, below 0.05 absolute threshold and
	// adaptive floor is also low → effective threshold stays at 0.05.
	// Should not advance the speech counter.
	for i := 0; i < 10; i++ {
		if evt := vad.Feed(speechChunk(100, 0.01)); evt != VADNone {
			t.Fatalf("quiet chunk %d: got %v, want VADNone", i, evt)
		}
	}
	if vad.InSpeech() {
		t.Fatal("should not enter speech on sub-threshold signal")
	}
}

// TestClientVAD_AdaptsToNoisyFloor simulates a noisy mic (floor ~0.20
// RMS, speech peaks ~0.71) with an absolute threshold of 0.02 that is
// well below the floor. Without adaptive noise tracking, everything
// would look like speech and SpeechStopped would never fire. With
// adaptive tracking, speech-vs-silence is gated on floor * margin so
// real pauses are detected even on noisy setups.
func TestClientVAD_AdaptsToNoisyFloor(t *testing.T) {
	t.Parallel()

	vad := NewClientVAD(16000, 0.02, 300, 100, 0, 3.0)

	// Prime the noise floor with 400ms of noisy "silence" (amp 0.28
	// → RMS ~0.20). Warm-up is 300ms so after this the floor is
	// trusted.
	for i := 0; i < 4; i++ {
		vad.Feed(speechChunk(100, 0.28))
	}
	snap := vad.Snapshot()
	if snap.NoiseFloor < 0.15 || snap.NoiseFloor > 0.25 {
		t.Fatalf("noise floor = %.3f, want ~0.20", snap.NoiseFloor)
	}
	if snap.EffectiveThr < 0.55 || snap.EffectiveThr > 0.65 {
		t.Fatalf("effective threshold = %.3f, want ~0.60 (floor * 3)", snap.EffectiveThr)
	}

	// Continue ambient noise — must NOT count as speech under adaptive
	// gating even though RMS(0.20) >> absolute threshold(0.02).
	for i := 0; i < 10; i++ {
		if evt := vad.Feed(speechChunk(100, 0.28)); evt != VADNone {
			t.Fatalf("ambient chunk %d: got %v, want VADNone", i, evt)
		}
	}
	if vad.InSpeech() {
		t.Fatal("ambient noise should not register as speech under adaptive gating")
	}

	// Real speech at amp 1.0 → RMS ~0.71, well above effective 0.60.
	// minSpeechMs=100 so the first 100ms chunk fires SpeechStarted.
	if evt := vad.Feed(speechChunk(100, 1.0)); evt != VADSpeechStarted {
		t.Fatalf("loud chunk: got %v, want VADSpeechStarted", evt)
	}
	vad.Feed(speechChunk(50, 1.0)) // sustain speech

	// Back to ambient → should fire SpeechStopped within minSilenceMs.
	var stopped bool
	for i := 0; i < 5; i++ {
		if vad.Feed(speechChunk(100, 0.28)) == VADSpeechStopped {
			stopped = true
			break
		}
	}
	if !stopped {
		t.Fatal("should have fired SpeechStopped after ambient stretch")
	}
}

// TestClientVAD_ShortUtteranceDoesNotEmitStop confirms that the
// min_utterance_ms gate suppresses SpeechStopped for speech episodes
// that were too short. The silence transition still resets the
// detector (so the next real utterance starts cleanly) — it just
// doesn't surface an event, so the pump doesn't commit a stub.
func TestClientVAD_ShortUtteranceDoesNotEmitStop(t *testing.T) {
	t.Parallel()

	vad := NewClientVAD(16000, 0.02, 300, 100, 1000, 3.0)
	primeQuietWarmup(t, vad)

	// 400ms of speech — starts speech (at minSpeechMs=100) but total
	// episode is below minUtteranceMs=1000.
	if evt := vad.Feed(speechChunk(400, 0.3)); evt != VADSpeechStarted {
		t.Fatalf("speech kick-off: got %v, want VADSpeechStarted", evt)
	}

	// Now silence for 400ms (past minSilence=300) — normally would fire
	// SpeechStopped, but episode was too short so VAD suppresses it.
	var sawStop bool
	for i := 0; i < 5; i++ {
		if vad.Feed(silenceChunk(100)) == VADSpeechStopped {
			sawStop = true
		}
	}
	if sawStop {
		t.Fatal("short utterance must not emit SpeechStopped (would commit a stub)")
	}
	if vad.InSpeech() {
		t.Fatal("detector should have reset to not-in-speech even without event")
	}
}

// TestClientVAD_LongUtteranceDoesEmitStop is the positive control —
// same setup but the episode exceeds min_utterance_ms.
func TestClientVAD_LongUtteranceDoesEmitStop(t *testing.T) {
	t.Parallel()

	vad := NewClientVAD(16000, 0.02, 300, 100, 1000, 3.0)
	primeQuietWarmup(t, vad)

	// Accumulate 1200ms of speech, comfortably above the 1000ms gate.
	vad.Feed(speechChunk(500, 0.3))
	vad.Feed(speechChunk(500, 0.3))
	vad.Feed(speechChunk(200, 0.3))
	if !vad.InSpeech() {
		t.Fatal("should be in speech after 1200ms")
	}

	var sawStop bool
	for i := 0; i < 5; i++ {
		if vad.Feed(silenceChunk(100)) == VADSpeechStopped {
			sawStop = true
			break
		}
	}
	if !sawStop {
		t.Fatal("long utterance should emit SpeechStopped")
	}
}

func TestClientVAD_ResetClearsState(t *testing.T) {
	t.Parallel()

	vad := NewClientVAD(16000, 0.02, 300, 100, 0, 3.0)
	primeQuietWarmup(t, vad)
	vad.Feed(speechChunk(200, 0.3))
	if !vad.InSpeech() {
		t.Fatal("pre-reset: should be in speech")
	}
	vad.Reset()
	if vad.InSpeech() {
		t.Fatal("post-reset: should not be in speech")
	}
	// Short speech after reset must re-earn minSpeechMs.
	if evt := vad.Feed(speechChunk(50, 0.3)); evt != VADNone {
		t.Fatalf("post-reset short speech: got %v, want VADNone", evt)
	}
}

func TestClientVAD_EmptyFeedNoOp(t *testing.T) {
	t.Parallel()

	vad := NewClientVAD(16000, 0.02, 300, 100, 0, 3.0)
	if evt := vad.Feed(nil); evt != VADNone {
		t.Fatalf("nil: got %v", evt)
	}
	if evt := vad.Feed([]int16{}); evt != VADNone {
		t.Fatalf("empty: got %v", evt)
	}
}

func TestRMSEnergy(t *testing.T) {
	t.Parallel()

	// Silence → 0
	if got := rmsEnergy(make([]int16, 1000)); got != 0 {
		t.Fatalf("silence rms = %v, want 0", got)
	}
	// Full-scale square wave → ~1.0
	full := make([]int16, 1000)
	for i := range full {
		if i%2 == 0 {
			full[i] = 32767
		} else {
			full[i] = -32767
		}
	}
	got := rmsEnergy(full)
	if got < 0.99 || got > 1.01 {
		t.Fatalf("full-scale rms = %v, want ~1.0", got)
	}
}
