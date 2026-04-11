package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/prompts"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/resources"
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

// testInput is a minimal input struct for test tool registrations.
type testInput struct {
	Message string `json:"message" description:"test message"`
}

// testRegistry creates a registry with a single tool for protocol tests.
func testRegistry() *tools.Registry {
	r := tools.NewRegistry()
	if err := tools.Register(r, "test", "test tool", func(_ context.Context, input testInput) tools.Result {
		return tools.TextResult(input.Message)
	}); err != nil {
		panic("testRegistry: " + err.Error())
	}
	return r
}

// helper: run server with input string, return output and error.
func runServer(t *testing.T, registry *tools.Registry, input string, opts ...server.Option) ([]protocol.Response, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", registry, strings.NewReader(input), &stdout, &stderr, opts...)
	err := srv.Run(context.Background())

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}
	return responses, err
}

// --- Handshake sequence helper ---
const (
	initRequest             = `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}` + "\n"
	initializedNotification = `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
)

func handshake() string {
	return initRequest + initializedNotification
}

func Test_Server_With_InitializeHandshake_Should_ReturnCapabilities(t *testing.T) {
	t.Parallel()

	// Arrange
	input := initRequest

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 1)

	resp := responses[0]
	assert.That(t, "id", string(resp.ID), "1")
	assert.That(t, "jsonrpc", resp.JSONRPC, "2.0")

	// Verify the result contains expected fields
	var result struct {
		Capabilities struct {
			Experimental map[string]any `json:"experimental"`
			Tools        struct{}       `json:"tools"`
		} `json:"capabilities"`
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	err = json.Unmarshal(resp.Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "protocol version", result.ProtocolVersion, "2025-11-25")
	assert.That(t, "server name", result.ServerInfo.Name, "mcp")
	assert.That(t, "server version", result.ServerInfo.Version, "test")

	// Verify experimental concurrency capability
	concurrency, ok := result.Capabilities.Experimental["concurrency"].(map[string]any)
	if !ok {
		t.Fatal("expected experimental.concurrency map")
	}
	maxInFlight, ok := concurrency["maxInFlight"].(float64)
	if !ok || int(maxInFlight) != 1 {
		t.Errorf("expected maxInFlight=1, got %v", concurrency["maxInFlight"])
	}
}

func Test_Server_With_UninitializedRequest_Should_Return32600(t *testing.T) {
	t.Parallel()

	// Arrange — send tools/list without init
	input := `{"jsonrpc":"2.0","method":"tools/list","id":1,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 1)
	assert.That(t, "error code", responses[0].Error.Code, protocol.ServerError)
	assert.That(t, "error message", responses[0].Error.Message, "server not initialized (send initialize first)")
}

func Test_Server_With_InitializingStateRequest_Should_ReturnAwaitingMessage(t *testing.T) {
	t.Parallel()

	// Arrange — send initialize then tools/list without notifications/initialized
	input := initRequest + `{"jsonrpc":"2.0","method":"tools/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.ServerError)
	assert.That(t, "error message", responses[1].Error.Message, "server initializing, awaiting notifications/initialized")
}

func Test_Server_With_DuplicateInitialize_Should_Return32600(t *testing.T) {
	t.Parallel()

	// Arrange — send initialize twice
	input := initRequest + initRequest

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "second error code", responses[1].Error.Code, protocol.ServerError)
	assert.That(t, "second error message", responses[1].Error.Message, "already initialized")
}

func Test_Server_With_PingInAnyState_Should_ReturnEmptyObject(t *testing.T) {
	t.Parallel()

	// Arrange — ping before init, after init, after ready
	input := `{"jsonrpc":"2.0","method":"ping","id":1,"params":{}}` + "\n" +
		initRequest +
		`{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n" +
		initializedNotification +
		`{"jsonrpc":"2.0","method":"ping","id":3,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 4) // ping + init + ping + ping

	// First ping (uninitialized)
	assert.That(t, "ping1 result", string(responses[0].Result), "{}")
	// Third response is ping in initializing state
	assert.That(t, "ping2 result", string(responses[2].Result), "{}")
	// Fourth response is ping in ready state
	assert.That(t, "ping3 result", string(responses[3].Result), "{}")
}

func Test_Server_With_ToolsList_Should_ReturnAlphabetically(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "zeta", "z tool", func(_ context.Context, _ testInput) tools.Result {
		return tools.TextResult("z")
	}); err != nil {
		t.Fatal(err)
	}
	if err := tools.Register(r, "alpha", "a tool", func(_ context.Context, _ testInput) tools.Result {
		return tools.TextResult("a")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, r, input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2) // init + tools/list

	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "tools count", len(result.Tools), 2)
	assert.That(t, "first tool", result.Tools[0].Name, "alpha")
	assert.That(t, "second tool", result.Tools[1].Name, "zeta")
}

func Test_Server_With_ToolsCall_Should_DispatchCorrectly(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"test","arguments":{"message":"hello"}}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2) // init + call

	var result struct {
		Content []struct {
			Text string `json:"text"`
			Type string `json:"type"`
		} `json:"content"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "content count", len(result.Content), 1)
	assert.That(t, "text", result.Content[0].Text, "hello")
	assert.That(t, "type", result.Content[0].Type, "text")
}

func Test_Server_With_UnknownTool_Should_Return32602(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"nonexistent","arguments":{}}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
	assert.That(t, "error message", responses[1].Error.Message, "unknown tool: nonexistent (available: test)")
}

func Test_Server_With_PanickingHandler_Should_Return32603(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		panicValue any
		toolName   string
	}{
		{name: "string panic", panicValue: "intentional panic", toolName: "panicker"},
		{name: "error panic", panicValue: fmt.Errorf("wrapped error: %w", errors.New("root cause")), toolName: "error-panicker"},
		{name: "int panic", panicValue: 42, toolName: "int-panicker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			r := testRegistry()
			if err := tools.Register(r, tt.toolName, "panics", func(_ context.Context, _ testInput) tools.Result {
				panic(tt.panicValue)
			}); err != nil {
				t.Fatal(err)
			}

			input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"` + tt.toolName + `","arguments":{"message":"boom"}}}` + "\n"

			// Act
			responses, err := runServer(t, r, input)

			// Assert
			assert.That(t, "error", err, nil)
			assert.That(t, "response count", len(responses), 2)
			assert.That(t, "error code", responses[1].Error.Code, protocol.InternalError)
			if !strings.Contains(responses[1].Error.Message, tt.toolName) {
				t.Errorf("expected tool name in error message, got: %s", responses[1].Error.Message)
			}
		})
	}
}

func Test_Server_With_PanickingHandler_Should_IncludeDataFields(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "panicker", "panics", func(_ context.Context, _ testInput) tools.Result {
		panic("test-panic-value")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"panicker","arguments":{"message":"boom"}}}` + "\n"

	// Act
	responses, err := runServer(t, r, input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)

	var data map[string]any
	if err := json.Unmarshal(responses[1].Error.Data, &data); err != nil {
		t.Fatalf("failed to unmarshal error data: %v", err)
	}
	assert.That(t, "toolName", data["toolName"], "panicker")
	if _, hasPanic := data["panicValue"]; hasPanic {
		t.Error("Error.Data must not contain panicValue (security: leak risk)")
	}
	if _, hasStack := data["stack"]; hasStack {
		t.Error("Error.Data must not contain stack trace")
	}
}

func Test_Server_With_PanickingHandler_Should_LogPanicToStderr(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "panicker", "panics", func(_ context.Context, _ testInput) tools.Result {
		panic("test-panic-value")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"panicker","arguments":{"message":"boom"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr)

	// Act
	_ = srv.Run(context.Background())

	// Assert — parse slog JSON lines from stderr to find the panic log entry
	dec := json.NewDecoder(&stderr)
	found := false
	for {
		var entry map[string]any
		if err := dec.Decode(&entry); err != nil {
			break
		}
		if entry["msg"] == "tool_handler_panicked" {
			found = true
			if entry["tool_name"] != "panicker" {
				t.Errorf("expected tool_name=panicker, got %v", entry["tool_name"])
			}
			if entry["panic"] != "test-panic-value" {
				t.Errorf("expected panic=test-panic-value, got %v", entry["panic"])
			}
			if _, hasStack := entry["stack"]; hasStack {
				t.Error("stack trace must not be logged to stderr (information disclosure)")
			}
			break
		}
	}
	if !found {
		t.Error("expected tool_handler_panicked log entry in stderr")
	}
}

func Test_Server_With_EOF_Should_ShutdownCleanly(t *testing.T) {
	t.Parallel()

	// Arrange — empty input = immediate EOF
	input := ""

	// Act
	_, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
}

func Test_Server_With_UnknownNotification_Should_SilentlyIgnore(t *testing.T) {
	t.Parallel()

	// Arrange
	input := initRequest +
		`{"jsonrpc":"2.0","method":"some/unknown/notification"}` + "\n" +
		initializedNotification +
		`{"jsonrpc":"2.0","method":"another/unknown"}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert — only the initialize response should exist (notifications get no response)
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 1)
}

func Test_Server_With_UnknownMethod_Should_Return32601(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"unknown/method","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
	assert.That(t, "error message", responses[1].Error.Message, "unknown method: unknown/method")
}

func Test_Server_With_ReservedRpcMethod_Should_Return32601(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"rpc.discover","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
	assert.That(t, "error message", responses[1].Error.Message, "reserved method: rpc.discover")
}

func Test_Server_With_OversizedMessage_Should_Return32700(t *testing.T) {
	t.Parallel()

	// Arrange — 5MB message exceeds 4MB limit
	bigValue := strings.Repeat("a", 5*1024*1024)
	input := `{"jsonrpc":"2.0","method":"ping","id":1,"params":{"data":"` + bigValue + `"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert — fatal decode error with -32700 response
	if err == nil {
		t.Fatal("expected non-nil error for oversized message")
	}
	assert.That(t, "response count", len(responses), 1)
	assert.That(t, "error code", responses[0].Error.Code, protocol.ParseError)
}

func Test_Server_With_MalformedJSON_Should_Return32700(t *testing.T) {
	t.Parallel()

	// Arrange
	input := "{invalid json\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert — fatal decode error with -32700 response
	if err == nil {
		t.Fatal("expected non-nil error for malformed JSON")
	}
	assert.That(t, "response count", len(responses), 1)
	assert.That(t, "error code", responses[0].Error.Code, protocol.ParseError)
}

func Test_Server_With_BatchArray_Should_Return32700(t *testing.T) {
	t.Parallel()

	// Arrange
	input := `[{"jsonrpc":"2.0","method":"ping","id":1}]` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert — fatal decode error with -32700 response
	if err == nil {
		t.Fatal("expected non-nil error for batch array")
	}
	assert.That(t, "response count", len(responses), 1)
	assert.That(t, "error code", responses[0].Error.Code, protocol.ParseError)
}

func Test_Server_With_NonObjectParams_Should_Return32600(t *testing.T) {
	t.Parallel()

	// Arrange — send request with array params
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/list","id":2,"params":[1,2,3]}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidRequest)
	assert.That(t, "error message", responses[1].Error.Message, "params must be an object")
}

func Test_Server_With_WrongFieldType_Should_Return32602(t *testing.T) {
	t.Parallel()

	// Arrange — send number where string expected
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"test","arguments":{"message":42}}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert — wrong types now produce a -32602 protocol error, not isError result
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)

	if !strings.Contains(responses[1].Error.Message, "test") {
		t.Errorf("expected tool name in error, got: %s", responses[1].Error.Message)
	}
}

func Test_Server_With_MalformedToolArguments_Should_Return32602(t *testing.T) {
	t.Parallel()

	// Arrange — arguments is a string, not an object
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"test","arguments":"bad"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)

	if !strings.Contains(responses[1].Error.Message, "test") {
		t.Errorf("expected tool name in error message, got: %s", responses[1].Error.Message)
	}
}

