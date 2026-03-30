package sessionlog

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
)

type Session struct {
	path string
	file *os.File
}

type colorWriter struct {
	w io.Writer
}

const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiCyan   = "\033[36m"
)

func Start() (*Session, error) {
	dir, err := logDir()
	if err != nil {
		return nil, err
	}

	name := time.Now().Format("20060102-150405") + ".log"
	path := filepath.Join(dir, name)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.SetOutput(io.MultiWriter(stderrWriter(), file))

	return &Session{
		path: path,
		file: file,
	}, nil
}

func (s *Session) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Session) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	log.SetOutput(os.Stderr)
	return s.file.Close()
}

func Dir() (string, error) {
	return logDir()
}

func logDir() (string, error) {
	base := strings.TrimSpace(os.Getenv("XDG_STATE_HOME"))
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}

	dir := filepath.Join(base, "vtt", "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func stderrWriter() io.Writer {
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return os.Stderr
	}
	return colorWriter{w: os.Stderr}
}

func (w colorWriter) Write(p []byte) (int, error) {
	color := logColor(string(p))
	if color == "" {
		return w.w.Write(p)
	}
	return w.w.Write([]byte(color + string(p) + ansiReset))
}

func logColor(line string) string {
	lower := strings.ToLower(line)

	switch {
	case strings.Contains(lower, " error"),
		strings.Contains(lower, "showerror"),
		strings.Contains(lower, "failed"),
		strings.Contains(lower, "missing"),
		strings.Contains(lower, "panic"),
		strings.Contains(lower, "stop recording:"),
		strings.Contains(lower, "transcribe audio:"),
		strings.Contains(lower, "insert transcript:"):
		return ansiRed
	case strings.Contains(lower, "warning"),
		strings.Contains(lower, "unavailable"),
		strings.Contains(lower, "fallback"),
		strings.Contains(lower, "too short"):
		return ansiYellow
	case strings.Contains(lower, "starting recording"),
		strings.Contains(lower, "recording started"),
		strings.Contains(lower, "listening"):
		return ansiBlue
	case strings.Contains(lower, "transcribing"),
		strings.Contains(lower, "transcription complete"):
		return ansiCyan
	case strings.Contains(lower, "audio captured successfully"),
		strings.Contains(lower, "transcript inserted"),
		strings.Contains(lower, " ok "):
		return ansiGreen
	default:
		return ""
	}
}
