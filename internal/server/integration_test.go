//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/andygeiss/mcp/internal/pkg/assert"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

func Test_Integration_With_FullPipeline_Should_CompleteSuccessfully(t *testing.T) {
	t.Parallel()

	// Arrange — full pipeline: initialize -> initialized -> tools/list -> tools/call
	input := `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","method":"tools/list","id":2,"params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"tools/call","id":3,"params":{"name":"test","arguments":{"message":"integration test"}}}` + "\n"

	r := tools.NewRegistry()
	tools.Register(r, "test", "test tool", func(_ context.Context, input testInput) tools.Result {
		return tools.TextResult(input.Message)
	})

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
	assert.That(t, "protocol version", initResult.ProtocolVersion, "2024-11-05")
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
}

func Test_Integration_With_PanickingHandler_Should_RecoverAndContinue(t *testing.T) {
	t.Parallel()

	// Arrange — panic tool followed by test tool proves server survives the panic.
	r := tools.NewRegistry()
	tools.Register(r, "panicker", "panics", func(_ context.Context, _ testInput) tools.Result {
		panic("boom")
	})
	tools.Register(r, "test", "test tool", func(_ context.Context, input testInput) tools.Result {
		return tools.TextResult(input.Message)
	})

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"panicker","arguments":{"message":"x"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"tools/call","id":3,"params":{"name":"test","arguments":{"message":"alive"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	responses := parseResponses(t, &stdout)
	assert.That(t, "response count", len(responses), 3) // init + panic + test

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

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "slow", "blocks", func(ctx context.Context, _ testInput) tools.Result {
		select {
		case <-time.After(10 * time.Second):
			return tools.TextResult("unreachable")
		case <-ctx.Done():
			return tools.ErrorResult("context cancelled")
		}
	})
	tools.Register(r, "test", "test tool", func(_ context.Context, input testInput) tools.Result {
		return tools.TextResult(input.Message)
	})

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"slow","arguments":{"message":"x"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"tools/call","id":3,"params":{"name":"test","arguments":{"message":"alive"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr, server.WithHandlerTimeout(50*time.Millisecond))

	// Act
	err := srv.Run(context.Background())

	// Assert
	assert.That(t, "error", err, nil)

	responses := parseResponses(t, &stdout)
	assert.That(t, "response count", len(responses), 3) // init + slow + test

	// Slow tool returns protocol-level error with timing diagnostics
	assert.That(t, "slow error code", responses[1].Error.Code, protocol.InternalError)
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
	assert.That(t, "error code", responses[1].Error.Code, protocol.InvalidParams)
	if !strings.Contains(responses[1].Error.Message, "nonexistent") {
		t.Errorf("expected tool name in error, got: %s", responses[1].Error.Message)
	}
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
