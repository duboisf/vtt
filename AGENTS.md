# Repo Rules

- Do not report work as done until the requested behavior has been verified locally.
- Keep going until the feature works end to end, or until a real external blocker is identified in logs or test output.
- When code changes affect behavior described in `docs/`, update the relevant docs in the same change.
- Use Test-Driven Development (TDD) for bug fixes: write a failing test that reproduces the bug first, then fix the code to make it pass.
- Never hardcode behavioral values. All tunable values (timeouts, thresholds, word counts, etc.) must have a default in `config.Default()` and be overridable via the config file.
- Never amend commits on `main`. The branch may have been pushed at any time. Always create new commits instead.
- Every behavioral branch should leave evidence in the session log. When you add a feature, guard, filter, or fast-path, also add a log line so a future session transcript shows it firing. Use `sessionlog.Tracef` for high-volume protocol events (audio frames, WS traffic), `Debugf` for routine state transitions, `Infof` for user-visible decisions (filtered transcript, submit mode toggle, config reload), `Warnf` / `Errorf` for recoverable / fatal problems. If the event is filtered out of the existing trace machinery (e.g. `dumpWSFrame` skipping `input_audio_buffer.append`), add an explicit log so the path is still visible. The bar: a user who pastes their session log into a chat should be able to tell exactly which code paths ran.

## Repo Context (Progressive Disclosure)

Docs are organized by depth. Read only as far as you need:

1. `docs/overview.md` — what the product does and key constraints
2. `docs/architecture.md` — which packages own which behavior
3. `docs/runtime-flow.md` — detailed execution path for a dictation session
4. `docs/debugging.md` — logs, tracing (Jaeger API), diagnostic tips

Start at the top. Stop when you have enough context for the task at hand.
