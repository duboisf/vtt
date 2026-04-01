package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"vocis/internal/config"
	"vocis/internal/recorder"
	"vocis/internal/securestore"
	"vocis/internal/sessionlog"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system dependencies",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor()
	},
}

func runDoctor() error {
	cfgPath, err := config.Path()
	if err != nil {
		return err
	}
	logDir, err := sessionlog.Dir()
	if err != nil {
		return err
	}

	checks := []struct {
		label string
		value string
		ok    bool
	}{
		{label: "display", value: os.Getenv("DISPLAY"), ok: os.Getenv("DISPLAY") != ""},
	}

	for _, check := range checks {
		status := "ok"
		if !check.ok {
			status = "missing"
		}
		fmt.Printf("%-14s %s (%s)\n", check.label, status, check.value)
	}

	for _, cmd := range []string{"xdotool", "xclip"} {
		path, ok := findExecutable(cmd)
		status := "ok"
		if !ok {
			status = "missing"
		}
		fmt.Printf("%-14s %s (%s)\n", cmd, status, path)
	}
	audioStatus := "ok"
	audioValue := "pulse server"
	if err := recorder.Check(); err != nil {
		audioStatus = "missing"
		audioValue = err.Error()
	}
	fmt.Printf("%-14s %s (%s)\n", "audio", audioStatus, audioValue)

	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("%-14s ok (%s)\n", "config", cfgPath)
	} else {
		fmt.Printf("%-14s missing (%s)\n", "config", cfgPath)
	}
	fmt.Printf("%-14s ok (%s)\n", "log-dir", logDir)

	store := securestore.New()
	if _, err := store.APIKey(); err == nil {
		fmt.Printf("%-14s ok (keyring or env)\n", "openai-key")
	} else {
		fmt.Printf("%-14s missing (%v)\n", "openai-key", err)
	}

	return nil
}
