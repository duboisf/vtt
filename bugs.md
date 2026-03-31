# Bugs

Open bugs observed during live testing.

## 1. First Word Missing In Target App

- Status: fix candidate in `a8b4e37`
- Reported: 2026-03-30
- Symptom:
  - On hotkey press, the beginning of speech can appear in the overlay.
  - When that same text is typed or pasted into the target window, the first word is sometimes missing.
- Current understanding:
  - This appears to affect target-window insertion more than transcription itself, because the overlay can still show the missing word.
- Latest assumption:
  - Live or final insertion was sometimes starting while hotkey modifiers were still effectively active, so the first typed characters or paste chord could be mangled.
- Proof added:
  - `internal/injector/injector_test.go` now verifies that live typing releases modifiers before the first typed characters are sent.
  - `internal/injector/injector_test.go` also verifies that final insertion releases modifiers before the terminal paste chord is sent.

## 2. Overlay Disappears Before Hotkey Release

- Status: fix candidate in `a8b4e37`
- Reported: 2026-03-30
- Symptom:
  - While still holding the hotkey in segmented mode, a chunk can be inserted and the overlay then disappears even though the hotkey has not been released.
  - In some runs, stopping speech without releasing the hotkey is enough to trigger the disappearance.
- Current understanding:
  - This likely means the app is still receiving or interpreting a synthetic stop/release path during live segment insertion.
- Latest assumption:
  - `xdotool keyup` during live insertion can emit tracked release events even though the user is still physically holding the hotkey.
  - Timing-only suppression is not sufficient because a synthetic release can look identical to a real one unless actual key state is checked.
- Proof added:
  - `internal/hotkeys/hotkeys_test.go` now verifies that suppressed releases do not emit `Up` while tracked keys are still physically down.
  - `internal/hotkeys/hotkeys_test.go` also verifies that `Up` does emit once the tracked keys are actually released.
