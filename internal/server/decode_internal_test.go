package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/andygeiss/mcp/internal/protocol"
)

// Test_handleDecodeErrorDuringInFlight_With_StructuralLimit_AndHandlerStuck_Should_LogAbandon
// covers the abandonment branch in handleDecodeErrorDuringInFlight: when a
// structural-limit error arrives mid-flight AND the in-flight handler never
// sends a response within 2*(handlerTimeout+safetyMargin), the safety timer
// fires and the function logs `handler_abandoned` carrying the request_id.
//
// In normal operation dispatchToolCall's own safety timer guarantees the
// handler response lands well before this branch fires, so the only way to
// exercise it is to construct an inFlightCh that nothing will ever send to.
// Synctest gives the test deterministic control over the virtual clock, so
// the 2.2s timer fires immediately rather than slowing the suite.
func Test_handleDecodeErrorDuringInFlight_With_StructuralLimit_AndHandlerStuck_Should_LogAbandon(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// Arrange — a Server whose inFlightCh exists but will never receive.
		// Simulates a tool handler goroutine that has been cancelled but is
		// not respecting context (the explicit case named in the package
		// docs as the limit of Go's goroutine model).
		var stdout, stderr bytes.Buffer
		enc := json.NewEncoder(&stdout)
		enc.SetEscapeHTML(false)
		levelVar := new(slog.LevelVar)
		levelVar.Set(slog.LevelInfo)
		s := &Server{
			enc:            enc,
			handlerTimeout: time.Second,
			logLevel:       levelVar,
			logger:         slog.New(slog.NewJSONHandler(&stderr, &slog.HandlerOptions{Level: levelVar})),
			safetyMargin:   100 * time.Millisecond,
		}
		s.inFlightCh = make(chan inFlightResult, 1) // never written
		s.inFlightID = json.RawMessage(`42`)

		// A structural-limit error is non-fatal in handleDecodeError, so the
		// function returns nil after writing the wire response and the logic
		// proceeds to the wait-for-handler block where the abandon timer arms.
		dr := decodeResult{err: &protocol.StructuralLimitError{
			Limit:  "maxStringLength",
			Actual: protocol.MaxJSONStringLen + 1,
			Max:    protocol.MaxJSONStringLen,
		}}

		// Act — synctest advances virtual time when the test goroutine blocks
		// in the function's select; the 2*(handlerTimeout+safetyMargin) timer
		// fires and the function logs handler_abandoned, then returns nil.
		err := s.handleDecodeErrorDuringInFlight(dr)
		// Assert
		if err != nil {
			t.Fatalf("expected nil error from structural-limit non-fatal path, got: %v", err)
		}

		// Stderr carries the abandonment log with the request_id.
		logs := stderr.String()
		if !strings.Contains(logs, `"msg":"handler_abandoned"`) {
			t.Fatalf("expected handler_abandoned log, got: %s", logs)
		}
		if !strings.Contains(logs, `"request_id":"42"`) {
			t.Fatalf("expected request_id=42 in abandon log, got: %s", logs)
		}

		// Stdout received the structural-limit error response with id=null.
		// Pin the contract that abandonment does NOT suppress the wire error
		// (the operator gets the structural rejection regardless of handler
		// fate).
		if !strings.Contains(stdout.String(), `"id":null`) {
			t.Fatalf("expected id=null on structural error response, got: %s", stdout.String())
		}
		if !strings.Contains(stdout.String(), "maxStringLength") {
			t.Fatalf("expected maxStringLength in wire response, got: %s", stdout.String())
		}

		// In-flight state must be cleared after abandon — the dispatch loop
		// would otherwise stay in the in-flight branch forever.
		if s.inFlightCh != nil {
			t.Fatalf("inFlightCh should be cleared after abandon, got non-nil")
		}
		if s.inFlightID != nil {
			t.Fatalf("inFlightID should be cleared after abandon, got: %s", s.inFlightID)
		}
	})
}
