package protocol_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
)

// stubPeer is a minimal Peer used to verify context plumbing.
type stubPeer struct {
	gotMethod string
	gotParams any
	resp      *protocol.Response
	err       error
}

func (s *stubPeer) SendRequest(_ context.Context, method string, params any) (*protocol.Response, error) {
	s.gotMethod = method
	s.gotParams = params
	return s.resp, s.err
}

func Test_PeerFromContext_With_NoPeer_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	// Arrange
	ctx := context.Background()

	// Act
	got := protocol.PeerFromContext(ctx)

	// Assert
	if got != nil {
		t.Fatalf("expected nil Peer from bare context, got %T", got)
	}
}

func Test_ContextWithPeer_Should_RoundTripPeer(t *testing.T) {
	t.Parallel()

	// Arrange
	stub := &stubPeer{}

	// Act
	ctx := protocol.ContextWithPeer(context.Background(), stub)
	got := protocol.PeerFromContext(ctx)

	// Assert
	if got != stub {
		t.Fatalf("PeerFromContext did not round-trip: got %v, want %v", got, stub)
	}
}

func Test_SendRequest_With_NoPeerInContext_Should_ReturnSentinel(t *testing.T) {
	t.Parallel()

	// Arrange
	ctx := context.Background()

	// Act
	_, err := protocol.SendRequest(ctx, "sampling/createMessage", nil)

	// Assert
	if !errors.Is(err, protocol.ErrNoPeerInContext) {
		t.Fatalf("expected ErrNoPeerInContext, got: %v", err)
	}
}

func Test_SendRequest_With_PeerInContext_Should_DelegateToPeer(t *testing.T) {
	t.Parallel()

	// Arrange
	stub := &stubPeer{resp: &protocol.Response{ID: json.RawMessage(`"x"`), JSONRPC: protocol.Version}}
	ctx := protocol.ContextWithPeer(context.Background(), stub)

	// Act
	resp, err := protocol.SendRequest(ctx, "sampling/createMessage", map[string]string{"k": "v"})

	// Assert
	assert.That(t, "no error", err, error(nil))
	assert.That(t, "method forwarded", stub.gotMethod, "sampling/createMessage")
	assert.That(t, "id echoed", string(resp.ID), `"x"`)
}

func Test_ErrorsIs_Should_MatchPendingRequestsFullSentinel(t *testing.T) {
	t.Parallel()

	// Arrange
	wrapped := errWrap(protocol.ErrPendingRequestsFull)

	// Act / Assert
	if !errors.Is(wrapped, protocol.ErrPendingRequestsFull) {
		t.Fatal("errors.Is should match wrapped ErrPendingRequestsFull")
	}
}

func Test_ErrorsIs_Should_MatchServerShutdownSentinel(t *testing.T) {
	t.Parallel()

	// Arrange
	wrapped := errWrap(protocol.ErrServerShutdown)

	// Act / Assert
	if !errors.Is(wrapped, protocol.ErrServerShutdown) {
		t.Fatal("errors.Is should match wrapped ErrServerShutdown")
	}
}

func Test_ErrorsAs_With_CapabilityNotAdvertised_Should_MatchAndExposeFields(t *testing.T) {
	t.Parallel()

	// Arrange
	src := &protocol.CapabilityNotAdvertisedError{Capability: protocol.CapSampling, Method: "sampling/createMessage"}

	// Act
	var got *protocol.CapabilityNotAdvertisedError
	ok := errors.As(error(src), &got)

	// Assert
	if !ok {
		t.Fatal("errors.As should match CapabilityNotAdvertisedError")
	}
	assert.That(t, "capability preserved", got.Capability, protocol.CapSampling)
	assert.That(t, "method preserved", got.Method, "sampling/createMessage")
}

func Test_ErrorsAs_With_ClientRejected_Should_MatchAndExposeFields(t *testing.T) {
	t.Parallel()

	// Arrange
	src := &protocol.ClientRejectedError{
		Code:    protocol.ErrCodeInvalidParams,
		Message: "bad input",
		Data:    json.RawMessage(`{"hint":"missing field x"}`),
	}

	// Act
	var got *protocol.ClientRejectedError
	ok := errors.As(error(src), &got)

	// Assert
	if !ok {
		t.Fatal("errors.As should match ClientRejectedError")
	}
	assert.That(t, "code preserved", got.Code, protocol.ErrCodeInvalidParams)
	assert.That(t, "message preserved", got.Message, "bad input")
	assert.That(t, "data preserved", string(got.Data), `{"hint":"missing field x"}`)
}

// errWrap wraps an error with %w so errors.Is must traverse the chain.
func errWrap(err error) error {
	return &wrappedError{inner: err}
}

type wrappedError struct{ inner error }

func (w *wrappedError) Error() string { return "wrapped: " + w.inner.Error() }
func (w *wrappedError) Unwrap() error { return w.inner }
