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
2. The injector captures the active target window so focus can be restored later.
3. [`internal/recorder/recorder.go`](/home/fred/git/vtt/internal/recorder/recorder.go) starts local microphone capture first.
4. In parallel, [`internal/openai/transcribe.go`](/home/fred/git/vtt/internal/openai/transcribe.go) creates a realtime transcription session through the official OpenAI SDK and connects the websocket.
5. Audio chunks that arrive before the websocket is ready are buffered in memory.
6. Once the websocket is ready, the buffered audio is flushed first, then live audio continues streaming normally.

Why this matters:

- the overlay can pop immediately
- the user can start speaking immediately
- the beginning of speech is less likely to be clipped by connection setup time

## Record Stop

When the hotkey stops dictation:

1. [`internal/app/app.go`](/home/fred/git/vtt/internal/app/app.go) stops local recording.
2. The overlay switches to transcribing.
3. The app waits for the audio pump to finish flushing local chunks into the realtime stream.
4. The OpenAI stream is committed.
5. The app waits for the final transcription event.

## Insert

After transcription completes:

1. [`internal/injector/injector.go`](/home/fred/git/vtt/internal/injector/injector.go) restores focus to the original window.
2. The transcript is inserted via clipboard paste or direct typing depending on config.
3. Terminal windows use the configured terminal paste shortcut.
4. The overlay shows success and then hides.

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
