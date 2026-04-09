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

// ErrInternalError creates a CodeError with the InternalError code.
func ErrInternalError(msg string) *CodeError {
	return &CodeError{Code: InternalError, Message: msg}
}

// ErrInvalidParams creates a CodeError with the InvalidParams code.
func ErrInvalidParams(msg string) *CodeError {
	return &CodeError{Code: InvalidParams, Message: msg}
}

// ErrInvalidRequest creates a CodeError with the InvalidRequest code.
func ErrInvalidRequest(msg string) *CodeError {
	return &CodeError{Code: InvalidRequest, Message: msg}
}

// ErrMethodNotFound creates a CodeError with the MethodNotFound code.
func ErrMethodNotFound(msg string) *CodeError {
	return &CodeError{Code: MethodNotFound, Message: msg}
}

// ErrParseError creates a CodeError with the ParseError code.
func ErrParseError(msg string) *CodeError {
	return &CodeError{Code: ParseError, Message: msg}
}

// ErrServerError creates a CodeError with the ServerError code for state-related
// rejections (not initialized, already initialized, server busy).
func ErrServerError(msg string) *CodeError {
	return &CodeError{Code: ServerError, Message: msg}
}

// ErrServerTimeout creates a CodeError with the ServerTimeout code for tool
// handler timeouts and cancellations.
func ErrServerTimeout(msg string) *CodeError {
	return &CodeError{Code: ServerTimeout, Message: msg}
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
