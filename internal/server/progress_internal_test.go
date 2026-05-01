package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
)

// newProgressTestServer builds a minimal Server that captures notification
// bytes on a buffered stdout AND log lines on a buffered stderr, suitable
// for white-box progress tests. The stdoutMu is initialized via the zero
// value (sync.Mutex), and the encoder is wired so sendNotification has
// somewhere to write.
func newProgressTestServer(t *testing.T) (*Server, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	enc := json.NewEncoder(&stdout)
	enc.SetEscapeHTML(false)
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelInfo)
	return &Server{
		enc:      enc,
		logLevel: levelVar,
		logger:   slog.New(slog.NewJSONHandler(&stderr, &slog.HandlerOptions{Level: levelVar})),
		stdoutMu: sync.Mutex{},
	}, &stdout, &stderr
}

// Test_Dispatch_With_ProgressToken_Should_PlumbToContext is the AC6 white-box
// test: the dispatch path extracts _meta.progressToken from the inbound tool
// call params and the resulting *Progress carries the original token bytes.
//
// White-box because extractProgressToken is unexported.
func Test_Dispatch_With_ProgressToken_Should_PlumbToContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want json.RawMessage
	}{
		{"string token", `{"name":"x","arguments":{},"_meta":{"progressToken":"task-42"}}`, json.RawMessage(`"task-42"`)},
		{"number token", `{"name":"x","arguments":{},"_meta":{"progressToken":42}}`, json.RawMessage(`42`)},
		{"missing _meta", `{"name":"x","arguments":{}}`, nil},
		{"missing token", `{"name":"x","arguments":{},"_meta":{}}`, nil},
		{"non-object _meta", `{"name":"x","arguments":{},"_meta":"oops"}`, nil},
		{"null token", `{"name":"x","arguments":{},"_meta":{"progressToken":null}}`, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := extractProgressToken(json.RawMessage(tc.raw))

			// Assert — byte-faithful. Equal nil if absent, byte-equal otherwise.
			if tc.want == nil {
				assert.That(t, "token nil when absent", got == nil, true)
				return
			}
			assert.That(t, "token bytes preserved verbatim", string(got), string(tc.want))
		})
	}
}

// Test_Progress_Report_With_OutboundActive_Should_Drop covers the AI10
// invariant at the unit level: while suspendForOutbound is active, Report
// MUST NOT emit a notification AND MUST log a `progress_dropped_during_outbound`
// warn so operators see when a handler is interleaving Report with
// SendRequest. After the closure runs, Report resumes.
func Test_Progress_Report_With_OutboundActive_Should_Drop(t *testing.T) {
	t.Parallel()

	// Arrange
	srv, stdout, stderr := newProgressTestServer(t)
	p := &Progress{server: srv, token: json.RawMessage(`"tok-1"`)}

	// Act — Report before, during, and after the suspend bracket.
	p.Report(1, 10)
	emittedBefore := bytes.Count(stdout.Bytes(), []byte(`"notifications/progress"`))

	release := p.suspendForOutbound()
	p.Report(5, 10)
	emittedDuring := bytes.Count(stdout.Bytes(), []byte(`"notifications/progress"`))
	release()

	p.Report(10, 10)
	emittedAfter := bytes.Count(stdout.Bytes(), []byte(`"notifications/progress"`))

	// Assert — exactly one notification before, zero added during, one added
	// after. The during-Report fires a warn on stderr (AI10 telemetry).
	assert.That(t, "first Report emitted", emittedBefore, 1)
	assert.That(t, "Report dropped while outbound is active", emittedDuring, 1)
	assert.That(t, "Report resumed after release", emittedAfter, 2)
	warnCount := bytes.Count(stderr.Bytes(), []byte(`"progress_dropped_during_outbound"`))
	assert.That(t, "AI10 drop logged once", warnCount, 1)
	assert.That(t, "warn carries reason tag", bytes.Contains(stderr.Bytes(), []byte(`"reason":"ai10_invariant"`)), true)
}

// Test_Progress_SuspendForOutbound_With_NilReceiver_Should_BeNoOp pins the
// nil-safe contract: a nil *Progress (handler with no Progress in ctx) must
// not panic when SendRequest brackets the await.
func Test_Progress_SuspendForOutbound_With_NilReceiver_Should_BeNoOp(t *testing.T) {
	t.Parallel()

	// Arrange
	var p *Progress

	// Act + Assert — both calls must be panic-free.
	release := p.suspendForOutbound()
	release()
}