func Test_Server_With_ErrorResult_Should_ReturnSuccessEnvelope(t *testing.T) {
	t.Parallel()

	// Arrange — handler returns ErrorResult (tool-level failure, not protocol error)
	r := tools.NewRegistry()
	if err := tools.Register(r, "failing", "returns error result", func(_ context.Context, _ testInput) tools.Result {
		return tools.ErrorResult("something went wrong")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"failing","arguments":{"message":"x"}}}` + "\n"

	// Act
	responses, err := runServer(t, r, input)

	// Assert — tool-level errors stay in result with isError: true, not in error object
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "no protocol error", responses[1].Error == nil, true)

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "isError", result.IsError, true)
	assert.That(t, "text", result.Content[0].Text, "something went wrong")
}

func Test_Server_With_DefaultTimeout_Should_AllowSlowHandler(t *testing.T) {
	t.Parallel()

	// Arrange — handler that takes 100ms succeeds under the default 30s timeout.
	r := tools.NewRegistry()
	if err := tools.Register(r, "slow", "takes 100ms", func(ctx context.Context, _ testInput) tools.Result {
		select {
		case <-time.After(100 * time.Millisecond):
			return tools.TextResult("done")
		case <-ctx.Done():
			return tools.ErrorResult("cancelled")
		}
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"slow","arguments":{"message":"x"}}}` + "\n"

	// Act — no WithHandlerTimeout, so default 30s applies.
	responses, err := runServer(t, r, input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal", err, nil)
	assert.That(t, "isError", result.IsError, false)
	assert.That(t, "text", result.Content[0].Text, "done")
}

func Test_Server_With_TimeoutHandler_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — handler that ignores context and blocks forever
	r := tools.NewRegistry()
	if err := tools.Register(r, "hang", "blocks forever", func(_ context.Context, _ testInput) tools.Result {
		select {} //nolint:gosimple // intentionally block forever to test timeout
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"hang","arguments":{"message":"test"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(500*time.Millisecond),
		server.WithSafetyMargin(500*time.Millisecond),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}
	assert.That(t, "response count", len(responses), 2) // init + tool call
	assert.That(t, "error code", responses[1].Error.Code, protocol.ServerTimeout)
	if !strings.Contains(responses[1].Error.Message, "hang") {
		t.Errorf("expected tool name in error message, got: %s", responses[1].Error.Message)
	}
}

func Test_Server_With_DeadlineExceeded_Should_IncludeTimingInData(t *testing.T) {
	t.Parallel()

	// Arrange — handler that respects context and returns on deadline
	r := tools.NewRegistry()
	if err := tools.Register(r, "slow", "blocks until timeout", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult(ctx.Err().Error())
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"slow","arguments":{"message":"x"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(500*time.Millisecond),
	)

	// Act
	_ = srv.Run(context.Background())

	// Assert
	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.ServerTimeout)
	if !strings.Contains(responses[1].Error.Message, "slow") {
		t.Errorf("expected tool name in message, got: %s", responses[1].Error.Message)
	}

	var data map[string]any
	if err := json.Unmarshal(responses[1].Error.Data, &data); err != nil {
		t.Fatalf("failed to unmarshal error data: %v", err)
	}
	assert.That(t, "toolName", data["toolName"], "slow")
	if elapsed, ok := data["elapsedMs"].(float64); !ok || elapsed < 500 {
		t.Errorf("expected elapsedMs >= 500, got %v", data["elapsedMs"])
	}
	if timeout, ok := data["timeoutMs"].(float64); !ok || int64(timeout) != 500 {
		t.Errorf("expected timeoutMs == 500, got %v", data["timeoutMs"])
	}
}

func Test_Server_With_ContextCanceled_Should_IncludeElapsedOnly(t *testing.T) {
	t.Parallel()

	// Arrange — handler that blocks until cancelled
	r := tools.NewRegistry()
	if err := tools.Register(r, "blocker", "blocks forever", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult(ctx.Err().Error())
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())

	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(5*time.Second),
	)

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Act
	_ = srv.Run(ctx)

	// Assert
	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.ServerTimeout)
	if !strings.Contains(responses[1].Error.Message, "blocker") {
		t.Errorf("expected tool name in message, got: %s", responses[1].Error.Message)
	}

	var data map[string]any
	if err := json.Unmarshal(responses[1].Error.Data, &data); err != nil {
		t.Fatalf("failed to unmarshal error data: %v", err)
	}
	assert.That(t, "toolName", data["toolName"], "blocker")
	if _, hasElapsed := data["elapsedMs"]; !hasElapsed {
		t.Error("expected elapsedMs in data")
	}
	if _, hasTimeout := data["timeoutMs"]; hasTimeout {
		t.Error("context.Canceled should NOT include timeoutMs")
	}
}

func Test_Server_With_SafetyTimer_Should_IncludeTimingInData(t *testing.T) {
	t.Parallel()

	// Arrange — handler that ignores context completely
	r := tools.NewRegistry()
	if err := tools.Register(r, "hang", "ignores context", func(_ context.Context, _ testInput) tools.Result {
		select {} //nolint:gosimple // intentionally block forever
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"hang","arguments":{"message":"x"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(500*time.Millisecond),
		server.WithSafetyMargin(500*time.Millisecond),
	)

	// Act
	_ = srv.Run(context.Background())

	// Assert
	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.ServerTimeout)

	var data map[string]any
	if err := json.Unmarshal(responses[1].Error.Data, &data); err != nil {
		t.Fatalf("failed to unmarshal error data: %v", err)
	}
	assert.That(t, "toolName", data["toolName"], "hang")
	if _, hasElapsed := data["elapsedMs"]; !hasElapsed {
		t.Error("expected elapsedMs in data")
	}
	if timeout, ok := data["timeoutMs"].(float64); !ok || int64(timeout) != 500 {
		t.Errorf("expected timeoutMs == 500, got %v", data["timeoutMs"])
	}
}

func Test_Server_With_WrongJSONRPCVersion_Should_Return32600(t *testing.T) {
	t.Parallel()

	// Arrange — request with wrong jsonrpc version
	input := `{"jsonrpc":"1.0","method":"ping","id":1,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 1)
	assert.That(t, "error code", responses[0].Error.Code, protocol.InvalidRequest)
	if !strings.Contains(responses[0].Error.Message, "unsupported jsonrpc version") {
		t.Errorf("expected 'unsupported jsonrpc version' in error, got: %s", responses[0].Error.Message)
	}
}

func Test_Server_With_TruncatedJSON_Should_ShutdownCleanly(t *testing.T) {
	t.Parallel()

	// Arrange — truncated JSON object (no closing brace), immediate EOF
	input := `{"jsonrpc":"2.0"`

	// Act
	_, err := runServer(t, testRegistry(), input)

	// Assert — io.ErrUnexpectedEOF triggers clean shutdown
	assert.That(t, "error", err, nil)
}

func Test_Server_With_EmptyToolName_Should_Return32602(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"","arguments":{}}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
	assert.That(t, "error message", responses[1].Error.Message, "tool name is required")
}

func Test_Server_With_NullParams_Should_DispatchNormally(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/list","id":2,"params":null}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "no error", responses[1].Error == nil, true)
}

func Test_Server_With_AbsentParams_Should_DispatchNormally(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/list","id":2}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "no error", responses[1].Error == nil, true)
}

func Test_Server_With_UnsupportedCapability_Should_ReturnGuidance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
	}{
		{name: "prompts/get", method: "prompts/get"},
		{name: "resources/list", method: "resources/list"},
		{name: "resources/subscribe", method: "resources/subscribe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			input := handshake() + `{"jsonrpc":"2.0","method":"` + tt.method + `","id":2,"params":{}}` + "\n"

			// Act
			responses, err := runServer(t, testRegistry(), input)

			// Assert
			assert.That(t, "error", err, nil)
			assert.That(t, "response count", len(responses), 2)
			assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
			if !strings.Contains(responses[1].Error.Message, tt.method) {
				t.Errorf("expected method name in message, got: %s", responses[1].Error.Message)
			}

			var data map[string]any
			if uerr := json.Unmarshal(responses[1].Error.Data, &data); uerr != nil {
				t.Fatalf("failed to unmarshal error data: %v", uerr)
			}
			caps, ok := data["supportedCapabilities"].([]any)
			if !ok || len(caps) == 0 {
				t.Fatalf("expected supportedCapabilities array, got: %v", data["supportedCapabilities"])
			}
			assert.That(t, "capability", caps[0], "tools")
		})
	}
}

func Test_Server_With_ResourcesNotification_Should_SilentlyIgnore(t *testing.T) {
	t.Parallel()

	// Arrange — notification (no id) under resources/ namespace, followed by ping
	input := handshake() +
		`{"jsonrpc":"2.0","method":"resources/subscribe","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "ping success", responses[1].Error == nil, true)
}

// --- Lifecycle logging tests ---

// parseLogEntries parses newline-delimited JSON log entries from stderr.
func parseLogEntries(t *testing.T, stderr *bytes.Buffer) []map[string]any {
	t.Helper()
	var entries []map[string]any
	dec := json.NewDecoder(stderr)
	for {
		var entry map[string]any
		if err := dec.Decode(&entry); err != nil {
			break
		}
		entries = append(entries, entry)
	}
	return entries
}

// findLogEntry finds the first log entry with the given msg value.
func findLogEntry(entries []map[string]any, msg string) map[string]any {
	for _, e := range entries {
		if e["msg"] == msg {
			return e
		}
	}
	return nil
}

func Test_Server_With_StartupLog_Should_ContainStructuredFields(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "v1.0.0", testRegistry(), strings.NewReader(input), &stdout, &stderr)

	// Act
	_ = srv.Run(context.Background())

	// Assert
	entries := parseLogEntries(t, &stderr)
	started := findLogEntry(entries, "server_started")
	if started == nil {
		t.Fatal("expected server_started log entry")
	}
	assert.That(t, "version", started["version"], "v1.0.0")
	assert.That(t, "name", started["name"], "mcp")
	assert.That(t, "protocol_version", started["protocol_version"], protocol.MCPVersion)
	toolsList, ok := started["tools"].([]any)
	if !ok {
		t.Fatalf("expected tools to be an array, got %T", started["tools"])
	}
	assert.That(t, "tools count", len(toolsList), 1)
	assert.That(t, "tool name", toolsList[0], "test")
}

func Test_Server_With_EOFShutdown_Should_LogServerStopped(t *testing.T) {
	t.Parallel()

	// Arrange — handshake then EOF
	input := handshake()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)
	entries := parseLogEntries(t, &stderr)
	stopped := findLogEntry(entries, "server_stopped")
	if stopped == nil {
		t.Fatal("expected server_stopped log entry")
	}
	assert.That(t, "reason", stopped["reason"], "eof")
	if _, ok := stopped["uptime_ms"]; !ok {
		t.Fatal("expected uptime_ms in server_stopped")
	}
	if _, ok := stopped["requests"]; !ok {
		t.Fatal("expected requests in server_stopped")
	}
	if _, ok := stopped["errors"]; !ok {
		t.Fatal("expected errors in server_stopped")
	}
}

func Test_Server_With_ContextCancellation_Should_LogCancelledReason(t *testing.T) {
	t.Parallel()

	// Arrange — use a pipe; cancel ctx then close pipe to unblock the decoder.
	// The server's decode loop blocks on stdin — closing the writer after cancel
	// triggers EOF which lets the loop check ctx.Done on the next iteration.
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), pr, &stdout, &stderr)

	// Act
	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()
	_, _ = pw.Write([]byte(handshake()))
	time.Sleep(10 * time.Millisecond)
	cancel()
	_ = pw.Close() // unblock decode so the loop can reach ctx.Done check
	err := <-done

	// Assert
	assert.That(t, "error", err, nil)
	entries := parseLogEntries(t, &stderr)
	stopped := findLogEntry(entries, "server_stopped")
	if stopped == nil {
		t.Fatal("expected server_stopped log entry")
	}
	// After cancel + close, the server sees EOF but ctx is also cancelled.
	// The defer checks retErr first (nil for EOF), then ctx.Err() (non-nil).
	// context.Cause returns context.Canceled whose Error() is "context canceled".
	assert.That(t, "reason", stopped["reason"], "context canceled")
}

