package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/prompts"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/resources"
	"github.com/andygeiss/mcp/internal/tools"
)

// newTestServer creates a minimal Server for internal unit tests.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	var stdout bytes.Buffer
	enc := json.NewEncoder(&stdout)
	enc.SetEscapeHTML(false)
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelInfo)
	return &Server{
		done:           make(chan struct{}),
		enc:            enc,
		handlerTimeout: time.Second,
		logLevel:       levelVar,
		logger:         slog.New(slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: levelVar})),
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

	// Assert
	assert.That(t, "error code", resp.Error.Code, protocol.ServerTimeout)
}

// Test_processInFlightResult_With_EncodeError_Should_ReturnError covers
// processInFlightResult lines 512-515: when encodeResponse returns an error
// (e.g., broken writer).
func Test_processInFlightResult_With_EncodeError_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
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

	// Act
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
	s.logLevel.Set(slog.LevelDebug)
	s.logger = slog.New(slog.NewJSONHandler(&stderr, &slog.HandlerOptions{Level: s.logLevel}))

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

	// Assert — fatal decode error returned even though encode failed
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

// Test_handleInitialize_With_SupportedProtocolVersion_Should_EchoClientVersion
// confirms the server echoes the client-sent protocolVersion when it matches
// the server's supported version.
func Test_handleInitialize_With_SupportedProtocolVersion_Should_EchoClientVersion(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	params := fmt.Sprintf(`{"protocolVersion":%q,"clientInfo":{"name":"c","version":"1"}}`, protocol.MCPVersion)
	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(params),
	}

	// Act
	resp := s.handleInitialize(msg)

	// Assert
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(resp.Result, &result)
	assert.That(t, "echoed protocolVersion", result.ProtocolVersion, protocol.MCPVersion)
}

// Test_handleInitialize_With_UnsupportedProtocolVersion_Should_ReturnServerVersion
// confirms the server falls back to its own version when the client requests a
// version the server does not support.
func Test_handleInitialize_With_UnsupportedProtocolVersion_Should_ReturnServerVersion(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"1999-01-01"}`),
	}

	// Act
	resp := s.handleInitialize(msg)

	// Assert
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(resp.Result, &result)
	assert.That(t, "server protocolVersion", result.ProtocolVersion, protocol.MCPVersion)
}

// Test_handleInitialize_With_AllRegistries_Should_AdvertiseAllCapabilities covers
// handleInitialize with prompts, resources, and tools registries all populated.
// R2 (capability honesty): a non-empty registry must advertise; an empty
// registry must NOT advertise.
func Test_handleInitialize_With_AllRegistries_Should_AdvertiseAllCapabilities(t *testing.T) {
	t.Parallel()

	// Arrange — populate every registry with one entry so all four
	// capabilities should advertise.
	s := newTestServer(t)
	if err := tools.Register(s.registry, "echo", "echo", tools.Echo); err != nil {
		t.Fatal(err)
	}
	s.prompts = prompts.NewRegistry()
	if err := prompts.Register(s.prompts, "p", "p", func(_ context.Context, _ struct{}) prompts.Result {
		return prompts.Result{}
	}); err != nil {
		t.Fatal(err)
	}
	s.resources = resources.NewRegistry()
	if err := resources.Register(s.resources, "file:///r", "r", "r", func(_ context.Context, _ string) (resources.Result, error) {
		return resources.Result{}, nil
	}); err != nil {
		t.Fatal(err)
	}

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

	var result struct {
		Capabilities struct {
			Logging   *json.RawMessage `json:"logging"`
			Prompts   *json.RawMessage `json:"prompts"`
			Resources *json.RawMessage `json:"resources"`
			Tools     *json.RawMessage `json:"tools"`
		} `json:"capabilities"`
	}
	_ = json.Unmarshal(resp.Result, &result)
	assert.That(t, "logging", result.Capabilities.Logging != nil, true)
	assert.That(t, "prompts", result.Capabilities.Prompts != nil, true)
	assert.That(t, "resources", result.Capabilities.Resources != nil, true)
	assert.That(t, "tools", result.Capabilities.Tools != nil, true)
}

// Test_handleInitialize_With_EmptyRegistries_Should_OmitOptionalCapabilities
// pins the R2 boundary at the unit level: registries that exist but contain
// no entries must not advertise their capability.
func Test_handleInitialize_With_EmptyRegistries_Should_OmitOptionalCapabilities(t *testing.T) {
	t.Parallel()

	// Arrange — registries are present but empty.
	s := newTestServer(t)
	s.prompts = prompts.NewRegistry()
	s.resources = resources.NewRegistry()

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"capabilities":{}}`),
	}

	// Act
	resp := s.handleInitialize(msg)

	// Assert — only logging is advertised; tools/prompts/resources stay quiet.
	assert.That(t, "no error", resp.Error == nil, true)

	var result struct {
		Capabilities struct {
			Logging   *json.RawMessage `json:"logging"`
			Prompts   *json.RawMessage `json:"prompts"`
			Resources *json.RawMessage `json:"resources"`
			Tools     *json.RawMessage `json:"tools"`
		} `json:"capabilities"`
	}
	_ = json.Unmarshal(resp.Result, &result)
	assert.That(t, "logging present", result.Capabilities.Logging != nil, true)
	assert.That(t, "prompts absent", result.Capabilities.Prompts == nil, true)
	assert.That(t, "resources absent", result.Capabilities.Resources == nil, true)
	assert.That(t, "tools absent", result.Capabilities.Tools == nil, true)
}

