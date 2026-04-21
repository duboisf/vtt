package recall

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Client is a thin wrapper around a Unix socket dial. Each request
// opens a fresh connection and closes it after the response — matches
// the daemon's per-connection framing.
type Client struct {
	SocketPath string
	DialTimeout time.Duration
}

// NewClient constructs a Client targeting the given socket. Pass the
// resolved path (use ResolveSocketPath if you have only the configured
// form).
func NewClient(socketPath string) *Client {
	return &Client{SocketPath: socketPath, DialTimeout: 2 * time.Second}
}

// List asks the daemon for the current ring buffer contents.
func (c *Client) List(ctx context.Context) ([]SegmentInfo, error) {
	resp, err := c.do(ctx, Request{Op: OpList})
	if err != nil {
		return nil, err
	}
	return resp.Segments, nil
}

// Transcribe asks the daemon to turn the given segment into text. If
// postprocess is true and post-processing is enabled in config, the
// daemon runs the LLM cleanup step too.
func (c *Client) Transcribe(ctx context.Context, id int64, postprocess bool) (string, error) {
	resp, err := c.do(ctx, Request{Op: OpTranscribe, SegmentID: id, PostProcess: postprocess})
	if err != nil {
		return "", err
	}
	return resp.Transcript, nil
}

// Drop removes a segment from the ring buffer.
func (c *Client) Drop(ctx context.Context, id int64) error {
	_, err := c.do(ctx, Request{Op: OpDrop, SegmentID: id})
	return err
}

// Status returns ring-buffer diagnostics.
func (c *Client) Status(ctx context.Context) (*StatsInfo, error) {
	resp, err := c.do(ctx, Request{Op: OpStatus})
	if err != nil {
		return nil, err
	}
	return resp.Stats, nil
}

// Shutdown asks the daemon to exit.
func (c *Client) Shutdown(ctx context.Context) error {
	_, err := c.do(ctx, Request{Op: OpShutdown})
	return err
}

func (c *Client) do(ctx context.Context, req Request) (Response, error) {
	req.Version = protocolVersion

	dialer := net.Dialer{Timeout: c.DialTimeout}
	conn, err := dialer.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return Response{}, fmt.Errorf("dial %s: %w", c.SocketPath, err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	}

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, fmt.Errorf("encode request: %w", err)
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != "" {
		return resp, fmt.Errorf("daemon: %s", resp.Error)
	}
	return resp, nil
}
