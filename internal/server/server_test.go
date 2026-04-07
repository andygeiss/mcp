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

	"github.com/andygeiss/mcp/internal/pkg/assert"
	"github.com/andygeiss/mcp/internal/protocol"
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
	tools.Register(r, "test", "test tool", func(_ context.Context, input testInput) tools.Result {
		return tools.TextResult(input.Message)
	})
	return r
}

// helper: run server with input string, return output and error.
func runServer(t *testing.T, registry *tools.Registry, input string) ([]protocol.Response, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", registry, strings.NewReader(input), &stdout, &stderr)
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
	assert.That(t, "protocol version", result.ProtocolVersion, "2025-06-18")
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
	assert.That(t, "error code", responses[0].Error.Code, protocol.InvalidRequest)
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
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidRequest)
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
	assert.That(t, "second error code", responses[1].Error.Code, protocol.InvalidRequest)
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
	tools.Register(r, "zeta", "z tool", func(_ context.Context, _ testInput) tools.Result {
		return tools.TextResult("z")
	})
	tools.Register(r, "alpha", "a tool", func(_ context.Context, _ testInput) tools.Result {
		return tools.TextResult("a")
	})

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
			tools.Register(r, tt.toolName, "panics", func(_ context.Context, _ testInput) tools.Result {
				panic(tt.panicValue)
			})

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
	tools.Register(r, "panicker", "panics", func(_ context.Context, _ testInput) tools.Result {
		panic("test-panic-value")
	})

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
	assert.That(t, "panicValue", data["panicValue"], "test-panic-value")
	if _, hasStack := data["stack"]; hasStack {
		t.Error("Error.Data must not contain stack trace")
	}
}

func Test_Server_With_PanickingHandler_Should_LogStackToStderr(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "panicker", "panics", func(_ context.Context, _ testInput) tools.Result {
		panic("test-panic-value")
	})

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
			stack, ok := entry["stack"].(string)
			if !ok || len(stack) == 0 {
				t.Error("expected non-empty stack trace in stderr log")
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
	tools.Register(r, "failing", "returns error result", func(_ context.Context, _ testInput) tools.Result {
		return tools.ErrorResult("something went wrong")
	})

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
	tools.Register(r, "slow", "takes 100ms", func(ctx context.Context, _ testInput) tools.Result {
		select {
		case <-time.After(100 * time.Millisecond):
			return tools.TextResult("done")
		case <-ctx.Done():
			return tools.ErrorResult("cancelled")
		}
	})

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
	tools.Register(r, "hang", "blocks forever", func(_ context.Context, _ testInput) tools.Result {
		select {} //nolint:gosimple // intentionally block forever to test timeout
	})

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"hang","arguments":{"message":"test"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(50*time.Millisecond),
		server.WithSafetyMargin(50*time.Millisecond),
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
	assert.That(t, "error code", responses[1].Error.Code, protocol.InternalError)
	if !strings.Contains(responses[1].Error.Message, "hang") {
		t.Errorf("expected tool name in error message, got: %s", responses[1].Error.Message)
	}
}

func Test_Server_With_DeadlineExceeded_Should_IncludeTimingInData(t *testing.T) {
	t.Parallel()

	// Arrange — handler that respects context and returns on deadline
	r := tools.NewRegistry()
	tools.Register(r, "slow", "blocks until timeout", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult(ctx.Err().Error())
	})

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"slow","arguments":{"message":"x"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(50*time.Millisecond),
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
	assert.That(t, "error code", responses[1].Error.Code, protocol.InternalError)
	if !strings.Contains(responses[1].Error.Message, "slow") {
		t.Errorf("expected tool name in message, got: %s", responses[1].Error.Message)
	}

	var data map[string]any
	if err := json.Unmarshal(responses[1].Error.Data, &data); err != nil {
		t.Fatalf("failed to unmarshal error data: %v", err)
	}
	assert.That(t, "toolName", data["toolName"], "slow")
	if elapsed, ok := data["elapsedMs"].(float64); !ok || elapsed < 50 {
		t.Errorf("expected elapsedMs >= 50, got %v", data["elapsedMs"])
	}
	if timeout, ok := data["timeoutMs"].(float64); !ok || int64(timeout) != 50 {
		t.Errorf("expected timeoutMs == 50, got %v", data["timeoutMs"])
	}
}

func Test_Server_With_ContextCanceled_Should_IncludeElapsedOnly(t *testing.T) {
	t.Parallel()

	// Arrange — handler that blocks until cancelled
	r := tools.NewRegistry()
	tools.Register(r, "blocker", "blocks forever", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult(ctx.Err().Error())
	})

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
	assert.That(t, "error code", responses[1].Error.Code, protocol.InternalError)
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
	tools.Register(r, "hang", "ignores context", func(_ context.Context, _ testInput) tools.Result {
		select {} //nolint:gosimple // intentionally block forever
	})

	input := handshake() + `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"hang","arguments":{"message":"x"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
		server.WithHandlerTimeout(50*time.Millisecond),
		server.WithSafetyMargin(50*time.Millisecond),
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
	assert.That(t, "error code", responses[1].Error.Code, protocol.InternalError)

	var data map[string]any
	if err := json.Unmarshal(responses[1].Error.Data, &data); err != nil {
		t.Fatalf("failed to unmarshal error data: %v", err)
	}
	assert.That(t, "toolName", data["toolName"], "hang")
	if _, hasElapsed := data["elapsedMs"]; !hasElapsed {
		t.Error("expected elapsedMs in data")
	}
	if timeout, ok := data["timeoutMs"].(float64); !ok || int64(timeout) != 50 {
		t.Errorf("expected timeoutMs == 50, got %v", data["timeoutMs"])
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
				assert.That(t, "error code", lastResp.Error.Code, protocol.InvalidRequest)
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
	tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	})

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
	tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		close(cancelled)
		return tools.ErrorResult("cancelled")
	})

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
	tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
		<-ctx.Done()
		return tools.ErrorResult("cancelled")
	})

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
