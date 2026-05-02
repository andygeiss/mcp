package server_test

// Synctest tests use testing/synctest to deterministically test concurrent
// behavior in the MCP server. Synctest creates an isolated "bubble" with
// virtual time — time.Sleep and channel operations advance a fake clock
// instead of waiting on wall-clock time. This eliminates flaky timing in
// tests that exercise handler timeout and context cancellation.
//
// Key synctest concepts:
//   - synctest.Test runs a function in an isolated bubble with virtual time
//   - synctest.Wait blocks until all goroutines in the bubble are durably blocked
//   - time.Sleep is durably blocking (advances virtual clock)
//   - Channel ops on in-bubble channels are durably blocking
//   - I/O operations (reads, writes) are NOT durably blocking
//
// These tests use strings.NewReader for stdin (no blocking I/O) so the
// concurrent behavior being tested is purely in the handler goroutine dispatch.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

// promptParamKey is the JSON-RPC param key for sampling/createMessage and
// elicitation/create payloads in tests. Hoisted (rather than rewritten) so the
// goconst linter does not flag the same literal across the bidi test files.
const promptParamKey = "prompt"

// Test_Server_With_SynctestHandlerTimeout_Should_TimeoutDeterministically
// verifies that a tool handler's context.WithTimeout fires correctly using
// virtual time. The handler blocks on <-ctx.Done() and returns when the
// timeout expires. Without synctest, this test would need real wall-clock
// waiting or fragile time.Sleep hacks.
func Test_Server_With_SynctestHandlerTimeout_Should_TimeoutDeterministically(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// Arrange — handler blocks until its timeout context fires
		r := tools.NewRegistry()
		if err := tools.Register(r, "blocker", "blocks until timed out", func(ctx context.Context, _ testInput) (struct{}, tools.Result) {
			<-ctx.Done()
			return struct{}{}, tools.ErrorResult("handler context expired")
		}); err != nil {
			t.Fatal(err)
		}

		input := handshake() +
			`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n"

		var stdout, stderr bytes.Buffer
		srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
			server.WithHandlerTimeout(5*time.Second))

		// Act — synctest advances virtual time to 5s when all goroutines block
		err := srv.Run(t.Context())

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

		// Timeout now returns protocol-level error with timing diagnostics
		assert.That(t, "error code", responses[1].Error.Code, protocol.ServerTimeout)
		if !strings.Contains(responses[1].Error.Message, "blocker") {
			t.Errorf("expected tool name in error message, got: %s", responses[1].Error.Message)
		}
	})
}

// Test_Server_With_SynctestContextCancellation_Should_ShutdownCleanly
// verifies that cancelling the parent context during active handler execution
// triggers a clean server shutdown. A goroutine cancels the context after 1
// virtual second while the handler is blocked. Without synctest, testing this
// race-free would be impossible.
func Test_Server_With_SynctestContextCancellation_Should_ShutdownCleanly(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// Arrange — handler blocks until context cancelled, cancel after 1s
		ctx, cancel := context.WithCancel(t.Context())

		r := tools.NewRegistry()
		if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) (struct{}, tools.Result) {
			<-ctx.Done()
			return struct{}{}, tools.ErrorResult("cancelled")
		}); err != nil {
			t.Fatal(err)
		}

		go func() {
			time.Sleep(1 * time.Second)
			cancel()
		}()

		input := handshake() +
			`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"x"}}}` + "\n"

		var stdout, stderr bytes.Buffer
		srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
			server.WithHandlerTimeout(time.Hour)) // Long timeout — cancellation should fire first

		// Act — synctest advances virtual time to 1s, cancel fires
		err := srv.Run(ctx)

		// Assert — clean shutdown, no panic, no goroutine leak
		assert.That(t, "error", err, nil)
	})
}

func Test_Server_With_ConcurrentRequest_Should_RejectWithServerBusy(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// Arrange
		r := tools.NewRegistry()
		if err := tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) (struct{}, tools.Result) {
			<-ctx.Done()
			return struct{}{}, tools.ErrorResult("cancelled")
		}); err != nil {
			t.Fatal(err)
		}

		input := handshake() +
			`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"blocker","arguments":{"message":"first"}}}` + "\n" +
			`{"jsonrpc":"2.0","method":"tools/call","id":3,"params":{"name":"blocker","arguments":{"message":"second"}}}` + "\n"

		var stdout, stderr bytes.Buffer
		srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
			server.WithHandlerTimeout(time.Hour))

		// Act
		err := srv.Run(t.Context())

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

		// Find the response for id:3 (the concurrent request)
		found := false
		for _, resp := range responses {
			if string(resp.ID) == "3" {
				found = true
				assert.That(t, "error code", resp.Error.Code, protocol.ServerError)
				if !strings.Contains(resp.Error.Message, "server busy") {
					t.Errorf("expected 'server busy' in message, got: %s", resp.Error.Message)
				}
			}
		}
		if !found {
			t.Fatal("expected response for id:3")
		}
	})
}

// Test_Server_With_CapabilityGate_Should_RejectSamplingWithoutAdvertisement covers
// AI9 NFR-R3 scenario (capability-gate side-effect-freeness). When the client
// did not advertise sampling, the handler's protocol.SendRequest must return
// *protocol.CapabilityNotAdvertisedError synchronously and ZERO bytes hit the
// wire (no outbound request is encoded).
func Test_Server_With_CapabilityGate_Should_RejectSamplingWithoutAdvertisement(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// Arrange — register a tool that tries to call sampling without the
		// client advertising the capability. Capture the typed error on return.
		var capErr *protocol.CapabilityNotAdvertisedError
		var sendErr error
		r := tools.NewRegistry()
		if err := tools.Register(r, "needsSampling", "calls sampling", func(ctx context.Context, _ testInput) (struct{}, tools.Result) {
			_, e := protocol.SendRequest(ctx, "sampling/createMessage", nil)
			sendErr = e
			if errors.As(e, &capErr) {
				return struct{}{}, tools.TextResult("rejected as expected")
			}
			return struct{}{}, tools.ErrorResult("unexpected outcome")
		}); err != nil {
			t.Fatal(err)
		}

		// Empty client capabilities — sampling NOT advertised.
		input := handshake() +
			`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"needsSampling","arguments":{"message":"x"}}}` + "\n"

		var stdout, stderr bytes.Buffer
		srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
			server.WithHandlerTimeout(time.Hour))

		// Act
		err := srv.Run(t.Context())

		// Assert
		assert.That(t, "run error", err, nil)
		if capErr == nil {
			t.Fatalf("expected *CapabilityNotAdvertisedError, got: %v", sendErr)
		}
		assert.That(t, "capability", capErr.Capability, protocol.CapSampling)
		assert.That(t, "method", capErr.Method, "sampling/createMessage")

		// Wire-side: stdout MUST NOT contain "sampling/createMessage" — the
		// gate is side-effect-free on absence (no outbound request emitted).
		if bytes.Contains(stdout.Bytes(), []byte(`"sampling/createMessage"`)) {
			t.Fatalf("AI9 violation: outbound emitted despite missing capability; stdout=%s", stdout.String())
		}

		// Total-message count: exactly the initialize response + the tool-call
		// response should appear on stdout. A regression that emits a side
		// effect via a different method name would add a third message and
		// fail this assertion — substring-only checks would miss it.
		assert.That(t, "exactly initialize+tool-call response (no outbound side effect)",
			strings.Count(stdout.String(), `"jsonrpc":"2.0"`), 2)
	})
}

// Test_SendRequest_With_ElicitationCreate_AndCapabilityNotAdvertised_Should_ReturnCapabilityError
// covers AI9 for the elicitation/create outbound route. When the client did
// NOT advertise the elicitation capability during initialize, the handler's
// protocol.SendRequest must return *protocol.CapabilityNotAdvertisedError
// synchronously and ZERO bytes mentioning "elicitation/create" hit stdout —
// proving the gate is the FIRST statement on the outbound path, not just
// AN early statement.
func Test_SendRequest_With_ElicitationCreate_AndCapabilityNotAdvertised_Should_ReturnCapabilityError(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// Arrange — register a tool that tries to elicit user input without
		// the client advertising the capability. Capture the typed error.
		var capErr *protocol.CapabilityNotAdvertisedError
		var sendErr error
		r := tools.NewRegistry()
		if err := tools.Register(r, "needsElicitation", "calls elicitation", func(ctx context.Context, _ testInput) (struct{}, tools.Result) {
			_, e := protocol.SendRequest(ctx, "elicitation/create", nil)
			sendErr = e
			if errors.As(e, &capErr) {
				return struct{}{}, tools.TextResult("rejected as expected")
			}
			return struct{}{}, tools.ErrorResult("unexpected outcome")
		}); err != nil {
			t.Fatal(err)
		}

		// Empty client capabilities — elicitation NOT advertised.
		input := handshake() +
			`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"needsElicitation","arguments":{"message":"x"}}}` + "\n"

		var stdout, stderr bytes.Buffer
		srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr,
			server.WithHandlerTimeout(time.Hour))

		// Act
		err := srv.Run(t.Context())

		// Assert — typed error surfaced through ContextWithPeer plumbing.
		assert.That(t, "run error", err, nil)
		if capErr == nil {
			t.Fatalf("expected *CapabilityNotAdvertisedError, got: %v", sendErr)
		}
		assert.That(t, "capability", capErr.Capability, protocol.CapElicitation)
		assert.That(t, "method", capErr.Method, "elicitation/create")

		// Wire-side: stdout MUST NOT contain "elicitation/create" — the gate
		// is side-effect-free on absence (no outbound request emitted).
		if bytes.Contains(stdout.Bytes(), []byte(`"elicitation/create"`)) {
			t.Fatalf("AI9 violation: outbound emitted despite missing capability; stdout=%s", stdout.String())
		}

		// Total-message count: exactly the initialize response + the tool-call
		// response should appear on stdout. A regression that emits a side
		// effect via a different method name would add a third message and
		// fail this assertion — substring-only checks would miss it.
		assert.That(t, "exactly initialize+tool-call response (no outbound side effect)",
			strings.Count(stdout.String(), `"jsonrpc":"2.0"`), 2)
	})
}

// init registers the Story 2.4 spec-conformance clauses for the elicitation/create
// outbound bidi path. The AI9 invariant covers the side-effect-free rejection
// when the capability is not advertised, and the happy-path covers the
// round-trip when it is. Both tests are colocated by package (server_test);
// the happy-path test lives in server_test.go.
func init() {
	protocol.Register(protocol.Clause{
		ID:      "MCP-2025-11-25/elicitation/MUST-gate-on-capability",
		Level:   protocol.LevelMUST,
		Section: "Q18 elicitation/create outbound",
		Summary: "When the client did not advertise the `elicitation` capability during initialize, the server MUST reject outbound `elicitation/create` requests with *protocol.CapabilityNotAdvertisedError and emit ZERO bytes on the wire (AI9 — first-statement gate, side-effect-free).",
		Tests: []func(*testing.T){
			Test_SendRequest_With_ElicitationCreate_AndCapabilityNotAdvertised_Should_ReturnCapabilityError,
		},
	})
	protocol.Register(protocol.Clause{
		ID:      "MCP-2025-11-25/elicitation/MUST-roundtrip-when-advertised",
		Level:   protocol.LevelMUST,
		Section: "Q18 elicitation/create outbound",
		Summary: "When the client advertised the `elicitation` capability, the server MUST encode an outbound `elicitation/create` request, await the correlated response, and surface the parsed result back to the tool handler that called protocol.SendRequest.",
		Tests: []func(*testing.T){
			Test_SendRequest_With_ElicitationCreate_AndCapabilityAdvertised_Should_RoundTrip,
		},
	})
}