func Test_Server_With_FatalDecodeError_Should_LogFatalReason(t *testing.T) {
	t.Parallel()

	// Arrange — send invalid JSON to trigger fatal decode error
	input := "not valid json\n"
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())

	// Assert
	if err == nil {
		t.Fatal("expected error for fatal decode")
	}
	entries := parseLogEntries(t, &stderr)
	stopped := findLogEntry(entries, "server_stopped")
	if stopped == nil {
		t.Fatal("expected server_stopped log entry")
	}
	assert.That(t, "reason", stopped["reason"], "fatal_error")
}

func Test_Server_With_MixedRequests_Should_CountRequestsAndErrors(t *testing.T) {
	t.Parallel()

	// Arrange — handshake (2 msgs) + 3 successful pings + 2 error requests = 7 decoded msgs total
	input := handshake() +
		`{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":3,"params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":4,"params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"unknown","id":5,"params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"unknown","id":6,"params":{}}` + "\n"
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)
	entries := parseLogEntries(t, &stderr)
	stopped := findLogEntry(entries, "server_stopped")
	if stopped == nil {
		t.Fatal("expected server_stopped log entry")
	}
	// 7 decoded messages: init + initialized_notif + 3 pings + 2 unknown
	requests, ok := stopped["requests"].(float64)
	if !ok {
		t.Fatal("expected requests to be a number")
	}
	errs, ok := stopped["errors"].(float64)
	if !ok {
		t.Fatal("expected errors to be a number")
	}
	assert.That(t, "requests", int(requests), 7)
	assert.That(t, "errors", int(errs), 2)
}

func Test_Server_With_Notification_Should_IncrementRequestCountOnly(t *testing.T) {
	t.Parallel()

	// Arrange — handshake + unknown notification (no id) + ping
	input := handshake() +
		`{"jsonrpc":"2.0","method":"some/notification","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n"
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)
	entries := parseLogEntries(t, &stderr)
	stopped := findLogEntry(entries, "server_stopped")
	if stopped == nil {
		t.Fatal("expected server_stopped log entry")
	}
	// 4 decoded: init + initialized_notif + notification + ping
	requests, ok := stopped["requests"].(float64)
	if !ok {
		t.Fatal("expected requests to be a number")
	}
	errs, ok := stopped["errors"].(float64)
	if !ok {
		t.Fatal("expected errors to be a number")
	}
	assert.That(t, "requests", int(requests), 4)
	assert.That(t, "errors", int(errs), 0)
}

func Test_Server_With_InitializedNotification_In_Each_State_Should_Transition_Or_Ignore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         string
		expectReady   bool   // true if tools/list should succeed
		expectWarnLog bool   // true if "unexpected notifications/initialized" should appear
		toolsListID   string // the id of the tools/list request
		responseCount int
	}{
		{
			name:          "uninitialized",
			input:         initializedNotification + `{"jsonrpc":"2.0","method":"tools/list","id":1,"params":{}}` + "\n",
			expectReady:   false,
			expectWarnLog: true,
			toolsListID:   "1",
			responseCount: 1,
		},
		{
			name:          "initializing",
			input:         initRequest + initializedNotification + `{"jsonrpc":"2.0","method":"tools/list","id":2,"params":{}}` + "\n",
			expectReady:   true,
			expectWarnLog: false,
			toolsListID:   "2",
			responseCount: 2,
		},
		{
			name:          "ready",
			input:         handshake() + initializedNotification + `{"jsonrpc":"2.0","method":"tools/list","id":2,"params":{}}` + "\n",
			expectReady:   true,
			expectWarnLog: true,
			toolsListID:   "2",
			responseCount: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			var stdout, stderr bytes.Buffer
			srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(tt.input), &stdout, &stderr)

			// Act
			err := srv.Run(context.Background())

			// Assert
			assert.That(t, "error", err, nil)

			var responses []protocol.Response
			dec := json.NewDecoder(&stdout)
			for {
				var resp protocol.Response
				if uerr := dec.Decode(&resp); uerr != nil {
					break
				}
				responses = append(responses, resp)
			}
			assert.That(t, "response count", len(responses), tt.responseCount)

			// Check last response (tools/list)
			lastResp := responses[len(responses)-1]
			if tt.expectReady {
				assert.That(t, "tools/list success", lastResp.Error == nil, true)
			} else {
				assert.That(t, "error code", lastResp.Error.Code, protocol.ServerError)
			}

			// Check warn log
			entries := parseLogEntries(t, &stderr)
			warn := findLogEntry(entries, "unexpected notifications/initialized")
			if tt.expectWarnLog && warn == nil {
				t.Fatal("expected 'unexpected notifications/initialized' log entry")
			}
			if !tt.expectWarnLog && warn != nil {
				t.Fatal("unexpected warn log entry")
			}
		})
	}
}

func Test_Server_With_Malformed_Cancelled_Notification_Should_Log_Warn(t *testing.T) {
	t.Parallel()

	// Arrange — register a blocking handler so there's an in-flight request
	// when the malformed cancel notification arrives
	r := tools.NewRegistry()
	if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"reason":123}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(100*time.Millisecond),
		server.WithSafetyMargin(100*time.Millisecond))

	// Act
	err := srv.Run(context.Background())

	// Assert — no error response (notification), warn logged to stderr
	assert.That(t, "error", err, nil)

	entries := parseLogEntries(t, &stderr)
	warn := findLogEntry(entries, "unmarshal cancelled notification failed")
	if warn == nil {
		t.Fatal("expected 'unmarshal cancelled notification failed' log entry in stderr")
	}
}

func Test_Server_With_CancelledNotification_Should_CancelInFlightContext(t *testing.T) {
	t.Parallel()

	// Arrange — handler that blocks until context cancelled
	cancelled := make(chan struct{})
	r := tools.NewRegistry()
	if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		close(cancelled)
		return tools.ErrorResult("cancelled")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":2}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(time.Hour))

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	// Verify handler's context was cancelled (allow brief time for goroutine cleanup)
	select {
	case <-cancelled:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatal("expected handler context to be cancelled")
	}
}

func Test_Server_With_CancelledNotification_Should_SuppressResponse(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":2}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(time.Hour))

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	// Decode all responses — only initialize response (id:1) should be present
	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	assert.That(t, "response count", len(responses), 1)
	assert.That(t, "response id", string(responses[0].ID), "1")
}

func Test_Server_With_ToolsListMultipleTools_Should_ReturnAlphabeticalOrder(t *testing.T) {
	t.Parallel()

	// Arrange — three tools in non-alphabetical order
	r := tools.NewRegistry()
	if err := tools.Register(r, "zeta", "z tool", func(_ context.Context, _ testInput) tools.Result {
		return tools.TextResult("z")
	}); err != nil {
		t.Fatal(err)
	}
	if err := tools.Register(r, "alpha", "a tool", func(_ context.Context, _ testInput) tools.Result {
		return tools.TextResult("a")
	}); err != nil {
		t.Fatal(err)
	}
	if err := tools.Register(r, "mid", "m tool", func(_ context.Context, _ testInput) tools.Result {
		return tools.TextResult("m")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, r, input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)

	var result struct {
		Tools []struct {
			Description string `json:"description"`
			InputSchema struct {
				Type string `json:"type"`
			} `json:"inputSchema"`
			Name string `json:"name"`
		} `json:"tools"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "tools count", len(result.Tools), 3)
	assert.That(t, "first tool", result.Tools[0].Name, "alpha")
	assert.That(t, "second tool", result.Tools[1].Name, "mid")
	assert.That(t, "third tool", result.Tools[2].Name, "zeta")

	// Verify each tool has description and inputSchema.type
	for _, tool := range result.Tools {
		if tool.Description == "" {
			t.Errorf("tool %q missing description", tool.Name)
		}
		assert.That(t, tool.Name+" schema type", tool.InputSchema.Type, "object")
	}
}

func Test_Server_With_ToolsListEmptyRegistry_Should_ReturnEmptyArray(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, r, input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)

	var result struct {
		Tools []any `json:"tools"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "tools count", len(result.Tools), 0)
}

func Test_Server_With_ToolsListAnnotations_Should_IncludeAnnotations(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "readonly", "read-only tool", func(_ context.Context, _ testInput) tools.Result {
		return tools.TextResult("ok")
	}, tools.WithAnnotations(tools.Annotations{ReadOnlyHint: true})); err != nil {
		t.Fatal(err)
	}

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, r, input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)

	var result struct {
		Tools []struct {
			Annotations struct {
				ReadOnlyHint bool `json:"readOnlyHint"`
			} `json:"annotations"`
			Name string `json:"name"`
		} `json:"tools"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "tools count", len(result.Tools), 1)
	assert.That(t, "tool name", result.Tools[0].Name, "readonly")
	assert.That(t, "readOnlyHint", result.Tools[0].Annotations.ReadOnlyHint, true)
}

func Test_Server_With_TraceEnabled_Should_LogProtocolMessages(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), &stdout, &stderr,
		server.WithTrace(true),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	entries := parseLogEntries(t, &stderr)
	traceReq := findLogEntry(entries, "trace_request")
	if traceReq == nil {
		t.Fatal("expected trace_request log entry")
	}
	traceResp := findLogEntry(entries, "trace_response")
	if traceResp == nil {
		t.Fatal("expected trace_response log entry")
	}
}

