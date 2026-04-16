package gnome

import (
	"context"
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"

	"vocis/internal/platform"
)

// FocusedWindow asks the vocis-gnome shell extension for the currently
// focused window's class, title, and Mutter window ID. The returned id is
// Mutter's stable uint64 stringified — it is NOT an X11 window ID and cannot
// be passed to xdotool. Vocis on Wayland uses class only (for terminal
// detection) and ignores the id for refocusing.
//
// This is the Wayland equivalent of `xdotool getactivewindow`. It opens its
// own short-lived bus connection so callers don't need to manage one. If the
// extension is not installed or unreachable, returns an error wrapping
// ErrExtensionNotInstalled — callers can detect that case via errors.Is and
// fall back to an empty Target.
func FocusedWindow(ctx context.Context) (platform.Target, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return platform.Target{}, fmt.Errorf("connect to session bus: %w", err)
	}
	defer conn.Close()

	obj := conn.Object(BusName, ObjectPath)
	call := obj.CallWithContext(ctx, Interface+".GetFocusedWindow", 0)
	if call.Err != nil {
		return platform.Target{}, fmt.Errorf("%w: %v", ErrExtensionNotInstalled, call.Err)
	}

	var wmClass, title, id string
	if err := call.Store(&wmClass, &title, &id); err != nil {
		return platform.Target{}, fmt.Errorf("decode GetFocusedWindow reply: %w", err)
	}

	return platform.Target{
		WindowID:    id,
		WindowClass: wmClass,
		WindowName:  title,
	}, nil
}

// IsExtensionUnreachable reports whether err indicates the vocis-gnome
// extension is not installed/enabled (vs. some other failure).
func IsExtensionUnreachable(err error) bool {
	return errors.Is(err, ErrExtensionNotInstalled)
}
