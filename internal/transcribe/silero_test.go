package transcribe

import (
	"os"
	"testing"
)

// TestSileroLoadAndInfer is a smoke test: load the ONNX Runtime
// shared library from the standard user path, load the embedded
// Silero model bytes, and run one inference on synthetic audio.
// Passes if the probability comes back finite and in [0,1].
// Skipped when libonnxruntime.so isn't available on the host.
func TestSileroLoadAndInfer(t *testing.T) {
	libPath := os.ExpandEnv("$HOME/opt/onnxruntime/lib/libonnxruntime.so")
	if _, err := os.Stat(libPath); err != nil {
		t.Skipf("libonnxruntime.so not at %s: %v", libPath, err)
	}

	if err := initSilero(libPath); err != nil {
		t.Fatalf("initSilero: %v", err)
	}

	vad, err := NewSileroVAD(400, 100, 0)
	if err != nil {
		t.Fatalf("NewSileroVAD: %v", err)
	}
	defer vad.Destroy()

	// Feed a 512-sample window of loud sine → probability should be
	// finite and within [0,1]. (Silero may or may not call this
	// "speech" — a pure sine is unusual input — but the output must
	// be a valid probability.)
	vad.Feed(speechChunk(32, 0.5)) // 32 ms = 512 samples at 16 kHz

	snap := vad.Snapshot()
	if snap.LastRMS < 0 || snap.LastRMS > 1 {
		t.Fatalf("probability %.4f out of [0,1]", snap.LastRMS)
	}
	t.Logf("silero probability on 512-sample sine: %.4f", snap.LastRMS)
}
