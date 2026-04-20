package transcribe

import (
	"fmt"

	"vocis/internal/config"
	"vocis/internal/sessionlog"
)

// buildVAD constructs the client-side VAD chosen by streaming.VADBackend.
// "" and "rms" produce an energy-threshold detector; "silero" produces
// the neural detector (which lazily initializes the ONNX runtime on
// first use). Returns an error the caller can surface to the user —
// falling back silently would hide misconfigurations.
func buildVAD(streaming config.StreamingConfig, sampleRate int) (VAD, error) {
	switch streaming.VADBackend {
	case "", "rms":
		v := NewClientVAD(
			sampleRate,
			streaming.Threshold,
			streaming.SilenceDurationMS,
			streaming.PrefixPaddingMS,
			streaming.MinUtteranceMS,
			0, // vadMargin: use default
		)
		sessionlog.Infof(
			"client VAD (rms): abs_threshold=%.3f silence=%dms prefix=%dms min_utterance=%dms",
			streaming.Threshold,
			streaming.SilenceDurationMS,
			streaming.PrefixPaddingMS,
			streaming.MinUtteranceMS,
		)
		return v, nil

	case "silero":
		if err := initSilero(streaming.OnnxruntimeLibrary); err != nil {
			return nil, fmt.Errorf("init silero: %w", err)
		}
		// Silero operates on 16 kHz internally; the caller's
		// sampleRate should also be 16 kHz. Log a warning if
		// anything else is in play (transcription will still work,
		// but the VAD windowing assumes 16k).
		if sampleRate != sileroSampleRate {
			sessionlog.Warnf("silero VAD expects 16 kHz but sampleRate=%d; results may be off", sampleRate)
		}
		v, err := NewSileroVAD(
			streaming.SilenceDurationMS,
			streaming.PrefixPaddingMS,
			streaming.MinUtteranceMS,
		)
		if err != nil {
			return nil, fmt.Errorf("new silero vad: %w", err)
		}
		sessionlog.Infof(
			"client VAD (silero): silence=%dms prefix=%dms min_utterance=%dms",
			streaming.SilenceDurationMS,
			streaming.PrefixPaddingMS,
			streaming.MinUtteranceMS,
		)
		return v, nil

	default:
		return nil, fmt.Errorf("unknown vad_backend %q", streaming.VADBackend)
	}
}
