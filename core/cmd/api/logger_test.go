package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestNewLoggerHonoursTheConfiguredLevel proves the observable effect of the
// LOG_LEVEL knob: at debug the low-level lines are emitted, and at info they are
// suppressed while info lines still pass. Without this, the variable could be
// parsed correctly and still change nothing about what the process logs.
func TestNewLoggerHonoursTheConfiguredLevel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		level     slog.Level
		wantDebug bool
	}{
		{level: slog.LevelDebug, wantDebug: true},
		{level: slog.LevelInfo, wantDebug: false},
	} {
		var out bytes.Buffer
		logger := newLogger(&out, tc.level)

		logger.Debug("email delivery is not configured in S1")
		logger.Info("starting CanchaLibre core API")

		logged := out.String()
		if tc.wantDebug && !strings.Contains(logged, "email delivery is not configured") {
			t.Fatalf("level %v did not emit the debug line: %q", tc.level, logged)
		}
		if !tc.wantDebug && strings.Contains(logged, "email delivery is not configured") {
			t.Fatalf("level %v emitted a debug line: %q", tc.level, logged)
		}
		if !strings.Contains(logged, "starting CanchaLibre core API") {
			t.Fatalf("level %v suppressed an info line: %q", tc.level, logged)
		}
	}
}
