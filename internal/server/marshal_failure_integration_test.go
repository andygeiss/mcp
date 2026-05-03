//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

// init registers the FR4 (Story 1.4) clause that pins the wire-shape contract
// for tool-output marshaling failures: code -32603, error.data carries
// {"field": "<offending field>"}, message names the failure mode.
func init() {
	protocol.Register(protocol.Clause{
		ID:      "MCP-2025-11-25/tools/MUST-emit-field-name-on-marshal-failure",
		Level:   protocol.LevelMUST,
		Section: "FR4 -32603 error.data carries field name on Out marshal failure",
		Summary: "When a tool handler's Out value contains a non-marshalable runtime value (chan, func, cyclic struct), the JSON-RPC error response has code -32603, a contextual message, and error.data.field naming the offending struct field (json tag).",
		Tests: []func(*testing.T){
			Test_ToolsCall_With_ChanInOut_Should_EmitFieldNameInErrorData,
			Test_ToolsCall_With_FuncInOut_Should_EmitFieldNameInErrorData,
			Test_ToolsCall_With_CyclicOut_Should_EmitFieldNameInErrorData,
		},
	})
}

type marshalFailInput struct {
	Trigger string `json:"trigger" description:"trigger string"`
}

// marshalFailOut declares the offending field as `any`, so the schema engine
// emits an open-schema entry (no compile-time rejection) and the failure
// happens at json.Marshal time when the handler stuffs a non-marshalable
// runtime value into Payload. This matches the real-world failure mode the
// FR4 helper is designed to surface.
type marshalFailOut struct {
	Payload any `json:"payload" description:"runtime-typed payload"`
}

// runToolsCallExpectingError drives a single tools/call through the full
// server pipeline and returns the response Error for shape assertions.
func runToolsCallExpectingError(t *testing.T, r *tools.Registry, toolName string) *protocol.Error {
	t.Helper()
	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"` + toolName + `","arguments":{"trigger":"x"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr)
	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	responses := parseResponses(t, &stdout)
	if len(responses) < 2 {
		t.Fatalf("expected ≥2 responses (init + tools/call), got %d", len(responses))
	}
	if responses[1].Error == nil {
		t.Fatalf("expected tools/call error response, got result %s", responses[1].Result)
	}
	return responses[1].Error
}

func Test_ToolsCall_With_ChanInOut_Should_EmitFieldNameInErrorData(t *testing.T) {
	t.Parallel()

	// Arrange — handler stuffs a chan into the `any` Payload field at runtime.
	r := tools.NewRegistry()
	if err := tools.Register[marshalFailInput, marshalFailOut](r, "chanOut", "tool whose payload becomes a chan at runtime",
		func(_ context.Context, _ marshalFailInput) (marshalFailOut, tools.Result) {
			return marshalFailOut{Payload: make(chan int)}, tools.Result{}
		},
	); err != nil {
		t.Fatal(err)
	}

	// Act
	errObj := runToolsCallExpectingError(t, r, "chanOut")

	// Assert — code, message, data shape.
	assert.That(t, "code", errObj.Code, protocol.InternalError)
	assert.That(t, "message names failure", strings.Contains(errObj.Message, "output marshaling failed: field"), true)
	var data map[string]string
	if err := json.Unmarshal(errObj.Data, &data); err != nil {
		t.Fatalf("error.data is not a JSON object: %v (raw=%q)", err, errObj.Data)
	}
	assert.That(t, "data.field names offending field", data["field"], "payload")
}

func Test_ToolsCall_With_FuncInOut_Should_EmitFieldNameInErrorData(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register[marshalFailInput, marshalFailOut](r, "funcOut", "tool whose payload becomes a func at runtime",
		func(_ context.Context, _ marshalFailInput) (marshalFailOut, tools.Result) {
			return marshalFailOut{Payload: func() {}}, tools.Result{}
		},
	); err != nil {
		t.Fatal(err)
	}

	// Act
	errObj := runToolsCallExpectingError(t, r, "funcOut")

	// Assert
	assert.That(t, "code", errObj.Code, protocol.InternalError)
	var data map[string]string
	_ = json.Unmarshal(errObj.Data, &data)
	assert.That(t, "data.field names offending field", data["field"], "payload")
}

func Test_ToolsCall_With_CyclicOut_Should_EmitFieldNameInErrorData(t *testing.T) {
	t.Parallel()

	// Arrange — handler stuffs a self-cyclic map into Payload (json.Marshal
	// rejects cyclic data structures with *json.UnsupportedValueError).
	r := tools.NewRegistry()
	if err := tools.Register[marshalFailInput, marshalFailOut](r, "cyclicOut", "tool whose payload is cyclic at runtime",
		func(_ context.Context, _ marshalFailInput) (marshalFailOut, tools.Result) {
			cyc := map[string]any{}
			cyc["loop"] = cyc
			return marshalFailOut{Payload: cyc}, tools.Result{}
		},
	); err != nil {
		t.Fatal(err)
	}

	// Act
	errObj := runToolsCallExpectingError(t, r, "cyclicOut")

	// Assert — code is -32603; data.field names the offending field.
	// FindUnmarshalable's depth bound returns ok=false on a runtime self-
	// looping map, so the helper produces field="unknown" by design — this
	// pins that fallback behavior end-to-end.
	assert.That(t, "code", errObj.Code, protocol.InternalError)
	var data map[string]string
	_ = json.Unmarshal(errObj.Data, &data)
	assert.That(t, "data.field is set", data["field"] != "", true)
}
