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

	"vocis/internal/sessionlog"
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
//
// An optional Persister mirrors mutations to disk; when nil the ring
// is memory-only (the default). Persister calls happen *outside* the
// ring's lock so slow I/O doesn't block the capture loop — the lock is
// held only long enough to collect evicted segments into a slice.
type Ring struct {
	mu          sync.Mutex
	segments    []*Segment
	nextID      int64
	maxSegments int
	retention   time.Duration
	totalSeen   int64
	now         func() time.Time
	persister   Persister
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

// SetPersister attaches a persister that mirrors mutations to disk.
// Intended for daemon setup — in practice called once after Reload and
// before the capture loop starts. Passing nil disables persistence.
func (r *Ring) SetPersister(p Persister) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.persister = p
}

// Reload replaces the ring contents with the given segments (assumed
// chronological, oldest-first) and advances nextID past the largest
// loaded id. Does NOT enforce retention or max count — callers filter
// beforehand so any disk cleanup can be paired with the in-memory drop.
// Does not trigger persister writes; the segments are already on disk.
func (r *Ring) Reload(segs []*Segment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.segments = append(r.segments[:0], segs...)
	r.nextID = 0
	for _, s := range segs {
		if s.ID > r.nextID {
			r.nextID = s.ID
		}
	}
	r.totalSeen = int64(len(segs))
}

// Add installs a fully-captured segment and returns its assigned ID.
// Older segments are evicted to satisfy the count + retention bounds.
// If a persister is attached, the new segment is saved and any evicted
// files are deleted — both outside the lock.
func (r *Ring) Add(seg *Segment) int64 {
	r.mu.Lock()
	r.nextID++
	seg.ID = r.nextID
	r.segments = append(r.segments, seg)
	r.totalSeen++
	evicted := r.evictLocked()
	p := r.persister
	r.mu.Unlock()

	if p != nil {
		if err := p.Save(seg); err != nil {
			sessionlog.Warnf("recall: persist segment %d: %v", seg.ID, err)
		}
		deleteEvicted(p, evicted)
	}
	return seg.ID
}

// List returns a shallow copy of currently-retained segments, oldest
// first. PCM slices are shared — callers must treat them as read-only.
// Evicted segments' files are deleted as a side effect.
func (r *Ring) List() []*Segment {
	r.mu.Lock()
	evicted := r.evictLocked()
	out := make([]*Segment, len(r.segments))
	copy(out, r.segments)
	p := r.persister
	r.mu.Unlock()

	if p != nil {
		deleteEvicted(p, evicted)
	}
	return out
}

// Get returns the segment with the given ID, or an error if evicted or
// never existed.
func (r *Ring) Get(id int64) (*Segment, error) {
	r.mu.Lock()
	evicted := r.evictLocked()
	var found *Segment
	for _, s := range r.segments {
		if s.ID == id {
			found = s
			break
		}
	}
	p := r.persister
	r.mu.Unlock()

	if p != nil {
		deleteEvicted(p, evicted)
	}
	if found == nil {
		return nil, fmt.Errorf("segment %d not found (evicted or unknown)", id)
	}
	return found, nil
}

// Drop removes a segment by ID. Not an error if it was already evicted.
func (r *Ring) Drop(id int64) {
	r.mu.Lock()
	var dropped bool
	for i, s := range r.segments {
		if s.ID == id {
			r.segments = append(r.segments[:i], r.segments[i+1:]...)
			dropped = true
			break
		}
	}
	p := r.persister
	r.mu.Unlock()

	if dropped && p != nil {
		if err := p.Delete(id); err != nil {
			sessionlog.Warnf("recall: delete persisted segment %d: %v", id, err)
		}
	}
}

// SetTranscript caches a transcript on the segment with the given ID.
// No-op if the segment has already been evicted. Rewrites the persisted
// copy outside the lock so the on-disk record includes the transcript.
func (r *Ring) SetTranscript(id int64, text string) {
	r.mu.Lock()
	var updated *Segment
	for _, s := range r.segments {
		if s.ID == id {
			s.Transcript = text
			updated = s
			break
		}
	}
	p := r.persister
	r.mu.Unlock()

	if updated != nil && p != nil {
		if err := p.Save(updated); err != nil {
			sessionlog.Warnf("recall: persist transcript for segment %d: %v", id, err)
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
	evicted := r.evictLocked()
	st := Stats{Count: len(r.segments), TotalSeen: r.totalSeen}
	if len(r.segments) > 0 {
		now := r.now()
		st.OldestAge = now.Sub(r.segments[0].StartedAt)
		st.NewestAge = now.Sub(r.segments[len(r.segments)-1].StartedAt)
		for _, s := range r.segments {
			st.TotalFrames += int64(len(s.PCM))
		}
	}
	p := r.persister
	r.mu.Unlock()

	if p != nil {
		deleteEvicted(p, evicted)
	}
	return st
}

// evictLocked drops segments that fall outside the count or retention
// bound and returns the evicted slice so callers can delete persisted
// copies after releasing the lock. Caller must hold r.mu.
func (r *Ring) evictLocked() []*Segment {
	var evicted []*Segment
	if r.retention > 0 {
		cutoff := r.now().Add(-r.retention)
		for len(r.segments) > 0 && r.segments[0].StartedAt.Before(cutoff) {
			evicted = append(evicted, r.segments[0])
			r.segments = r.segments[1:]
		}
	}
	if r.maxSegments > 0 && len(r.segments) > r.maxSegments {
		drop := len(r.segments) - r.maxSegments
		evicted = append(evicted, r.segments[:drop]...)
		r.segments = r.segments[drop:]
	}
	return evicted
}

// deleteEvicted best-effort removes the persisted copies for segments
// the ring just dropped. Errors are logged but never propagate —
// persistence is a convenience, not a correctness requirement.
func deleteEvicted(p Persister, evicted []*Segment) {
	for _, e := range evicted {
		if err := p.Delete(e.ID); err != nil {
			sessionlog.Warnf("recall: delete persisted segment %d: %v", e.ID, err)
		}
	}
}
