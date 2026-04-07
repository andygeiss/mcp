// Package server implements the MCP server lifecycle, dispatch, and resilience.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/tools"
)

const (
	defaultHandlerTimeout = 30 * time.Second
	defaultSafetyMargin   = time.Second
)

// Server states. stateUninitialized must be iota 0 so the zero value of
// Server.state matches the uninitialized lifecycle stage.
const (
	stateUninitialized = iota
	stateInitializing
	stateReady
)

// Server is the MCP server that communicates over stdin/stdout using JSON-RPC 2.0.
// It manages a three-state lifecycle: uninitialized → initializing (after
// initialize request) → ready (after notifications/initialized). Methods
// other than ping and initialize are rejected until the server reaches the
// ready state.
type Server struct {
	cancelInFlight    context.CancelFunc
	counting          *countingReader
	dec               *json.Decoder
	enc               *json.Encoder
	errorCount        int
	handlerTimeout    time.Duration
	inFlightCancelled bool
	inFlightCh        chan inFlightResult
	inFlightID        json.RawMessage
	logger            *slog.Logger
	name              string
	registry          *tools.Registry
	requestCount      int
	safetyMargin      time.Duration
	state             int
	trace             bool
	version           string
}

// inFlightResult carries the outcome of an async tool call handler.
type inFlightResult struct {
	isError bool
	resp    protocol.Response
}

// cancelledParams is the expected structure of notifications/cancelled params.
type cancelledParams struct {
	Reason    string          `json:"reason,omitempty"`
	RequestID json.RawMessage `json:"requestId"`
}

// decodeResult carries the outcome of an async decode operation.
type decodeResult struct {
	err      error
	exceeded bool
	msg      protocol.Request
}

// Option configures the Server.
type Option func(*Server)

// WithHandlerTimeout sets the maximum duration for tool handler execution.
// The default is 30 seconds.
func WithHandlerTimeout(d time.Duration) Option {
	return func(s *Server) {
		s.handlerTimeout = d
	}
}

// WithTrace enables protocol trace mode. When enabled, every incoming request
// and outgoing response is logged to stderr. Zero overhead when disabled.
func WithTrace(enabled bool) Option {
	return func(s *Server) {
		s.trace = enabled
	}
}

// WithSafetyMargin sets the additional time after handler timeout before the
// safety timer fires to force-fail unresponsive handlers. The default is 1s.
func WithSafetyMargin(d time.Duration) Option {
	return func(s *Server) {
		s.safetyMargin = d
	}
}

