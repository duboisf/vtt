# Bugs

Open bugs observed during live testing.

## 1. First Word Missing In Target App

- Status: open
- Reported: 2026-03-30
- Symptom:
  - On hotkey press, the beginning of speech can appear in the overlay.
  - When that same text is typed or pasted into the target window, the first word is sometimes missing.
- Current understanding:
  - This appears to affect target-window insertion more than transcription itself, because the overlay can still show the missing word.

## 2. Overlay Disappears Before Hotkey Release

- Status: open
- Reported: 2026-03-30
- Symptom:
  - While still holding the hotkey in segmented mode, a chunk can be inserted and the overlay then disappears even though the hotkey has not been released.
  - In some runs, stopping speech without releasing the hotkey is enough to trigger the disappearance.
- Current understanding:
  - This likely means the app is still receiving or interpreting a synthetic stop/release path during live segment insertion.
