package config

import "testing"

func TestDefaultConfigIsValid(t *testing.T) {
	t.Parallel()

	if err := Default().Validate(); err != nil {
		t.Fatalf("validate default config: %v", err)
	}
}

func TestConfigRejectsInvalidHotkeyMode(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.HotkeyMode = "press"

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid hotkey mode to be rejected")
	}
}
