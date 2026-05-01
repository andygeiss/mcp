package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/tools"
)

// inFlightResult carries the outcome of an async tool call handler.
type inFlightResult struct {
	isError bool
	resp    protocol.Response
}

// toolCallParams is the expected structure of tools/call params.
type toolCallParams struct {
	Arguments json.RawMessage `json:"arguments"`
	Name      string          `json:"name"`
}

// panicDiag is the machine-readable diagnostic attached to Error.Data when a
// tool handler panics. The actual panic value is logged to stderr only —
// never sent to the client (security).
type panicDiag struct {
	ToolName string `json:"toolName"`
}

// timingDiag is the machine-readable diagnostic attached to Error.Data for
// timeout and cancellation errors.
type timingDiag struct {
	ElapsedMs int64  `json:"elapsedMs"`
	TimeoutMs int64  `json:"timeoutMs,omitempty"`
	ToolName  string `json:"toolName"`
}

// toolOutcome carries the result of an async tool handler goroutine.
type toolOutcome struct {
	err    *toolError
	result tools.Result
}

// toolError carries the error code, message, and structured data from dispatchToolCall.
type toolError struct {
	code    int
	data    any
	message string
}

// extractProgressToken extracts the _meta.progressToken from raw tool call
// params. Returns nil if absent OR if the token is the JSON literal `null`
// (which a map decode would otherwise plumb through as the 4-byte RawMessage
// `null`, causing Report to emit `"progressToken":null` on the wire — a spec
// violation since the client did not actually opt in to progress).
//
// The _meta field uses a leading underscore per the MCP spec, so it is
// extracted via map access rather than struct tags to satisfy the camelCase
// linter rule.
func extractProgressToken(raw json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	meta, ok := m["_meta"]
	if !ok {
		return nil
	}
	var metaObj map[string]json.RawMessage
	if json.Unmarshal(meta, &metaObj) != nil {
		return nil
	}
	tok := metaObj["progressToken"]
	if bytes.Equal(tok, []byte("null")) {
		return nil
	}
	return tok
}

// startToolCallAsync validates tool call params and, if valid, spawns the
// handler in a background goroutine. Returns (errorResp, false) if validation
// fails, or (_, true) if the handler was started successfully.
func (s *Server) startToolCallAsync(ctx context.Context, msg protocol.Request) (protocol.Response, bool) {
	// Inject a per-request slog.Logger so logs emitted from within the
	// async tool path automatically carry request_id without manual plumbing.
	ctx = withRequestLogger(ctx, s.logger, msg.ID)

	var params toolCallParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		loggerFromContext(ctx, s.logger).Warn("invalid_tools_call_params", "error", err)
		return s.errorResponse(ctx, msg.ID, protocol.ErrInvalidParams("invalid tools/call params")), false
	}
	if len(params.Arguments) == 0 || bytes.Equal(params.Arguments, []byte("null")) {
		params.Arguments = json.RawMessage("{}")
	}
	if params.Name == "" {
		return s.errorResponse(ctx, msg.ID, protocol.ErrInvalidParams("tool name is required")), false
	}
	tool, ok := s.registry.Lookup(params.Name)
	if !ok {
		names := s.registry.Names()
		available := strings.Join(names, ", ")
		return s.errorResponse(ctx, msg.ID, invalidParamError(
			"unknown tool: "+params.Name+" (available: "+available+")",
			"name",
			params.Name,
			names,
		)), false
	}

	callCtx, cancel := context.WithCancel(ctx)
	s.cancelInFlight = cancel
	s.inFlightID = msg.ID
	s.inFlightCh = make(chan inFlightResult, 1)
	// Reset at spawn so a stale cancelled notification for the previous request
	// cannot suppress this handler's response.
	s.inFlightCancelled.Store(false)

	// Inject progress notifier into handler context. The request-scoped
	// logger (already attached to callCtx via withRequestLogger above) is
	// captured on the Progress so AI10 drop warnings carry request_id
	// without the handler having to plumb context to the warn site.
	prog := &Progress{
		server: s,
		logger: loggerFromContext(callCtx, s.logger),
		token:  extractProgressToken(msg.Params),
	}
	callCtx = withProgress(callCtx, prog)
	// Inject the server as the outbound Peer so handlers reach the bidi path
	// via protocol.SendRequest without importing internal/server (Invariant I1).
	callCtx = protocol.ContextWithPeer(callCtx, s)

	go func() {
		defer cancel()
		resp := s.executeToolCall(callCtx, msg.ID, tool, params)
		s.inFlightCh <- inFlightResult{isError: resp.Error != nil, resp: resp}
	}()

	return protocol.Response{}, true
}

