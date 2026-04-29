package server

import (
	"context"
	"encoding/json"
	"log/slog"
)

// loggerCtxKey is the unexported type for the per-request logger key. Keeping
// the type unexported prevents foreign packages from injecting or reading
// values under the same key.
type loggerCtxKey struct{}

// withRequestLogger returns ctx carrying base augmented with a per-request
// `request_id` attribute derived from id. Subsequent code in the dispatch
// path retrieves the augmented logger via loggerFromContext, so logs about
// a specific request automatically include its id without manual plumbing.
// When id is empty (notification), base is stored as-is.
func withRequestLogger(ctx context.Context, base *slog.Logger, id json.RawMessage) context.Context {
	logger := base
	if len(id) > 0 {
		logger = base.With("request_id", string(id))
	}
	return context.WithValue(ctx, loggerCtxKey{}, logger)
}

// loggerFromContext returns the per-request logger attached to ctx, or
// fallback if none was attached. Always returns a non-nil logger so call
// sites never need a nil check.
func loggerFromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if logger, ok := ctx.Value(loggerCtxKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return fallback
}
