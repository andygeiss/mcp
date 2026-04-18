package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrPendingRequestsFull is returned by Peer.SendRequest when the server's
// pending-request correlation map is at capacity. It signals back-pressure —
// the caller should retry later.
var ErrPendingRequestsFull = errors.New("protocol: pending server-to-client requests full")

// ErrServerShutdown is returned by Peer.SendRequest when the server is shutting
// down before the client's response arrives. Handlers receiving this error
// should treat the outbound as abandoned and return promptly.
var ErrServerShutdown = errors.New("protocol: server shutting down")

// CapabilityNotAdvertisedError is returned synchronously by Peer.SendRequest when
// the server attempts a method that requires a client capability that was not
// advertised during initialize. Match via errors.AsType[*CapabilityNotAdvertisedError].
type CapabilityNotAdvertisedError struct {
	Capability Capability
	Method     string
}

// Error implements the error interface.
func (e *CapabilityNotAdvertisedError) Error() string {
	return fmt.Sprintf("protocol: client did not advertise capability %q (method %q)", e.Capability, e.Method)
}

// ClientRejectedError wraps a JSON-RPC 2.0 error object returned by the client
// for a server-initiated request. Match via errors.AsType[*ClientRejectedError]
// to inspect the structured Code/Message/Data fields.
type ClientRejectedError struct {
	Code    ErrorCode
	Data    json.RawMessage
	Message string
}

// Error implements the error interface.
func (e *ClientRejectedError) Error() string {
	return fmt.Sprintf("protocol: client rejected request: %s (code %d)", e.Message, e.Code)
}
