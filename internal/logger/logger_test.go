package logger

import (
	"io"
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"INFO":    slog.LevelInfo,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"unknown": slog.LevelInfo,
		"":        slog.LevelInfo,
	}
	for input, want := range tests {
		if got := ParseLevel(input); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", input, got, want)
		}
	}
}

type formatProbe struct {
	evaluated *bool
}

func (p formatProbe) String() string {
	*p.evaluated = true
	return "probe"
}

func TestDebugfSkipsFormattingWhenDebugDisabled(t *testing.T) {
	previous := slog.Default()
	defer slog.SetDefault(previous)
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	evaluated := false
	Debugf("formatted %v", formatProbe{evaluated: &evaluated})
	if evaluated {
		t.Fatal("Debugf evaluated formatting arguments while debug logging was disabled")
	}
}
