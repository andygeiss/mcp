package server

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/andygeiss/mcp/internal/protocol"
)

// loggingSetLevelParams is the expected structure of logging/setLevel params.
type loggingSetLevelParams struct {
	Level string `json:"level"`
}

// rfc5424ToSlog maps the eight RFC 5424 severity keywords required by the MCP
// logging/setLevel spec to slog levels. debug → Debug; info/notice → Info;
// warning → Warn; error and above → Error.
var rfc5424ToSlog = map[string]slog.Level{
	"alert":     slog.LevelError,
	"critical":  slog.LevelError,
	"debug":     slog.LevelDebug,
	"emergency": slog.LevelError,
	"error":     slog.LevelError,
	"info":      slog.LevelInfo,
	"notice":    slog.LevelInfo,
	"warning":   slog.LevelWarn,
}

// validLogLevels returns the eight acceptable RFC 5424 keywords in a stable
// alphabetical order, so structured error.data on invalid-level rejection
// reports the same expected list across calls.
func validLogLevels() []string {
	return []string{
		"alert", "critical", "debug", "emergency",
		"error", "info", "notice", "warning",
	}
}

// handleLoggingSetLevel sets the server's stderr log level. The level must be
// one of the eight RFC 5424 severity keywords required by MCP; any other value
// is rejected with -32602.
func (s *Server) handleLoggingSetLevel(ctx context.Context, msg protocol.Request) protocol.Response {
	var params loggingSetLevelParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.errorResponse(ctx, msg.ID, protocol.ErrInvalidParams("invalid logging/setLevel params"))
	}
	slogLevel, ok := rfc5424ToSlog[params.Level]
	if !ok {
		return s.errorResponse(ctx, msg.ID, invalidParamError(
			"invalid log level: "+params.Level,
			"level",
			params.Level,
			validLogLevels(),
		))
	}
	s.logLevel.Set(slogLevel)
	loggerFromContext(ctx, s.logger).Info("log_level_changed", "level", params.Level)

	return s.marshalResult(ctx, msg.ID, json.RawMessage("{}"),
		"marshal_logging_set_level", "failed to marshal logging/setLevel result")
}
