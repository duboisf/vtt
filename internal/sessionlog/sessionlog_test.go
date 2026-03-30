package sessionlog

import "testing"

func TestLogColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		line string
		want string
	}{
		{line: "transcription complete: 42 characters", want: ansiCyan},
		{line: "audio captured successfully: sample.wav (32000 bytes)", want: ansiGreen},
		{line: "hotkey ctrl+shift+space unavailable, using f8", want: ansiYellow},
		{line: "insert transcript: paste text failed", want: ansiRed},
		{line: "starting recording for window=123", want: ansiBlue},
		{line: "loaded config: /tmp/config.json", want: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.line, func(t *testing.T) {
			t.Parallel()

			if got := logColor(tt.line); got != tt.want {
				t.Fatalf("logColor(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}
