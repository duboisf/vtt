package injector

import (
	"testing"

	"vtt/internal/config"
)

func TestTerminalDetectionIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	inj := New(config.Default().Insertion)
	if !inj.isTerminal("alacritty") {
		t.Fatal("expected alacritty to be treated as a terminal")
	}
}
