package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/andygeiss/mcp/internal/protocol"
)

// cancelledParams is the expected structure of notifications/cancelled params.
type cancelledParams struct {
	Reason    string          `json:"reason,omitempty"`
	RequestID json.RawMessage `json:"requestId"`
}

// dispatch routes a decoded message to the appropriate handler.
// Returns (response, true) if a response should be sent, or (_, false) for notifications.
func (s *Server) dispatch(ctx context.Context, msg protocol.Request) (protocol.Response, bool) {
	isNotification := len(msg.ID) == 0

	if vErr := protocol.Validate(msg); vErr != nil {
		if isNotification {
			return protocol.Response{}, false
		}
		return s.errorResponse(msg.ID, vErr), true
	}

	if isNotification {
		s.handleNotification(msg)
		return protocol.Response{}, false
	}

	// ping always works in any state.
	if msg.Method == protocol.MethodPing {
		resp, err := protocol.NewResultResponse(msg.ID, json.RawMessage("{}"))
		if err != nil {
			return s.errorResponse(msg.ID, protocol.ErrInternalError("failed to marshal ping result")), true
		}
		return resp, true
	}

	// rpc.* is reserved by JSON-RPC 2.0 §4.3 — reject in any state with
	// -32601 (method not found), never the state-gate -32000.
	if strings.HasPrefix(msg.Method, protocol.PrefixRPC) {
		return s.errorResponse(msg.ID, protocol.ErrMethodNotFound("reserved method: "+msg.Method)), true
	}

	return s.dispatchByState(ctx, msg), true
}

// dispatchByState enforces the initialization state machine for non-ping requests.
func (s *Server) dispatchByState(ctx context.Context, msg protocol.Request) protocol.Response {
	switch s.state {
	case stateUninitialized:
		if msg.Method != protocol.MethodInitialize {
			return s.errorResponse(msg.ID, protocol.ErrServerError("server not initialized (send initialize first)"))
		}
		return s.handleInitialize(msg)

	case stateInitializing:
		if msg.Method == protocol.MethodInitialize {
			return s.errorResponse(msg.ID, protocol.ErrServerError("initialize already received, awaiting notifications/initialized"))
		}
		return s.errorResponse(msg.ID, protocol.ErrServerError("server initializing, awaiting notifications/initialized"))

	case stateReady:
		if msg.Method == protocol.MethodInitialize {
			return s.errorResponse(msg.ID, protocol.ErrServerError("already initialized"))
		}
		return s.handleMethod(ctx, msg)

	default:
		return s.errorResponse(msg.ID, protocol.ErrInternalError("unknown server state"))
	}
}

// errorResponse builds a JSON-RPC error response from any error. If the error
// is a *protocol.CodeError, its code and message are used directly.
// Otherwise, the error falls back to -32603 (internal error).
func (s *Server) errorResponse(id json.RawMessage, err error) protocol.Response {
	s.errorCount.Add(1)
	pe, ok := errors.AsType[*protocol.CodeError](err)
	if !ok {
		s.logger.Error("internal_error", "error", err)
		return protocol.NewErrorResponse(id, protocol.InternalError, "internal error")
	}
	return protocol.NewErrorResponseFromCodeError(id, pe)
}

// handleNotification processes notification messages (no response sent).
// Per MCP: notifications are fire-and-forget — unknown or out-of-state
// notifications are silently ignored, never logged at non-debug level, and
// never responded to.
func (s *Server) handleNotification(msg protocol.Request) {
	switch msg.Method {
	case protocol.NotificationCancelled:
		s.handleCancelledNotification(msg)
	case protocol.NotificationInitialized:
		if s.state != stateInitializing {
			return
		}
		s.state = stateReady
		s.logger.Info("server_ready")
	}
}

// handleCancelledNotification cancels the in-flight tool handler if the
// request ID matches. Non-matching, stale, or malformed cancellations are
// silently ignored per the notification contract.
func (s *Server) handleCancelledNotification(msg protocol.Request) {
	if s.cancelInFlight == nil {
		return
	}
	var params cancelledParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}
	if !bytes.Equal(params.RequestID, s.inFlightID) {
		return
	}
	s.inFlightCancelled.Store(true)
	s.cancelInFlight()
}

// sendNotification writes a JSON-RPC 2.0 notification to stdout. Notifications
// have no ID and expect no response. Safe for concurrent use from handler goroutines.
func (s *Server) sendNotification(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		s.logger.Error("marshal_notification_params", "error", err, "method", method)
		return fmt.Errorf("marshal notification params: %w", err)
	}
	n := protocol.Notification{
		JSONRPC: protocol.Version,
		Method:  method,
		Params:  json.RawMessage(raw),
	}
	if s.trace {
		if traceRaw, err := json.Marshal(n); err != nil {
			s.logger.Warn("trace_marshal_notification", "error", err)
		} else {
			s.logger.Debug("trace_notification", "direction", "→", "message", string(traceRaw))
		}
	}
	s.stdoutMu.Lock()
	err = s.enc.Encode(&n)
	s.stdoutMu.Unlock()
	if err != nil {
		s.logger.Error("encode_notification", "error", err, "method", method)
		return fmt.Errorf("encode notification: %w", err)
	}
	return nil
}

// encodeResponse writes a JSON-RPC response to stdout with optional trace logging.
// All stdout writes are serialized via stdoutMu to prevent interleaved output
// when notifications are sent concurrently from a tool handler goroutine.
func (s *Server) encodeResponse(resp protocol.Response) error {
	if s.trace {
		if raw, err := json.Marshal(resp); err != nil {
			s.logger.Warn("trace_marshal_response", "error", err)
		} else {
			s.logger.Debug("trace_response", "direction", "→", "message", string(raw))
		}
	}
	s.stdoutMu.Lock()
	err := protocol.Encode(s.enc, resp)
	s.stdoutMu.Unlock()
	if err != nil {
		s.logger.Error("encode_error", "error", err)
		return fmt.Errorf("encode error: %w", err)
	}
	return nil
}
