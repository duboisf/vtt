# Architecture

This repo is intentionally small. The main runtime path is split by responsibility rather than by many layers.

Top-level entry points:

- [`cmd/vocis/`](/home/fred/git/vtt/cmd/vocis/): Cobra-based CLI with one file per command (`root.go`, `serve.go`, `init_cmd.go`, `doctor.go`, `key.go`)
- [`README.md`](/home/fred/git/vtt/README.md): user-facing setup and usage
- [`config.example.yaml`](/home/fred/git/vtt/config.example.yaml): config shape example

Primary runtime packages:

- [`internal/app/app.go`](/home/fred/git/vtt/internal/app/app.go): orchestration for hotkeys, recorder lifecycle, overlay updates, submit mode, post-processing, and insertion
- [`internal/config/config.go`](/home/fred/git/vtt/internal/config/config.go): default config, validation, load/save logic, template expansion for overlay text
- [`internal/hotkeys/hotkeys.go`](/home/fred/git/vtt/internal/hotkeys/hotkeys.go): global X11 hotkey registration, hold/toggle behavior, and tap detection for submit mode
- [`internal/overlay/overlay.go`](/home/fred/git/vtt/internal/overlay/overlay.go): always-on-top X11 overlay rendering with configurable text, animations, connection status, and Escape key grab
- [`internal/recorder/recorder.go`](/home/fred/git/vtt/internal/recorder/recorder.go): in-process PulseAudio/PipeWire microphone capture and live sample stream
- [`internal/openai/transcribe.go`](/home/fred/git/vtt/internal/openai/transcribe.go): OpenAI realtime transcription — connect with retries, buffered audio handoff, turn assembly, and finalization
- [`internal/openai/postprocess.go`](/home/fred/git/vtt/internal/openai/postprocess.go): LLM post-processing to clean up filler words and hesitations
- [`internal/injector/injector.go`](/home/fred/git/vtt/internal/injector/injector.go): focus restore, clipboard handling, paste/type insertion, and Enter keypress for submit mode
- [`internal/audio/duck.go`](/home/fred/git/vtt/internal/audio/duck.go): lower speaker volume during recording via `wpctl`
- [`internal/securestore/keyring.go`](/home/fred/git/vtt/internal/securestore/keyring.go): keyring-backed API key storage
- [`internal/sessionlog/sessionlog.go`](/home/fred/git/vtt/internal/sessionlog/sessionlog.go): per-session logs on disk with DEBUG/INFO/WARN/ERROR levels
- [`internal/telemetry/telemetry.go`](/home/fred/git/vtt/internal/telemetry/telemetry.go): OpenTelemetry tracing (OTLP/gRPC exporter, span helpers)

Package relationships:

- `main` loads config and starts `app`
- `app` owns the runtime lifecycle
- `app` depends on `hotkeys`, `overlay`, `recorder`, `openai`, `injector`, `audio`, `securestore`, and `telemetry`
- `openai.DictationSession` owns realtime stream readiness, early-audio buffering, connect retries, segmented turn assembly, and release-time finalization
- `openai.PostProcess` handles LLM cleanup with timeout and fallback
- `recorder` and `openai` are the audio/transcription edge
- `injector` is the desktop automation edge
- `audio` manages speaker volume ducking

Useful rule of thumb:

- if the bug is about when recording starts or stops, start in `app` and `hotkeys`
- if the bug is about missed words, segmented turns, or audio timing, start in `recorder` and `openai`
- if the bug is about pasting into the wrong place or wrong shortcut, start in `injector`
- if the bug is about what the user sees, start in `overlay`
- if the bug is about post-processing quality, check the prompt in `config`
- if the bug is about volume levels, check `audio`

If you need execution details rather than file ownership, continue to `runtime-flow.md`.
