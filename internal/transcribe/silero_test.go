package transcribe

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveOnnxruntimeLibraryExpandsEnvVars pins the behavior that a
// configured `streaming.onnxruntime_library` path is run through
// os.ExpandEnv before os.Stat, so users can keep literal $HOME /
// $XDG_* forms in their yaml and have them work across machines. The
// auto-discovery list already expands env vars; the configured path
// should be consistent.
func TestResolveOnnxruntimeLibraryExpandsEnvVars(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "libonnxruntime.so")
	if err := os.WriteFile(libPath, []byte("not a real library"), 0o600); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	t.Setenv("VOCIS_TEST_ONNX_DIR", dir)

	resolved, err := resolveOnnxruntimeLibrary("$VOCIS_TEST_ONNX_DIR/libonnxruntime.so")
	if err != nil {
		t.Fatalf("resolveOnnxruntimeLibrary with $VAR: %v", err)
	}
	if resolved != libPath {
		t.Fatalf("resolved = %q, want %q", resolved, libPath)
	}
}

func TestResolveOnnxruntimeLibraryRejectsMissingConfiguredPath(t *testing.T) {
	_, err := resolveOnnxruntimeLibrary("/nonexistent/libonnxruntime.so")
	if err == nil {
		t.Fatal("expected error for missing configured path, got nil")
	}
}

// speechChunk returns `ms` of a 200Hz sine at the given amplitude
// (0-1), as 16 kHz PCM16. Useful for handing Silero synthetic input
// of a known shape in tests.
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
	if snap.LastProb < 0 || snap.LastProb > 1 {
		t.Fatalf("probability %.4f out of [0,1]", snap.LastProb)
	}
	t.Logf("silero probability on 512-sample sine: %.4f", snap.LastProb)
}
