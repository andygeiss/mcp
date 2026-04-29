//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

// assertNoGoroutineLeaks verifies that goroutine count has not grown beyond
// a tolerance. The async dispatch model spawns decode goroutines and handler
// goroutines that may briefly overlap with parallel tests, so the tolerance
// accounts for both runtime background goroutines and parallel test activity.
func assertNoGoroutineLeaks(t *testing.T, before int) {
	t.Helper()
	// Allow goroutines to settle — decode and handler goroutines may still be
	// cleaning up (deferred cancels, channel sends) immediately after Run returns.
	for range 20 {
		runtime.Gosched()
		time.Sleep(10 * time.Millisecond)
		if runtime.NumGoroutine() <= before+2 {
			return
		}
	}
	after := runtime.NumGoroutine()
	if after > before+2 {
		buf := make([]byte, 64*1024)
		n := runtime.Stack(buf, true)
		t.Errorf("goroutine leak: before=%d, after=%d (tolerance: 2)\n%s", before, after, buf[:n])
	}
}

func Test_Integration_With_FullPipeline_Should_CompleteSuccessfully(t *testing.T) {
	t.Parallel()
	goroutinesBefore := runtime.NumGoroutine()

	// Arrange — full pipeline: initialize -> initialized -> tools/list -> tools/call
	input := `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","method":"tools/list","id":2,"params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"tools/call","id":3,"params":{"name":"test","arguments":{"message":"integration test"}}}` + "\n"

	r := tools.NewRegistry()
	if err := tools.Register(r, "test", "test tool", func(_ context.Context, input testInput) tools.Result {
		return tools.TextResult(input.Message)
	}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "1.0.0", r, strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "run error", err, nil)

	// Parse all responses
	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}

	assert.That(t, "response count", len(responses), 3)
	assert.That(t, "init id", string(responses[0].ID), "1")
	assert.That(t, "list id", string(responses[1].ID), "2")
	assert.That(t, "call id", string(responses[2].ID), "3")

	// Response 1: initialize
	var initResult struct {
		Capabilities struct {
			Tools struct{} `json:"tools"`
		} `json:"capabilities"`
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	err = json.Unmarshal(responses[0].Result, &initResult)
	assert.That(t, "init unmarshal", err, nil)
	assert.That(t, "protocol version", initResult.ProtocolVersion, "2025-11-25")
	assert.That(t, "server name", initResult.ServerInfo.Name, "mcp")
	assert.That(t, "server version", initResult.ServerInfo.Version, "1.0.0")

	// Response 2: tools/list
	var listResult struct {
		Tools []struct {
			Description string `json:"description"`
			InputSchema struct {
				Properties map[string]struct {
					Description string `json:"description"`
					Type        string `json:"type"`
				} `json:"properties"`
				Required []string `json:"required"`
				Type     string   `json:"type"`
			} `json:"inputSchema"`
			Name string `json:"name"`
		} `json:"tools"`
	}
	err = json.Unmarshal(responses[1].Result, &listResult)
	assert.That(t, "list unmarshal", err, nil)
	assert.That(t, "tools count", len(listResult.Tools), 1)
	assert.That(t, "tool name", listResult.Tools[0].Name, "test")
	assert.That(t, "tool description", listResult.Tools[0].Description, "test tool")
	assert.That(t, "schema type", listResult.Tools[0].InputSchema.Type, "object")
	assert.That(t, "message prop type", listResult.Tools[0].InputSchema.Properties["message"].Type, "string")

	// Response 3: tools/call
	var callResult struct {
		Content []struct {
			Text string `json:"text"`
			Type string `json:"type"`
		} `json:"content"`
	}
	err = json.Unmarshal(responses[2].Result, &callResult)
	assert.That(t, "call unmarshal", err, nil)
	assert.That(t, "content count", len(callResult.Content), 1)
	assert.That(t, "call text", callResult.Content[0].Text, "integration test")
	assert.That(t, "call type", callResult.Content[0].Type, "text")

	assertNoGoroutineLeaks(t, goroutinesBefore)
}

