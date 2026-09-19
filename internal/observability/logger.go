package observability

import (
	"context"
	"log/slog"
	"os"

	"github.com/pablo-banker/junglegaming-test/internal/config"
)

type contextAttrsKey struct{}

// WithAttrs returns a context whose log records carry the given attributes.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	existing, _ := ctx.Value(contextAttrsKey{}).([]slog.Attr)

	merged := make([]slog.Attr, 0, len(existing)+len(attrs))
	merged = append(merged, existing...)
	merged = append(merged, attrs...)

	return context.WithValue(ctx, contextAttrsKey{}, merged)
}

// NewLogger creates the JSON logger. Records logged with a context include its attributes.
func NewLogger(cfg config.Config) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	})

	return slog.New(contextHandler{Handler: handler})
}

// contextHandler adds the attributes stored by WithAttrs to every record.
type contextHandler struct {
	slog.Handler
}

// Handle writes the record with the context attributes.
func (h contextHandler) Handle(ctx context.Context, record slog.Record) error {
	if attrs, ok := ctx.Value(contextAttrsKey{}).([]slog.Attr); ok {
		record.AddAttrs(attrs...)
	}

	return h.Handler.Handle(ctx, record)
}

// WithAttrs keeps the context handler when attributes are bound to the logger.
func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup keeps the context handler when a group is opened.
func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{Handler: h.Handler.WithGroup(name)}
}
