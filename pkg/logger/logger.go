// Package logger provides a thin wrapper around log/slog for structured logging.
package logger

import (
	"log/slog"
	"os"
)

// Init sets the default slog logger to a JSON handler writing to stderr.
func Init(level slog.Level) {
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h))
}