// executeToolCall runs a tool call synchronously (called from a goroutine).
// It calls dispatchToolCall and processes the result into a protocol.Response.
func (s *Server) executeToolCall(ctx context.Context, id json.RawMessage, tool tools.Tool, params toolCallParams) protocol.Response {
	result, toolErr := s.dispatchToolCall(ctx, tool, params.Arguments)
	if toolErr != nil {
		data, err := json.Marshal(toolErr.data)
		if err != nil {
			s.logger.Warn("marshal_tool_error_data", "error", err)
			data = nil
		}
		ce := &protocol.CodeError{Code: toolErr.code, Message: toolErr.message}
		ce.Data = data
		return protocol.NewErrorResponseFromCodeError(id, ce)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		s.logger.Error("marshal_tool_result", "error", err, "tool_name", params.Name)
		ce := protocol.ErrInternalError("failed to marshal tool result")
		return protocol.NewErrorResponseFromCodeError(id, ce)
	}
	if len(resultJSON) > maxResultSize {
		s.logger.Warn("tool_result_truncated", "tool_name", params.Name, "size", len(resultJSON), "limit", maxResultSize)
		result = tools.TextResult(fmt.Sprintf("[result truncated: exceeded maximum size of %d bytes]", maxResultSize))
		if resultJSON, err = json.Marshal(result); err != nil {
			s.logger.Error("marshal_truncated_result", "error", err, "tool_name", params.Name)
			ce := protocol.ErrInternalError("failed to marshal truncated result")
			return protocol.NewErrorResponseFromCodeError(id, ce)
		}
	}

	resp, err := protocol.NewResultResponse(id, json.RawMessage(resultJSON))
	if err != nil {
		s.logger.Error("marshal_tool_result", "error", err, "tool_name", params.Name)
		ce := protocol.ErrInternalError("failed to marshal tool result")
		return protocol.NewErrorResponseFromCodeError(id, ce)
	}
	return resp
}

// cancelAndAwaitInFlight cancels the in-flight handler and waits for it to
// complete within the handler timeout + safety margin.
func (s *Server) cancelAndAwaitInFlight() {
	if s.cancelInFlight == nil {
		return
	}
	s.cancelInFlight()
	timer := time.NewTimer(2 * (s.handlerTimeout + s.safetyMargin))
	defer timer.Stop()
	select {
	case <-s.inFlightCh:
	case <-timer.C:
		s.logger.Warn("handler_abandoned", "request_id", string(s.inFlightID))
	}
	s.clearInFlight()
}

// processInFlightResult handles a completed tool call result. If the request
// was cancelled via notifications/cancelled, the response is suppressed per
// MCP spec (receivers SHOULD NOT send a response for cancelled requests).
func (s *Server) processInFlightResult(ifr inFlightResult) error {
	if s.inFlightCancelled.Load() {
		s.clearInFlight()
		return nil
	}
	if ifr.isError {
		s.errorCount.Add(1)
	}
	if err := s.encodeResponse(ifr.resp); err != nil {
		s.clearInFlight()
		return err
	}
	s.clearInFlight()
	return nil
}

// clearInFlight resets all in-flight state after a tool call completes.
func (s *Server) clearInFlight() {
	s.cancelInFlight = nil
	s.inFlightCancelled.Store(false)
	s.inFlightCh = nil
	s.inFlightID = nil
}

