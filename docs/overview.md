# Overview

`vtt` is a Linux X11 voice-to-text helper written in Go.

At a high level:

- a global hotkey starts dictation
- the overlay appears immediately
- local microphone capture starts first
- early audio is buffered while the OpenAI realtime transcription session connects
- audio is streamed to OpenAI
- on release or stop, the stream is committed
- the final transcript is inserted back into the previously focused app

Important constraints:

- Linux X11 only for now
- PulseAudio / PipeWire input capture
- OpenAI API key stored in the system keyring or provided by `OPENAI_API_KEY`
- overlay is intentionally lightweight and non-interactive

Core product choices:

- `hold` mode is the default hotkey behavior
- `toggle` mode is also supported by config
- very short recordings are silently discarded
- terminal windows use a terminal-safe paste shortcut
- transcription is realtime-streamed, not uploaded from a WAV file

If you only need the “what is this thing” version of the repo, stop here.