func Test_Integration_With_PanickingHandler_Should_RecoverAndContinue(t *testing.T) {
	t.Parallel()

	// Arrange — panic tool followed by test tool proves server survives the panic.
	// Uses io.Pipe to sequence messages: the second tools/call is sent after the
	// first response arrives, matching the maxInFlight:1 protocol contract.
	r := tools.NewRegistry()
	if err := tools.Register(r, "panicker", "panics", func(_ context.Context, _ testInput) tools.Result {
		panic("boom")
	}); err != nil {
		t.Fatal(err)
	}
	if err := tools.Register(r, "test", "test tool", func(_ context.Context, input testInput) tools.Result {
		return tools.TextResult(input.Message)
	}); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, pr, &stdout, &stderr)

	done := make(chan error, 1)
	go func() { done <- srv.Run(context.Background()) }()

	// Send handshake + first tools/call
	_, _ = pw.Write([]byte(handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"panicker","arguments":{"message":"x"}}}` + "\n"))

	// Wait for panic response (brief settle)
	time.Sleep(50 * time.Millisecond)

	// Send second tools/call + close
	_, _ = pw.Write([]byte(`{"jsonrpc":"2.0","method":"tools/call","id":3,"params":{"name":"test","arguments":{"message":"alive"}}}` + "\n"))
	_ = pw.Close()

	// Act
	err := <-done

	// Assert
	assert.That(t, "error", err, nil)

	responses := parseResponses(t, &stdout)
	assert.That(t, "response count", len(responses), 3) // init + panic + test
	assert.That(t, "init id", string(responses[0].ID), "1")
	assert.That(t, "panic id", string(responses[1].ID), "2")
	assert.That(t, "test id", string(responses[2].ID), "3")

	assert.That(t, "panic error code", responses[1].Error.Code, protocol.InternalError)

	var testResult struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	err = json.Unmarshal(responses[2].Result, &testResult)
	assert.That(t, "test unmarshal", err, nil)
	assert.That(t, "test text", testResult.Content[0].Text, "alive")
}

func Test_Integration_With_SlowHandler_Should_TimeoutAndContinue(t *testing.T) {
	t.Parallel()

	// Arrange — uses io.Pipe to sequence messages after the slow handler times out.
	r := tools.NewRegistry()
	if err := tools.Register(r, "slow", "blocks", func(ctx context.Context, _ testInput) tools.Result {
		select {
		case <-time.After(10 * time.Second):
			return tools.TextResult("unreachable")
		case <-ctx.Done():
			return tools.ErrorResult("context cancelled")
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := tools.Register(r, "test", "test tool", func(_ context.Context, input testInput) tools.Result {
		return tools.TextResult(input.Message)
	}); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, pr, &stdout, &stderr, server.WithHandlerTimeout(50*time.Millisecond))

	done := make(chan error, 1)
	go func() { done <- srv.Run(context.Background()) }()

	// Send handshake + slow tools/call
	_, _ = pw.Write([]byte(handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"slow","arguments":{"message":"x"}}}` + "\n"))

	// Wait for timeout to fire (50ms handler timeout + safety margin)
	time.Sleep(200 * time.Millisecond)

	// Send second tools/call + close
	_, _ = pw.Write([]byte(`{"jsonrpc":"2.0","method":"tools/call","id":3,"params":{"name":"test","arguments":{"message":"alive"}}}` + "\n"))
	_ = pw.Close()

	// Act
	err := <-done

	// Assert
	assert.That(t, "error", err, nil)

	responses := parseResponses(t, &stdout)
	assert.That(t, "response count", len(responses), 3) // init + slow + test
	assert.That(t, "init id", string(responses[0].ID), "1")
	assert.That(t, "slow id", string(responses[1].ID), "2")
	assert.That(t, "test id", string(responses[2].ID), "3")

	// Slow tool returns protocol-level error with timing diagnostics
	assert.That(t, "slow error code", responses[1].Error.Code, protocol.ServerTimeout)
	if !strings.Contains(responses[1].Error.Message, "slow") {
		t.Errorf("expected tool name in error message, got: %s", responses[1].Error.Message)
	}

	// Test tool succeeds after the timeout
	var testResult struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	err = json.Unmarshal(responses[2].Result, &testResult)
	assert.That(t, "test unmarshal", err, nil)
	assert.That(t, "test text", testResult.Content[0].Text, "alive")
}

