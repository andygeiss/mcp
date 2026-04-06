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
	counting       *countingReader
	dec            *json.Decoder
	enc            *json.Encoder
	handlerTimeout time.Duration
	logger         *slog.Logger
	name           string
	registry       *tools.Registry
	safetyMargin   time.Duration
	state          int
	trace          bool
	version        string
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

// Run starts the sequential dispatch loop: decode → dispatch → encode. It
// processes one message at a time until EOF (clean shutdown, returns nil),
// context cancellation (clean shutdown, returns nil), or a fatal decode error
// (returns error after sending a -32700 response). The caller should os.Exit
// based on whether Run returns nil.
//
// Shutdown limitation: context cancellation is checked between messages. If
// the decoder is blocked waiting for stdin, the server cannot exit until stdin
// produces data or EOF. This is inherent to io.Reader — it has no cancellation
// mechanism. In practice, the parent process closing stdin triggers EOF.
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("server_started", "version", s.version)
	// Sequential dispatch — no goroutine pools. stdin/stdout has one reader
	// and one writer; concurrency would add complexity without throughput benefit.
	for {
		select {
		case <-ctx.Done():
			// cause is informational only; never branch on cause value
			cause := "context_cancelled"
			if c := context.Cause(ctx); c != nil {
				cause = c.Error()
			}
			s.logger.Info("server_shutting_down", "reason", cause)
			return nil
		default:
		}

		s.counting.Reset()
		msg, err := protocol.Decode(s.dec)
		if err != nil {
			return s.handleDecodeError(err)
		}
		// The v1 json.Decoder may successfully decode a value even when the
		// counting reader returned errMessageTooLarge alongside the data bytes.
		// Check the counter explicitly after each decode.
		if s.counting.Exceeded() {
			return s.handleDecodeError(fmt.Errorf("decode message: %w", errMessageTooLarge))
		}

		if s.trace {
			raw, _ := json.Marshal(msg)
			s.logger.Info("trace_request", "direction", "←", "message", string(raw))
		}

		resp, respond := s.dispatch(ctx, msg)
		if !respond {
			continue
		}

		if s.trace {
			raw, _ := json.Marshal(resp)
			s.logger.Info("trace_response", "direction", "→", "message", string(raw))
		}

		if err := protocol.Encode(s.enc, resp); err != nil {
			s.logger.Error("encode_error", "error", err)
			return fmt.Errorf("encode error: %w", err)
		}
	}
}

// handleDecodeError processes errors from the decoder, returning nil for clean
// shutdown (EOF) or an error for fatal conditions.
func (s *Server) handleDecodeError(err error) error {
	// Check for size limit BEFORE EOF — the countingReader returns
	// errMessageTooLarge which errors.Is can match through any wrapping.
	isTooLarge := errors.Is(err, errMessageTooLarge)

	if !isTooLarge && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
		s.logger.Info("server_shutting_down", "reason", "eof")
		return nil
	}

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
func (s *Server) dispatch(ctx context.Context, msg protocol.Request) (protocol.Response, bool) {
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
		return s.handleMethod(ctx, msg), true

	default:
		return s.errorResponse(msg.ID, protocol.ErrInternalError("unknown server state")), true
	}
}

// errorResponse builds a JSON-RPC error response from any error. If the error
// is a *protocol.CodeError, its code and message are used directly.
// Otherwise, the error falls back to -32603 (internal error).
func (s *Server) errorResponse(id json.RawMessage, err error) protocol.Response {
	pe, ok := errors.AsType[*protocol.CodeError](err)
	if !ok {
		return protocol.NewErrorResponse(id, protocol.InternalError, err.Error())
	}
	return protocol.NewErrorResponseFromCodeError(id, pe)
}

// handleNotification processes notification messages (no response sent).
func (s *Server) handleNotification(msg protocol.Request) {
	if msg.Method == protocol.NotificationInitialized && s.state == stateInitializing {
		s.state = stateReady
		s.logger.Info("server_ready")
	}
	// All unknown notifications are silently ignored.
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
func (s *Server) handleMethod(ctx context.Context, msg protocol.Request) protocol.Response {
	switch {
	case strings.HasPrefix(msg.Method, "prompts/"):
		return s.handleUnsupportedCapability(msg)
	case strings.HasPrefix(msg.Method, "resources/"):
		return s.handleUnsupportedCapability(msg)
	case msg.Method == protocol.MethodToolsCall:
		return s.handleToolsCall(ctx, msg)
	case msg.Method == protocol.MethodToolsList:
		return s.handleToolsList(msg)
	case strings.HasPrefix(msg.Method, "rpc."):
		return s.errorResponse(msg.ID, protocol.ErrMethodNotFound("reserved method: "+msg.Method))
	default:
		return s.errorResponse(msg.ID, protocol.ErrMethodNotFound("unknown method: "+msg.Method))
	}
}

// handleUnsupportedCapability returns a -32601 response with guidance data
// for methods in unsupported MCP namespaces (resources/, prompts/).
func (s *Server) handleUnsupportedCapability(msg protocol.Request) protocol.Response {
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

// handleToolsCall dispatches a tool call with panic recovery and timeout.
func (s *Server) handleToolsCall(ctx context.Context, msg protocol.Request) protocol.Response {
	var params toolCallParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.errorResponse(msg.ID, protocol.ErrInvalidParams("invalid tools/call params: "+err.Error()))
	}

	// Normalize absent or null arguments to {}.
	if len(params.Arguments) == 0 || bytes.Equal(params.Arguments, []byte("null")) {
		params.Arguments = json.RawMessage("{}")
	}

	if params.Name == "" {
		return s.errorResponse(msg.ID, protocol.ErrInvalidParams("tool name is required"))
	}

	tool, ok := s.registry.Lookup(params.Name)
	if !ok {
		available := strings.Join(s.registry.Names(), ", ")
		return s.errorResponse(msg.ID, protocol.ErrInvalidParams("unknown tool: "+params.Name+" (available: "+available+")"))
	}

	result, toolErr := s.dispatchToolCall(ctx, tool, params.Arguments)
	if toolErr != nil {
		data, _ := json.Marshal(toolErr.data)
		ce := protocol.ErrInternalError(toolErr.message)
		ce.Data = data
		return s.errorResponse(msg.ID, ce)
	}

	// Check result size before marshaling the response.
	resultJSON, err := json.Marshal(result)
	if err != nil {
		s.logger.Error("marshal_tool_result", "error", err, "tool_name", params.Name)
		return s.errorResponse(msg.ID, protocol.ErrInternalError("failed to marshal tool result"))
	}
	if len(resultJSON) > maxResultSize {
		s.logger.Warn("tool_result_truncated", "tool_name", params.Name, "size", len(resultJSON), "limit", maxResultSize)
		result = tools.TextResult(fmt.Sprintf("[result truncated: exceeded maximum size of %d bytes]", maxResultSize))
		resultJSON, _ = json.Marshal(result)
	}

	resp, err := protocol.NewResultResponse(msg.ID, json.RawMessage(resultJSON))
	if err != nil {
		s.logger.Error("marshal_tool_result", "error", err, "tool_name", params.Name)
		return s.errorResponse(msg.ID, protocol.ErrInternalError("failed to marshal tool result"))
	}
	return resp
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
			pe, ok := errors.AsType[*protocol.CodeError](err)
			if ok {
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
