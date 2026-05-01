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

// Test_Server_With_FullLifecycle_Should_OnlyOutputValidJsonRpc verifies that
// every byte on stdout is part of a valid JSON-RPC 2.0 response. No debug
// output, no blank lines, no stray bytes.
func Test_Server_With_FullLifecycle_Should_OnlyOutputValidJsonRpc(t *testing.T) {
	t.Parallel()

	// Arrange — exercise all method types
	r := tools.NewRegistry()
	if err := tools.Register(r, "test", "test tool", func(_ context.Context, input testInput) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult(input.Message)
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"tools/list","id":3,"params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"tools/call","id":4,"params":{"name":"test","arguments":{"message":"hello"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())
	assert.That(t, "run error", err, nil)

	// Assert — parse ALL stdout bytes as JSON-RPC responses
	raw := stdout.Bytes()
	dec := json.NewDecoder(bytes.NewReader(raw))
	var responseCount int
	var consumed int

	for dec.More() {
		var resp protocol.Response
		startOffset := dec.InputOffset()
		if derr := dec.Decode(&resp); derr != nil {
			t.Fatalf("invalid JSON at offset %d: %v", startOffset, derr)
		}
		if resp.JSONRPC != "2.0" {
			t.Errorf("response %d: expected jsonrpc 2.0, got %q", responseCount, resp.JSONRPC)
		}
		responseCount++
		consumed = int(dec.InputOffset())
	}

	// Verify no trailing garbage after the last response
	trailing := strings.TrimSpace(string(raw[consumed:]))
	if trailing != "" {
		t.Errorf("trailing bytes after last response: %q", trailing)
	}

	// Expected: initialize + ping + tools/list + tools/call = 4 responses
	assert.That(t, "response count", responseCount, 4)
}
