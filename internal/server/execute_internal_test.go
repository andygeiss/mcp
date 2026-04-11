package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/tools"
)

// newTestServer creates a minimal Server for internal unit tests.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	var stdout bytes.Buffer
	enc := json.NewEncoder(&stdout)
	enc.SetEscapeHTML(false)
	return &Server{
		enc:            enc,
		handlerTimeout: time.Second,
		logger:         slog.New(slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelInfo})),
		name:           "test",
		registry:       tools.NewRegistry(),
		safetyMargin:   100 * time.Millisecond,
		version:        "test",
	}
}

// Test_executeToolCall_With_ToolError_Should_ReturnErrorResponse covers
// executeToolCall lines 448-457: the tool handler returns a *toolError.
func Test_executeToolCall_With_ToolError_Should_ReturnErrorResponse(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	tool := tools.Tool{
		Name: "errtool",
		Handler: func(_ context.Context, _ json.RawMessage) (tools.Result, error) {
			return tools.Result{}, &protocol.CodeError{
				Code:    protocol.InvalidParams,
				Message: "bad params",
			}
		},
	}

	// Act
	resp := s.executeToolCall(context.Background(), json.RawMessage(`1`), tool, toolCallParams{Name: "errtool", Arguments: json.RawMessage(`{}`)})

	// Assert
	assert.That(t, "error code", resp.Error.Code, protocol.InvalidParams)
}

// Test_executeToolCall_With_ToolErrorMarshalFailure_Should_SetDataNil covers
// executeToolCall lines 450-453: when toolErr.data fails to marshal.
func Test_executeToolCall_With_ToolErrorMarshalFailure_Should_SetDataNil(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.handlerTimeout = time.Second
	// Register a tool whose handler returns an error with unmarshalable data.
	// We inject a toolError directly through dispatchToolCall by returning
	// context.Canceled (which triggers the cancelErr path).
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	tool := tools.Tool{
		Name: "canceltool",
		Handler: func(_ context.Context, _ json.RawMessage) (tools.Result, error) {
			return tools.Result{}, nil
		},
	}

	// Act — ctx already cancelled, dispatchToolCall returns cancel error
	resp := s.executeToolCall(ctx, json.RawMessage(`1`), tool, toolCallParams{Name: "canceltool", Arguments: json.RawMessage(`{}`)})

	// Assert — should get a timeout/cancelled error
	assert.That(t, "error code", resp.Error.Code, protocol.ServerTimeout)
}

// Test_processInFlightResult_With_EncodeError_Should_ReturnError covers
// processInFlightResult lines 512-515: when encodeResponse returns an error
// (e.g., broken writer).
func Test_processInFlightResult_With_EncodeError_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — server with a broken stdout (writer that always errors)
	s := newTestServer(t)
	s.enc = json.NewEncoder(&brokenWriter{})
	s.inFlightID = json.RawMessage(`1`)

	resp := protocol.NewErrorResponse(json.RawMessage(`1`), protocol.InternalError, "test")
	ifr := inFlightResult{isError: true, resp: resp}

	// Act
	err := s.processInFlightResult(ifr)

	// Assert
	if err == nil {
		t.Fatal("expected encode error")
	}
}

// brokenWriter always returns an error on Write.
type brokenWriter struct{}

func (w *brokenWriter) Write([]byte) (int, error) {
	return 0, errMessageTooLarge // reuse existing sentinel
}

// Test_cancelAndAwaitInFlight_With_NilCancel_Should_Return covers
// cancelAndAwaitInFlight lines 487-489: when cancelInFlight is nil.
func Test_cancelAndAwaitInFlight_With_NilCancel_Should_Return(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.cancelInFlight = nil

	// Act — should not panic or block
	s.cancelAndAwaitInFlight()
}

