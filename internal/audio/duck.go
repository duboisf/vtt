package audio

import (
	"fmt"
	"os/exec"
	"strings"

	"vocis/internal/sessionlog"
)

// Ducker lowers the default audio sink volume while recording and restores it after.
type Ducker struct {
	savedVolume string
	duckLevel   float64
}

// NewDucker creates a ducker that will lower the default sink to the given level (0.0–1.0).
// A duckLevel of 0 disables ducking.
func NewDucker(duckLevel float64) *Ducker {
	return &Ducker{duckLevel: duckLevel}
}

// Duck saves the current default sink volume and lowers it.
func (d *Ducker) Duck() {
	if d.duckLevel <= 0 {
		return
	}

	out, err := exec.Command("wpctl", "get-volume", "@DEFAULT_AUDIO_SINK@").Output()
	if err != nil {
		sessionlog.Warnf("duck: failed to get volume: %v", err)
		return
	}
	d.savedVolume = parseVolume(strings.TrimSpace(string(out)))
	if d.savedVolume == "" {
		return
	}

	if err := exec.Command("wpctl", "set-volume", "@DEFAULT_AUDIO_SINK@", fmt.Sprintf("%.2f", d.duckLevel)).Run(); err != nil {
		sessionlog.Warnf("duck: failed to lower volume: %v", err)
		d.savedVolume = ""
		return
	}
	sessionlog.Infof("ducked audio volume=%s → %.0f%%", d.savedVolume, d.duckLevel*100)
}

// Restore returns the default sink volume to its pre-duck level.
func (d *Ducker) Restore() {
	if d.savedVolume == "" {
		return
	}

	if err := exec.Command("wpctl", "set-volume", "@DEFAULT_AUDIO_SINK@", d.savedVolume).Run(); err != nil {
		sessionlog.Warnf("duck: failed to restore volume: %v", err)
		return
	}
	sessionlog.Infof("restored audio volume=%s", d.savedVolume)
	d.savedVolume = ""
}

// parseVolume extracts the numeric value from wpctl output like "Volume: 0.84".
func parseVolume(output string) string {
	parts := strings.Fields(output)
	for i, part := range parts {
		if part == "Volume:" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
