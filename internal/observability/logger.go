package observability

import (
	"log/slog"
	"os"
)

// NewLogger creates the application JSON logger.
func NewLogger() *slog.Logger {
	handler := slog.NewJSONHandler(
		os.Stdout,
		&slog.HandlerOptions{
			Level: slog.LevelInfo,
		},
	)

	return slog.New(handler)
}