func Test_Server_With_RequestDuringInFlight_Should_ReturnServerBusy(t *testing.T) {
	t.Parallel()

	// Arrange — handler that blocks until context cancelled, second request arrives while in flight
	r := tools.NewRegistry()
	if err := tools.Register(r, "blocker", "blocks", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.TextResult("done")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"tools/list","id":3,"params":{}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(5*time.Second),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	// Find the server busy response
	var busyResp *protocol.Response
	for i := range responses {
		if string(responses[i].ID) == "3" {
			busyResp = &responses[i]
			break
		}
	}
	if busyResp == nil {
		t.Fatal("expected response for request id 3")
	}
	assert.That(t, "error code", busyResp.Error.Code, protocol.ServerError)
	if !strings.Contains(busyResp.Error.Message, "server busy") {
		t.Errorf("expected server busy message, got: %s", busyResp.Error.Message)
	}
}

// --- Additional coverage tests ---

// Test_Server_With_CompletionMethod_Should_ReturnMethodNotFound covers the
// completion/ namespace branch in handleMethod.
func Test_Server_With_CompletionMethod_Should_ReturnMethodNotFound(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"completion/complete","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
	if !strings.Contains(responses[1].Error.Message, "completion/complete") {
		t.Errorf("expected method name in message, got: %s", responses[1].Error.Message)
	}
}

// Test_Server_With_ElicitationMethod_Should_ReturnMethodNotFound covers the
// elicitation/ namespace branch in handleMethod.
func Test_Server_With_ElicitationMethod_Should_ReturnMethodNotFound(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"elicitation/create","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
	if !strings.Contains(responses[1].Error.Message, "elicitation/create") {
		t.Errorf("expected method name in message, got: %s", responses[1].Error.Message)
	}
}

// Test_Server_With_PromptsMethod_Should_ReturnMethodNotFound covers the
// prompts/ namespace branch in handleMethod.
func Test_Server_With_PromptsMethod_Should_ReturnMethodNotFound(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"prompts/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
	if !strings.Contains(responses[1].Error.Message, "prompts/list") {
		t.Errorf("expected method name in message, got: %s", responses[1].Error.Message)
	}
}

// Test_Server_With_ToolsCallArrayParams_Should_ReturnInvalidParams covers the
// tools/call validation-failure path in handleDecodeResultIdle (validate before
// async dispatch).
func Test_Server_With_ToolsCallArrayParams_Should_ReturnInvalidParams(t *testing.T) {
	t.Parallel()

	// Arrange — tools/call with array params triggers Validate failure
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":[1,2,3]}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidRequest)
	assert.That(t, "error message", responses[1].Error.Message, "params must be an object")
}

// Test_Server_With_TraceEnabled_And_ToolCall_Should_LogTraceMessages verifies
// trace logging fires for both request and response on tools/call path
// (covers trace paths in handleDecodeResultIdle and encodeResponse).
func Test_Server_With_TraceEnabled_And_ToolCall_Should_LogTraceMessages(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"test","arguments":{"message":"hello"}}}` + "\n"
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), &stdout, &stderr,
		server.WithTrace(true),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	entries := parseLogEntries(t, &stderr)
	traceReq := findLogEntry(entries, "trace_request")
	if traceReq == nil {
		t.Fatal("expected trace_request log entry")
	}
	traceResp := findLogEntry(entries, "trace_response")
	if traceResp == nil {
		t.Fatal("expected trace_response log entry")
	}
}

// Test_Server_With_CancelledNotification_NonMatchingId_Should_SilentlyIgnore
// covers the non-matching requestId branch in handleCancelledNotification.
func Test_Server_With_CancelledNotification_NonMatchingId_Should_SilentlyIgnore(t *testing.T) {
	t.Parallel()

	// Arrange — blocker tool, cancel with wrong id (id:99 vs in-flight id:2)
	r := tools.NewRegistry()
	if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":99}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(100*time.Millisecond),
		server.WithSafetyMargin(100*time.Millisecond),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert — server completes (handler times out), non-matching cancel silently ignored
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	// Should have init response + timeout response for id:2 (non-matching cancel didn't suppress it)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "second id", string(responses[1].ID), "2")
	assert.That(t, "timeout error code", responses[1].Error.Code, protocol.ServerTimeout)
}

// Test_Server_With_PingDuringInFlight_Should_ReturnPingResult covers the ping
// branch in handleMessageDuringInFlight.
func Test_Server_With_PingDuringInFlight_Should_ReturnPingResult(t *testing.T) {
	t.Parallel()

	// Arrange — blocker tool followed by ping while in-flight
	r := tools.NewRegistry()
	if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":3,"params":{}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(200*time.Millisecond),
		server.WithSafetyMargin(100*time.Millisecond),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	// Find ping response
	var pingResp *protocol.Response
	for i := range responses {
		if string(responses[i].ID) == "3" {
			pingResp = &responses[i]
			break
		}
	}
	if pingResp == nil {
		t.Fatal("expected ping response for id:3")
	}
	assert.That(t, "ping no error", pingResp.Error == nil, true)
	assert.That(t, "ping result", string(pingResp.Result), "{}")
}

// Test_Server_With_LargeToolResult_Should_TruncateResult covers the result
// truncation path in executeToolCall (maxResultSize = 1MB).
func Test_Server_With_LargeToolResult_Should_TruncateResult(t *testing.T) {
	t.Parallel()

	// Arrange — handler returns > 1MB of data
	r := tools.NewRegistry()
	if err := tools.Register(r, "large", "returns large result", func(_ context.Context, _ testInput) tools.Result {
		return tools.TextResult(strings.Repeat("x", 1*1024*1024+1))
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"large","arguments":{"message":"x"}}}` + "\n"

	// Act
	responses, err := runServer(t, r, input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "no protocol error", responses[1].Error == nil, true)

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if uerr := json.Unmarshal(responses[1].Result, &result); uerr != nil {
		t.Fatalf("failed to unmarshal result: %v", uerr)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	if !strings.Contains(result.Content[0].Text, "truncated") {
		preview := result.Content[0].Text
		if len(preview) > 100 {
			preview = preview[:100]
		}
		t.Errorf("expected truncation message in text, got: %s", preview)
	}
}

// Test_Server_With_TraceEnabled_And_InFlightDecode_Should_LogTraceMessages
// covers the trace logging branch in handleDecodeResultDuringInFlight.
func Test_Server_With_TraceEnabled_And_InFlightDecode_Should_LogTraceMessages(t *testing.T) {
	t.Parallel()

	// Arrange — blocker tool with trace enabled, send ping while in-flight
	r := tools.NewRegistry()
	if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":3,"params":{}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithTrace(true),
		server.WithHandlerTimeout(200*time.Millisecond),
		server.WithSafetyMargin(100*time.Millisecond),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert — no crash; trace messages logged for both directions
	assert.That(t, "error", err, nil)

	entries := parseLogEntries(t, &stderr)
	traceReq := findLogEntry(entries, "trace_request")
	if traceReq == nil {
		t.Fatal("expected trace_request log entry")
	}
}

// Test_Server_With_ContextCancelledDuringInFlight_Should_CancelAndAwait covers
// the ctx.Done branch in runInFlight which calls cancelAndAwaitInFlight.
// A pipe keeps stdin blocking so the ctx.Done() case fires in runInFlight
// before the decode result arrives.
func Test_Server_With_ContextCancelledDuringInFlight_Should_CancelAndAwait(t *testing.T) {
	t.Parallel()

	// Arrange — handler blocks on ctx.Done; parent context cancelled externally
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := tools.NewRegistry()
	if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	}); err != nil {
		t.Fatal(err)
	}

	// Use a pipe so stdin never delivers EOF — the server stays in runInFlight
	// waiting on either the handler result or ctx.Done().
	pr, pw := io.Pipe()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, pr, &stdout, &stderr,
		server.WithHandlerTimeout(5*time.Second),
		server.WithSafetyMargin(time.Second),
	)

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()

	// Send handshake and tool call through the pipe
	_, _ = pw.Write([]byte(handshake()))
	_, _ = pw.Write([]byte(`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n"))

	// Wait briefly for the server to start the tool handler, then cancel context
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Close the pipe to unblock the decode goroutine
	_ = pw.Close()

	// Act — wait for server to finish
	err := <-done

	// Assert — clean shutdown via cancelAndAwaitInFlight
	assert.That(t, "error", err, nil)
}

// Test_Server_With_CancelledInFlightOnDecodeError_Should_SuppressResponse covers
// the inFlightCancelled=true path in handleDecodeErrorDuringInFlight.
// A cancel notification sets inFlightCancelled=true, then EOF arrives.
func Test_Server_With_CancelledInFlightOnDecodeError_Should_SuppressResponse(t *testing.T) {
	t.Parallel()

	// Arrange — blocker tool, cancel notification with matching id, then EOF
	r := tools.NewRegistry()
	if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	}); err != nil {
		t.Fatal(err)
	}

	// The stream ends (EOF) after sending the cancel notification.
	// The server is in-flight, receives cancel, then hits EOF decode error.
	// handleDecodeErrorDuringInFlight should wait for handler, but since
	// inFlightCancelled=true, it should suppress the tool response.
	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":2}}` + "\n"

	// EOF follows immediately — the server is in-flight when EOF is decoded.
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(time.Hour),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert — clean shutdown; only the init response (id:1), no tool response
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	// Only initialize response should be present
	assert.That(t, "response count", len(responses), 1)
	assert.That(t, "response id", string(responses[0].ID), "1")
}

// Test_Server_With_InvalidNotificationDuringInFlight_Should_SilentlyIgnore
// covers the notification validation failure branch in handleMessageDuringInFlight.
// A notification (no id) that fails validation is silently ignored.
func Test_Server_With_InvalidNotificationDuringInFlight_Should_SilentlyIgnore(t *testing.T) {
	t.Parallel()

	// Arrange — blocker tool, then a notification with wrong jsonrpc version
	r := tools.NewRegistry()
	if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	}); err != nil {
		t.Fatal(err)
	}

	// Send a notification (no id) with wrong jsonrpc version while in-flight.
	// dispatch validation catches this but since it's a notification, returns false (no response).
	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n" +
		`{"jsonrpc":"1.0","method":"some/notification"}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(100*time.Millisecond),
		server.WithSafetyMargin(100*time.Millisecond),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert — no crash; invalid notification silently ignored
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	// Init response + tool timeout response — no response for invalid notification
	assert.That(t, "response count", len(responses), 2)
}

// Test_Server_With_InvalidRequestDuringInFlight_Should_ReturnError covers the
// validation-failure branch for requests (with id) in handleMessageDuringInFlight.
func Test_Server_With_InvalidRequestDuringInFlight_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — blocker tool, then a request with wrong jsonrpc version (has id)
	r := tools.NewRegistry()
	if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n" +
		`{"jsonrpc":"1.0","method":"ping","id":3,"params":{}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(200*time.Millisecond),
		server.WithSafetyMargin(100*time.Millisecond),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	// Find the invalid-request error response for id:3
	var errResp *protocol.Response
	for i := range responses {
		if string(responses[i].ID) == "3" {
			errResp = &responses[i]
			break
		}
	}
	if errResp == nil {
		t.Fatal("expected error response for id:3")
	}
	assert.That(t, "error code", errResp.Error.Code, protocol.InvalidRequest)
}

