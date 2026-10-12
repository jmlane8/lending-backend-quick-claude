package httpx

import (
	"context"
	"log/slog"
)

type loggerKey struct{}

// WithLogger returns a context carrying the request-scoped logger.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// LoggerFrom returns the request-scoped logger (tagged with request_id),
// or slog.Default when none is set.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
