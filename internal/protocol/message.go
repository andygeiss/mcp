package protocol

import "encoding/json"

// Error represents a JSON-RPC 2.0 error object.
type Error struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data,omitempty"`
	Message string          `json:"message"`
}

// CodeError is a typed error that carries a JSON-RPC error code from the
// point of creation. Dispatch unwraps it at the boundary using errors.AsType
// to build the response directly. Handlers that don't return CodeError
// get the -32603 (internal error) fallback.
type CodeError struct {
	Code    int
	Data    json.RawMessage
	Message string
}

// Error implements the error interface.
func (e *CodeError) Error() string {
	return e.Message
}

// Request represents a JSON-RPC 2.0 request or notification.
type Request struct {
	ID      json.RawMessage `json:"id,omitempty"`
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	Error   *Error          `json:"error,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
}
