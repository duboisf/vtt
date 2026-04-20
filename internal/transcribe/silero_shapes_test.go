package transcribe

import (
	"os"
	"testing"

	ort "github.com/yalue/onnxruntime_go"
)

// TestSileroModelShapes prints the model's input/output specs so we can
// verify what Silero expects (state shape, audio frame size). Skips
// when libonnxruntime.so isn't available. Diagnostic — not an
// invariant check.
func TestSileroModelShapes(t *testing.T) {
	libPath := os.ExpandEnv("$HOME/opt/onnxruntime/lib/libonnxruntime.so")
	if _, err := os.Stat(libPath); err != nil {
		t.Skipf("libonnxruntime.so not at %s: %v", libPath, err)
	}
	ort.SetSharedLibraryPath(libPath)
	if err := ort.InitializeEnvironment(); err != nil {
		t.Fatalf("init: %v", err)
	}

	inputs, outputs, err := ort.GetInputOutputInfoWithONNXData(sileroModelBytes)
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	for _, i := range inputs {
		t.Logf("INPUT  %s: shape=%v dtype=%d", i.Name, i.Dimensions, i.DataType)
	}
	for _, o := range outputs {
		t.Logf("OUTPUT %s: shape=%v dtype=%d", o.Name, o.Dimensions, o.DataType)
	}
}