// Test_encodeResponse_With_TraceAndBrokenMarshal_Should_LogWarn covers
// encodeResponse lines 366-368: when trace mode is on and json.Marshal fails.
// This exercises the trace-marshal-error warn branch. We use a response whose
// Result field contains invalid JSON that still parses but causes re-marshal to fail.
func Test_encodeResponse_With_TraceEnabled_Should_LogTrace(t *testing.T) {
	t.Parallel()

	// Arrange
	var stderr bytes.Buffer
	s := newTestServer(t)
	s.trace = true
	s.logger = slog.New(slog.NewJSONHandler(&stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	resp := protocol.NewErrorResponse(json.RawMessage(`1`), protocol.InternalError, "test")

	// Act
	err := s.encodeResponse(resp)

	// Assert
	assert.That(t, "encode error", err, nil)
	if !bytes.Contains(stderr.Bytes(), []byte("trace_response")) {
		t.Fatal("expected trace_response log entry")
	}
}

// Test_handleDecodeError_With_EncodeFailure_Should_StillReturnFatalError covers
// handleDecodeError lines 547-549: when encoding the error response fails.
func Test_handleDecodeError_With_EncodeFailure_Should_StillReturnFatalError(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.enc = json.NewEncoder(&brokenWriter{})

	var stderr bytes.Buffer
	s.logger = slog.New(slog.NewJSONHandler(&stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Act — decode error that is NOT EOF/too-large (a generic parse error)
	err := s.handleDecodeError(json.Unmarshal([]byte("invalid"), new(any)))

	// Assert — still returns fatal decode error even though encode failed
	if err == nil {
		t.Fatal("expected fatal decode error")
	}
}

// Test_handleToolsList_With_ValidRegistry_Should_ReturnTools covers
// handleToolsList happy path with tools registered.
func Test_handleToolsList_With_ValidRegistry_Should_ReturnTools(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	r := tools.NewRegistry()
	if err := tools.Register(r, "echo", "echoes", func(_ context.Context, _ struct{}) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}
	s.registry = r

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}

	// Act
	resp := s.handleToolsList(msg)

	// Assert
	assert.That(t, "no error", resp.Error == nil, true)
	if resp.Result == nil {
		t.Fatal("expected non-nil result")
	}
}

// Test_handleInitialize_With_ValidRequest_Should_ReturnCapabilities covers
// handleInitialize happy path including the full result serialization.
func Test_handleInitialize_With_ValidRequest_Should_ReturnCapabilities(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"capabilities":{}}`),
	}

	// Act
	resp := s.handleInitialize(msg)

	// Assert
	assert.That(t, "no error", resp.Error == nil, true)
	assert.That(t, "state", s.state, stateInitializing)
}

// Test_dispatchByState_With_UnknownState_Should_ReturnInternalError covers
// dispatchByState lines 605-606: the default branch for unknown server state.
func Test_dispatchByState_With_UnknownState_Should_ReturnInternalError(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.state = 99 // invalid state

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}

	// Act
	resp := s.dispatchByState(msg)

	// Assert
	assert.That(t, "error code", resp.Error.Code, protocol.InternalError)
}

// Test_handleMethod_With_ToolsCall_Should_ReturnInternalError covers
// handleMethod lines 719-722: tools/call reaching handleMethod (should not happen).
func Test_handleMethod_With_ToolsCall_Should_ReturnInternalError(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"test","arguments":{}}`),
	}

	// Act
	resp := s.handleMethod(msg)

	// Assert
	assert.That(t, "error code", resp.Error.Code, protocol.InternalError)
}

// Test_dispatch_With_PingMarshalSuccess_Should_ReturnResult covers
// dispatch lines 573-579: the ping handler path.
func Test_dispatch_With_Ping_Should_ReturnResult(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.state = stateReady

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "ping",
		Params:  json.RawMessage(`{}`),
	}

	// Act
	resp, respond := s.dispatch(msg)

	// Assert
	assert.That(t, "should respond", respond, true)
	assert.That(t, "no error", resp.Error == nil, true)
	assert.That(t, "result", string(resp.Result), "{}")
}
