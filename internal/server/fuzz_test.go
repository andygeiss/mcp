//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

// Fuzz_Server_Pipeline sends arbitrary bytes through the full server pipeline
// (decode → dispatch → handle → encode) and asserts no panics occur and any
// stdout output is valid JSON-RPC.
func Fuzz_Server_Pipeline(f *testing.F) {
	// Seed corpus: valid sequences, malformed JSON, edge cases
	f.Add(`{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{},"clientInfo":{"name":"fuzz"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n")
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1,"params":{}}` + "\n")
	f.Add(`{invalid json}` + "\n")
	f.Add(``)
	f.Add(`null` + "\n")
	f.Add(`42` + "\n")
	f.Add(`[{"jsonrpc":"2.0","method":"ping","id":1}]` + "\n")
	f.Add(strings.Repeat("A", 100) + "\n")
	f.Add("\x00\x01\x02\x03\x04\x05" + "\n")
	f.Add(`{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"echo","arguments":{"text":"hello"}}}` + "\n")
	// Initialize → shutdown sequence
	f.Add(`{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","method":"shutdown","id":2}` + "\n")
	// Duplicate request IDs — server must handle gracefully
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1,"params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":1,"params":{}}` + "\n")
	// Rapid-fire pings — stress test sequential dispatch
	f.Add(strings.Repeat(`{"jsonrpc":"2.0","method":"ping","id":1,"params":{}}`+"\n", 100))

	f.Fuzz(func(t *testing.T, input string) {
		r := tools.NewRegistry()
		tools.Register(r, "echo", "echo tool", func(_ context.Context, in struct {
			Text string `json:"text" description:"text to echo"`
		},
		) tools.Result {
			return tools.TextResult(in.Text)
		})

		var stdout, stderr bytes.Buffer
		srv := server.NewServer("fuzz", "test", r, strings.NewReader(input), &stdout, &stderr)

		// Must not panic — errors from Run are acceptable
		_ = srv.Run(context.Background())

		// If anything was written to stdout, it must be valid JSON-RPC
		if stdout.Len() > 0 {
			dec := json.NewDecoder(&stdout)
			for dec.More() {
				var resp protocol.Response
				if err := dec.Decode(&resp); err != nil {
					t.Fatalf("stdout contains invalid JSON: %v", err)
				}
				if resp.JSONRPC != "2.0" {
					t.Fatalf("stdout response missing jsonrpc 2.0, got: %q", resp.JSONRPC)
				}
			}
		}
	})
}
