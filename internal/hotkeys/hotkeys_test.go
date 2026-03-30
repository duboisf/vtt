package hotkeys

import (
	"testing"
	"time"
)

func TestRegistrationEmitsDownAndUp(t *testing.T) {
	t.Parallel()

	r := &Registration{
		down: make(chan struct{}, 1),
		up:   make(chan struct{}, 1),
	}

	r.handlePress()
	expectEvent(t, r.down)

	r.handleRelease()
	expectEventWithin(t, r.up, autoRepeatReleaseDelay+40*time.Millisecond)
}

func TestRegistrationSuppressesAutoRepeatRelease(t *testing.T) {
	t.Parallel()

	r := &Registration{
		down: make(chan struct{}, 1),
		up:   make(chan struct{}, 1),
	}

	r.handlePress()
	expectEvent(t, r.down)

	r.handleRelease()
	time.Sleep(autoRepeatReleaseDelay / 2)
	r.handlePress()
	expectNoEvent(t, r.up, autoRepeatReleaseDelay+40*time.Millisecond)
	expectNoEvent(t, r.down, 40*time.Millisecond)

	r.handleRelease()
	expectEventWithin(t, r.up, autoRepeatReleaseDelay+40*time.Millisecond)
}

func expectEvent(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	expectEventWithin(t, ch, time.Second)
}

func expectEventWithin(t *testing.T, ch <-chan struct{}, timeout time.Duration) {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ch:
	case <-timer.C:
		t.Fatalf("expected event within %s", timeout)
	}
}

func expectNoEvent(t *testing.T, ch <-chan struct{}, timeout time.Duration) {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ch:
		t.Fatal("expected no event")
	case <-timer.C:
	}
}
