package transcribe

// VAD is the voice-activity-detection interface used by the dictation
// pump. The caller feeds PCM16 chunks from the recorder and gets back
// a VADEvent per chunk (None, SpeechStarted, SpeechStopped). Two
// implementations ship: an energy-threshold detector (ClientVAD, fast
// and self-contained) and a neural detector wrapping Silero VAD
// (sileroVAD, needs ONNX Runtime at runtime but much more robust on
// noisy mics).
type VAD interface {
	Feed(samples []int16) VADEvent
	Reset()
	InSpeech() bool
	Snapshot() VADSnapshot
}

// Compile-time check that ClientVAD satisfies the interface. Silero
// adds its own assertion.
var _ VAD = (*ClientVAD)(nil)