// Test_handleLoggingSetLevel_With_ValidLevel_Should_Succeed covers
// handleLoggingSetLevel happy path.
func Test_handleLoggingSetLevel_With_ValidLevel_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "logging/setLevel",
		Params:  json.RawMessage(`{"level":"debug"}`),
	}

	// Act
	resp := s.handleLoggingSetLevel(msg)

	// Assert
	assert.That(t, "no error", resp.Error == nil, true)
	assert.That(t, "log level set", s.logLevel.Level(), slog.LevelDebug)
}

// Test_handleResourcesRead_With_TemplateMatch_Should_ReturnContent covers
// handleResourcesRead lines 1155-1157: template fallback when static lookup fails.
func Test_handleResourcesRead_With_TemplateMatch_Should_ReturnContent(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	reg := resources.NewRegistry()
	_ = resources.RegisterTemplate(reg, "file://{path}", "File", "Read a file",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "template content"), nil
		},
	)
	s.resources = reg

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "resources/read",
		Params:  json.RawMessage(`{"uri":"file://readme.md"}`),
	}

	// Act
	resp := s.handleResourcesRead(context.Background(), msg)

	// Assert
	assert.That(t, "no error", resp.Error == nil, true)
}

// Test_handleResourcesMethod_With_UnknownMethod_Should_ReturnMethodNotFound covers
// handleResourcesMethod default branch.
func Test_handleResourcesMethod_With_UnknownMethod_Should_ReturnMethodNotFound(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.resources = resources.NewRegistry()

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "resources/subscribe",
		Params:  json.RawMessage(`{}`),
	}

	// Act
	resp := s.handleResourcesMethod(context.Background(), msg)

	// Assert
	assert.That(t, "error code", resp.Error.Code, protocol.MethodNotFound)
}

// Test_handlePromptsMethod_With_UnknownMethod_Should_ReturnMethodNotFound covers
// handlePromptsMethod default branch.
func Test_handlePromptsMethod_With_UnknownMethod_Should_ReturnMethodNotFound(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.prompts = prompts.NewRegistry()

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "prompts/complete",
		Params:  json.RawMessage(`{}`),
	}

	// Act
	resp := s.handlePromptsMethod(context.Background(), msg)

	// Assert
	assert.That(t, "error code", resp.Error.Code, protocol.MethodNotFound)
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
	resp := s.dispatchByState(context.Background(), msg)

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
	resp := s.handleMethod(context.Background(), msg)

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
	resp, respond := s.dispatch(context.Background(), msg)

	// Assert
	assert.That(t, "should respond", respond, true)
	assert.That(t, "no error", resp.Error == nil, true)
	assert.That(t, "result", string(resp.Result), "{}")
}

