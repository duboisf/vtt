// Package recall implements the `vocis recall` always-on dictation mode:
// a daemon that captures microphone audio continuously, segments it with
// Silero VAD, and keeps a bounded ring buffer of speech episodes. Users
// browse the ring buffer via `vocis recall pick` and transcribe any
// segment on demand.
//
// See docs/overview.md for the user-facing description.
package recall

import (
	"fmt"
	"sync"
	"time"
)

// Segment is a single VAD-bounded speech episode held in the ring
// buffer. PCM is mono int16 at SampleRate. Transcript is populated on
// first transcribe request and cached so repeat picks are free.
type Segment struct {
	ID         int64
	StartedAt  time.Time
	Duration   time.Duration
	SampleRate int
	PCM        []int16
	PeakLevel  float64 // 0-1, peak absolute sample during the segment

	// Cached transcript. Written once by the daemon after a successful
	// transcribe request; read under the ring buffer lock.
	Transcript string
}

// Ring is a time- and count-bounded segment store. It is safe for
// concurrent use. Eviction happens on Add and on every read — old
// segments drop out even if nothing new is being added.
type Ring struct {
	mu               sync.Mutex
	segments         []*Segment
	nextID           int64
	maxSegments      int
	retention        time.Duration
	totalSeen        int64 // monotonic count for diagnostics
	now              func() time.Time
}

// NewRing constructs a ring bounded by count and/or retention window.
// Either bound can be zero to disable that axis, but at least one must
// be set — the config validator already enforces this; this constructor
// simply trusts the caller.
func NewRing(maxSegments int, retention time.Duration) *Ring {
	return &Ring{
		maxSegments: maxSegments,
		retention:   retention,
		now:         time.Now,
	}
}

// Add installs a fully-captured segment and returns its assigned ID.
// Older segments are evicted to satisfy the count + retention bounds.
func (r *Ring) Add(seg *Segment) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	seg.ID = r.nextID
	r.segments = append(r.segments, seg)
	r.totalSeen++

	r.evictLocked()
	return seg.ID
}

// List returns a shallow copy of currently-retained segments, oldest
// first. PCM slices are shared — callers must treat them as read-only.
func (r *Ring) List() []*Segment {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.evictLocked()
	out := make([]*Segment, len(r.segments))
	copy(out, r.segments)
	return out
}

// Get returns the segment with the given ID, or an error if evicted or
// never existed.
func (r *Ring) Get(id int64) (*Segment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.evictLocked()
	for _, s := range r.segments {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, fmt.Errorf("segment %d not found (evicted or unknown)", id)
}

// Drop removes a segment by ID. Not an error if it was already evicted.
func (r *Ring) Drop(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, s := range r.segments {
		if s.ID == id {
			r.segments = append(r.segments[:i], r.segments[i+1:]...)
			return
		}
	}
}

// SetTranscript caches a transcript on the segment with the given ID.
// No-op if the segment has already been evicted.
func (r *Ring) SetTranscript(id int64, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, s := range r.segments {
		if s.ID == id {
			s.Transcript = text
			return
		}
	}
}

// Stats is a point-in-time view for status reporting.
type Stats struct {
	Count       int
	TotalSeen   int64
	OldestAge   time.Duration
	NewestAge   time.Duration
	TotalFrames int64
}

func (r *Ring) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.evictLocked()
	st := Stats{Count: len(r.segments), TotalSeen: r.totalSeen}
	if len(r.segments) > 0 {
		now := r.now()
		st.OldestAge = now.Sub(r.segments[0].StartedAt)
		st.NewestAge = now.Sub(r.segments[len(r.segments)-1].StartedAt)
		for _, s := range r.segments {
			st.TotalFrames += int64(len(s.PCM))
		}
	}
	return st
}

// evictLocked drops segments that fall outside the count or retention
// bound. Caller must hold r.mu.
func (r *Ring) evictLocked() {
	if r.retention > 0 {
		cutoff := r.now().Add(-r.retention)
		for len(r.segments) > 0 && r.segments[0].StartedAt.Before(cutoff) {
			r.segments = r.segments[1:]
		}
	}
	if r.maxSegments > 0 && len(r.segments) > r.maxSegments {
		drop := len(r.segments) - r.maxSegments
		r.segments = r.segments[drop:]
	}
}