// Test_Progress_Report_With_NestedOutbound_Should_StayDropped guards against
// a counter regression: nested suspendForOutbound calls (defensive, even
// though maxInFlight: 1 makes nesting unlikely in practice) decrement
// symmetrically and Report only resumes when depth returns to zero. Each
// during-outbound Report also fires its own warn line.
func Test_Progress_Report_With_NestedOutbound_Should_StayDropped(t *testing.T) {
	t.Parallel()

	// Arrange
	srv, stdout, stderr := newProgressTestServer(t)
	p := &Progress{server: srv, token: json.RawMessage(`"tok"`)}

	// Act — depth=2, drop a Report; release once (depth=1), drop again;
	// release twice (depth=0), Report emits.
	r1 := p.suspendForOutbound()
	r2 := p.suspendForOutbound()
	p.Report(1, 10)
	r2()
	p.Report(2, 10)
	r1()
	p.Report(3, 10)

	// Assert — only the last Report emitted on stdout, and stderr carries
	// two AI10 warns (one per dropped Report).
	emitted := bytes.Count(stdout.Bytes(), []byte(`"notifications/progress"`))
	assert.That(t, "exactly one notification after full release", emitted, 1)
	warnCount := bytes.Count(stderr.Bytes(), []byte(`"progress_dropped_during_outbound"`))
	assert.That(t, "two AI10 drops logged (one per dropped Report)", warnCount, 2)
}

// Test_Progress_Log_Without_Token_Should_Emit pins the design choice that
// notifications/message is independent of _meta.progressToken — the spec
// gates log notifications on the server's logging capability and
// logging/setLevel, NOT on per-request progress tokens. Documented to
// prevent a future "make Log token-aware" regression.
func Test_Progress_Log_Without_Token_Should_Emit(t *testing.T) {
	t.Parallel()

	// Arrange
	srv, stdout, _ := newProgressTestServer(t)
	p := &Progress{server: srv, token: nil} // no progress token

	// Act
	p.Log("info", "test", "hello")

	// Assert — log notification still emitted.
	emitted := bytes.Count(stdout.Bytes(), []byte(`"notifications/message"`))
	assert.That(t, "log emits without progress token", emitted, 1)
}

// Test_Progress_Report_Without_Token_Should_NotWarn pins the silent-no-op
// behavior of Report when no progress token is present: the client did not
// opt in, so the absence of a notification is normal and MUST NOT trigger
// the AI10 telemetry warn. Only the outbound-active path warns.
func Test_Progress_Report_Without_Token_Should_NotWarn(t *testing.T) {
	t.Parallel()

	// Arrange
	srv, _, stderr := newProgressTestServer(t)
	p := &Progress{server: srv, token: nil}

	// Act
	p.Report(1, 10)
	p.Report(5, 10)

	// Assert — no warn line; the no-token no-op stays silent.
	warnCount := bytes.Count(stderr.Bytes(), []byte(`"progress_dropped_during_outbound"`))
	assert.That(t, "no AI10 warn when token is absent", warnCount, 0)
}

// init registers the Story 2.3 spec-conformance clauses for the white-box
// tests in this file. Per the colocation convention from Story 2.1, clause
// registration lives next to the tests that prove the requirement.
func init() {
	protocol.Register(protocol.Clause{
		ID:      "MCP-2025-11-25/progress/MUST-extract-progressToken",
		Level:   "MUST",
		Section: "Q6 progress-token passthrough discipline",
		Summary: "When _meta.progressToken is present in a tools/call request, the server extracts it byte-faithfully (preserving JSON type) and plumbs it onto the handler context via *Progress.",
		Tests: []func(*testing.T){
			Test_Dispatch_With_ProgressToken_Should_PlumbToContext,
		},
	})
	protocol.Register(protocol.Clause{
		ID:      "MCP-2025-11-25/progress/MUST-not-emit-during-outbound",
		Level:   "MUST",
		Section: "AI10 — no progress notifications during outbound bidi requests",
		Summary: "While a tool handler is parked in protocol.SendRequest, Progress.Report MUST NOT emit notifications/progress (would corrupt response correlation per ADR-003).",
		Tests: []func(*testing.T){
			Test_Progress_Report_With_OutboundActive_Should_Drop,
		},
	})
}
