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
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/andygeiss/mcp/internal/pkg/assert"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

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
		tools.Register(r, "blocker", "blocks until timed out", func(ctx context.Context, _ testInput) tools.Result {
			<-ctx.Done()
			return tools.ErrorResult("handler context expired")
		})

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
		assert.That(t, "error code", responses[1].Error.Code, protocol.InternalError)
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
		tools.Register(r, "blocker", "blocks until cancelled", func(ctx context.Context, _ testInput) tools.Result {
			<-ctx.Done()
			return tools.ErrorResult("cancelled")
		})

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
