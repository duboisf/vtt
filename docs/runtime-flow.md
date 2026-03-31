# Runtime Flow

This page is the detailed path for one dictation session.

## Startup

When `vtt serve` runs:

1. [`cmd/vtt/main.go`](/home/fred/git/vtt/cmd/vtt/main.go) starts a session log.
2. [`internal/config/config.go`](/home/fred/git/vtt/internal/config/config.go) loads config.
3. [`internal/securestore/keyring.go`](/home/fred/git/vtt/internal/securestore/keyring.go) resolves the OpenAI API key.
4. [`internal/app/app.go`](/home/fred/git/vtt/internal/app/app.go) creates the runtime dependencies and registers the hotkey.

## Record Start

When the hotkey starts dictation:

1. [`internal/app/app.go`](/home/fred/git/vtt/internal/app/app.go) shows the overlay immediately.
2. [`internal/recorder/recorder.go`](/home/fred/git/vtt/internal/recorder/recorder.go) starts local microphone capture immediately.
3. The injector captures the active target window after capture has already started so focus can be restored later.
4. [`internal/openai/transcribe.go`](/home/fred/git/vtt/internal/openai/transcribe.go) starts a `DictationSession`.
5. The `DictationSession` creates the realtime transcription session through the official OpenAI SDK and connects the websocket.
6. Audio chunks that arrive before the websocket is ready are buffered in memory inside the dictation session.
7. Once the websocket is ready, the buffered audio is flushed first, then live audio continues streaming normally.

Why this matters:

- the overlay can pop immediately
- the user can start speaking immediately
- the beginning of speech is less likely to be clipped by window lookup or connection setup time

## Record Stop

When the hotkey stops dictation:

1. [`internal/app/app.go`](/home/fred/git/vtt/internal/app/app.go) stops local recording.
2. The overlay switches to transcribing.
3. [`internal/openai/transcribe.go`](/home/fred/git/vtt/internal/openai/transcribe.go) finalizes the `DictationSession`.
4. The dictation session decides whether there is any trailing audio left that still needs a final commit.
5. If segmented mode already emitted the spoken chunks and there is no trailing audio left, finalization returns without forcing an extra commit.
6. Otherwise the OpenAI stream is committed and the final transcription event is awaited.

## Insert

After transcription completes:

1. [`internal/injector/injector.go`](/home/fred/git/vtt/internal/injector/injector.go) restores focus to the original window.
2. The transcript is inserted via clipboard paste or direct typing depending on config.
3. Terminal windows use the configured terminal paste shortcut.
4. The overlay shows success and then hides.

## Segmented Streaming

When `streaming.mode = "segment"`:

1. OpenAI server VAD detects pauses while the hotkey is still held.
2. Completed phrases are emitted as segment events from the dictation session.
3. [`internal/app/app.go`](/home/fred/git/vtt/internal/app/app.go) updates the overlay and asks [`internal/injector/injector.go`](/home/fred/git/vtt/internal/injector/injector.go) to type those completed phrases live.
4. On release, finalization only flushes trailing speech that has not already been emitted as a completed segment.

This is why segmented mode should feel more live than release-only mode without waiting for the whole utterance to finish.

## Short Recordings

Very short recordings are treated as a silent cancel:

- [`internal/recorder/recorder.go`](/home/fred/git/vtt/internal/recorder/recorder.go) returns `ErrRecordingTooShort`
- [`internal/app/app.go`](/home/fred/git/vtt/internal/app/app.go) catches that and hides the overlay
- no user-facing error is shown for that case

## Logging

Each `serve` session writes a log file under:

- `~/.local/state/vtt/sessions/`

The log is the best place to look for:

- realtime session setup failures
- streaming errors
- insertion failures
- hotkey fallback decisions

## Verification Standard

This repo intentionally uses a high bar before calling work done:

- unit tests where they make sense
- successful build
- local runtime verification for behavior changes whenever feasible

That rule is summarized in [`AGENTS.md`](/home/fred/git/vtt/AGENTS.md).
