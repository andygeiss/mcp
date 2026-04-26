//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

// Test_Conformance_Runner discovers all .request.jsonl files in
// testdata/conformance/ and verifies the server produces structurally
// valid responses for each request sequence.
func Test_Conformance_Runner(t *testing.T) {
	t.Parallel()

	pattern := filepath.Join("testdata", "conformance", "*.request.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob testdata: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no conformance test files found in testdata/conformance/")
	}

	for _, reqFile := range matches {
		name := strings.TrimSuffix(filepath.Base(reqFile), ".request.jsonl")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runConformanceTest(t, reqFile)
		})
	}
}

func runConformanceTest(t *testing.T, reqFile string) {
	t.Helper()

	reqData, err := os.ReadFile(filepath.Clean(reqFile))
	if err != nil {
		t.Fatalf("read request file: %v", err)
	}

	respFile := strings.Replace(reqFile, ".request.jsonl", ".response.jsonl", 1)
	var expectedResponses []json.RawMessage
	if respData, err := os.ReadFile(filepath.Clean(respFile)); err == nil {
		for line := range strings.SplitSeq(strings.TrimSpace(string(respData)), "\n") {
			if line != "" {
				expectedResponses = append(expectedResponses, json.RawMessage(line))
			}
		}
	}

	r := tools.NewRegistry()
	if err := tools.Register(r, "test", "test tool", func(_ context.Context, input testInput) tools.Result {
		return tools.TextResult(input.Message)
	}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, bytes.NewReader(reqData), &stdout, &stderr)

	_ = srv.Run(context.Background())

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var resp protocol.Response
		if derr := dec.Decode(&resp); derr != nil {
			break
		}
		responses = append(responses, resp)
	}

	// Count expected responses: each request line with an "id" field gets a response
	expectedCount := 0
	for line := range strings.SplitSeq(strings.TrimSpace(string(reqData)), "\n") {
		if line == "" {
			continue
		}
		var msg struct {
			ID json.RawMessage `json:"id,omitempty"`
		}
		if json.Unmarshal([]byte(line), &msg) == nil && len(msg.ID) > 0 {
			expectedCount++
		}
	}

	// When a golden response file exists, its line count is authoritative
	// (handles edge cases like batch-array-rejection where the request has no
	// parseable id but the server still writes an error response).
	if len(expectedResponses) > 0 {
		expectedCount = len(expectedResponses)
	}

	assert.That(t, "response count", len(responses), expectedCount)

	for i, resp := range responses {
		if resp.JSONRPC != protocol.Version {
			t.Errorf("response %d: expected jsonrpc 2.0, got %q", i, resp.JSONRPC)
		}
	}

	if len(expectedResponses) > 0 {
		if len(expectedResponses) != len(responses) {
			t.Fatalf("expected %d responses in %s, got %d", len(expectedResponses), respFile, len(responses))
		}
		for i, expected := range expectedResponses {
			actual, _ := json.Marshal(responses[i])
			var compactExpected, compactActual bytes.Buffer
			_ = json.Compact(&compactExpected, expected)
			_ = json.Compact(&compactActual, actual)
			if !bytes.Equal(compactExpected.Bytes(), compactActual.Bytes()) {
				t.Errorf("response %d mismatch:\n  want: %s\n  got:  %s", i, compactExpected.String(), compactActual.String())
			}
		}
	}
}
