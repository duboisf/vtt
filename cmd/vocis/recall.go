package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"vocis/internal/config"
	"vocis/internal/recall"
	"vocis/internal/securestore"
	"vocis/internal/sessionlog"
	"vocis/internal/telemetry"
)

var (
	recallPickID          int64
	recallPickPostprocess bool
)

var recallCmd = &cobra.Command{
	Use:   "recall",
	Short: "Always-on dictation: capture continuously, transcribe on demand",
	Long: `Wokis Recall — an alternative to the push-to-talk serve mode. The
daemon captures microphone audio continuously, segments it with Silero
VAD, and keeps a bounded ring buffer of speech episodes. Use
"vocis recall pick" to browse recent segments and transcribe one on
demand.`,
}

var recallStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Run the recall daemon in the foreground",
	Long: `Starts the recall daemon: opens the configured microphone, runs Silero
VAD, and listens on the configured Unix socket for list/transcribe/drop
requests from the other recall subcommands. Runs until killed (Ctrl-C /
SIGTERM) or until "vocis recall stop" asks it to exit.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRecallStart()
	},
}

var recallPickCmd = &cobra.Command{
	Use:   "pick",
	Short: "Transcribe one of the recent segments",
	Long: `Lists the current ring buffer and asks you to pick a segment by ID
(interactive prompt, or --id to skip it). The daemon transcribes the
segment and the final text is written to stdout.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRecallPick()
	},
}

var recallStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report ring-buffer stats from a running daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRecallStatus()
	},
}

var recallStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Ask the running daemon to exit",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRecallStop()
	},
}

func init() {
	recallPickCmd.Flags().Int64Var(&recallPickID, "id", 0,
		"transcribe this segment id without showing the interactive picker")
	recallPickCmd.Flags().BoolVar(&recallPickPostprocess, "postprocess", false,
		"run the configured LLM cleanup on the transcript before printing")

	recallCmd.AddCommand(recallStartCmd)
	recallCmd.AddCommand(recallPickCmd)
	recallCmd.AddCommand(recallStatusCmd)
	recallCmd.AddCommand(recallStopCmd)
	rootCmd.AddCommand(recallCmd)
}

func runRecallStart() error {
	session, err := sessionlog.Start()
	if err != nil {
		return err
	}
	defer session.Close()

	cfg, path, err := config.Load()
	if err != nil {
		return err
	}
	sessionlog.Infof("vocis %s recall start (config=%s)", version, path)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Init(ctx, cfg.Telemetry, version)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	defer shutdownTelemetry(context.Background())

	apiKey := ""
	if cfg.Transcription.Backend != config.BackendLemonade {
		key, err := securestore.New().APIKey()
		if err != nil {
			return fmt.Errorf("load api key: %w", err)
		}
		apiKey = key
	}

	d := recall.NewDaemon(recall.DaemonOpts{Config: cfg, APIKey: apiKey})
	fmt.Fprintln(os.Stderr, "recall daemon started — speak normally; use `vocis recall pick` from another terminal to transcribe a segment")
	return d.Run(ctx)
}

func runRecallPick() error {
	cfg, _, err := config.Load()
	if err != nil {
		return err
	}
	socket, err := recall.ResolveSocketPath(cfg.Recall.SocketPath)
	if err != nil {
		return err
	}
	client := recall.NewClient(socket)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	id := recallPickID
	if id == 0 {
		segs, err := client.List(ctx)
		if err != nil {
			return err
		}
		if len(segs) == 0 {
			fmt.Fprintln(os.Stderr, "no segments in buffer yet — speak into the mic first")
			return nil
		}
		printSegmentTable(os.Stderr, segs)
		id, err = promptSegmentID(segs)
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "transcribing segment #%d...\n", id)
	text, err := client.Transcribe(ctx, id, recallPickPostprocess)
	if err != nil {
		return err
	}
	fmt.Println(text)
	return nil
}

func runRecallStatus() error {
	cfg, _, err := config.Load()
	if err != nil {
		return err
	}
	socket, err := recall.ResolveSocketPath(cfg.Recall.SocketPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := recall.NewClient(socket)
	stats, err := client.Status(ctx)
	if err != nil {
		return err
	}
	if stats == nil {
		fmt.Println("no stats returned")
		return nil
	}
	fmt.Printf("socket:    %s\n", socket)
	fmt.Printf("segments:  %d (ever captured: %d)\n", stats.Count, stats.TotalSeen)
	if stats.Count > 0 {
		fmt.Printf("oldest:    %s ago\n", time.Duration(stats.OldestAgeMS)*time.Millisecond)
		fmt.Printf("newest:    %s ago\n", time.Duration(stats.NewestAgeMS)*time.Millisecond)
		fmt.Printf("frames:    %d\n", stats.TotalFrames)
	}
	return nil
}

func runRecallStop() error {
	cfg, _, err := config.Load()
	if err != nil {
		return err
	}
	socket, err := recall.ResolveSocketPath(cfg.Recall.SocketPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := recall.NewClient(socket)
	if err := client.Shutdown(ctx); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "shutdown requested")
	return nil
}

// printSegmentTable renders a compact table of ring-buffer segments to
// w. Stable columns so users can predict what they're picking from.
func printSegmentTable(w *os.File, segs []recall.SegmentInfo) {
	fmt.Fprintln(w, "  id    age        dur      peak   transcript")
	fmt.Fprintln(w, "  ----  ---------  -------  -----  -------------------------------------------------")
	now := time.Now()
	for _, s := range segs {
		age := now.Sub(s.StartedAt).Round(time.Second)
		dur := time.Duration(s.DurationMS) * time.Millisecond
		preview := "(not transcribed yet)"
		if s.Transcribed {
			preview = truncateOneLine(s.CachedText, 48)
		}
		fmt.Fprintf(w, "  %4d  %9s  %6.2fs  %4.2f   %s\n",
			s.ID, age, dur.Seconds(), s.PeakLevel, preview)
	}
}

func truncateOneLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// promptSegmentID reads a segment ID from stdin and validates it
// against the list we fetched. Blank input picks the most recent one,
// which is the common case.
func promptSegmentID(segs []recall.SegmentInfo) (int64, error) {
	latest := segs[len(segs)-1].ID
	fmt.Fprintf(os.Stderr, "pick an id [default %d, latest]: ", latest)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return 0, fmt.Errorf("read pick: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return latest, nil
	}
	id, err := strconv.ParseInt(line, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("not a valid id: %q", line)
	}
	for _, s := range segs {
		if s.ID == id {
			return id, nil
		}
	}
	return 0, fmt.Errorf("id %d not in ring buffer — try one from the list above", id)
}