// runToolHandler executes the tool handler in a goroutine with panic recovery
// and timeout. Sends the outcome on the returned channel.
func (s *Server) runToolHandler(ctx context.Context, tool tools.Tool, params json.RawMessage, start time.Time) chan toolOutcome {
	ch := make(chan toolOutcome, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("tool_handler_panicked", "tool_name", tool.Name, "panic", r)
				ch <- toolOutcome{err: &toolError{
					code:    protocol.InternalError,
					data:    &panicDiag{ToolName: tool.Name},
					message: fmt.Sprintf("internal error: tool %q panicked", tool.Name),
				}}
			}
		}()
		hctx, cancel := context.WithTimeout(ctx, s.handlerTimeout)
		defer cancel()
		result, err := tool.Handler(hctx, params)
		if err != nil {
			if pe, ok := errors.AsType[*protocol.CodeError](err); ok {
				ch <- toolOutcome{err: &toolError{code: pe.Code, data: pe.Data, message: pe.Message}}
			} else {
				s.logger.Error("tool_handler_error", "tool_name", tool.Name, "error", err)
				ch <- toolOutcome{err: &toolError{
					code:    protocol.InternalError,
					message: fmt.Sprintf("internal error: tool %q failed", tool.Name),
				}}
			}
			return
		}
		if errors.Is(hctx.Err(), context.DeadlineExceeded) {
			ch <- toolOutcome{err: &toolError{
				code:    protocol.ServerTimeout,
				data:    &timingDiag{ElapsedMs: time.Since(start).Milliseconds(), TimeoutMs: s.handlerTimeout.Milliseconds(), ToolName: tool.Name},
				message: fmt.Sprintf("tool %q execution timed out", tool.Name),
			}}
			return
		}
		ch <- toolOutcome{result: result}
	}()
	return ch
}

// dispatchToolCall invokes a tool handler with timeout enforcement and context
// cancellation. Returns the result and a *toolError if the handler timed out or
// was cancelled.
//
// Limitation: Go cannot kill goroutines. If a handler ignores ctx.Done()
// and exceeds the handler timeout + safety margin, the goroutine is
// abandoned. Handlers MUST respect context cancellation to guarantee
// bounded goroutine lifetime. The server proceeds to the next request
// regardless, but the abandoned goroutine may hold resources until it
// returns naturally.
func (s *Server) dispatchToolCall(ctx context.Context, tool tools.Tool, params json.RawMessage) (tools.Result, *toolError) {
	start := time.Now()
	ch := s.runToolHandler(ctx, tool, params, start)
	deadline := time.NewTimer(s.handlerTimeout + s.safetyMargin)
	defer deadline.Stop()
	cancelErr := func() *toolError {
		s.logger.Error("tool_handler_cancelled", "tool_name", tool.Name)
		return &toolError{
			code:    protocol.ServerTimeout,
			data:    &timingDiag{ElapsedMs: time.Since(start).Milliseconds(), ToolName: tool.Name},
			message: fmt.Sprintf("tool %q execution cancelled", tool.Name),
		}
	}
	select {
	case o := <-ch:
		return o.result, o.err
	case <-ctx.Done():
		return tools.Result{}, cancelErr()
	case <-deadline.C:
		// Prefer cancellation over safety-timer timeout when both fire.
		select {
		case <-ctx.Done():
			return tools.Result{}, cancelErr()
		default:
		}
		s.logger.Error("tool_handler_timeout", "tool_name", tool.Name)
		return tools.Result{}, &toolError{
			code:    protocol.ServerTimeout,
			data:    &timingDiag{ElapsedMs: time.Since(start).Milliseconds(), TimeoutMs: s.handlerTimeout.Milliseconds(), ToolName: tool.Name},
			message: fmt.Sprintf("tool %q execution timed out", tool.Name),
		}
	}
}