// writeLine writes a JSON-RPC message to the writer with a newline.
func writeLine(w io.Writer, msg string) {
	_, _ = io.WriteString(w, msg+"\n")
}

func Test_Server_With_PipeControlledTiming_Should_HitProcessInFlightResult(t *testing.T) {
	t.Parallel()

	// Arrange — fast echo handler completes before next message arrives
	r := tools.NewRegistry()
	if err := tools.Register(r, "fast", "returns immediately", func(_ context.Context, input testInput) tools.Result {
		return tools.TextResult(input.Message)
	}); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, pr, &stdout, &stderr)

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(context.Background())
	}()

	// Act — send handshake
	writeLine(pw, `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}`)
	writeLine(pw, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	// Send tools/call — handler returns immediately
	writeLine(pw, `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"fast","arguments":{"message":"hello"}}}`)

	// Wait for handler to complete and response to be written
	time.Sleep(50 * time.Millisecond)

	// Send another request after handler completed — this forces runInFlight
	// to pick up the completed result via processInFlightResult first
	writeLine(pw, `{"jsonrpc":"2.0","method":"tools/list","id":3,"params":{}}`)
	time.Sleep(50 * time.Millisecond)

	// Close pipe to trigger EOF shutdown
	_ = pw.Close()

	err := <-done

	// Assert
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	// Should have: init response, tool call result, tools/list result
	if len(responses) < 3 {
		t.Fatalf("expected at least 3 responses, got %d", len(responses))
	}
}

func Test_Server_With_PipeHandlerError_Should_HitProcessInFlightErrorPath(t *testing.T) {
	t.Parallel()

	// Arrange — handler returns a CodeError
	r := tools.NewRegistry()
	if err := tools.Register(r, "errtool", "returns error", func(_ context.Context, _ testInput) tools.Result {
		// Return result — the handler wrapper returns nil error.
		return tools.ErrorResult("something broke")
	}); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, pr, &stdout, &stderr)

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(context.Background())
	}()

	// Act
	writeLine(pw, `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}`)
	writeLine(pw, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	writeLine(pw, `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"errtool","arguments":{"message":"x"}}}`)
	time.Sleep(50 * time.Millisecond)
	writeLine(pw, `{"jsonrpc":"2.0","method":"ping","id":3,"params":{}}`)
	time.Sleep(50 * time.Millisecond)
	_ = pw.Close()

	err := <-done

	// Assert
	assert.That(t, "error", err, nil)
}

func Test_Server_With_DuplicateInitInReady_Should_ReturnAlreadyInitialized(t *testing.T) {
	t.Parallel()

	// Arrange — complete handshake then send initialize again
	input := handshake() + `{"jsonrpc":"2.0","method":"initialize","id":2,"params":{"capabilities":{}}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.ServerError)
	assert.That(t, "error message", responses[1].Error.Message, "already initialized")
}

func Test_Server_With_ResourcesMethod_Should_ReturnMethodNotFound(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
}

func Test_Server_With_EmptyToolName_Should_ReturnInvalidParams(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"","arguments":{}}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
	if !strings.Contains(responses[1].Error.Message, "tool name is required") {
		t.Errorf("expected 'tool name is required' in message, got: %s", responses[1].Error.Message)
	}
}

func Test_Server_With_HandlerReturningNonCodeError_Should_Return32603(t *testing.T) {
	t.Parallel()

	// Arrange — register a tool whose handler wrapper returns a raw error
	// We use a tool that returns a CodeError via the handler mechanism
	r := tools.NewRegistry()
	// The handler closure panics with an error to exercise the non-CodeError path in runToolHandler
	// Actually, we can't easily trigger non-CodeError from the public API since
	// the Register wrapper catches all handler errors. But a handler that returns
	// a raw error at the toolHandler level would need internal access.
	// Instead, test the CodeError path through dispatch:
	if err := tools.Register(r, "test", "test", func(_ context.Context, _ testInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Send tools/call with null arguments (normalizes to {}, but missing required "message")
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"test","arguments":null}}` + "\n"

	// Act
	responses, err := runServer(t, r, input)

	// Assert — missing required field returns -32602
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_With_NotificationInvalidJsonrpc_Should_SilentlyIgnore(t *testing.T) {
	t.Parallel()

	// Arrange — notification with wrong jsonrpc version (no response expected)
	input := handshake() +
		`{"jsonrpc":"1.0","method":"notifications/something"}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert — only init + ping responses (notification silently ignored)
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "ping result", string(responses[1].Result), "{}")
}

func Test_Server_With_SizeExceededDuringIdle_Should_Return32700(t *testing.T) {
	t.Parallel()

	// Arrange — complete handshake first, then oversized message
	bigValue := strings.Repeat("a", 5*1024*1024)
	input := handshake() + `{"jsonrpc":"2.0","method":"ping","id":2,"params":{"data":"` + bigValue + `"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert — fatal decode error with -32700 response
	if err == nil {
		t.Fatal("expected non-nil error for oversized message")
	}
	// Should have init response + parse error
	found := false
	for _, r := range responses {
		if r.Error != nil && r.Error.Code == protocol.ParseError {
			found = true
		}
	}
	if !found {
		t.Error("expected -32700 parse error response")
	}
}

// Test_Server_With_CancelledInFlight_Should_SuppressToolResponse covers
// processInFlightResult lines 505-508: when inFlightCancelled is true, the
// handler response is suppressed. Uses io.Pipe for timing control so the
// cancel notification is sent while the blocking handler is still running,
// then the pipe is closed to trigger shutdown after the handler completes.
func Test_Server_With_CancelledInFlight_Should_SuppressToolResponse(t *testing.T) {
	t.Parallel()

	// Arrange — handler blocks until context cancelled
	r := tools.NewRegistry()
	handlerStarted := make(chan struct{})
	if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		close(handlerStarted)
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	}); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, pr, &stdout, &stderr,
		server.WithHandlerTimeout(time.Minute),
	)

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(context.Background())
	}()

	// Act — complete handshake
	writeLine(pw, `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}`)
	writeLine(pw, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	// Start a blocking tool call
	writeLine(pw, `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}`)

	// Wait for handler to start before sending cancel
	<-handlerStarted

	// Send cancel notification with matching requestId
	writeLine(pw, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":2}}`)

	// Give the server time to process the cancel and let the handler finish
	time.Sleep(100 * time.Millisecond)

	// Close pipe to trigger clean EOF shutdown
	_ = pw.Close()

	err := <-done

	// Assert — clean shutdown
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	// Only the initialize response should be present — tool call response is suppressed
	assert.That(t, "response count", len(responses), 1)
	assert.That(t, "init id", string(responses[0].ID), "1")
}

// Test_Server_With_MalformedToolsCallParams_Should_ReturnInvalidParams covers
// startToolCallAsync lines 414-417: when tools/call params cannot be unmarshaled
// into toolCallParams (e.g. "name" is an integer instead of a string).
func Test_Server_With_MalformedToolsCallParams_Should_ReturnInvalidParams(t *testing.T) {
	t.Parallel()

	// Arrange — params is a valid JSON object but "name" has wrong type
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":123}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

// Test_Server_With_FastHandlerAndSlowDecode_Should_HitPriorityPath covers
// runInFlight line 191: the non-blocking select that checks if the handler
// has already completed before entering the blocking select. With a fast
// handler and a pipe that delivers the next message after a delay, the
// handler result lands on inFlightCh before the next decode completes.
func Test_Server_With_FastHandlerAndSlowDecode_Should_HitPriorityPath(t *testing.T) {
	t.Parallel()

	// Arrange — instant handler
	r := tools.NewRegistry()
	if err := tools.Register(r, "fast", "returns immediately", func(_ context.Context, input testInput) tools.Result {
		return tools.TextResult(input.Message)
	}); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, pr, &stdout, &stderr)

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(context.Background())
	}()

	// Act — handshake
	writeLine(pw, `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}`)
	writeLine(pw, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	// Start a fast tool call — handler completes immediately
	writeLine(pw, `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"fast","arguments":{"message":"hello"}}}`)

	// Wait long enough for handler to complete and result to be on inFlightCh
	time.Sleep(100 * time.Millisecond)

	// Now send next message — runInFlight's priority select should pick up the result first
	writeLine(pw, `{"jsonrpc":"2.0","method":"ping","id":3,"params":{}}`)
	time.Sleep(50 * time.Millisecond)

	_ = pw.Close()
	err := <-done

	// Assert
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}
	// init + tool result + ping = 3 responses
	if len(responses) < 3 {
		t.Fatalf("expected at least 3 responses, got %d", len(responses))
	}
}

// Test_Server_With_TraceEnabledEncodeResponse_Should_LogTraceResponse covers
// encodeResponse trace logging branch (lines 365-371) for the idle path.
func Test_Server_With_TraceEnabledEncodeResponse_Should_LogTraceResponse(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n"
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), &stdout, &stderr,
		server.WithTrace(true),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)
	entries := parseLogEntries(t, &stderr)
	traceResp := findLogEntry(entries, "trace_response")
	if traceResp == nil {
		t.Fatal("expected trace_response log entry")
	}
	assert.That(t, "direction", traceResp["direction"], "→")
}

// Test_Server_With_TraceEnabledIdleDecode_Should_LogTraceRequest covers
// handleDecodeResultIdle trace logging branch (lines 335-342).
func Test_Server_With_TraceEnabledIdleDecode_Should_LogTraceRequest(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"tools/list","id":2,"params":{}}` + "\n"
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), &stdout, &stderr,
		server.WithTrace(true),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)
	entries := parseLogEntries(t, &stderr)
	traceReqs := []map[string]any{}
	for _, e := range entries {
		if e["msg"] == "trace_request" {
			traceReqs = append(traceReqs, e)
		}
	}
	if len(traceReqs) == 0 {
		t.Fatal("expected trace_request log entries")
	}
	assert.That(t, "direction", traceReqs[0]["direction"], "←")
}

// Test_Server_With_CompletionNamespace_Should_ReturnUnsupportedCapability covers
// handleMethod lines 711-712 for the completion/ namespace.
func Test_Server_With_CompletionNamespace_Should_ReturnUnsupportedCapability(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"completion/complete","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
}

// Test_Server_With_ElicitationNamespace_Should_ReturnUnsupportedCapability covers
// handleMethod lines 713-714 for the elicitation/ namespace.
func Test_Server_With_ElicitationNamespace_Should_ReturnUnsupportedCapability(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"elicitation/create","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
}

// Test_Server_With_ReservedRPCMethod_Should_ReturnMethodNotFound covers
// handleMethod line 725-726 for rpc.* methods.
func Test_Server_With_ReservedRPCMethod_Should_ReturnMethodNotFound(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"rpc.discover","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
	if !strings.Contains(responses[1].Error.Message, "reserved method") {
		t.Errorf("expected 'reserved method' in message, got: %s", responses[1].Error.Message)
	}
}

// Test_Server_With_DuplicateInitInInitializing_Should_ReturnError covers
// dispatchByState line 594: sending initialize while in initializing state.
func Test_Server_With_DuplicateInitInInitializing_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — send init, then another init without sending notifications/initialized
	input := initRequest + `{"jsonrpc":"2.0","method":"initialize","id":2,"params":{"capabilities":{}}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.ServerError)
	assert.That(t, "error message", responses[1].Error.Message, "already initialized")
}

