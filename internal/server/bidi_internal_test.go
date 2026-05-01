package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
)

// Test_SendRequest_With_ElicitationCreate_AndCapabilityNotAdvertised_Should_LeavePendingMapEmpty
// is the white-box companion to the black-box AI9 negative test in
// synctest_test.go. The black-box version proves the wire-side (no bytes
// mentioning "elicitation/create" reach stdout); this version proves the
// server-state side (pending map untouched, outbound ID counter not
// incremented). Together they pin all THREE side-effect-zero properties the
// spec calls out for AC3.
//
// Constructed minimally — only the fields the AI9 path reads. Empty
// clientCaps means CapElicitation is not advertised, so the gate must reject
// before any side effect.
func Test_SendRequest_With_ElicitationCreate_AndCapabilityNotAdvertised_Should_LeavePendingMapEmpty(t *testing.T) {
	t.Parallel()

	// Arrange — server with empty client capabilities; elicitation NOT advertised.
	var stdout bytes.Buffer
	enc := json.NewEncoder(&stdout)
	enc.SetEscapeHTML(false)
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelInfo)
	s := &Server{
		enc:      enc,
		logLevel: levelVar,
		logger:   slog.New(slog.DiscardHandler),
		pending:  make(map[string]pendingEntry),
		stdoutMu: sync.Mutex{},
	}
	s.clientCaps.Store(&protocol.ClientCapabilities{}) // explicit: zero capabilities

	// Act — call (*Server).SendRequest directly so we can inspect server
	// state without race against a Run loop.
	_, err := s.SendRequest(context.Background(), "elicitation/create", nil)

	// Assert — typed error.
	var capErr *protocol.CapabilityNotAdvertisedError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected *CapabilityNotAdvertisedError, got: %v", err)
	}
	assert.That(t, "capability", capErr.Capability, protocol.CapElicitation)
	assert.That(t, "method", capErr.Method, "elicitation/create")

	// Assert — pending map untouched (registerPending was never called).
	s.pendingMu.Lock()
	pendingLen := len(s.pending)
	s.pendingMu.Unlock()
	assert.That(t, "pending map empty after AI9 reject", pendingLen, 0)

	// Assert — outbound ID counter untouched (Add(1) was never called).
	assert.That(t, "outbound id counter not incremented", s.outboundIDCounter.Load(), int64(0))

	// Assert — no bytes on outbound writer (Encode was never called). This is
	// the buf.Len() == 0 form the spec Dev Notes explicitly recommend.
	assert.That(t, "no bytes written to outbound writer", stdout.Len(), 0)
}