// Test_executeToolCall_With_SuccessfulResult_Should_ReturnResult covers
// the happy path in executeToolCall: successful dispatch + marshal + response.
func Test_executeToolCall_With_SuccessfulResult_Should_ReturnResult(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	tool := tools.Tool{
		Name: "ok",
		Handler: func(_ context.Context, _ json.RawMessage) (tools.Result, error) {
			return tools.TextResult("hello"), nil
		},
	}

	// Act
	resp := s.executeToolCall(context.Background(), json.RawMessage(`1`), tool, toolCallParams{Name: "ok", Arguments: json.RawMessage(`{}`)})

	// Assert
	assert.That(t, "no error", resp.Error == nil, true)
	if resp.Result == nil {
		t.Fatal("expected non-nil result")
	}
}

// Test_executeToolCall_With_GenericHandlerError_Should_ReturnInternalError covers
// executeToolCall + runToolHandler: handler returns generic error (not CodeError).
func Test_executeToolCall_With_GenericHandlerError_Should_ReturnInternalError(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	tool := tools.Tool{
		Name: "generr",
		Handler: func(_ context.Context, _ json.RawMessage) (tools.Result, error) {
			return tools.Result{}, errors.New("something broke")
		},
	}

	// Act
	resp := s.executeToolCall(context.Background(), json.RawMessage(`1`), tool, toolCallParams{Name: "generr", Arguments: json.RawMessage(`{}`)})

	// Assert
	assert.That(t, "error code", resp.Error.Code, protocol.InternalError)
}

// Test_sendNotification_With_UnmarshalableParams_Should_ReturnError covers
// sendNotification lines 464-468: when json.Marshal(params) fails.
func Test_sendNotification_With_UnmarshalableParams_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)

	// Act — pass a channel which can't be marshaled
	err := s.sendNotification("test/method", make(chan int))

	// Assert
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

// Test_sendNotification_With_BrokenWriter_Should_ReturnError covers
// sendNotification lines 482-487: when enc.Encode fails.
func Test_sendNotification_With_BrokenWriter_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.enc = json.NewEncoder(&brokenWriter{})

	// Act
	err := s.sendNotification("test/method", map[string]string{"key": "val"})

	// Assert
	if err == nil {
		t.Fatal("expected encode error")
	}
}

// Test_handleMessageDuringInFlight_With_ServerBusy_Should_ReturnBusyError covers
// handleMessageDuringInFlight line 539: non-ping request while handler in flight.
func Test_handleMessageDuringInFlight_With_ServerBusy_Should_ReturnBusyError(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.state = stateReady

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}

	// Act
	err := s.handleMessageDuringInFlight(msg)

	// Assert — server encodes the busy error response, no fatal error
	assert.That(t, "no fatal error", err, nil)
}

// Test_handleMessageDuringInFlight_With_PingDuringInFlight_Should_RespondWithPong covers
// handleMessageDuringInFlight lines 531-537: ping while handler in flight.
func Test_handleMessageDuringInFlight_With_PingDuringInFlight_Should_RespondWithPong(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "ping",
		Params:  json.RawMessage(`{}`),
	}

	// Act
	err := s.handleMessageDuringInFlight(msg)

	// Assert
	assert.That(t, "no error", err, nil)
}

// Test_handleMessageDuringInFlight_With_NotificationDuringInFlight_Should_HandleSilently covers
// handleMessageDuringInFlight lines 526-528: notification while handler in flight.
func Test_handleMessageDuringInFlight_With_NotificationDuringInFlight_Should_HandleSilently(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.state = stateReady

	msg := protocol.Request{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}

	// Act
	err := s.handleMessageDuringInFlight(msg)

	// Assert
	assert.That(t, "no error", err, nil)
}

