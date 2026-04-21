package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
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
	recallPickSelection   string
	recallPickPostprocess bool
	recallPickJoin        string
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
	Short: "Transcribe one or more of the recent segments",
	Long: `Lists the current ring buffer and asks you to pick segments to
transcribe. The selection accepts a comma-separated mix of:

    3       single id
    3-5     closed range (inclusive)
    3-      id 3 and every newer segment
    -5      every segment up to id 5
    all, *  everything currently buffered

Example: "3,5-7,10-" transcribes ids 3, 5, 6, 7, and anything ≥ 10.

Transcripts for multiple segments are joined with a space (override with
--join). Use --ids to skip the interactive prompt.`,
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
	recallPickCmd.Flags().StringVar(&recallPickSelection, "ids", "",
		"selection string (e.g. \"3\", \"3-5\", \"3-\", \"-5\", \"all\", or a comma-separated mix) — skips the interactive prompt")
	recallPickCmd.Flags().BoolVar(&recallPickPostprocess, "postprocess", false,
		"run the configured LLM cleanup on each transcript before joining")
	recallPickCmd.Flags().StringVar(&recallPickJoin, "join", " ",
		"separator inserted between segment transcripts when selecting multiple")

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

	// List runs on a short deadline; the transcribe calls each get their
	// own deadline below. A single long deadline around everything would
	// have to cover N transcriptions plus user thinking time at the
	// prompt, which is awkward to bound.
	listCtx, listCancel := context.WithTimeout(context.Background(), 5*time.Second)
	segs, err := client.List(listCtx)
	listCancel()
	if err != nil {
		return err
	}
	if len(segs) == 0 {
		fmt.Fprintln(os.Stderr, "no segments in buffer yet — speak into the mic first")
		return nil
	}

	availableIDs := make([]int64, len(segs))
	for i, s := range segs {
		availableIDs[i] = s.ID
	}

	var ids []int64
	if sel := strings.TrimSpace(recallPickSelection); sel != "" {
		ids, err = recall.ParseSelection(sel, availableIDs)
		if err != nil {
			return err
		}
	} else {
		printSegmentTable(os.Stderr, segs)
		ids, err = promptSegmentSelection(availableIDs)
		if err != nil {
			return err
		}
	}

	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		fmt.Fprintf(os.Stderr, "transcribing segment #%d...\n", id)
		txCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		text, err := client.Transcribe(txCtx, id, recallPickPostprocess)
		cancel()
		if err != nil {
			return fmt.Errorf("segment %d: %w", id, err)
		}
		parts = append(parts, strings.TrimSpace(text))
	}
	fmt.Println(strings.Join(parts, recallPickJoin))
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

// promptSegmentSelection reads a selection string from stdin and turns
// it into a concrete list of IDs to transcribe. Blank input defaults to
// the most recent segment, which is the common case.
func promptSegmentSelection(available []int64) ([]int64, error) {
	latest := available[len(available)-1]
	fmt.Fprintf(os.Stderr,
		"pick [default %d=latest; accepts id, range like 3-5, open range 3-, or \"all\"]: ", latest)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read pick: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return []int64{latest}, nil
	}
	return recall.ParseSelection(line, available)
}
