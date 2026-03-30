package injector

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/atotto/clipboard"

	"vtt/internal/config"
)

type Target struct {
	WindowID    string
	WindowClass string
	WindowName  string
}

type Injector struct {
	cfg config.InsertionConfig
}

func New(cfg config.InsertionConfig) *Injector {
	return &Injector{cfg: cfg}
}

func (i *Injector) CaptureTarget(ctx context.Context) (Target, error) {
	windowID, err := i.runTrimmed(ctx, "xdotool", "getactivewindow")
	if err != nil {
		return Target{}, fmt.Errorf("active window: %w", err)
	}

	className, _ := i.runTrimmed(ctx, "xdotool", "getwindowclassname", windowID)
	windowName, _ := i.runTrimmed(ctx, "xdotool", "getwindowname", windowID)

	return Target{
		WindowID:    windowID,
		WindowClass: className,
		WindowName:  windowName,
	}, nil
}

func (i *Injector) Insert(ctx context.Context, target Target, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	if target.WindowID != "" {
		if _, err := i.runTrimmed(ctx, "xdotool", "windowactivate", "--sync", target.WindowID); err != nil {
			return fmt.Errorf("restore focus: %w", err)
		}
		time.Sleep(120 * time.Millisecond)
	}

	mode := i.resolveMode(target.WindowClass)
	switch mode {
	case "type":
		args := []string{"type", "--clearmodifiers"}
		if i.cfg.TypeDelayMS >= 0 {
			args = append(args, "--delay", fmt.Sprintf("%d", i.cfg.TypeDelayMS))
		}
		if target.WindowID != "" {
			args = append(args, "--window", target.WindowID)
		}
		args = append(args, "--", text)
		if _, err := i.runTrimmed(ctx, "xdotool", args...); err != nil {
			return fmt.Errorf("type text: %w", err)
		}
		return nil
	default:
		return i.paste(ctx, target, text)
	}
}

func (i *Injector) paste(ctx context.Context, target Target, text string) error {
	originalClipboard := ""
	if i.cfg.RestoreClipboard {
		clip, err := clipboard.ReadAll()
		if err == nil {
			originalClipboard = clip
		}
	}

	if err := clipboard.WriteAll(text); err != nil {
		return fmt.Errorf("clipboard write: %w", err)
	}

	pasteKey := i.cfg.DefaultPasteKey
	if i.isTerminal(target.WindowClass) {
		pasteKey = i.cfg.TerminalPasteKey
	}

	args := []string{"key", "--clearmodifiers"}
	if target.WindowID != "" {
		args = append(args, "--window", target.WindowID)
	}
	args = append(args, normalizeKeyChord(pasteKey)...)

	if _, err := i.runTrimmed(ctx, "xdotool", args...); err != nil {
		return fmt.Errorf("paste text: %w", err)
	}

	if i.cfg.RestoreClipboard {
		restore := originalClipboard
		go func() {
			time.Sleep(250 * time.Millisecond)
			if restore == "" {
				return
			}
			_ = clipboard.WriteAll(restore)
		}()
	}

	return nil
}

func (i *Injector) resolveMode(windowClass string) string {
	if i.cfg.Mode != "auto" {
		return i.cfg.Mode
	}
	return "clipboard"
}

func (i *Injector) isTerminal(windowClass string) bool {
	for _, candidate := range i.cfg.TerminalClasses {
		if strings.EqualFold(candidate, windowClass) {
			return true
		}
	}
	return false
}

func normalizeKeyChord(chord string) []string {
	chord = strings.ReplaceAll(strings.ToLower(chord), "+", "+")
	return []string{chord}
}

func (i *Injector) runTrimmed(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s %v: %s", name, args, msg)
	}
	return strings.TrimSpace(string(out)), nil
}
