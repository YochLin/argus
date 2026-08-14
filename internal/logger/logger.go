// Package logger provides the application's small logging facade.
//
// The implementation is log/slog, while callers import this package as logger:
//
//	import "argus/internal/logger"
//
// Keeping the facade here lets the application configure one handler and one
// level for the bot, scheduler, web server, and command-line tools without
// making every package know about the output destination.
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Configure replaces slog's default logger with a text logger writing to w.
// Text output preserves the current human-readable log shape while adding a
// level field. JSON can be introduced later at the process boundary without
// changing call sites.
func Configure(w io.Writer, level slog.Level) {
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	})))
}

// ParseLevel parses LOG_LEVEL-style values. Unknown or empty values default
// to INFO so a typo never makes the application silently stop logging.
func ParseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Debug logs a structured debug message.
func Debug(msg string, args ...any) { slog.Debug(msg, args...) }

// Info logs a structured informational message.
func Info(msg string, args ...any) { slog.Info(msg, args...) }

// Warn logs a structured warning message.
func Warn(msg string, args ...any) { slog.Warn(msg, args...) }

// Error logs a structured error message.
func Error(msg string, args ...any) { slog.Error(msg, args...) }

// Debugf is the compatibility form for messages that still use printf-style
// formatting while callers are migrated to structured fields incrementally.
func Debugf(format string, args ...any) { Debug(fmt.Sprintf(format, args...)) }

// Infof is the compatibility form for printf-style informational messages.
func Infof(format string, args ...any) { Info(fmt.Sprintf(format, args...)) }

// Warnf is the compatibility form for printf-style warnings.
func Warnf(format string, args ...any) { Warn(fmt.Sprintf(format, args...)) }

// Errorf is the compatibility form for printf-style errors.
func Errorf(format string, args ...any) { Error(fmt.Sprintf(format, args...)) }

// Fatalf logs an error and terminates the process. It retains the behavior of
// the standard logger.Fatalf calls used by startup and scheduler registration.
func Fatalf(format string, args ...any) {
	Errorf(format, args...)
	os.Exit(1)
}