// Test_handleMessageDuringInFlight_With_InvalidNotification_Should_IgnoreSilently covers
// handleMessageDuringInFlight lines 520-521: invalid notification is silently ignored.
func Test_handleMessageDuringInFlight_With_InvalidNotification_Should_IgnoreSilently(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)

	msg := protocol.Request{
		JSONRPC: "1.0", // invalid version
		Method:  "notifications/something",
	}

	// Act
	err := s.handleMessageDuringInFlight(msg)

	// Assert
	assert.That(t, "no error", err, nil)
}

// Test_handleMessageDuringInFlight_With_InvalidRequest_Should_ReturnValidationError covers
// handleMessageDuringInFlight lines 519-523: request with validation error.
func Test_handleMessageDuringInFlight_With_InvalidRequest_Should_ReturnValidationError(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)

	msg := protocol.Request{
		JSONRPC: "1.0", // invalid version
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
	}

	// Act
	err := s.handleMessageDuringInFlight(msg)

	// Assert — server encodes error response, no fatal error
	assert.That(t, "no fatal error", err, nil)
}

// Test_processInFlightResult_With_CancelledRequest_Should_SuppressResponse covers
// processInFlightResult lines 642-644: cancelled request suppresses response.
func Test_processInFlightResult_With_CancelledRequest_Should_SuppressResponse(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.inFlightCancelled.Store(true)
	s.inFlightID = json.RawMessage(`1`)
	s.inFlightCh = make(chan inFlightResult, 1)

	resp := protocol.NewErrorResponse(json.RawMessage(`1`), protocol.InternalError, "test")
	ifr := inFlightResult{isError: true, resp: resp}

	// Act
	err := s.processInFlightResult(ifr)

	// Assert
	assert.That(t, "no error", err, nil)
	assert.That(t, "in-flight cleared", s.inFlightID == nil, true)
}

