//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

// Fuzz_Server_E2E pipes arbitrary bytes through the full server (decode →
// dispatch → handle → encode) over a buffer transport and asserts three
// foundational invariants in one target (Q51, C5):
//
//   - FP #1 — stdout discipline: every byte written to stdout is a complete,
//     valid, newline-delimited JSON-RPC 2.0 response.
//   - FP #2 — server trusts no peer: arbitrary input never panics; errors
//     from Run are acceptable but the server must shut down cleanly.
//   - FP #4 — state-machine integrity: every response carries result XOR
//     error (never both, never neither) and a structurally valid id (null,
//     number, or quoted string — never a boolean, array, or object).
//
// Buffer transport keeps the target deterministic; a 2-second context
// deadline prevents inputs that legitimately stall (handshake without
// follow-up + slow handler) from dominating fuzz wall time.
func Fuzz_Server_E2E(f *testing.F) {
	// Seed corpus — valid sequences, malformed JSON, and post-M1 structural-
	// limit triggers. Every seed must satisfy the three invariants above.
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
	// Notification with id (X6) — must dispatch as a request and reject
	// notifications/* with -32601 rather than silently ignore.
	f.Add(`{"jsonrpc":"2.0","method":"notifications/cancelled","id":42,"params":{}}` + "\n")
	// Invalid id types — must reject with -32600 each, no panic
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":true,"params":{}}` + "\n")
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":[1,2],"params":{}}` + "\n")
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1.5,"params":{}}` + "\n")
	// M1a structural-limit triggers — oversized string and many keys.
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1,"params":{"d":"` + strings.Repeat("a", 4096) + `"}}` + "\n")
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1,"params":{` +
		strings.TrimSuffix(strings.Repeat(`"k":1,`, 16), ",") + `}}` + "\n")
	// Malformed close-brackets — depth-underflow guard must not panic
	f.Add(`}` + "\n")
	f.Add(`]` + "\n")
	f.Add(`{"a":1}}` + "\n")
	// Trailing garbage after a valid object (Q37) — decoder rejects the
	// next read but stdout must remain valid up to the failure.
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1,"params":{}}garbage` + "\n")
	// Mixed valid + invalid — ping succeeds, second message fails, third pings.
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1,"params":{}}` + "\n" +
		`{not json}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n")

	f.Fuzz(func(t *testing.T, input string) {
		r := tools.NewRegistry()
		if err := tools.Register(r, "echo", "echo tool", func(_ context.Context, in struct {
			Text string `json:"text" description:"text to echo"`
		},
		) tools.Result {
			return tools.TextResult(in.Text)
		}); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		srv := server.NewServer("fuzz", "test", r, strings.NewReader(input), &stdout, &stderr)

		// Bounded deadline: any input that legitimately stalls (partial
		// handshake, slow handler) must not dominate fuzz wall time. 2s is
		// far above the fast paths and well below the 30s handler timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// FP #2: must not panic — errors from Run are acceptable.
		_ = srv.Run(ctx)

		// FP #1 + FP #4: walk every response on stdout and assert structural
		// invariants. An empty stdout is valid (e.g. a single notification).
		if stdout.Len() == 0 {
			return
		}
		dec := json.NewDecoder(&stdout)
		for dec.More() {
			var resp protocol.Response
			if err := dec.Decode(&resp); err != nil {
				t.Fatalf("FP #1 violated: stdout contains invalid JSON: %v\nstdout: %q", err, stdout.String())
			}
			if resp.JSONRPC != "2.0" {
				t.Fatalf("FP #1 violated: response missing jsonrpc 2.0, got %q", resp.JSONRPC)
			}
			// FP #4: result XOR error. JSON-RPC 2.0 §5 forbids both or neither.
			hasResult := len(resp.Result) > 0
			hasError := resp.Error != nil
			if hasResult == hasError {
				t.Fatalf("FP #4 violated: response has result=%v error=%v (must be exactly one)", hasResult, hasError)
			}
			// FP #4: id structural validity. Codec accepts null, number, or
			// quoted-string ids; anything else must never appear on stdout.
			if len(resp.ID) > 0 {
				switch resp.ID[0] {
				case 'n', '"', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
					// valid leading byte: null, string, number (including negatives)
				default:
					t.Fatalf("FP #4 violated: response id has invalid leading byte %q (id=%q)", resp.ID[0], string(resp.ID))
				}
			}
		}
	})
}
