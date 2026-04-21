package recall

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultSocketPath returns the auto-resolved socket path used when the
// user has not set recall.socket_path in config. Prefers
// $XDG_RUNTIME_DIR/vocis/recall.sock because that directory is per-user,
// tmpfs, and auto-cleaned on session end. Falls back to
// /tmp/vocis-recall-<uid>.sock if XDG_RUNTIME_DIR isn't set (e.g.
// running as root or inside a container without a session).
func DefaultSocketPath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("VOCIS_RECALL_SOCKET")); configured != "" {
		return configured, nil
	}
	if runtime := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); runtime != "" {
		return filepath.Join(runtime, "vocis", "recall.sock"), nil
	}
	return fmt.Sprintf("/tmp/vocis-recall-%d.sock", os.Getuid()), nil
}

// ResolveSocketPath returns the configured socket path when set, or the
// default. It does NOT create parent directories — the daemon does that
// when it binds.
func ResolveSocketPath(configured string) (string, error) {
	if s := strings.TrimSpace(configured); s != "" {
		return s, nil
	}
	return DefaultSocketPath()
}
