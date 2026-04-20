package app

import (
	"context"
	"testing"
	"time"

	"vocis/internal/config"
	"vocis/internal/transcribe"
	"vocis/internal/platform"
)

func TestHandleDictationEventUpdatesOverlayWithPartialText(t *testing.T) {
	t.Parallel()

	fakeOverlay := &overlayStub{}
	app := &App{
		cfg: config.Config{
			Streaming: config.StreamingConfig{ShowPartialOverlay: true},
		},
		overlay: fakeOverlay,
	}
	state := &recordingState{
		target: platform.Target{WindowClass: "Gedit"},
	}

	err := app.handleDictationEvent(context.Background(), state, transcribe.DictationEvent{
		Type: transcribe.DictationEventPartial,
		Text: "hello world",
	})
	if err != nil {
		t.Fatalf("handleDictationEvent: %v", err)
	}

	if fakeOverlay.windowClass != "Gedit" {
		t.Fatalf("windowClass = %q, want Gedit", fakeOverlay.windowClass)
	}
	if fakeOverlay.listeningText != "hello world" {
		t.Fatalf("listeningText = %q, want hello world", fakeOverlay.listeningText)
	}
}

// TestPartialAppendsBelowAccumulatedSegments locks down live-subtitle
// behavior: the in-flight partial renders on its own line below the
// committed segments. When the matching `completed` event arrives, the
// canonical text replaces the partial in place (covered separately).
func TestPartialAppendsBelowAccumulatedSegments(t *testing.T) {
	t.Parallel()

	fakeOverlay := &overlayStub{}
	app := &App{
		cfg: config.Config{
			Streaming: config.StreamingConfig{ShowPartialOverlay: true},
		},
		overlay: fakeOverlay,
	}
	state := &recordingState{
		target:      platform.Target{WindowClass: "Gedit"},
		liveText:    "Hello world.",
		displayText: "Hello world.",
	}

	_ = app.handleDictationEvent(context.Background(), state, transcribe.DictationEvent{
		Type: transcribe.DictationEventPartial,
		Text: "this is more",
	})

	if fakeOverlay.listeningText != "Hello world.\nthis is more" {
		t.Fatalf("listeningText = %q, want committed + newline + partial", fakeOverlay.listeningText)
	}
	if state.currentPartial != "this is more" {
		t.Fatalf("currentPartial = %q, want %q", state.currentPartial, "this is more")
	}
}

// TestPartialReplacesPreviousPartial confirms the in-place update
// behavior — newer partial overwrites the previous one rather than
// appending.
func TestPartialReplacesPreviousPartial(t *testing.T) {
	t.Parallel()

	fakeOverlay := &overlayStub{}
	app := &App{
		cfg: config.Config{
			Streaming: config.StreamingConfig{ShowPartialOverlay: true},
		},
		overlay: fakeOverlay,
	}
	state := &recordingState{
		target:         platform.Target{WindowClass: "Gedit"},
		displayText:    "Hello world.",
		currentPartial: "this is",
	}

	_ = app.handleDictationEvent(context.Background(), state, transcribe.DictationEvent{
		Type: transcribe.DictationEventPartial,
		Text: "this is more text",
	})

	if fakeOverlay.listeningText != "Hello world.\nthis is more text" {
		t.Fatalf("listeningText = %q, want previous partial replaced", fakeOverlay.listeningText)
	}
}

// TestSegmentClearsPartial confirms that when a turn completes, the
// canonical segment text replaces the in-flight partial (no double
// rendering of "this is more text" + "this is more text, you know").
func TestSegmentClearsPartial(t *testing.T) {
	t.Parallel()

	fakeOverlay := &overlayStub{}
	app := &App{
		cfg: config.Config{
			Streaming: config.StreamingConfig{ShowPartialOverlay: true},
		},
		overlay: fakeOverlay,
	}
	state := &recordingState{
		target:         platform.Target{WindowClass: "Gedit"},
		displayText:    "Hello world.",
		currentPartial: "this is mo",
	}

	_ = app.handleDictationEvent(context.Background(), state, transcribe.DictationEvent{
		Type: transcribe.DictationEventSegment,
		Text: "this is more text, finalized.",
	})

	if state.currentPartial != "" {
		t.Fatalf("currentPartial = %q, want cleared after segment", state.currentPartial)
	}
	want := "Hello world.\nthis is more text, finalized."
	if fakeOverlay.listeningText != want {
		t.Fatalf("listeningText = %q, want %q", fakeOverlay.listeningText, want)
	}
}

func TestEmptyPartialDoesNotFlashHelperWhenSegmentsExist(t *testing.T) {
	t.Parallel()

	fakeOverlay := &overlayStub{}
	app := &App{
		cfg: config.Config{
			Streaming: config.StreamingConfig{ShowPartialOverlay: true},
		},
		overlay: fakeOverlay,
	}
	state := &recordingState{
		target:      platform.Target{WindowClass: "Gedit"},
		liveText:    "Hello world.",
		displayText: "Hello world.",
	}

	// Set initial text so we can detect if it gets cleared.
	fakeOverlay.listeningText = "Hello world."

	_ = app.handleDictationEvent(context.Background(), state, transcribe.DictationEvent{
		Type: transcribe.DictationEventPartial,
		Text: "",
	})

	if fakeOverlay.listeningText != "Hello world." {
		t.Fatalf("listeningText = %q, want unchanged display text", fakeOverlay.listeningText)
	}
}

