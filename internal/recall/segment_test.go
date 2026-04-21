package recall

import (
	"testing"
	"time"
)

func TestRing_EvictsByCount(t *testing.T) {
	r := NewRing(3, 0)
	now := time.Now()
	for i := 0; i < 5; i++ {
		r.Add(&Segment{StartedAt: now.Add(time.Duration(i) * time.Second)})
	}
	got := r.List()
	if len(got) != 3 {
		t.Fatalf("expected 3 retained segments, got %d", len(got))
	}
	// Oldest-evicted first: ids 3, 4, 5 should remain.
	if got[0].ID != 3 || got[2].ID != 5 {
		t.Fatalf("unexpected retained ids: %d, %d, %d", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestRing_EvictsByAge(t *testing.T) {
	base := time.Now()
	r := NewRing(0, 5*time.Second)
	r.now = func() time.Time { return base }

	r.Add(&Segment{StartedAt: base.Add(-10 * time.Second)}) // too old
	r.Add(&Segment{StartedAt: base.Add(-2 * time.Second)})  // ok
	r.Add(&Segment{StartedAt: base})                        // ok

	got := r.List()
	if len(got) != 2 {
		t.Fatalf("expected 2 retained segments, got %d", len(got))
	}
	for _, s := range got {
		if base.Sub(s.StartedAt) > 5*time.Second {
			t.Fatalf("segment %d is older than retention (age=%s)", s.ID, base.Sub(s.StartedAt))
		}
	}
}

func TestRing_GetAndDrop(t *testing.T) {
	r := NewRing(10, 0)
	id := r.Add(&Segment{StartedAt: time.Now()})
	if _, err := r.Get(id); err != nil {
		t.Fatalf("Get(%d): %v", id, err)
	}
	r.Drop(id)
	if _, err := r.Get(id); err == nil {
		t.Fatal("expected error after Drop")
	}
}

func TestRing_SetTranscript(t *testing.T) {
	r := NewRing(10, 0)
	id := r.Add(&Segment{StartedAt: time.Now()})
	r.SetTranscript(id, "hello there")
	seg, err := r.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if seg.Transcript != "hello there" {
		t.Fatalf("transcript not cached: %q", seg.Transcript)
	}
}

func TestRing16_PushAndSnapshot(t *testing.T) {
	buf := newRing16(4)
	buf.push([]int16{1, 2, 3})
	if got := buf.snapshot(); !equalInt16(got, []int16{1, 2, 3}) {
		t.Fatalf("unexpected: %v", got)
	}
	buf.push([]int16{4, 5, 6}) // total 6 pushed, capacity 4 → oldest 2 drop
	if got := buf.snapshot(); !equalInt16(got, []int16{3, 4, 5, 6}) {
		t.Fatalf("unexpected after overflow: %v", got)
	}
}

func equalInt16(a, b []int16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