func Test_Integration_With_OversizedMessage_Should_Reject(t *testing.T) {
	t.Parallel()

	// Arrange — 5MB message exceeds 4MB limit
	bigValue := strings.Repeat("a", 5*1024*1024)
	input := `{"jsonrpc":"2.0","method":"ping","id":1,"params":{"data":"` + bigValue + `"}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())

	// Assert — fatal decode error
	if err == nil {
		t.Fatal("expected non-nil error for oversized message")
	}

	responses := parseResponses(t, &stdout)
	assert.That(t, "response count", len(responses), 1)
	assert.That(t, "error id", string(responses[0].ID), "null")
	assert.That(t, "error code", responses[0].Error.Code, protocol.ParseError)
}

func Test_Integration_With_UnknownTool_Should_Return32602(t *testing.T) {
	t.Parallel()

	// Arrange
	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"nonexistent","arguments":{}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	responses := parseResponses(t, &stdout)
	assert.That(t, "response count", len(responses), 2) // init + error
	assert.That(t, "init id", string(responses[0].ID), "1")
	assert.That(t, "error id", string(responses[1].ID), "2")
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
	if !strings.Contains(responses[1].Error.Message, "nonexistent") {
		t.Errorf("expected tool name in error, got: %s", responses[1].Error.Message)
	}
}

func Test_Server_With_OversizedString_Should_RejectNonFatallyAndContinue(t *testing.T) {
	t.Parallel()

	// Arrange — handshake, then a request whose params hold a string longer
	// than MaxJSONStringLen, then a valid ping. The connection must survive
	// the structural-limit rejection so the ping response arrives.
	bigValue := strings.Repeat("a", protocol.MaxJSONStringLen+1)
	input := handshake() +
		`{"jsonrpc":"2.0","method":"ping","id":2,"params":{"data":"` + bigValue + `"}}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":3,"params":{}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())

	// Assert — connection survives, ping after the rejection succeeds.
	assert.That(t, "run error", err, nil)

	responses := parseResponses(t, &stdout)
	assert.That(t, "response count", len(responses), 3) // init + structural error + ping
	assert.That(t, "init id", string(responses[0].ID), "1")
	assert.That(t, "structural error id", string(responses[1].ID), "null")
	assert.That(t, "structural error code", responses[1].Error.Code, protocol.ServerTimeout)
	if !strings.Contains(responses[1].Error.Message, "maxStringLength") {
		t.Errorf("expected maxStringLength in error, got: %s", responses[1].Error.Message)
	}
	assert.That(t, "ping id", string(responses[2].ID), "3")
	assert.That(t, "ping result", string(responses[2].Result), "{}")
}

func Test_Server_With_TooManyKeys_Should_RejectNonFatallyAndContinue(t *testing.T) {
	t.Parallel()

	// Arrange — handshake, then params object with MaxJSONKeysPerObject + 1 keys,
	// then a valid ping that proves the connection survived.
	var sb strings.Builder
	sb.WriteString(handshake())
	sb.WriteString(`{"jsonrpc":"2.0","method":"ping","id":2,"params":{`)
	for i := range protocol.MaxJSONKeysPerObject + 1 {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`"k`)
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(`":1`)
	}
	sb.WriteString("}}\n")
	sb.WriteString(`{"jsonrpc":"2.0","method":"ping","id":3,"params":{}}` + "\n")

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(sb.String()), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())

	// Assert — connection survives, ping after the rejection succeeds.
	assert.That(t, "run error", err, nil)

	responses := parseResponses(t, &stdout)
	assert.That(t, "response count", len(responses), 3) // init + structural error + ping
	assert.That(t, "init id", string(responses[0].ID), "1")
	assert.That(t, "structural error id", string(responses[1].ID), "null")
	assert.That(t, "structural error code", responses[1].Error.Code, protocol.ServerTimeout)
	if !strings.Contains(responses[1].Error.Message, "maxKeysPerObject") {
		t.Errorf("expected maxKeysPerObject in error, got: %s", responses[1].Error.Message)
	}
	assert.That(t, "ping id", string(responses[2].ID), "3")
	assert.That(t, "ping result", string(responses[2].Result), "{}")
}

// parseResponses reads all JSON-RPC responses from the buffer.
func parseResponses(t *testing.T, buf *bytes.Buffer) []protocol.Response {
	t.Helper()
	var responses []protocol.Response
	dec := json.NewDecoder(buf)
	for {
		var resp protocol.Response
		if err := dec.Decode(&resp); err != nil {
			break
		}
		responses = append(responses, resp)
	}
	return responses
}

// alwaysFailWriter returns an error on every Write — used to drive the
// encoder-error branch in handleDecodeError when the structural-limit
// response cannot be flushed to stdout.
type alwaysFailWriter struct{}

func (alwaysFailWriter) Write(_ []byte) (int, error) {
	return 0, io.ErrShortWrite
}

func Test_Server_With_StructuralLimit_AndFailingStdout_Should_LogEncodeError(t *testing.T) {
	t.Parallel()

	// Arrange — drive an oversized-string request through a server whose
	// stdout always errors. The structural-limit branch in handleDecodeError
	// must log the encode failure on stderr (the only observable signal,
	// since stdout is broken). No initialize is needed: structural-limit
	// rejection runs before state-machine gating.
	bigValue := strings.Repeat("a", protocol.MaxJSONStringLen+1)
	input := `{"jsonrpc":"2.0","method":"x","id":1,"params":{"data":"` + bigValue + `"}}` + "\n"

	var stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), alwaysFailWriter{}, &stderr)

	// Act — server should exit cleanly when the input ends, even with a
	// failing stdout, because the structural-limit branch is non-fatal and
	// the encode failure is logged rather than propagated.
	_ = srv.Run(context.Background())

	// Assert — encode_error key appears in the slog JSON output on stderr.
	if !strings.Contains(stderr.String(), `"encode_error"`) {
		t.Fatalf("expected encode_error log on stderr, got: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), `"decode_structural_limit"`) {
		t.Fatalf("expected decode_structural_limit log on stderr, got: %s", stderr.String())
	}
}
