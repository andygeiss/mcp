package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

// Test_ProgressReport_Without_Token_Should_NotEmit is the AC6 integration
// test for the negative path: a handler that calls Progress.Report when the
// originating tools/call request did NOT include _meta.progressToken must
// not produce any notifications/progress on the wire. Emitting one would be
// a spec violation (the client never opted in to progress).
func Test_ProgressReport_Without_Token_Should_NotEmit(t *testing.T) {
	t.Parallel()

	// Arrange — register a tool that calls Report unconditionally.
	r := tools.NewRegistry()
	if err := tools.Register(r, "no-token", "no-token tool", func(ctx context.Context, _ testInput) (struct{}, tools.Result) {
		p := server.ProgressFromContext(ctx)
		p.Report(1, 5)
		p.Report(3, 5)
		p.Report(5, 5)
		return struct{}{}, tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// No _meta.progressToken in the tool-call params.
	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"no-token","arguments":{"message":"x"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())
	assert.That(t, "run error", err, nil)

	// Assert — exactly two messages on stdout: init response + tool result.
	// Zero notifications/progress.
	dec := json.NewDecoder(&stdout)
	var messages []json.RawMessage
	for dec.More() {
		var raw json.RawMessage
		if derr := dec.Decode(&raw); derr != nil {
			break
		}
		messages = append(messages, raw)
	}
	assert.That(t, "message count", len(messages), 2)
	progressCount := bytes.Count(stdout.Bytes(), []byte(`"notifications/progress"`))
	assert.That(t, "no progress notifications emitted", progressCount, 0)
}

// Test_ProgressReport_With_StringToken_Should_PreserveType is the AC5/AC6
// golden-asserted test: a request carrying _meta.progressToken: "task-42"
// must produce a notifications/progress whose progressToken field is the
// exact bytes "task-42" (string type), not 42 (number) and not over-encoded
// "\"task-42\"". Pinning the byte shape catches double-encoding regressions.
func Test_ProgressReport_With_StringToken_Should_PreserveType(t *testing.T) {
	t.Parallel()

	// Arrange — handler emits exactly one progress notification.
	r := tools.NewRegistry()
	if err := tools.Register(r, "tagged", "tagged tool", func(ctx context.Context, _ testInput) (struct{}, tools.Result) {
		server.ProgressFromContext(ctx).Report(7, 9)
		return struct{}{}, tools.TextResult("done")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"tagged","arguments":{"message":"x"},"_meta":{"progressToken":"task-42"}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())
	assert.That(t, "run error", err, nil)

	// Assert — find the progress notification and pin its bytes.
	dec := json.NewDecoder(&stdout)
	var found json.RawMessage
	for dec.More() {
		var raw json.RawMessage
		if derr := dec.Decode(&raw); derr != nil {
			break
		}
		if bytes.Contains(raw, []byte(`"notifications/progress"`)) {
			found = raw
			break
		}
	}
	if found == nil {
		t.Fatal("no notifications/progress emitted")
	}

	// Golden: byte-for-byte JSON shape. Order of struct fields is determined
	// by the progressParams struct declaration order — pinning it here turns
	// any future tag/order drift into a test failure rather than a silent
	// wire-format regression.
	want := `{"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":7,"progressToken":"task-42","total":9}}`
	got := strings.TrimSpace(string(found))
	assert.That(t, "byte-faithful progress notification", got, want)
}

// Test_ProgressReport_With_NumberToken_Should_PreserveType is the AC5
// type-preservation companion: a numeric progressToken survives the
// extraction → emission roundtrip without becoming a string.
func Test_ProgressReport_With_NumberToken_Should_PreserveType(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "numbered", "numbered tool", func(ctx context.Context, _ testInput) (struct{}, tools.Result) {
		server.ProgressFromContext(ctx).Report(2, 5)
		return struct{}{}, tools.TextResult("done")
	}); err != nil {
		t.Fatal(err)
	}

	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"numbered","arguments":{"message":"x"},"_meta":{"progressToken":42}}}` + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, strings.NewReader(input), &stdout, &stderr)

	// Act
	err := srv.Run(context.Background())
	assert.That(t, "run error", err, nil)

	// Assert — token bytes are `42` (unquoted), proving the numeric type was
	// preserved through json.RawMessage rather than re-marshaled as a string.
	want := `{"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":2,"progressToken":42,"total":5}}`
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var raw json.RawMessage
		if derr := dec.Decode(&raw); derr != nil {
			break
		}
		if bytes.Contains(raw, []byte(`"notifications/progress"`)) {
			assert.That(t, "numeric token preserved", strings.TrimSpace(string(raw)), want)
			return
		}
	}
	t.Fatal("no notifications/progress emitted")
}

// Test_ProgressReport_DuringOutboundBidi_Should_NotInterleave drives the AC4
// invariant end-to-end: a handler that interleaves Progress.Report with
// protocol.SendRequest must not emit a progress notification while the
// outbound is in flight. Combined with the white-box
// Test_Progress_Report_With_OutboundActive_Should_Drop test, this covers
// the AI10 contract from both unit and integration angles.
//
// Synchronization is channel-based: the test signals the handler-side
// goroutine to fire its during-Report only after the test has read the
// outbound off the wire (which means SendRequest has incremented
// outboundDepth and is parked on the response). The goroutine reports
// completion before the test releases the response — making the AI10 drop
// observation deterministic on any scheduler.
func Test_ProgressReport_DuringOutboundBidi_Should_NotInterleave(t *testing.T) {
	t.Parallel()

	// Channels shared with the handler closure so the test drives timing.
	fireReport := make(chan struct{})
	duringDone := make(chan struct{})

	// Arrange — handler that brackets SendRequest with Reports plus an
	// in-flight Report from a goroutine that waits for the test's signal.
	r := tools.NewRegistry()
	if err := tools.Register(r, "bidir-progress", "bidi+progress tool", func(ctx context.Context, _ testInput) (struct{}, tools.Result) {
		p := server.ProgressFromContext(ctx)
		p.Report(1, 3) // before — emits

		go func() {
			defer close(duringDone)
			<-fireReport   // test fires this AFTER it has decoded the outbound
			p.Report(2, 3) // during — must be dropped (AI10)
		}()

		_, err := protocol.SendRequest(ctx, "sampling/createMessage", map[string]string{promptParamKey: "x"})
		<-duringDone
		if err != nil {
			return struct{}{}, tools.ErrorResult("send request failed: " + err.Error())
		}
		p.Report(3, 3) // after — emits
		return struct{}{}, tools.TextResult("done")
	}); err != nil {
		t.Fatal(err)
	}

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	t.Cleanup(func() { _ = stdoutW.Close() })
	var stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", r, stdinR, stdoutW, &stderr)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				done <- fmt.Errorf("server panic: %v", rec)
			}
		}()
		done <- srv.Run(ctx)
	}()
	// AI10 telemetry assertions below need stderr — but they assert on a
	// specific count, not WARN/ERROR cleanliness. Drain via the bidi helper
	// would also call assertCleanStderr, which would conflict with the
	// expected single warn record. So this test owns its drainage inline:
	// stdinW.Close + stdoutR.Close + <-done at end of body.

	// Handshake advertising sampling capability so AI9 lets the outbound through.
	bidirHandshake := `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"protocolVersion":"` + protocol.MCPVersion + `","capabilities":{"sampling":{}}}}` + "\n" + initializedNotification
	if _, werr := stdinW.Write([]byte(bidirHandshake)); werr != nil {
		t.Fatalf("write handshake: %v", werr)
	}

	dec := json.NewDecoder(stdoutR)
	var initResp json.RawMessage
	if derr := dec.Decode(&initResp); derr != nil {
		t.Fatalf("decode init response: %v", derr)
	}

	// Tool call WITH a progress token.
	if _, werr := stdinW.Write([]byte(`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"bidir-progress","arguments":{"message":"x"},"_meta":{"progressToken":"bidi-tok"}}}` + "\n")); werr != nil {
		t.Fatalf("write tool call: %v", werr)
	}

	// Drain the wire until we see the outbound sampling/createMessage request.
	// Anything that arrives before it (notifications/progress for the
	// before-Report) is collected so the test can later assert on it.
	var pre []json.RawMessage
	var srvReq struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	for {
		var raw json.RawMessage
		if derr := dec.Decode(&raw); derr != nil {
			t.Fatalf("decode: %v", derr)
		}
		if bytes.Contains(raw, []byte(`"sampling/createMessage"`)) {
			if jerr := json.Unmarshal(raw, &srvReq); jerr != nil {
				t.Fatalf("unmarshal outbound: %v", jerr)
			}
			break
		}
		pre = append(pre, raw)
	}

	// At this point SendRequest has incremented outboundDepth and is parked on
	// the response (it had to encode the outbound under stdoutMu before we
	// could decode it here). Releasing the goroutine NOW guarantees its
	// Report sees depth>0 and is dropped by the AI10 gate.
	close(fireReport)
	<-duringDone

	// Send the response back to release SendRequest.
	resp := `{"jsonrpc":"2.0","id":` + string(srvReq.ID) + `,"result":"ok"}` + "\n"
	if _, werr := stdinW.Write([]byte(resp)); werr != nil {
		t.Fatalf("write outbound response: %v", werr)
	}

	// Drain remaining output: post-Report notification + tool result.
	var post []json.RawMessage
	var toolResp protocol.Response
	for {
		var raw json.RawMessage
		if derr := dec.Decode(&raw); derr != nil {
			break
		}
		if bytes.Contains(raw, []byte(`"id":2`)) {
			_ = json.Unmarshal(raw, &toolResp)
			break
		}
		post = append(post, raw)
	}

	// Drain the server before reading stderr for the AI10 telemetry
	// assertions. Closing both pipe ends unblocks Run if it's parked
	// writing to stdoutW for any reason; the receive picks up the run
	// error (or panic).
	_ = stdinW.Close()
	_ = stdoutR.Close()
	if rerr := <-done; rerr != nil {
		t.Fatalf("server run: %v", rerr)
	}

	// Assert — AI10 invariant: exactly two progress notifications total
	// (the before-Report on the pre side, the after-Report on the post side).
	// The during-Report (which fired while SendRequest was parked) must NOT
	// have emitted anything.
	preProgress := countProgressNotifications(pre)
	postProgress := countProgressNotifications(post)
	assert.That(t, "before-Report emitted on pre side", preProgress, 1)
	assert.That(t, "after-Report emitted on post side", postProgress, 1)
	assert.That(t, "no progress emitted during outbound (AI10)", preProgress+postProgress, 2)
	assert.That(t, "tool result delivered", toolResp.Error == nil, true)

	// AI10 telemetry: the dropped Report MUST have surfaced as a
	// `progress_dropped_during_outbound` warn line on stderr (request-scoped
	// logger). Without this assertion, a regression that drops the Report for
	// any other reason (missed window, wrong gate) would still pass.
	warnCount := bytes.Count(stderr.Bytes(), []byte(`"progress_dropped_during_outbound"`))
	assert.That(t, "AI10 drop logged on stderr", warnCount, 1)
	assert.That(t, "warn carries reason tag", bytes.Contains(stderr.Bytes(), []byte(`"reason":"ai10_invariant"`)), true)
}

// countProgressNotifications counts how many of the given raw JSON messages
// are notifications/progress. Used by the AI10 integration test to assert
// the number of emissions on each side of the outbound await.
func countProgressNotifications(msgs []json.RawMessage) int {
	n := 0
	for _, m := range msgs {
		if bytes.Contains(m, []byte(`"notifications/progress"`)) {
			n++
		}
	}
	return n
}

// init registers the Story 2.3 black-box clauses. The white-box dispatch and
// AI10-drop clauses are registered in progress_internal_test.go (package
// server) where their test functions live.
func init() {
	protocol.Register(protocol.Clause{
		ID:      "MCP-2025-11-25/progress/MUST-not-emit-without-token",
		Level:   protocol.LevelMUST,
		Section: "Q6 progress-token passthrough discipline",
		Summary: "Progress.Report MUST NOT emit notifications/progress when the originating request omitted _meta.progressToken (client did not opt in).",
		Tests: []func(*testing.T){
			Test_ProgressReport_Without_Token_Should_NotEmit,
		},
	})
	protocol.Register(protocol.Clause{
		ID:      "MCP-2025-11-25/progress/MUST-preserve-token-type",
		Level:   protocol.LevelMUST,
		Section: "Q6 progress-token passthrough discipline",
		Summary: "When emitting notifications/progress the server MUST preserve the original JSON type of progressToken byte-for-byte (string stays string, number stays number) — never re-marshal.",
		Tests: []func(*testing.T){
			Test_ProgressReport_With_StringToken_Should_PreserveType,
			Test_ProgressReport_With_NumberToken_Should_PreserveType,
		},
	})
}
