// Package protocol implements JSON-RPC 2.0 types and codec for the MCP server.
package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Decode reads one JSON-RPC 2.0 message from the decoder. It detects batch
// arrays (returning a parse error), normalizes absent or null params to {},
// and returns the decoded Request.
func Decode(dec *json.Decoder) (Request, error) {
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return Request{}, fmt.Errorf("decode message: %w", err)
	}

	// Batch detection: JSON array at top level is not supported.
	if len(raw) > 0 && raw[0] == '[' {
		return Request{}, errors.New("batch requests not supported")
	}

	var msg Request
	if err := json.Unmarshal(raw, &msg); err != nil {
		return Request{}, fmt.Errorf("decode message: %w", err)
	}

	// Normalize absent or null params to {}.
	if len(msg.Params) == 0 || bytes.Equal(msg.Params, []byte("null")) {
		msg.Params = json.RawMessage("{}")
	}

	return msg, nil
}

// Validate checks structural validity of a decoded JSON-RPC 2.0 request:
// version must be "2.0", method must be non-empty, and params (if present)
// must be a JSON object. Returns nil when the request is valid.
func Validate(req Request) *CodeError {
	if req.JSONRPC != Version {
		return ErrInvalidRequest("unsupported jsonrpc version")
	}
	if req.Method == "" {
		return ErrInvalidRequest("method is required")
	}
	if len(req.Params) > 0 && req.Params[0] != '{' {
		return ErrInvalidRequest("params must be an object")
	}
	if len(req.ID) > 0 {
		switch req.ID[0] {
		case '"', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '-', 'n':
			// valid: string, number, null
		default:
			return ErrInvalidRequest("id must be a string, number, or null")
		}
	}
	return nil
}

// Encode writes one JSON-RPC 2.0 response to the encoder.
func Encode(enc *json.Encoder, resp Response) error {
	resp.JSONRPC = Version
	if err := enc.Encode(&resp); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

// NewErrorResponse creates a JSON-RPC 2.0 error response.
func NewErrorResponse(id json.RawMessage, code int, message string) Response {
	return Response{
		Error: &Error{
			Code:    code,
			Message: message,
		},
		ID:      id,
		JSONRPC: Version,
	}
}

// NewErrorResponseFromCodeError creates a JSON-RPC 2.0 error response from a
// CodeError, propagating its Data field into the wire Error object.
func NewErrorResponseFromCodeError(id json.RawMessage, ce *CodeError) Response {
	return Response{
		Error: &Error{
			Code:    ce.Code,
			Data:    ce.Data,
			Message: ce.Message,
		},
		ID:      id,
		JSONRPC: Version,
	}
}

// NewResultResponse creates a JSON-RPC 2.0 success response by marshaling the
// given result value to JSON.
func NewResultResponse(id json.RawMessage, result any) (Response, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return Response{}, fmt.Errorf("marshal result: %w", err)
	}
	return Response{
		ID:      id,
		JSONRPC: Version,
		Result:  json.RawMessage(raw),
	}, nil
}

// NullID returns a json.RawMessage representing JSON null, used for error responses
// where the request ID is unknown. Returned as a fresh copy to prevent mutation.
func NullID() json.RawMessage {
	return json.RawMessage("null")
}