// NewServer creates a new MCP server. The name and version identify the server
// in initialize responses. The registry holds all registered tools. Pass
// io.Reader/io.Writer (not *os.File) so tests can inject bytes.Buffer for
// stdin/stdout/stderr. Protocol output goes to stdout exclusively; all
// diagnostics are logged to stderr via slog.JSONHandler.
func NewServer(name, version string, registry *tools.Registry, stdin io.Reader, stdout, stderr io.Writer, opts ...Option) *Server {
	cr := newCountingReader(stdin, maxMessageSize)
	s := &Server{
		counting:       cr,
		dec:            json.NewDecoder(cr),
		enc:            json.NewEncoder(stdout),
		handlerTimeout: defaultHandlerTimeout,
		safetyMargin:   defaultSafetyMargin,
		logger:         slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
		name:           name,
		registry:       registry,
		state:          stateUninitialized,
		version:        version,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Run starts the dispatch loop. It decodes messages from stdin and dispatches
// them to the appropriate handler. Tool calls are dispatched asynchronously so
// the decode loop can continue reading messages for cancellation and ping
// support while a handler is in flight. The server advertises maxInFlight: 1,
// so only one tool handler runs at a time; additional requests while a handler
// is in flight are rejected with -32600.
//
// Run returns nil for clean shutdown (EOF or context cancellation) or an error
// for fatal decode conditions (after sending a -32700 response).
func (s *Server) Run(ctx context.Context) error {
	startTime := time.Now()
	s.logger.Info("server_started",
		"name", s.name,
		"protocol_version", protocol.MCPVersion,
		"tools", s.registry.Names(),
		"version", s.version,
	)
	var runErr error
	defer func() {
		s.logShutdown(ctx, runErr, startTime)
	}()

	decodeCh := make(chan decodeResult, 1)
	startDecode := func() {
		go func() {
			s.counting.Reset()
			msg, err := protocol.Decode(s.dec)
			decodeCh <- decodeResult{msg: msg, err: err, exceeded: s.counting.Exceeded()}
		}()
	}

	startDecode()

	for {
		var loopErr error
		if s.inFlightCh != nil {
			loopErr = s.runInFlight(ctx, decodeCh, startDecode)
		} else {
			loopErr = s.runIdle(ctx, decodeCh, startDecode)
		}
		if errors.Is(loopErr, errShutdown) {
			return runErr
		}
		if loopErr != nil {
			runErr = loopErr
			return runErr
		}
	}
}

// errShutdown is a sentinel indicating clean shutdown from ctx.Done or EOF.
var errShutdown = errors.New("shutdown")

// runInFlight handles one iteration of the dispatch loop while a tool handler
// is in flight. Returns errShutdown for clean exit, a real error for fatal
// conditions, or nil to continue the loop.
func (s *Server) runInFlight(ctx context.Context, decodeCh chan decodeResult, startDecode func()) error {
	// Prioritize handler completion over new messages to avoid rejecting
	// requests that arrive after the handler has already finished.
	select {
	case ifr := <-s.inFlightCh:
		return s.processInFlightResult(ifr)
	default:
	}

	select {
	case ifr := <-s.inFlightCh:
		return s.processInFlightResult(ifr)

	case dr := <-decodeCh:
		err := s.handleDecodeResultDuringInFlight(ctx, dr)
		if err != nil || dr.err != nil || dr.exceeded {
			if err != nil {
				return err
			}
			return errShutdown
		}
		startDecode()
		return nil

	case <-ctx.Done():
		s.cancelAndAwaitInFlight()
		return errShutdown
	}
}

// runIdle handles one iteration of the dispatch loop when no handler is in
// flight. Returns errShutdown for clean exit, a real error for fatal
// conditions, or nil to continue the loop.
func (s *Server) runIdle(ctx context.Context, decodeCh chan decodeResult, startDecode func()) error {
	select {
	case dr := <-decodeCh:
		err := s.handleDecodeResultIdle(ctx, dr)
		if err != nil || dr.err != nil || dr.exceeded {
			if err != nil {
				return err
			}
			return errShutdown
		}
		startDecode()
		return nil

	case <-ctx.Done():
		return errShutdown
	}
}

// logShutdown emits the structured shutdown log entry.
func (s *Server) logShutdown(ctx context.Context, runErr error, startTime time.Time) {
	var reason string
	switch {
	case runErr != nil:
		reason = "fatal_error"
	case ctx.Err() != nil:
		reason = "context_cancelled"
		if c := context.Cause(ctx); c != nil {
			reason = c.Error()
		}
	default:
		reason = "eof"
	}
	s.logger.Info("server_stopped",
		"errors", s.errorCount,
		"reason", reason,
		"requests", s.requestCount,
		"uptime_ms", time.Since(startTime).Milliseconds(),
	)
}

// handleDecodeResultDuringInFlight processes a decode result that arrived while
// a tool handler is in flight. On decode error: waits for the handler, sends its
// response, then returns the decode error. On success: processes the message.
func (s *Server) handleDecodeResultDuringInFlight(ctx context.Context, dr decodeResult) error {
	if dr.err != nil || dr.exceeded {
		return s.handleDecodeErrorDuringInFlight(dr)
	}

	s.requestCount++
	if s.trace {
		// Marshal cannot fail: protocol.Request contains only JSON-safe fields
		// (string, json.RawMessage).
		raw, _ := json.Marshal(dr.msg)
		s.logger.Info("trace_request", "direction", "←", "message", string(raw))
	}

	// Check if handler completed concurrently with the decode.
	select {
	case ifr := <-s.inFlightCh:
		if err := s.processInFlightResult(ifr); err != nil {
			return err
		}
		return s.handleDecodeResultIdle(ctx, decodeResult{msg: dr.msg})
	default:
	}

	if err := s.handleMessageDuringInFlight(dr.msg); err != nil {
		s.cancelAndAwaitInFlight()
		return err
	}
	return nil
}

// handleDecodeErrorDuringInFlight processes a decode error while a tool handler
// is in flight. It waits for the handler to complete and sends its response
// before returning the decode error.
func (s *Server) handleDecodeErrorDuringInFlight(dr decodeResult) error {
	decErr := dr.err
	if dr.exceeded {
		decErr = fmt.Errorf("decode message: %w", errMessageTooLarge)
	}
	decodeRunErr := s.handleDecodeError(decErr)
	// Wait for handler. Use 2x the handler budget to outlast the internal safety timer.
	timer := time.NewTimer(2 * (s.handlerTimeout + s.safetyMargin))
	select {
	case ifr := <-s.inFlightCh:
		timer.Stop()
		if !s.inFlightCancelled {
			if ifr.isError {
				s.errorCount++
			}
			if encErr := s.encodeResponse(ifr.resp); encErr != nil && decodeRunErr == nil {
				decodeRunErr = encErr
			}
		}
	case <-timer.C:
		s.logger.Warn("handler_abandoned", "request_id", string(s.inFlightID))
	}
	s.clearInFlight()
	return decodeRunErr
}

// handleDecodeResultIdle processes a decode result when no handler is in flight.
// Returns a non-nil error for fatal conditions.
func (s *Server) handleDecodeResultIdle(ctx context.Context, dr decodeResult) error {
	if dr.err != nil || dr.exceeded {
		decErr := dr.err
		if dr.exceeded {
			decErr = fmt.Errorf("decode message: %w", errMessageTooLarge)
		}
		return s.handleDecodeError(decErr)
	}

	s.requestCount++
	if s.trace {
		// Marshal cannot fail: protocol.Request contains only JSON-safe fields
		// (string, json.RawMessage).
		raw, _ := json.Marshal(dr.msg)
		s.logger.Info("trace_request", "direction", "←", "message", string(raw))
	}

	// Intercept tools/call for async dispatch when ready.
	if dr.msg.Method == protocol.MethodToolsCall && s.state == stateReady && len(dr.msg.ID) > 0 {
		if vErr := protocol.Validate(dr.msg); vErr != nil {
			return s.encodeResponse(s.errorResponse(dr.msg.ID, vErr))
		}
		errResp, started := s.startToolCallAsync(ctx, dr.msg)
		if !started {
			return s.encodeResponse(errResp)
		}
		return nil
	}

	// Normal synchronous dispatch for all other messages.
	resp, respond := s.dispatch(dr.msg)
	if respond {
		return s.encodeResponse(resp)
	}
	return nil
}

// encodeResponse writes a JSON-RPC response to stdout with optional trace logging.
func (s *Server) encodeResponse(resp protocol.Response) error {
	if s.trace {
		// Marshal cannot fail: protocol.Response contains only JSON-safe fields
		// (string, int, json.RawMessage).
		raw, _ := json.Marshal(resp)
		s.logger.Info("trace_response", "direction", "→", "message", string(raw))
	}
	if err := protocol.Encode(s.enc, resp); err != nil {
		s.logger.Error("encode_error", "error", err)
		return fmt.Errorf("encode error: %w", err)
	}
	return nil
}

// handleMessageDuringInFlight processes messages that arrive while a tool
// handler is executing. Ping is answered immediately; notifications (including
// cancellation) are handled normally; all other requests are rejected with
// -32600 since maxInFlight is 1.
func (s *Server) handleMessageDuringInFlight(msg protocol.Request) error {
	isNotification := len(msg.ID) == 0

	if vErr := protocol.Validate(msg); vErr != nil {
		if isNotification {
			return nil
		}
		return s.encodeResponse(s.errorResponse(msg.ID, vErr))
	}

	if isNotification {
		s.handleNotification(msg)
		return nil
	}

	if msg.Method == protocol.MethodPing {
		resp, err := protocol.NewResultResponse(msg.ID, json.RawMessage("{}"))
		if err != nil {
			return s.encodeResponse(s.errorResponse(msg.ID, protocol.ErrInternalError("failed to marshal ping result")))
		}
		return s.encodeResponse(resp)
	}

	return s.encodeResponse(s.errorResponse(msg.ID, protocol.ErrInvalidRequest("server busy: request in flight (maxInFlight is 1)")))
}

// startToolCallAsync validates tool call params and, if valid, spawns the
// handler in a background goroutine. Returns (errorResp, false) if validation
// fails, or (_, true) if the handler was started successfully.
func (s *Server) startToolCallAsync(ctx context.Context, msg protocol.Request) (protocol.Response, bool) {
	var params toolCallParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.errorResponse(msg.ID, protocol.ErrInvalidParams("invalid tools/call params: "+err.Error())), false
	}
	if len(params.Arguments) == 0 || bytes.Equal(params.Arguments, []byte("null")) {
		params.Arguments = json.RawMessage("{}")
	}
	if params.Name == "" {
		return s.errorResponse(msg.ID, protocol.ErrInvalidParams("tool name is required")), false
	}
	tool, ok := s.registry.Lookup(params.Name)
	if !ok {
		available := strings.Join(s.registry.Names(), ", ")
		return s.errorResponse(msg.ID, protocol.ErrInvalidParams("unknown tool: "+params.Name+" (available: "+available+")")), false
	}

	callCtx, cancel := context.WithCancel(ctx)
	s.cancelInFlight = cancel
	s.inFlightID = msg.ID
	s.inFlightCh = make(chan inFlightResult, 1)

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
			slog.Warn("marshal tool error data failed", "error", err)
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
		// Marshal cannot fail: tools.TextResult contains only string fields.
		resultJSON, _ = json.Marshal(result)
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
	if s.inFlightCancelled {
		s.clearInFlight()
		return nil
	}
	if ifr.isError {
		s.errorCount++
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
	s.inFlightCancelled = false
	s.inFlightCh = nil
	s.inFlightID = nil
}

// handleDecodeError processes errors from the decoder, returning nil for clean
// shutdown (EOF) or an error for fatal conditions.
func (s *Server) handleDecodeError(err error) error {
	// Check for size limit BEFORE EOF — the countingReader returns
	// errMessageTooLarge which errors.Is can match through any wrapping.
	isTooLarge := errors.Is(err, errMessageTooLarge)

	if !isTooLarge && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
		return nil
	}

	s.errorCount++
	msg := err.Error()
	if isTooLarge {
		msg = "message exceeds 4MB size limit"
	}

	s.logger.Error("decode_error", "error", err)
	resp := protocol.NewErrorResponse(protocol.NullID(), protocol.ParseError, msg)
	if encErr := protocol.Encode(s.enc, resp); encErr != nil {
		s.logger.Error("encode_error", "error", encErr)
	}
	return fmt.Errorf("fatal decode error: %w", err)
}

// dispatch routes a decoded message to the appropriate handler.
// Returns (response, true) if a response should be sent, or (_, false) for notifications.
func (s *Server) dispatch(msg protocol.Request) (protocol.Response, bool) {
	isNotification := len(msg.ID) == 0

	// Centralized request validation.
	if vErr := protocol.Validate(msg); vErr != nil {
		if isNotification {
			return protocol.Response{}, false
		}
		return s.errorResponse(msg.ID, vErr), true
	}

	// Handle notifications.
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

	// State machine enforcement.
	switch s.state {
	case stateUninitialized:
		if msg.Method != protocol.MethodInitialize {
			return s.errorResponse(msg.ID, protocol.ErrInvalidRequest("server not initialized (send initialize first)")), true
		}
		return s.handleInitialize(msg), true

	case stateInitializing:
		if msg.Method == protocol.MethodInitialize {
			return s.errorResponse(msg.ID, protocol.ErrInvalidRequest("already initialized")), true
		}
		return s.errorResponse(msg.ID, protocol.ErrInvalidRequest("server initializing, awaiting notifications/initialized")), true

	case stateReady:
		if msg.Method == protocol.MethodInitialize {
			return s.errorResponse(msg.ID, protocol.ErrInvalidRequest("already initialized")), true
		}
		return s.handleMethod(msg), true

	default:
		return s.errorResponse(msg.ID, protocol.ErrInternalError("unknown server state")), true
	}
}

// errorResponse builds a JSON-RPC error response from any error. If the error
// is a *protocol.CodeError, its code and message are used directly.
// Otherwise, the error falls back to -32603 (internal error).
func (s *Server) errorResponse(id json.RawMessage, err error) protocol.Response {
	s.errorCount++
	pe, ok := errors.AsType[*protocol.CodeError](err)
	if !ok {
		return protocol.NewErrorResponse(id, protocol.InternalError, err.Error())
	}
	return protocol.NewErrorResponseFromCodeError(id, pe)
}

// handleNotification processes notification messages (no response sent).
func (s *Server) handleNotification(msg protocol.Request) {
	switch msg.Method {
	case protocol.NotificationCancelled:
		s.handleCancelledNotification(msg)
	case protocol.NotificationInitialized:
		if s.state != stateInitializing {
			s.logger.Warn("unexpected notifications/initialized", "current_state", s.state)
			return
		}
		s.state = stateReady
		s.logger.Info("server_ready")
	}
	// All unknown notifications are silently ignored.
}

// handleCancelledNotification cancels the in-flight tool handler if the
// request ID matches. Non-matching or stale cancellations are silently ignored.
func (s *Server) handleCancelledNotification(msg protocol.Request) {
	if s.cancelInFlight == nil {
		return
	}
	var params cancelledParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Warn("unmarshal cancelled notification failed", "error", err)
		return
	}
	if !bytes.Equal(params.RequestID, s.inFlightID) {
		return
	}
	s.inFlightCancelled = true
	s.cancelInFlight()
}

