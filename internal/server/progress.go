package server

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/andygeiss/mcp/internal/protocol"
)

type progressKey struct{}

// Progress provides notification sending from within a tool handler.
// Methods are nil-safe — calling them on a nil receiver is a no-op.
type Progress struct {
	server *Server
	token  json.RawMessage // from _meta.progressToken, nil if client didn't request progress
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

// Report sends a progress notification. No-op if the client didn't provide a token
// or if the receiver is nil.
func (p *Progress) Report(current, total int64) {
	if p == nil || p.token == nil {
		return
	}
	_ = p.server.sendNotification(protocol.NotificationProgress, progressParams{
		Progress:      current,
		ProgressToken: p.token,
		Total:         total,
	})
}

// Log sends a notifications/message log entry. No-op if the receiver is nil.
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

// ProgressFromContext extracts the Progress from context. Returns nil if absent.
func ProgressFromContext(ctx context.Context) *Progress {
	p, _ := ctx.Value(progressKey{}).(*Progress)
	return p
}

// SendRequestFromContext sends a JSON-RPC 2.0 request to the client from within
// a tool handler. Returns the client's response. Returns an error if called
// outside of a tool handler context.
func SendRequestFromContext(ctx context.Context, method string, params any) (protocol.Response, error) {
	p := ProgressFromContext(ctx)
	if p == nil || p.server == nil {
		return protocol.Response{}, errors.New("SendRequest: not in a tool handler context")
	}
	return p.server.SendRequest(ctx, method, params)
}

// withProgress injects a Progress into the context.
func withProgress(ctx context.Context, p *Progress) context.Context {
	return context.WithValue(ctx, progressKey{}, p)
}