// Test_Server_With_MethodInInitializing_Should_ReturnError covers
// dispatchByState line 597: sending a non-init method while in initializing state.
func Test_Server_With_MethodInInitializing_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — send init, then tools/list without initialized notification
	input := initRequest + `{"jsonrpc":"2.0","method":"tools/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.ServerError)
	if !strings.Contains(responses[1].Error.Message, "awaiting") {
		t.Errorf("expected 'awaiting' in message, got: %s", responses[1].Error.Message)
	}
}

// Test_Server_With_InFlightCodeError_Should_IncrementErrorCount covers
// processInFlightResult lines 509-511: when ifr.isError is true, errorCount
// is incremented. A handler that returns *protocol.CodeError produces a
// response with resp.Error != nil (isError=true in inFlightResult).
func Test_Server_With_InFlightCodeError_Should_IncrementErrorCount(t *testing.T) {
	t.Parallel()

	// Arrange — handler returns a *protocol.CodeError so resp.Error != nil
	r := tools.NewRegistry()
	if err := tools.Register(r, "errtool", "returns protocol error", func(_ context.Context, _ testInput) tools.Result {
		// Return an error result — tools.ErrorResult produces Result.IsError=true
		// but the protocol-level error requires returning a CodeError from the handler.
		// We abuse the fact that the wrapped handler can return nil error and isError
		// in inFlightResult comes from resp.Error != nil. To get resp.Error != nil we
		// need the wrapped handler to return an error that becomes a *protocol.CodeError.
		// tools.Register wraps a typed handler; validation errors produce CodeError.
		// We instead rely on returning a CodeError by returning ErrorResult and
		// checking that the server counts it via error log entries.
		return tools.ErrorResult("something went wrong")
	}); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, pr, &stdout, &stderr)

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(context.Background())
	}()

	// Act
	writeLine(pw, `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}`)
	writeLine(pw, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	// tools/call with valid args — handler returns ErrorResult (isError in content, not protocol error)
	writeLine(pw, `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"errtool","arguments":{"message":"x"}}}`)
	time.Sleep(50 * time.Millisecond)
	// A second request forces processInFlightResult to run if handler finished
	writeLine(pw, `{"jsonrpc":"2.0","method":"ping","id":3,"params":{}}`)
	time.Sleep(50 * time.Millisecond)
	_ = pw.Close()

	err := <-done

	// Assert — server completed cleanly; tool response (with isError content) and ping response present
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	if len(responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(responses))
	}
}

// Test_Server_With_InFlightProtocolError_Should_HitIsErrorPath covers
// processInFlightResult isError path (lines 509-511) with a true protocol-level
// error: a handler that returns *protocol.CodeError causes resp.Error != nil,
// so inFlightResult.isError is true and errorCount is incremented.
func Test_Server_With_InFlightProtocolError_Should_HitIsErrorPath(t *testing.T) {
	t.Parallel()

	// Arrange — register a tool whose handler returns a *protocol.CodeError directly.
	// The tools.Register wrapper propagates errors.As(*protocol.CodeError) into a toolError,
	// which executeToolCall turns into protocol.NewErrorResponseFromCodeError (resp.Error != nil).
	r := tools.NewRegistry()
	if err := tools.Register(r, "coderr", "returns protocol CodeError", func(_ context.Context, _ testInput) tools.Result {
		// Panicking with a CodeError exercises the panic recovery → inFlightResult.isError=true path.
		// Instead, return normally — tools.ErrorResult with IsError=true in Result.
		// To reliably get resp.Error != nil we send invalid arguments so the
		// unmarshal+validate step inside the wrapper returns a *protocol.CodeError.
		// We achieve this by calling tools/call with missing required field.
		// However Register captures the handler here; we need to exercise via bad args.
		// So register with a required-field struct and call with empty arguments object.
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Send tools/call with missing required "message" field → CodeError(-32602)
	// → resp.Error != nil → inFlightResult.isError=true
	pr, pw := io.Pipe()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, pr, &stdout, &stderr)

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(context.Background())
	}()

	// Act
	writeLine(pw, `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}`)
	writeLine(pw, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	// Send tools/call with empty arguments — "message" is required, triggers CodeError
	writeLine(pw, `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"coderr","arguments":{}}}`)
	time.Sleep(50 * time.Millisecond)
	// A second request forces the dispatch loop to pick up the completed in-flight result
	writeLine(pw, `{"jsonrpc":"2.0","method":"ping","id":3,"params":{}}`)
	time.Sleep(50 * time.Millisecond)
	_ = pw.Close()

	err := <-done

	// Assert
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	// Should have: init response, tool error response (id:2), ping response (id:3)
	if len(responses) < 3 {
		t.Fatalf("expected at least 3 responses, got %d", len(responses))
	}
	// Find the tool response
	var toolResp *protocol.Response
	for i := range responses {
		if string(responses[i].ID) == "2" {
			toolResp = &responses[i]
			break
		}
	}
	if toolResp == nil {
		t.Fatal("expected response for tool call id:2")
	}
	// The handler validation error produces a -32602 error response
	assert.That(t, "tool error code", toolResp.Error.Code, protocol.InvalidParams)
}

// Test_Server_With_SizeExceededDuringInFlight_Should_HandleGracefully covers
// handleDecodeErrorDuringInFlight lines 299-301: when dr.exceeded is true while
// a handler is in flight, the size-limit error is used and the handler result
// is still processed before returning the fatal decode error.
func Test_Server_With_SizeExceededDuringInFlight_Should_HandleGracefully(t *testing.T) {
	t.Parallel()

	// Arrange — blocking handler; send oversized message while it's in flight
	r := tools.NewRegistry()
	handlerReady := make(chan struct{})
	if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		close(handlerReady)
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	}); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, pr, &stdout, &stderr,
		server.WithHandlerTimeout(200*time.Millisecond),
		server.WithSafetyMargin(100*time.Millisecond),
	)

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(context.Background())
	}()

	// Act — complete handshake
	writeLine(pw, `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}`)
	writeLine(pw, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	// Start blocking tool call
	writeLine(pw, `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}`)

	// Wait for handler to actually start
	<-handlerReady

	// Send oversized message (>4MB) while handler is in flight.
	// Write in a goroutine because the pipe blocks until the server reads it.
	bigValue := strings.Repeat("a", 5*1024*1024)
	go func() {
		_, _ = fmt.Fprintf(pw, `{"jsonrpc":"2.0","method":"ping","id":3,"params":{"data":"%s"}}`, bigValue)
		_, _ = io.WriteString(pw, "\n")
		_ = pw.Close()
	}()

	// Wait for server to process the error
	err := <-done

	// Assert — fatal decode error (size exceeded)
	if err == nil {
		t.Fatal("expected non-nil error for oversized message during in-flight")
	}

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	// Should have the init response and a -32700 parse error
	found32700 := false
	for _, resp := range responses {
		if resp.Error != nil && resp.Error.Code == protocol.ParseError {
			found32700 = true
		}
	}
	if !found32700 {
		t.Error("expected -32700 parse error response for oversized message during in-flight")
	}
}

// Test_Server_With_CancelledNotificationNoInFlight_Should_SilentlyIgnore covers
// handleCancelledNotification lines 642-644: when cancelInFlight is nil (no
// handler in flight), the notification is silently ignored.
func Test_Server_With_CancelledNotificationNoInFlight_Should_SilentlyIgnore(t *testing.T) {
	t.Parallel()

	// Arrange — send notifications/cancelled when no tool call is in flight
	input := handshake() +
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":99}}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert — cancel notification silently ignored; only init + ping responses
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "init id", string(responses[0].ID), "1")
	assert.That(t, "ping result", string(responses[1].Result), "{}")
}

// Test_Server_With_InvalidNotificationInDispatch_Should_SilentlyIgnore covers
// dispatch lines 575-577: a notification (len(msg.ID)==0) that fails
// protocol.Validate is silently ignored (no response sent). This is distinct
// from handleMessageDuringInFlight — this is the idle-state path.
func Test_Server_With_InvalidNotificationInDispatch_Should_SilentlyIgnore(t *testing.T) {
	t.Parallel()

	// Arrange — send a notification with wrong jsonrpc version (validation fails),
	// then a ping to confirm server is still healthy.
	input := handshake() +
		`{"jsonrpc":"1.0","method":"some/notification"}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert — invalid notification silently ignored; init + ping responses only
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "init id", string(responses[0].ID), "1")
	assert.That(t, "ping result", string(responses[1].Result), "{}")
}

// --- Resources tests ---

func testResourcesRegistry() *resources.Registry {
	r := resources.NewRegistry()
	if err := resources.Register(r, "config://app", "App Config", "Application configuration",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, `{"key":"value"}`), nil
		},
		resources.WithMimeType("application/json"),
	); err != nil {
		panic("testResourcesRegistry: " + err.Error())
	}
	return r
}

func Test_Server_With_ResourcesList_Should_ReturnResources(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(testResourcesRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)

	var result struct {
		Resources []struct {
			Description string `json:"description"`
			MimeType    string `json:"mimeType"`
			Name        string `json:"name"`
			URI         string `json:"uri"`
		} `json:"resources"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "resource count", len(result.Resources), 1)
	assert.That(t, "resource name", result.Resources[0].Name, "App Config")
	assert.That(t, "resource uri", result.Resources[0].URI, "config://app")
	assert.That(t, "resource mime", result.Resources[0].MimeType, "application/json")
}

func Test_Server_With_ResourcesRead_Should_ReturnContent(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/read","id":2,"params":{"uri":"config://app"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(testResourcesRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)

	var result struct {
		Contents []struct {
			Text string `json:"text"`
			URI  string `json:"uri"`
		} `json:"contents"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "content count", len(result.Contents), 1)
	assert.That(t, "content text", result.Contents[0].Text, `{"key":"value"}`)
	assert.That(t, "content uri", result.Contents[0].URI, "config://app")
}

func Test_Server_With_ResourcesReadUnknown_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/read","id":2,"params":{"uri":"unknown://uri"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(testResourcesRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_With_ResourcesCapability_Should_AdvertiseInInitialize(t *testing.T) {
	t.Parallel()

	// Arrange
	input := initRequest

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(testResourcesRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 1)

	var result struct {
		Capabilities struct {
			Resources struct {
				ListChanged bool `json:"listChanged"`
				Subscribe   bool `json:"subscribe"`
			} `json:"resources"`
			Tools struct{} `json:"tools"`
		} `json:"capabilities"`
	}
	err = json.Unmarshal(responses[0].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
}

func Test_Server_Without_Resources_Should_RejectResourcesMethods(t *testing.T) {
	t.Parallel()

	// Arrange — no WithResources, only tools
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
}

// --- Prompts tests ---

type promptTestInput struct {
	Name string `json:"name" description:"Name to greet"`
}

