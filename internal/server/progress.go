package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/andygeiss/mcp/internal/protocol"
)

type progressKey struct{}

// Progress provides notification sending from within a tool handler.
// Methods are nil-safe — calling them on a nil receiver is a no-op.
type Progress struct {
	server        *Server
	logger        *slog.Logger    // request-scoped logger (carries request_id) for AI10 drop warns
	token         json.RawMessage // from _meta.progressToken, nil if client didn't request progress
	outboundDepth atomic.Int32    // AI10: >0 while parked in protocol.SendRequest; Report drops while >0
}

// progressParams is the notification payload for notifications/progress.
type progressParams struct {
	Progress      int64           `json:"progress"`
	ProgressToken json.RawMessage `json:"progressToken"`
	Total         int64           `json:"total,omitempty"`
}

// messageParams is the notification payload for notifications/message.
type messageParams struct {
	Data   string `json:"data"`
	Level  string `json:"level"`
	Logger string `json:"logger,omitempty"`
}

// Log sends a notifications/message log entry. No-op if the receiver is nil.
//
// Log is intentionally NOT gated on the progress token — notifications/message
// is governed by the server's `logging` capability and `logging/setLevel`, not
// the per-request `_meta.progressToken`. Coupling the two would silence logs
// for every request that omits a progress token, which is the common case.
func (p *Progress) Log(level, logger, data string) {
	if p == nil {
		return
	}
	_ = p.server.sendNotification(protocol.NotificationMessage, messageParams{
		Data:   data,
		Level:  level,
		Logger: logger,
	})
}

// Report sends a progress notification. No-op if the receiver is nil, the
// client did not provide a `_meta.progressToken`, or the handler is currently
// parked in protocol.SendRequest (AI10 invariant from ADR-003).
//
// AI10 enforcement: (*Server).SendRequest brackets its await with
// `defer ProgressFromContext(ctx).suspendForOutbound()()`, so any Report
// invocation between outbound dispatch and response correlation is dropped
// here. This prevents notifications/progress from interleaving with the
// awaited inbound response on the wire — which would otherwise corrupt the
// pending-request map's response correlation.
//
// AI10 drops are surfaced as `progress_dropped_during_outbound` warnings on
// stderr (request-scoped logger, carries `request_id`) so operators see when
// a handler is interleaving Report with SendRequest — a contract violation
// that the gate silently corrects but should be fixed in the handler. The
// log is best-effort: the only side effect of the drop is the warn line and
// the absent notification.
func (p *Progress) Report(current, total int64) {
	if p == nil || p.token == nil {
		return
	}
	if p.outboundDepth.Load() > 0 {
		p.warnLogger().Warn("progress_dropped_during_outbound",
			"reason", "ai10_invariant",
			"progress", current,
			"total", total,
		)
		return
	}
	_ = p.server.sendNotification(protocol.NotificationProgress, progressParams{
		Progress:      current,
		ProgressToken: p.token,
		Total:         total,
	})
}

// warnLogger returns the request-scoped logger, falling back to the server's
// base logger if the Progress was constructed without one (test paths).
// Always returns a non-nil logger.
func (p *Progress) warnLogger() *slog.Logger {
	if p.logger != nil {
		return p.logger
	}
	return p.server.logger
}

// suspendForOutbound increments the AI10 outbound-await depth and returns a
// closure that decrements it. Idiomatic use:
//
//	defer ProgressFromContext(ctx).suspendForOutbound()()
//
// Nil-safe — callers do not need to nil-check the *Progress. The returned
// closure is idempotent (sync.Once-guarded): a double-invocation cannot drive
// the counter below zero and silently disable AI10 for the rest of the
// request. This bounds misuse without changing the well-formed defer-pattern
// path.
func (p *Progress) suspendForOutbound() func() {
	if p == nil {
		return func() {}
	}
	p.outboundDepth.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() { p.outboundDepth.Add(-1) })
	}
}

// ProgressFromContext extracts the Progress from context. Returns nil if absent.
func ProgressFromContext(ctx context.Context) *Progress {
	p, _ := ctx.Value(progressKey{}).(*Progress)
	return p
}

// withProgress injects a Progress into the context.
func withProgress(ctx context.Context, p *Progress) context.Context {
	return context.WithValue(ctx, progressKey{}, p)
}
