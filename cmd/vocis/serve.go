package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"vocis/internal/app"
	"vocis/internal/audio"
	"vocis/internal/config"
	"vocis/internal/platform"
	"vocis/internal/platform/gnome"
	x11 "vocis/internal/platform/x11"
	"vocis/internal/sessionlog"
	"vocis/internal/telemetry"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the voice-to-text service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServe()
	},
}

func runServe() error {
	session, err := sessionlog.Start()
	if err != nil {
		return err
	}
	defer session.Close()

	sessionlog.Infof("vocis %s", version)
	sessionlog.Infof("session log: %s", session.Path())

	cfg, path, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Init(ctx, cfg.Telemetry, version)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	defer shutdownTelemetry(context.Background())

	if cfg.Telemetry.Enabled {
		sessionlog.Infof("telemetry enabled, exporting to %s", cfg.Telemetry.Endpoint)
	}

	sessionlog.Infof("loaded config: %s", path)
	sessionlog.Infof("hotkey: %s", cfg.Hotkey)

	ov, err := x11.NewOverlay(cfg.Overlay)
	if err != nil {
		return fmt.Errorf("init overlay: %w", err)
	}

	return app.New(cfg, app.Deps{
		Overlay:        ov,
		Injector:       x11.NewInjector(cfg.Insertion, cfg.Hotkey, pickTargetCapture()),
		Ducker:         audio.NewDucker(cfg.Recording.DuckVolume),
		RegisterHotkey: pickHotkeyRegistrar(),
	}).Run(ctx)
}

// pickTargetCapture returns an override for the injector's CaptureTarget on
// systems where xdotool can't see the focused window. On Wayland we ask the
// vocis-gnome shell extension. On X11 (or when the extension is missing) we
// return nil and let the injector use its default xdotool path.
func pickTargetCapture() x11.TargetCapture {
	if !isWaylandSession() || !gnome.Available() {
		return nil
	}
	sessionlog.Infof("target capture: vocis-gnome extension")
	return func(ctx context.Context) (platform.Target, error) {
		return gnome.FocusedWindow(ctx)
	}
}

// pickHotkeyRegistrar selects a global hotkey backend based on the running
// session. On Wayland, X11 grabs do not see native Wayland keystrokes, so we
// prefer the vocis-gnome shell extension if it's installed and reachable on
// the session bus. Falls back to X11 (XGrabKey via XWayland) otherwise — that
// fallback only works for X11/XWayland focused windows on Wayland sessions.
func pickHotkeyRegistrar() app.HotkeyRegistrar {
	if isWaylandSession() {
		if gnome.Available() {
			sessionlog.Infof("hotkey backend: vocis-gnome shell extension")
			return func(shortcut string) (app.HotkeySource, error) {
				return gnome.Register(shortcut)
			}
		}
		sessionlog.Warnf("hotkey backend: vocis-gnome extension not detected on session bus, falling back to x11/XGrabKey (will not see Wayland-native keys)")
	}
	sessionlog.Infof("hotkey backend: x11 (XGrabKey)")
	return func(shortcut string) (app.HotkeySource, error) {
		return x11.Register(shortcut)
	}
}

func isWaylandSession() bool {
	if strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland") {
		return true
	}
	return os.Getenv("WAYLAND_DISPLAY") != ""
}