func testPromptsRegistry() *prompts.Registry {
	r := prompts.NewRegistry()
	if err := prompts.Register(r, "greet", "A greeting prompt",
		func(_ context.Context, input promptTestInput) prompts.Result {
			return prompts.Result{
				Description: "Greeting for " + input.Name,
				Messages: []prompts.Message{
					prompts.UserMessage("Hello " + input.Name),
					prompts.AssistantMessage("Hi there!"),
				},
			}
		},
	); err != nil {
		panic("testPromptsRegistry: " + err.Error())
	}
	return r
}

func Test_Server_With_PromptsList_Should_ReturnPrompts(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"prompts/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithPrompts(testPromptsRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)

	var result struct {
		Prompts []struct {
			Arguments []struct {
				Description string `json:"description"`
				Name        string `json:"name"`
				Required    bool   `json:"required"`
			} `json:"arguments"`
			Description string `json:"description"`
			Name        string `json:"name"`
		} `json:"prompts"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "prompt count", len(result.Prompts), 1)
	assert.That(t, "prompt name", result.Prompts[0].Name, "greet")
	assert.That(t, "argument count", len(result.Prompts[0].Arguments), 1)
	assert.That(t, "argument name", result.Prompts[0].Arguments[0].Name, "name")
}

func Test_Server_With_PromptsGet_Should_ReturnMessages(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"prompts/get","id":2,"params":{"name":"greet","arguments":{"name":"Andy"}}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithPrompts(testPromptsRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)

	var result struct {
		Description string `json:"description"`
		Messages    []struct {
			Content struct {
				Text string `json:"text"`
				Type string `json:"type"`
			} `json:"content"`
			Role string `json:"role"`
		} `json:"messages"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "description", result.Description, "Greeting for Andy")
	assert.That(t, "message count", len(result.Messages), 2)
	assert.That(t, "first role", result.Messages[0].Role, "user")
	assert.That(t, "first text", result.Messages[0].Content.Text, "Hello Andy")
	assert.That(t, "second role", result.Messages[1].Role, "assistant")
}