func TestHandleDictationEventAccumulatesSegments(t *testing.T) {
	t.Parallel()

	fakeOverlay := &overlayStub{}
	app := &App{
		cfg: config.Config{
			HotkeyMode: "hold",
			Streaming:  config.StreamingConfig{},
		},
		overlay: fakeOverlay,
	}
	state := &recordingState{
		target: platform.Target{WindowID: "42", WindowClass: "Gedit"},
	}
	app.recording = state

	for _, seg := range []string{"segment one", " segment two"} {
		err := app.handleDictationEvent(context.Background(), state, transcribe.DictationEvent{
			Type: transcribe.DictationEventSegment,
			Text: seg,
		})
		if err != nil {
			t.Fatalf("handleDictationEvent: %v", err)
		}
	}

	if state.liveText != "segment one segment two" {
		t.Fatalf("liveText = %q, want %q", state.liveText, "segment one segment two")
	}
	if fakeOverlay.listeningText != "segment one\nsegment two" {
		t.Fatalf("listeningText = %q, want newline-separated display", fakeOverlay.listeningText)
	}
}

func TestHandleUpDoesNothingWhenNotRecording(t *testing.T) {
	t.Parallel()

	app := &App{
		cfg: config.Config{
			HotkeyMode: "hold",
			Streaming: config.StreamingConfig{
				},
		},
		overlay: &overlayStub{},
	}

	app.handleUp(context.Background())

	if app.recording != nil {
		t.Fatal("expected no recording state")
	}
	if app.transcribing {
		t.Fatal("expected transcribing to remain false")
	}
}

func TestHandleDownDismissesOldOverlayWhileTranscribing(t *testing.T) {
	t.Parallel()

	fakeOverlay := &overlayStub{}
	cfg := config.Default()
	cfg.HotkeyMode = "hold"
	app := &App{
		cfg:          cfg,
		overlay:      fakeOverlay,
		transcribing: true,
	}

	app.handleDown(context.Background())

	if fakeOverlay.warningText == "" {
		t.Fatal("expected cancellation warning overlay")
	}
	if !app.completionOverlayDismissed() {
		t.Fatal("expected completion overlay to be dismissed")
	}
}

func TestShowCompletionSuccessStaysHiddenAfterDismiss(t *testing.T) {
	t.Parallel()

	fakeOverlay := &overlayStub{}
	app := &App{
		overlay: fakeOverlay,
	}
	app.dismissCompletionOverlay = true

	app.showCompletionSuccess("hello")

	if fakeOverlay.successText != "" {
		t.Fatalf("successText = %q, want empty", fakeOverlay.successText)
	}
	if fakeOverlay.hideCalls != 1 {
		t.Fatalf("hideCalls = %d, want 1", fakeOverlay.hideCalls)
	}
}

type overlayStub struct {
	windowClass    string
	listeningText  string
	animatedChunks []string
	successText    string
	warningText    string
	hideCalls      int
}

func (o *overlayStub) ShowHint(string)      {}
func (o *overlayStub) ShowListening(string, string)  {}
func (o *overlayStub) SetConnected(string)            {}
func (o *overlayStub) SetConnecting(int, int)         {}
func (o *overlayStub) SetSubmitMode(bool)             {}
func (o *overlayStub) AnimateChunk(text string) {
	o.animatedChunks = append(o.animatedChunks, text)
}
func (o *overlayStub) ShowFinishing(string, string, time.Duration) {}
func (o *overlayStub) SetFinishingPhase(string, time.Duration)    {}
func (o *overlayStub) ExtendFinishingPhase(string, time.Duration) {}
func (o *overlayStub) SetFinishingText(string)                    {}
func (o *overlayStub) ShowSuccess(text string) {
	o.successText = text
}
func (o *overlayStub) ShowError(error)    {}
func (o *overlayStub) ShowWarning(text string) { o.warningText = text }
func (o *overlayStub) GrabEscape() <-chan struct{} { return make(chan struct{}) }
func (o *overlayStub) UngrabEscape()              {}
func (o *overlayStub) SetLevel(float64) {}
func (o *overlayStub) Hide() {
	o.hideCalls++
}
func (o *overlayStub) Close() {}
func (o *overlayStub) SetListeningText(windowClass, text string) {
	o.windowClass = windowClass
	o.listeningText = text
}

type injectorStub struct {
	inserted     []string
	liveInserted []string
	err          error
}

func (i *injectorStub) CaptureTarget(context.Context) (platform.Target, error) {
	return platform.Target{}, nil
}

func (i *injectorStub) Insert(_ context.Context, _ platform.Target, text string) error {
	if i.err != nil {
		return i.err
	}
	i.inserted = append(i.inserted, text)
	return nil
}

func (i *injectorStub) PressEnter(_ context.Context, _ platform.Target) error { return nil }

func (i *injectorStub) InsertLive(_ context.Context, _ platform.Target, text string) error {
	if i.err != nil {
		return i.err
	}
	i.liveInserted = append(i.liveInserted, text)
	return nil
}


