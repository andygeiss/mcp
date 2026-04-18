package protocol

import (
	"context"
	"errors"
)

// Peer is the stable surface tool and prompt handlers use to send JSON-RPC 2.0
// requests back to the client (sampling, elicitation, roots/list).
//
// The method set and parameter/return types are a v1.x stability commitment per
// ADR-003 §Peer Stability Surface — any change is a MAJOR version bump.
// Signatures must reference only stdlib types and protocol-native types.
type Peer interface {
	SendRequest(ctx context.Context, method string, params any) (*Response, error)
}

// peerContextKey is the unexported key used to attach a Peer to a context.
type peerContextKey struct{}

// ContextWithPeer returns a derived context carrying the given Peer.
func ContextWithPeer(ctx context.Context, p Peer) context.Context {
	return context.WithValue(ctx, peerContextKey{}, p)
}

// PeerFromContext returns the Peer attached to ctx, or nil if none is set.
func PeerFromContext(ctx context.Context) Peer {
	p, _ := ctx.Value(peerContextKey{}).(Peer)
	return p
}

// ErrNoPeerInContext is returned by SendRequest when the supplied context has
// no Peer attached. It usually means the call originated outside a tool or
// prompt handler.
var ErrNoPeerInContext = errors.New("protocol: no peer in context")

// SendRequest is a convenience wrapper that extracts the Peer from ctx and
// invokes Peer.SendRequest. Returns ErrNoPeerInContext when no Peer is attached.
func SendRequest(ctx context.Context, method string, params any) (*Response, error) {
	p := PeerFromContext(ctx)
	if p == nil {
		return nil, ErrNoPeerInContext
	}
	return p.SendRequest(ctx, method, params)
}

// MethodCapability returns the client capability required to invoke the given
// method as a server-initiated request. The boolean is false when the method
// is not capability-gated (e.g., notifications/* or methods only used as
// client-to-server requests).
func MethodCapability(method string) (Capability, bool) {
	switch method {
	case "elicitation/create":
		return CapElicitation, true
	case "roots/list":
		return CapRoots, true
	case "sampling/createMessage":
		return CapSampling, true
	default:
		return "", false
	}
}
