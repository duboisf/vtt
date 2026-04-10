package x11

import (
	"testing"
	"time"

	"vocis/internal/app"
	"vocis/internal/hotkey"
)

// Compile-time check: Registration must satisfy app.HotkeySource.
var _ app.HotkeySource = (*Registration)(nil)

func TestRegistrationExposesTapFromState(t *testing.T) {
	t.Parallel()

	// Create a Registration with an embedded State (no real X11 connection).
	r := &Registration{
		State: hotkey.NewState("ctrl+shift+space", nil),
	}

	// Press → Down.
	r.HandlePress()
	select {
	case <-r.Down():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected Down event")
	}

	// Release + press → Tap (not a new Down).
	r.HandleRelease()
	r.HandlePress()
	select {
	case <-r.Tap():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected Tap event after release+press")
	}

	// No extra Down should have been emitted.
	select {
	case <-r.Down():
		t.Fatal("unexpected extra Down")
	default:
	}
}

func TestReleaseKeyNamesIncludesModifiersAndTriggerKey(t *testing.T) {
	t.Parallel()

	got, err := hotkey.ReleaseKeyNames("ctrl+shift+space")
	if err != nil {
		t.Fatalf("ReleaseKeyNames: %v", err)
	}

	want := []string{"Control_L", "Control_R", "Shift_L", "Shift_R", "space"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d; got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q; got=%v", i, got[i], want[i], got)
		}
	}
}