// initializeResult is the result of the initialize method.
type initializeResult struct {
	Capabilities    initializeCapabilities `json:"capabilities"`
	ProtocolVersion string                 `json:"protocolVersion"`
	ServerInfo      serverInfo             `json:"serverInfo"`
}

type initializeCapabilities struct {
	Experimental map[string]any `json:"experimental,omitempty"`
	Tools        struct{}       `json:"tools"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// handleInitialize processes the initialize request.
func (s *Server) handleInitialize(msg protocol.Request) protocol.Response {
	s.state = stateInitializing

	result := initializeResult{
		Capabilities: initializeCapabilities{
			Experimental: map[string]any{
				"concurrency": map[string]any{
					"maxInFlight": protocol.MaxConcurrentRequests,
				},
			},
		},
		ProtocolVersion: protocol.MCPVersion,
		ServerInfo:      serverInfo{Name: s.name, Version: s.version},
	}

	resp, err := protocol.NewResultResponse(msg.ID, result)
	if err != nil {
		s.logger.Error("marshal_initialize", "error", err)
		return s.errorResponse(msg.ID, protocol.ErrInternalError("failed to marshal initialize result"))
	}
	return resp
}

// capabilityGuidance is the Error.Data for unsupported capability namespaces.
type capabilityGuidance struct {
	Hint                  string   `json:"hint"`
	SupportedCapabilities []string `json:"supportedCapabilities"`
}

// handleMethod dispatches ready-state methods.
func (s *Server) handleMethod(msg protocol.Request) protocol.Response {
	switch {
	case strings.HasPrefix(msg.Method, protocol.NamespaceCompletion):
		return s.handleUnsupportedCapability(msg)
	case strings.HasPrefix(msg.Method, protocol.NamespaceElicitation):
		return s.handleUnsupportedCapability(msg)
	case strings.HasPrefix(msg.Method, protocol.NamespacePrompts):
		return s.handleUnsupportedCapability(msg)
	case strings.HasPrefix(msg.Method, protocol.NamespaceResources):
		return s.handleUnsupportedCapability(msg)
	case msg.Method == protocol.MethodToolsCall:
		// tools/call in ready state is intercepted by Run for async dispatch.
		// If we reach here, something unexpected happened.
		return s.errorResponse(msg.ID, protocol.ErrInternalError("unexpected tools/call in handleMethod"))
	case msg.Method == protocol.MethodToolsList:
		return s.handleToolsList(msg)
	case strings.HasPrefix(msg.Method, protocol.PrefixRPC):
		return s.errorResponse(msg.ID, protocol.ErrMethodNotFound("reserved method: "+msg.Method))
	default:
		return s.errorResponse(msg.ID, protocol.ErrMethodNotFound("unknown method: "+msg.Method))
	}
}

// handleUnsupportedCapability returns a -32601 response with guidance data
// for methods in unsupported MCP namespaces (completion/, elicitation/, prompts/, resources/).
func (s *Server) handleUnsupportedCapability(msg protocol.Request) protocol.Response {
	// Marshal cannot fail: capabilityGuidance contains only JSON-safe fields
	// (string, []string).
	data, _ := json.Marshal(&capabilityGuidance{
		Hint:                  "this server supports tools only; use tools/list and tools/call",
		SupportedCapabilities: []string{"tools"},
	})
	ce := protocol.ErrMethodNotFound("method not found: " + msg.Method)
	ce.Data = data
	return s.errorResponse(msg.ID, ce)
}

// toolsListResult is the result of tools/list.
type toolsListResult struct {
	Tools []tools.Tool `json:"tools"`
}

// handleToolsList returns all registered tools.
func (s *Server) handleToolsList(msg protocol.Request) protocol.Response {
	result := toolsListResult{Tools: s.registry.Tools()}
	resp, err := protocol.NewResultResponse(msg.ID, result)
	if err != nil {
		s.logger.Error("marshal_tools_list", "error", err)
		return s.errorResponse(msg.ID, protocol.ErrInternalError("failed to marshal tools list"))
	}
	return resp
}

// toolCallParams is the expected structure of tools/call params.
type toolCallParams struct {
	Arguments json.RawMessage `json:"arguments"`
	Name      string          `json:"name"`
}

// panicDiag is the machine-readable diagnostic attached to Error.Data when a
// tool handler panics. Stack traces are deliberately excluded (security).
type panicDiag struct {
	PanicValue string `json:"panicValue"`
	ToolName   string `json:"toolName"`
}

// timingDiag is the machine-readable diagnostic attached to Error.Data for
// timeout and cancellation errors.
type timingDiag struct {
	ElapsedMs int64  `json:"elapsedMs"`
	TimeoutMs int64  `json:"timeoutMs,omitempty"`
	ToolName  string `json:"toolName"`
}

// toolError carries the error code, message, and structured data from dispatchToolCall.
type toolError struct {
	code    int
	data    any
	message string
}

// dispatchToolCall invokes a tool handler with panic recovery and timeout.
// Returns the result and a *toolError if the handler panicked, timed out, or
// was cancelled.
//
// Limitation: Go cannot kill goroutines. If a handler ignores ctx.Done()
// and exceeds the handler timeout + safety margin, the goroutine is
// abandoned. Handlers MUST respect context cancellation to guarantee
// bounded goroutine lifetime. The server proceeds to the next request
// regardless, but the abandoned goroutine may hold resources until it
// returns naturally.
func (s *Server) dispatchToolCall(ctx context.Context, tool tools.Tool, params json.RawMessage) (tools.Result, *toolError) {
	type outcome struct {
		err    *toolError
		result tools.Result
	}
	start := time.Now()
	ch := make(chan outcome, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				s.logger.Error("tool_handler_panicked", "tool_name", tool.Name, "panic", r, "stack", string(buf[:n]))
				ch <- outcome{err: &toolError{
					code:    protocol.InternalError,
					data:    &panicDiag{PanicValue: fmt.Sprint(r), ToolName: tool.Name},
					message: fmt.Sprintf("internal error: tool %q panicked", tool.Name),
				}}
			}
		}()
		hctx, cancel := context.WithTimeout(ctx, s.handlerTimeout)
		defer cancel()
		result, err := tool.Handler(hctx, params)
		if err != nil {
			var pe *protocol.CodeError
			if errors.As(err, &pe) {
				ch <- outcome{err: &toolError{
					code:    pe.Code,
					data:    pe.Data,
					message: pe.Message,
				}}
			} else {
				ch <- outcome{err: &toolError{
					code:    protocol.InternalError,
					message: err.Error(),
				}}
			}
			return
		}
		if errors.Is(hctx.Err(), context.DeadlineExceeded) {
			ch <- outcome{err: &toolError{
				code: protocol.InternalError,
				data: &timingDiag{
					ElapsedMs: time.Since(start).Milliseconds(),
					TimeoutMs: s.handlerTimeout.Milliseconds(),
					ToolName:  tool.Name,
				},
				message: fmt.Sprintf("tool %q execution timed out", tool.Name),
			}}
			return
		}
		ch <- outcome{result: result}
	}()
	deadline := time.NewTimer(s.handlerTimeout + s.safetyMargin)
	defer deadline.Stop()
	select {
	case o := <-ch:
		return o.result, o.err
	case <-ctx.Done():
		s.logger.Error("tool_handler_cancelled", "tool_name", tool.Name)
		return tools.Result{}, &toolError{
			code: protocol.InternalError,
			data: &timingDiag{
				ElapsedMs: time.Since(start).Milliseconds(),
				ToolName:  tool.Name,
			},
			message: fmt.Sprintf("tool %q execution cancelled", tool.Name),
		}
	case <-deadline.C:
		s.logger.Error("tool_handler_timeout", "tool_name", tool.Name)
		return tools.Result{}, &toolError{
			code: protocol.InternalError,
			data: &timingDiag{
				ElapsedMs: time.Since(start).Milliseconds(),
				TimeoutMs: s.handlerTimeout.Milliseconds(),
				ToolName:  tool.Name,
			},
			message: fmt.Sprintf("tool %q execution timed out", tool.Name),
		}
	}
}
