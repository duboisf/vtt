package main

import (
	"fmt"
	"sync"

	"github.com/jfreymuth/pulse"
)

// Player streams PCM16 mono audio to the default PulseAudio sink as
// chunks arrive. Push-side: caller calls Write(samples) on the same
// cadence the realtime WS sends them. Pull-side: pulse asks for
// fixed-size buffers via the Int16Reader callback. A small in-memory
// queue smooths the mismatch.
type Player struct {
	client *pulse.Client
	stream *pulse.PlaybackStream

	mu     sync.Mutex
	buf    []int16
	closed bool
}

func NewPlayer(rate int) (*Player, error) {
	client, err := pulse.NewClient(pulse.ClientApplicationName("lemonade-probe playback"))
	if err != nil {
		return nil, fmt.Errorf("pulse client: %w", err)
	}
	p := &Player{client: client}

	reader := pulse.Int16Reader(func(out []int16) (int, error) {
		p.mu.Lock()
		n := copy(out, p.buf)
		p.buf = p.buf[n:]
		p.mu.Unlock()
		// Pad the rest with silence and return len(out), nil. Returning
		// (0, nil) on an empty queue makes pulse busy-loop the reader
		// (100% CPU); padding lets pulse pace callbacks at the actual
		// playback rate so WS sends keep flowing.
		for i := n; i < len(out); i++ {
			out[i] = 0
		}
		return len(out), nil
	})

	stream, err := client.NewPlayback(reader,
		pulse.PlaybackSampleRate(rate),
		pulse.PlaybackMono,
		pulse.PlaybackMediaName("lemonade-probe playback"),
		pulse.PlaybackLatency(0.10),
	)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("open playback: %w", err)
	}
	p.stream = stream
	stream.Start()
	return p, nil
}

func (p *Player) Write(samples []int16) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if !p.closed {
		p.buf = append(p.buf, samples...)
	}
	p.mu.Unlock()
}

func (p *Player) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	if p.stream != nil {
		// Don't Drain — our reader always returns full silence, so
		// Drain would never see "buffer empty" and would block forever.
		// Stop+Close cuts off whatever's still queued; for a probe
		// that's fine.
		p.stream.Stop()
		p.stream.Close()
	}
	if p.client != nil {
		p.client.Close()
	}
}
