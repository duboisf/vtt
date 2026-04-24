package transcribe

import "strings"

// deltaStrategyForModel picks a transcription.delta merge strategy based
// on the model name:
//
//   - whisper-1 (and any Whisper variant), gemma on Lemonade's FLM
//     runtime: delta = full turn transcript so far → replace.
//   - gpt-4o-transcribe / gpt-4o-mini-transcribe: delta = incremental
//     new text → append.
//
// Semantics follow the model, not the backend. Case-insensitive substring
// matching on "whisper" or "gemma" catches known cumulative emitters:
// OpenAI's whisper-1, Lemonade ids like whisper-v3-turbo-FLM and
// gemma4-it-e2b-FLM, plus any vendor that keeps the family label.
// Unknown or empty model names default to incremental (the modern OpenAI
// convention) — if a new model turns out to emit cumulative deltas, the
// symptom is visible immediately as duplicated prefixes on partials, and
// the fix is to add it here.
func deltaStrategyForModel(model string) func(existing, delta string) string {
	lower := strings.ToLower(model)
	if strings.Contains(lower, "whisper") || strings.Contains(lower, "gemma") {
		return mergeCumulativeDelta
	}
	return mergeIncrementalDelta
}

// mergeIncrementalDelta treats each transcription.delta as new text to
// append to the running partial.
func mergeIncrementalDelta(existing, delta string) string {
	return existing + delta
}

// mergeCumulativeDelta treats each transcription.delta as the full
// transcript so far — replace rather than append.
func mergeCumulativeDelta(_, delta string) string {
	return delta
}