// Test_routeResponse_With_UnknownID_Should_DropSilently covers
// routeResponse lines 264-268: response with unknown ID is silently dropped.
func Test_routeResponse_With_UnknownID_Should_DropSilently(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.pending = make(map[string]pendingEntry)

	resp := &protocol.Response{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"unknown-id"`),
	}

	// Act — should not panic
	s.routeResponse(resp)
}

// Test_handleDecodeErrorDuringInFlight_With_ExceededSize_Should_ReturnError covers
// handleDecodeErrorDuringInFlight lines 397-399: when dr.exceeded is true.
func Test_handleDecodeErrorDuringInFlight_With_ExceededSize_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.inFlightCh = make(chan inFlightResult, 1)
	s.inFlightID = json.RawMessage(`1`)

	// Simulate the handler completing
	resp, _ := protocol.NewResultResponse(json.RawMessage(`1`), json.RawMessage(`{}`))
	s.inFlightCh <- inFlightResult{resp: resp}

	// Act
	err := s.handleDecodeErrorDuringInFlight(decodeResult{exceeded: true})

	// Assert — fatal decode error returned but handler response still sent
	if err == nil {
		t.Fatal("expected fatal decode error")
	}
}

// Test_handleDecodeErrorDuringInFlight_With_DecodeError_Should_WaitForHandler covers
// handleDecodeErrorDuringInFlight: handler completes with an error response.
func Test_handleDecodeErrorDuringInFlight_With_DecodeError_Should_WaitForHandler(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.inFlightCh = make(chan inFlightResult, 1)
	s.inFlightID = json.RawMessage(`1`)

	// Handler completed with error response
	errResp := protocol.NewErrorResponse(json.RawMessage(`1`), protocol.InternalError, "handler error")
	s.inFlightCh <- inFlightResult{isError: true, resp: errResp}

	// Act
	err := s.handleDecodeErrorDuringInFlight(decodeResult{err: errMessageTooLarge})

	// Assert
	if err == nil {
		t.Fatal("expected fatal decode error")
	}
}

// Test_handleDecodeResultDuringInFlight_With_DecodeError_Should_Delegate covers
// handleDecodeResultDuringInFlight lines 362-363: when dr.err is set.
func Test_handleDecodeResultDuringInFlight_With_DecodeError_Should_Delegate(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.inFlightCh = make(chan inFlightResult, 1)
	s.inFlightID = json.RawMessage(`1`)

	// Handler completes normally
	resp, _ := protocol.NewResultResponse(json.RawMessage(`1`), json.RawMessage(`{}`))
	s.inFlightCh <- inFlightResult{resp: resp}

	// Act — decode error during in-flight
	err := s.handleDecodeResultDuringInFlight(context.Background(), decodeResult{err: errMessageTooLarge})

	// Assert — should return fatal decode error
	if err == nil {
		t.Fatal("expected fatal error")
	}
}

// Test_handleDecodeResultDuringInFlight_With_Message_Should_HandleBusy covers
// handleDecodeResultDuringInFlight lines 366-389: valid message arrives while in flight.
func Test_handleDecodeResultDuringInFlight_With_Message_Should_HandleBusy(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.state = stateReady
	s.inFlightCh = make(chan inFlightResult, 1)
	s.inFlightID = json.RawMessage(`1`)

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}

	// Act — a request arrives while handler is in flight
	err := s.handleDecodeResultDuringInFlight(context.Background(), decodeResult{msg: msg})

	// Assert — busy response sent, no fatal error
	assert.That(t, "no fatal error", err, nil)
}

// Test_handlePromptsGet_With_HandlerError_Should_ReturnInternalError covers
// handlePromptsGet lines 1238-1241: when the prompt handler returns an error.
func Test_handlePromptsGet_With_HandlerError_Should_ReturnInternalError(t *testing.T) {
	t.Parallel()

	// Arrange — exercise the error path via handleResourcesRead, which uses
	// the same handler-error pattern as handlePromptsGet.
	s := newTestServer(t)
	resReg := resources.NewRegistry()
	_ = resources.Register(resReg, "err://test", "Err", "fails",
		func(_ context.Context, _ string) (resources.Result, error) {
			return resources.Result{}, errors.New("read failed")
		},
	)
	s.resources = resReg

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "resources/read",
		Params:  json.RawMessage(`{"uri":"err://test"}`),
	}

	// Act
	resp := s.handleResourcesRead(context.Background(), msg)

	// Assert
	assert.That(t, "error code", resp.Error.Code, protocol.InternalError)
}

// Test_handleResourcesRead_With_HandlerCodeError_Should_PassthroughCode covers
// handleResourcesRead: when the resource handler returns a *protocol.CodeError,
// its code and message are used instead of falling back to InternalError.
func Test_handleResourcesRead_With_HandlerCodeError_Should_PassthroughCode(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	reg := resources.NewRegistry()
	_ = resources.Register(reg, "err://code", "CodeErr", "returns CodeError",
		func(_ context.Context, _ string) (resources.Result, error) {
			return resources.Result{}, protocol.ErrInvalidParams("bad resource uri format")
		},
	)
	s.resources = reg

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "resources/read",
		Params:  json.RawMessage(`{"uri":"err://code"}`),
	}

	// Act
	resp := s.handleResourcesRead(context.Background(), msg)

	// Assert
	assert.That(t, "error code", resp.Error.Code, protocol.InvalidParams)
	assert.That(t, "error message", resp.Error.Message, "bad resource uri format")
}

// Test_handleResourcesList_With_Registry_Should_ReturnResources covers
// handleResourcesList happy path with direct function call.
func Test_handleResourcesList_With_Registry_Should_ReturnResources(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	reg := resources.NewRegistry()
	_ = resources.Register(reg, "config://app", "App Config", "desc",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "data"), nil
		},
	)
	s.resources = reg

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "resources/list",
		Params:  json.RawMessage(`{}`),
	}

	// Act
	resp := s.handleResourcesList(msg)

	// Assert
	assert.That(t, "no error", resp.Error == nil, true)
}

// Test_handlePromptsList_With_Registry_Should_ReturnPrompts covers
// handlePromptsList happy path with direct function call.
func Test_handlePromptsList_With_Registry_Should_ReturnPrompts(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	reg := prompts.NewRegistry()
	_ = prompts.Register(reg, "greet", "greeting",
		func(_ context.Context, _ struct{}) prompts.Result {
			return prompts.Result{}
		},
	)
	s.prompts = reg

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "prompts/list",
		Params:  json.RawMessage(`{}`),
	}

	// Act
	resp := s.handlePromptsList(msg)

	// Assert
	assert.That(t, "no error", resp.Error == nil, true)
}

// Test_SendRequest_With_UnmarshalableParams_Should_ReturnError covers
// SendRequest lines 233-235: when json.Marshal(params) fails.
func Test_SendRequest_With_UnmarshalableParams_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.pending = make(map[string]pendingEntry)

	// Act
	_, err := s.SendRequest(context.Background(), "test/method", make(chan int))

	// Assert
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

// Test_SendRequest_With_ContextCancelled_Should_CleanupPendingEntry verifies
// that a SendRequest caller whose context times out before the client responds
// (a) returns ctx.Err(), and (b) removes its entry from the pending map so the
// map cannot grow without bound under misbehaving clients.
func Test_SendRequest_With_ContextCancelled_Should_CleanupPendingEntry(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.pending = make(map[string]pendingEntry)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Act — no client response will ever arrive; ctx.Done fires first.
	_, err := s.SendRequest(ctx, "ping", struct{}{})

	// Assert
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got: %v", err)
	}
	s.pendingMu.Lock()
	size := len(s.pending)
	s.pendingMu.Unlock()
	assert.That(t, "pending cleaned", size, 0)
}

// Test_SendRequest_With_ServerShutdown_Should_CleanupPendingEntry verifies
// that when the server's Run loop exits with a SendRequest still in flight,
// the caller unblocks on s.done and the pending entry is removed. Without
// this cleanup, shutdown leaks a goroutine per pending server-to-client
// request.
func Test_SendRequest_With_ServerShutdown_Should_CleanupPendingEntry(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.pending = make(map[string]pendingEntry)

	// Act — call SendRequest in a goroutine, then close s.done to simulate
	// Run returning. No ctx cancellation, no client response.
	errCh := make(chan error, 1)
	go func() {
		_, err := s.SendRequest(context.Background(), "ping", struct{}{})
		errCh <- err
	}()

	// Give SendRequest time to enter its select, then trigger shutdown.
	// 20ms is generous on loaded CI; the select blocks until one of the
	// three channels fires.
	time.Sleep(20 * time.Millisecond)
	close(s.done)

	// Assert
	select {
	case err := <-errCh:
		if !errors.Is(err, protocol.ErrServerShutdown) {
			t.Fatalf("expected protocol.ErrServerShutdown, got: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SendRequest did not unblock on s.done close")
	}
	s.pendingMu.Lock()
	size := len(s.pending)
	s.pendingMu.Unlock()
	assert.That(t, "pending cleaned", size, 0)
}

// Test_SendRequest_With_PendingMapFull_Should_ReturnBackpressureError verifies
// that SendRequest refuses to grow the correlation map past maxPendingRequests
// and returns the protocol.ErrPendingRequestsFull sentinel.
func Test_SendRequest_With_PendingMapFull_Should_ReturnBackpressureError(t *testing.T) {
	t.Parallel()

	// Arrange — preload the map at capacity with dummy entries.
	s := newTestServer(t)
	s.pending = make(map[string]pendingEntry, maxPendingRequests)
	for i := range maxPendingRequests {
		s.pending[fmt.Sprintf("k-%d", i)] = pendingEntry{respChan: make(chan *protocol.Response, 1)}
	}

	// Act
	_, err := s.SendRequest(context.Background(), "test/method", struct{}{})

	// Assert
	if !errors.Is(err, protocol.ErrPendingRequestsFull) {
		t.Fatalf("expected protocol.ErrPendingRequestsFull, got: %v", err)
	}
	assert.That(t, "map size unchanged", s.pendingCount(), maxPendingRequests)
}

// Test_handleDecodeResultDuringInFlight_With_ConcurrentCompletion_Should_ProcessBoth covers
// handleDecodeResultDuringInFlight lines 376-383: handler completes before message is fully handled.
func Test_handleDecodeResultDuringInFlight_With_ConcurrentCompletion_Should_ProcessBoth(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.state = stateReady
	s.inFlightCh = make(chan inFlightResult, 1)
	s.inFlightID = json.RawMessage(`1`)

	// Simulate handler completing before the message is processed
	resp, _ := protocol.NewResultResponse(json.RawMessage(`1`), json.RawMessage(`{}`))
	s.inFlightCh <- inFlightResult{resp: resp}

	// New message arrives (a ping)
	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "ping",
		Params:  json.RawMessage(`{}`),
	}

	// Act — handler is already complete when this runs, so the non-blocking select picks it up
	err := s.handleDecodeResultDuringInFlight(context.Background(), decodeResult{msg: msg})

	// Assert
	assert.That(t, "no error", err, nil)
}

// Test_handleDecodeResultDuringInFlight_With_TraceEnabled_Should_LogTrace covers
// handleDecodeResultDuringInFlight lines 367-373: trace logging when receiving
// a message during in-flight with trace mode enabled.
func Test_handleDecodeResultDuringInFlight_With_TraceEnabled_Should_LogTrace(t *testing.T) {
	t.Parallel()

	// Arrange
	var stderr bytes.Buffer
	s := newTestServer(t)
	s.state = stateReady
	s.trace = true
	s.logLevel.Set(slog.LevelDebug)
	s.logger = slog.New(slog.NewJSONHandler(&stderr, &slog.HandlerOptions{Level: s.logLevel}))
	s.inFlightCh = make(chan inFlightResult, 1)
	s.inFlightID = json.RawMessage(`1`)

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "ping",
		Params:  json.RawMessage(`{}`),
	}

	// Act
	err := s.handleDecodeResultDuringInFlight(context.Background(), decodeResult{msg: msg})

	// Assert
	assert.That(t, "no error", err, nil)
	if !bytes.Contains(stderr.Bytes(), []byte("trace_request")) {
		t.Fatal("expected trace_request log entry")
	}
}

// Test_handleDecodeResultDuringInFlight_With_ConcurrentCompletionEncodeError_Should_ReturnError covers
// handleDecodeResultDuringInFlight lines 378-380: handler completes concurrently
// but processInFlightResult fails due to broken writer.
func Test_handleDecodeResultDuringInFlight_With_ConcurrentCompletionEncodeError_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.state = stateReady
	s.enc = json.NewEncoder(&brokenWriter{})
	s.inFlightCh = make(chan inFlightResult, 1)
	s.inFlightID = json.RawMessage(`1`)

	// Simulate handler completing
	resp, _ := protocol.NewResultResponse(json.RawMessage(`1`), json.RawMessage(`{}`))
	s.inFlightCh <- inFlightResult{resp: resp}

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "ping",
		Params:  json.RawMessage(`{}`),
	}

	// Act
	err := s.handleDecodeResultDuringInFlight(context.Background(), decodeResult{msg: msg})

	// Assert — processInFlightResult should fail due to broken writer
	if err == nil {
		t.Fatal("expected encode error from processInFlightResult")
	}
}

// Test_handleDecodeResultDuringInFlight_With_BusyEncodeError_Should_CancelAndReturn covers
// handleDecodeResultDuringInFlight lines 385-388: handleMessageDuringInFlight fails
// due to broken writer, triggering cancelAndAwaitInFlight.
func Test_handleDecodeResultDuringInFlight_With_BusyEncodeError_Should_CancelAndReturn(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.state = stateReady
	s.enc = json.NewEncoder(&brokenWriter{})
	s.cancelInFlight = func() {}
	s.inFlightCh = make(chan inFlightResult, 1)
	s.inFlightID = json.RawMessage(`1`)

	// Handler completes so cancelAndAwaitInFlight can read from channel
	resp, _ := protocol.NewResultResponse(json.RawMessage(`1`), json.RawMessage(`{}`))
	s.inFlightCh <- inFlightResult{resp: resp}

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}

	// Act — handleMessageDuringInFlight will fail encoding the busy error,
	// which triggers cancelAndAwaitInFlight
	err := s.handleDecodeResultDuringInFlight(context.Background(), decodeResult{msg: msg})

	// Assert
	if err == nil {
		t.Fatal("expected encode error")
	}
}

// Test_handleMessageDuringInFlight_With_BrokenWriter_Should_ReturnError covers
// handleMessageDuringInFlight line 539 with encode error.
func Test_handleMessageDuringInFlight_With_BrokenWriter_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.state = stateReady
	s.enc = json.NewEncoder(&brokenWriter{})
	s.cancelInFlight = func() {}
	s.inFlightCh = make(chan inFlightResult, 1)
	s.inFlightID = json.RawMessage(`1`)

	// Handler completes normally
	resp, _ := protocol.NewResultResponse(json.RawMessage(`1`), json.RawMessage(`{}`))
	s.inFlightCh <- inFlightResult{resp: resp}

	msg := protocol.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}

	// Act — encoding the busy error response will fail, triggering cancelAndAwaitInFlight
	err := s.handleMessageDuringInFlight(msg)

	// Assert — should return encode error
	if err == nil {
		t.Fatal("expected encode error")
	}
}

// Test_sendNotification_With_TraceEnabled_Should_LogTrace covers
// sendNotification lines 474-479: trace logging branch.
func Test_sendNotification_With_TraceEnabled_Should_LogTrace(t *testing.T) {
	t.Parallel()

	// Arrange
	var stderr bytes.Buffer
	s := newTestServer(t)
	s.trace = true
	s.logLevel.Set(slog.LevelDebug)
	s.logger = slog.New(slog.NewJSONHandler(&stderr, &slog.HandlerOptions{Level: s.logLevel}))

	// Act
	err := s.sendNotification("test/method", map[string]string{"key": "val"})

	// Assert
	assert.That(t, "no error", err, nil)
	if !bytes.Contains(stderr.Bytes(), []byte("trace_notification")) {
		t.Fatal("expected trace_notification log entry in stderr")
	}
}

// Test_routeResponse_With_DuplicateResponse_Should_NotBlock covers the
// non-blocking send + delete-under-lock behavior that prevents a malicious
// or buggy client from wedging the decode goroutine by sending two responses
// for the same server→client request ID.
func Test_routeResponse_With_DuplicateResponse_Should_NotBlock(t *testing.T) {
	t.Parallel()

	// Arrange
	s := newTestServer(t)
	s.pending = make(map[string]pendingEntry)
	id := json.RawMessage(`1`)
	ch := make(chan *protocol.Response, 1)
	s.pending[string(id)] = pendingEntry{respChan: ch}

	// Act — first response is delivered and consumed; a second identical
	// response must be dropped (no waiter left, channel buffer full) without
	// blocking the routing goroutine.
	first := &protocol.Response{ID: id, JSONRPC: protocol.Version, Result: json.RawMessage(`1`)}
	second := &protocol.Response{ID: id, JSONRPC: protocol.Version, Result: json.RawMessage(`2`)}

	done := make(chan struct{})
	go func() {
		s.routeResponse(first)
		s.routeResponse(second)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("routeResponse blocked on duplicate delivery")
	}

	// Assert — only the first response reached the waiter; the second was
	// dropped. The pending entry was removed on first delivery.
	delivered := <-ch
	assert.That(t, "first result", string(delivered.Result), "1")
	s.pendingMu.Lock()
	_, stillPending := s.pending[string(id)]
	s.pendingMu.Unlock()
	assert.That(t, "pending entry cleaned", stillPending, false)
}

// pendingCount returns the number of in-flight outbound requests in the
// correlation map. Test-only primitive used by synctest scenarios to assert
// zero-leak invariant after each scenario completes.
func (s *Server) pendingCount() int {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	return len(s.pending)
}
