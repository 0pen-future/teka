// Package logger builds the application slog.Logger and carries
// request-scoped loggers through context.
package logger

import (
	"context"
	"log/slog"
	"os"
)

// New returns a JSON logger for production and a text logger otherwise.
func New(production bool, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if production {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

type ctxKey struct{}

// IntoContext returns a context carrying l.
func IntoContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the request-scoped logger, or slog.Default() when none
// was attached.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