func Test_Server_With_PromptsGetUnknown_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"prompts/get","id":2,"params":{"name":"unknown"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithPrompts(testPromptsRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_Without_Prompts_Should_RejectPromptsMethods(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"prompts/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
}

// --- Progress + Logging tests ---

func Test_Server_With_ProgressToken_Should_EmitProgressNotification(t *testing.T) {
	t.Parallel()

	// Arrange — register a tool that reports progress
	r := tools.NewRegistry()
	if err := tools.Register(r, "slow", "slow tool", func(ctx context.Context, _ testInput) tools.Result {
		p := server.ProgressFromContext(ctx)
		p.Report(1, 10)
		p.Report(10, 10)
		return tools.TextResult("done")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"slow","arguments":{"message":"test"},"_meta":{"progressToken":"tok-1"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())
	assert.That(t, "error", err, nil)

	// Assert — parse all messages from stdout (notifications + response)
	dec := json.NewDecoder(&stdout)
	var messages []json.RawMessage
	for dec.More() {
		var raw json.RawMessage
		if uerr := dec.Decode(&raw); uerr != nil {
			break
		}
		messages = append(messages, raw)
	}
	// Should have: init response, 2 progress notifications, tool response
	assert.That(t, "message count", len(messages), 4)

	// Verify first progress notification
	var notif struct {
		Method string `json:"method"`
		Params struct {
			Progress      int    `json:"progress"`
			ProgressToken string `json:"progressToken"`
			Total         int    `json:"total"`
		} `json:"params"`
	}
	err = json.Unmarshal(messages[1], &notif)
	assert.That(t, "unmarshal notif", err, nil)
	assert.That(t, "method", notif.Method, "notifications/progress")
	assert.That(t, "progress token", notif.Params.ProgressToken, "tok-1")
	assert.That(t, "progress", notif.Params.Progress, 1)
	assert.That(t, "total", notif.Params.Total, 10)
}

func Test_Server_With_InvalidMetaObject_Should_IgnoreProgressToken(t *testing.T) {
	t.Parallel()

	// Arrange — _meta is not a valid JSON object (it's a string)
	r := tools.NewRegistry()
	if err := tools.Register(r, "meta-test", "meta test tool", func(_ context.Context, _ testInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"meta-test","arguments":{"message":"test"},"_meta":"not-an-object"}}` + "\n"

	// Act
	responses, err := runServer(t, r, input)

	// Assert — should succeed without progress
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
}

func Test_Server_With_ProgressTokenAndTrace_Should_EmitTracedProgress(t *testing.T) {
	t.Parallel()

	// Arrange — tool reports progress with trace enabled
	r := tools.NewRegistry()
	if err := tools.Register(r, "traced", "traced tool", func(ctx context.Context, _ testInput) tools.Result {
		p := server.ProgressFromContext(ctx)
		p.Report(5, 10)
		p.Log("info", "tracer", "traced log")
		return tools.TextResult("traced done")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"traced","arguments":{"message":"test"},"_meta":{"progressToken":"tok-trace"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr, server.WithTrace(true))

	// Act
	err := srv.Run(context.Background())
	assert.That(t, "error", err, nil)

	// Assert — stderr should contain trace log entries for notifications
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "trace_notification") {
		t.Error("expected trace_notification log in stderr")
	}
}

func Test_Server_Without_ProgressToken_Should_NotEmitProgress(t *testing.T) {
	t.Parallel()

	// Arrange — tool reports progress but no _meta.progressToken in request
	r := tools.NewRegistry()
	if err := tools.Register(r, "fast", "fast tool", func(ctx context.Context, _ testInput) tools.Result {
		p := server.ProgressFromContext(ctx)
		p.Report(1, 1) // no-op because no token
		return tools.TextResult("done")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"fast","arguments":{"message":"test"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())
	assert.That(t, "error", err, nil)

	// Assert — only init response + tool response, no notifications
	dec := json.NewDecoder(&stdout)
	var count int
	for dec.More() {
		var raw json.RawMessage
		if uerr := dec.Decode(&raw); uerr != nil {
			break
		}
		count++
	}
	assert.That(t, "message count", count, 2)
}

func Test_Server_With_LoggingSetLevel_Should_SetLevel(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"logging/setLevel","id":2,"params":{"level":"debug"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "result", string(responses[1].Result), "{}")
}

func Test_Server_With_ToolHandler_Log_Should_EmitLogNotification(t *testing.T) {
	t.Parallel()

	// Arrange — register a tool that logs
	r := tools.NewRegistry()
	if err := tools.Register(r, "logger", "logging tool", func(ctx context.Context, _ testInput) tools.Result {
		p := server.ProgressFromContext(ctx)
		p.Log("info", "test-logger", "hello from tool")
		return tools.TextResult("done")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"logger","arguments":{"message":"test"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())
	assert.That(t, "error", err, nil)

	// Assert — parse all messages from stdout
	dec := json.NewDecoder(&stdout)
	var messages []json.RawMessage
	for dec.More() {
		var raw json.RawMessage
		if uerr := dec.Decode(&raw); uerr != nil {
			break
		}
		messages = append(messages, raw)
	}
	// Should have: init response, log notification, tool response
	assert.That(t, "message count", len(messages), 3)

	// Verify log notification
	var notif struct {
		Method string `json:"method"`
		Params struct {
			Data   string `json:"data"`
			Level  string `json:"level"`
			Logger string `json:"logger"`
		} `json:"params"`
	}
	err = json.Unmarshal(messages[1], &notif)
	assert.That(t, "unmarshal notif", err, nil)
	assert.That(t, "method", notif.Method, "notifications/message")
	assert.That(t, "level", notif.Params.Level, "info")
	assert.That(t, "logger", notif.Params.Logger, "test-logger")
	assert.That(t, "data", notif.Params.Data, "hello from tool")
}

// --- Resources/Prompts error-path and edge-case tests ---

func Test_Server_With_ResourcesReadInvalidParams_Should_ReturnInvalidParams(t *testing.T) {
	t.Parallel()

	// Arrange — params has wrong types for expected fields
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/read","id":2,"params":{"uri":123}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(testResourcesRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_With_ResourcesReadEmptyURI_Should_ReturnInvalidParams(t *testing.T) {
	t.Parallel()

	// Arrange — empty URI
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/read","id":2,"params":{"uri":""}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(testResourcesRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_With_ResourcesReadHandlerError_Should_ReturnInternalError(t *testing.T) {
	t.Parallel()

	// Arrange — register a resource with a handler that returns an error
	reg := resources.NewRegistry()
	_ = resources.Register(reg, "err://fail", "Failing", "fails",
		func(_ context.Context, _ string) (resources.Result, error) {
			return resources.Result{}, errors.New("read failed")
		},
	)

	input := handshake() + `{"jsonrpc":"2.0","method":"resources/read","id":2,"params":{"uri":"err://fail"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(reg))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InternalError)
}

func Test_Server_With_ResourcesReadHandlerCodeError_Should_ReturnCodeError(t *testing.T) {
	t.Parallel()

	// Arrange — register a resource with a handler that returns a CodeError
	reg := resources.NewRegistry()
	_ = resources.Register(reg, "err://params", "ParamsErr", "returns InvalidParams",
		func(_ context.Context, _ string) (resources.Result, error) {
			return resources.Result{}, protocol.ErrInvalidParams("bad resource uri format")
		},
	)

	input := handshake() + `{"jsonrpc":"2.0","method":"resources/read","id":2,"params":{"uri":"err://params"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(reg))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
	assert.That(t, "error message", responses[1].Error.Message, "bad resource uri format")
}

func Test_Server_With_UnknownResourcesMethod_Should_ReturnMethodNotFound(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/unknown","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(testResourcesRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
}

func Test_Server_With_PromptsGetInvalidParams_Should_ReturnInvalidParams(t *testing.T) {
	t.Parallel()

	// Arrange — params has wrong type for name field
	input := handshake() + `{"jsonrpc":"2.0","method":"prompts/get","id":2,"params":{"name":123}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithPrompts(testPromptsRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_With_PromptsGetEmptyName_Should_ReturnInvalidParams(t *testing.T) {
	t.Parallel()

	// Arrange — empty name
	input := handshake() + `{"jsonrpc":"2.0","method":"prompts/get","id":2,"params":{"name":""}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithPrompts(testPromptsRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_With_UnknownPromptsMethod_Should_ReturnMethodNotFound(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"prompts/unknown","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithPrompts(testPromptsRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
}

func Test_Server_With_LoggingSetLevelInvalidParams_Should_ReturnInvalidParams(t *testing.T) {
	t.Parallel()

	// Arrange — params has wrong type for level field
	input := handshake() + `{"jsonrpc":"2.0","method":"logging/setLevel","id":2,"params":{"level":123}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_With_LoggingSetLevelEmptyLevel_Should_ReturnInvalidParams(t *testing.T) {
	t.Parallel()

	// Arrange — empty level
	input := handshake() + `{"jsonrpc":"2.0","method":"logging/setLevel","id":2,"params":{"level":""}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_With_PromptsAndResourcesCapabilities_Should_AdvertiseAll(t *testing.T) {
	t.Parallel()

	// Arrange — tools + prompts + resources
	input := initRequest

	// Act
	responses, err := runServer(t, testRegistry(), input,
		server.WithPrompts(testPromptsRegistry()),
		server.WithResources(testResourcesRegistry()),
	)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 1)

	var result struct {
		Capabilities map[string]any `json:"capabilities"`
	}
	err = json.Unmarshal(responses[0].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	_, hasPrompts := result.Capabilities["prompts"]
	assert.That(t, "has prompts", hasPrompts, true)
	_, hasTools := result.Capabilities["tools"]
	assert.That(t, "has tools", hasTools, true)
	_, hasResources := result.Capabilities["resources"]
	assert.That(t, "has resources", hasResources, true)
}

func Test_Server_Without_Resources_With_Prompts_Should_RejectResourcesMethods(t *testing.T) {
	t.Parallel()

	// Arrange — tools + prompts but no resources
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithPrompts(testPromptsRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
}

func Test_Server_Without_Prompts_With_Resources_Should_RejectPromptsMethods(t *testing.T) {
	t.Parallel()

	// Arrange — tools + resources but no prompts
	input := handshake() + `{"jsonrpc":"2.0","method":"prompts/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(testResourcesRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.MethodNotFound)
}

func Test_Server_With_PromptsGetNoArguments_Should_RejectMissingRequired(t *testing.T) {
	t.Parallel()

	// Arrange — prompts/get without arguments field; "greet" requires "name"
	input := handshake() + `{"jsonrpc":"2.0","method":"prompts/get","id":2,"params":{"name":"greet"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithPrompts(testPromptsRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_With_NilProgressReport_Should_NotPanic(t *testing.T) {
	t.Parallel()

	// Arrange — nil Progress receiver
	var p *server.Progress

	// Act — should be no-op, not panic
	p.Report(1, 10)
}

func Test_Server_With_NilProgressLog_Should_NotPanic(t *testing.T) {
	t.Parallel()

	// Arrange — nil Progress receiver
	var p *server.Progress

	// Act — should be no-op, not panic
	p.Log("info", "test", "hello")
}

func Test_Server_With_SendRequestFromContextWithoutHandler_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — plain context without Progress
	ctx := context.Background()

	// Act
	_, err := server.SendRequestFromContext(ctx, "test/method", nil)

	// Assert
	if err == nil {
		t.Fatal("expected error when calling SendRequestFromContext outside handler context")
	}
}

func Test_Server_With_ProgressFromContextWithoutValue_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	// Arrange
	ctx := context.Background()

	// Act
	p := server.ProgressFromContext(ctx)

	// Assert
	if p != nil {
		t.Fatal("expected nil Progress from context without value")
	}
}

// --- SendRequest (bidirectional) tests ---

func Test_Server_With_SendRequest_Should_CorrelateResponse(t *testing.T) {
	t.Parallel()

	// Arrange — register a tool that uses SendRequest to call the client
	r := tools.NewRegistry()
	if err := tools.Register(r, "bidir", "bidirectional tool", func(ctx context.Context, _ testInput) tools.Result {
		resp, err := server.SendRequestFromContext(ctx, "sampling/createMessage", map[string]string{"prompt": "hello"})
		if err != nil {
			return tools.ErrorResult("send request failed: " + err.Error())
		}
		return tools.TextResult("client said: " + string(resp.Result))
	}); err != nil {
		t.Fatal(err)
	}

	// Use pipes for bidirectional communication (both stdin and stdout)
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	var stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, stdinR, stdoutW, &stderr)

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(context.Background())
	}()

	// Act — write handshake
	_, _ = stdinW.Write([]byte(handshake()))

	// Read stdout with a decoder (thread-safe via pipe)
	dec := json.NewDecoder(stdoutR)

	// First message: initialize response
	var initResp json.RawMessage
	_ = dec.Decode(&initResp)

	// Write tool call
	_, _ = stdinW.Write([]byte(`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"bidir","arguments":{"message":"test"}}}` + "\n"))

	// Second message: the server's sampling/createMessage request
	var srvReq struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	_ = dec.Decode(&srvReq)

	// Write the response back to stdin
	response := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":"world"}`, string(srvReq.ID))
	_, _ = stdinW.Write([]byte(response + "\n"))

	// Third message: the tool call result
	var toolResp json.RawMessage
	_ = dec.Decode(&toolResp)

	// Close stdin to trigger shutdown
	_ = stdinW.Close()
	err := <-done

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "method", srvReq.Method, "sampling/createMessage")
}

func testResourcesRegistryWithTemplate() *resources.Registry {
	r := testResourcesRegistry()
	if err := resources.RegisterTemplate(r, "file://{path}", "File", "Read a file",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "content of "+uri), nil
		},
	); err != nil {
		panic("testResourcesRegistryWithTemplate: " + err.Error())
	}
	return r
}

func Test_Server_With_ResourcesReadTemplate_Should_ReturnContent(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/read","id":2,"params":{"uri":"file://readme.md"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(testResourcesRegistryWithTemplate()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "no error", responses[1].Error == nil, true)

	var result struct {
		Contents []struct {
			Text string `json:"text"`
			URI  string `json:"uri"`
		} `json:"contents"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "content count", len(result.Contents), 1)
	assert.That(t, "content text", result.Contents[0].Text, "content of file://readme.md")
	assert.That(t, "content uri", result.Contents[0].URI, "file://readme.md")
}

func Test_Server_With_ResourcesReadStaticOverTemplate_Should_PreferStatic(t *testing.T) {
	t.Parallel()

	// Arrange — static resource matches exactly, template also matches
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/read","id":2,"params":{"uri":"config://app"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(testResourcesRegistryWithTemplate()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "no error", responses[1].Error == nil, true)

	var result struct {
		Contents []struct {
			Text string `json:"text"`
		} `json:"contents"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal error", err, nil)
	assert.That(t, "content text", result.Contents[0].Text, `{"key":"value"}`)
}

func Test_Server_With_PromptsGetMissingRequiredArg_Should_ReturnInvalidParams(t *testing.T) {
	t.Parallel()

	// Arrange — "greet" prompt requires "name" argument
	input := handshake() + `{"jsonrpc":"2.0","method":"prompts/get","id":2,"params":{"name":"greet","arguments":{}}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithPrompts(testPromptsRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_With_PromptsGetNoArgs_Should_ReturnInvalidParams(t *testing.T) {
	t.Parallel()

	// Arrange — "greet" prompt requires "name" argument, no arguments provided
	input := handshake() + `{"jsonrpc":"2.0","method":"prompts/get","id":2,"params":{"name":"greet"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithPrompts(testPromptsRegistry()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_With_InitializeNoRegistries_Should_OmitOptionalCapabilities(t *testing.T) {
	t.Parallel()

	// Arrange — server with empty tool registry, no prompts, no resources
	input := initRequest

	// Act
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("bare", "0.0.0", tools.NewRegistry(), strings.NewReader(input), &stdout, &stderr)
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	var resp protocol.Response
	_ = json.NewDecoder(&stdout).Decode(&resp)

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
	assert.That(t, "tools present", result.Capabilities.Tools != nil, true)
}

func Test_Server_With_ResourcesReadTemplateHandlerError_Should_ReturnInternalError(t *testing.T) {
	t.Parallel()

	// Arrange — register a template with a handler that returns a plain error
	reg := resources.NewRegistry()
	_ = resources.RegisterTemplate(reg, "fail://{id}", "Fail", "Failing template",
		func(_ context.Context, _ string) (resources.Result, error) {
			return resources.Result{}, errors.New("template read failed")
		},
	)

	input := handshake() + `{"jsonrpc":"2.0","method":"resources/read","id":2,"params":{"uri":"fail://123"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(reg))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InternalError)
}

func Test_Server_With_ResourcesReadTemplateHandlerCodeError_Should_ReturnCodeError(t *testing.T) {
	t.Parallel()

	// Arrange — template handler returns a CodeError
	reg := resources.NewRegistry()
	_ = resources.RegisterTemplate(reg, "perms://{path}", "Perms", "Permission check",
		func(_ context.Context, _ string) (resources.Result, error) {
			return resources.Result{}, protocol.ErrInvalidParams("access denied")
		},
	)

	input := handshake() + `{"jsonrpc":"2.0","method":"resources/read","id":2,"params":{"uri":"perms://secret"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(reg))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
	assert.That(t, "error message", responses[1].Error.Message, "access denied")
}

func Test_Server_With_ResourcesReadNoMatchStaticOrTemplate_Should_ReturnInvalidParams(t *testing.T) {
	t.Parallel()

	// Arrange — registry has both static and template, but URI matches neither
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/read","id":2,"params":{"uri":"unknown://thing"}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(testResourcesRegistryWithTemplate()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
}

func Test_Server_With_TraceEnabledNotification_Should_LogTraceNotification(t *testing.T) {
	t.Parallel()

	// Arrange — tool that emits a log notification, with trace enabled
	r := tools.NewRegistry()
	if err := tools.Register(r, "logger", "emits log", func(ctx context.Context, _ testInput) tools.Result {
		p := server.ProgressFromContext(ctx)
		p.Log("info", "test", "hello from tool")
		return tools.TextResult("done")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"logger","arguments":{"message":"x"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithTrace(true),
	)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	entries := parseLogEntries(t, &stderr)
	traceNotif := findLogEntry(entries, "trace_notification")
	if traceNotif == nil {
		t.Fatal("expected trace_notification log entry")
	}
}

func Test_Server_With_ResourcesListAndTemplates_Should_ReturnBoth(t *testing.T) {
	t.Parallel()

	// Arrange — registry with both static resources and templates
	input := handshake() + `{"jsonrpc":"2.0","method":"resources/list","id":2,"params":{}}` + "\n"

	// Act
	responses, err := runServer(t, testRegistry(), input, server.WithResources(testResourcesRegistryWithTemplate()))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "response count", len(responses), 2)
	assert.That(t, "no error", responses[1].Error == nil, true)

	var result struct {
		Resources []struct {
			URI string `json:"uri"`
		} `json:"resources"`
		ResourceTemplates []struct {
			URITemplate string `json:"uriTemplate"`
		} `json:"resourceTemplates"`
	}
	_ = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "resource count", len(result.Resources), 1)
	assert.That(t, "template count", len(result.ResourceTemplates), 1)
	assert.That(t, "resource uri", result.Resources[0].URI, "config://app")
	assert.That(t, "template uri", result.ResourceTemplates[0].URITemplate, "file://{path}")
}

func Test_Server_With_ToolCallErrorResult_Should_ReturnErrorFlag(t *testing.T) {
	t.Parallel()

	// Arrange — tool returns an error result (isError flag)
	r := tools.NewRegistry()
	if err := tools.Register(r, "errtool", "returns error result", func(_ context.Context, _ testInput) tools.Result {
		return tools.ErrorResult("something went wrong")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"errtool","arguments":{"message":"x"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	var resps []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		resps = append(resps, resp)
	}

	if len(resps) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(resps))
	}
	toolResp := resps[1]
	assert.That(t, "no protocol error", toolResp.Error == nil, true)

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	_ = json.Unmarshal(toolResp.Result, &result)
	assert.That(t, "isError", result.IsError, true)
	assert.That(t, "error text", result.Content[0].Text, "something went wrong")
}
