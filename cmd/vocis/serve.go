package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"vocis/internal/app"
	"vocis/internal/config"
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

	sessionlog.Infof("session log: %s", session.Path())

	cfg, path, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Init(ctx, cfg.Telemetry)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	defer shutdownTelemetry(context.Background())

	if cfg.Telemetry.Enabled {
		sessionlog.Infof("telemetry enabled, exporting to %s", cfg.Telemetry.Endpoint)
	}

	sessionlog.Infof("loaded config: %s", path)
	sessionlog.Infof("hotkey: %s", cfg.Hotkey)

	return app.New(cfg).Run(ctx)
}
