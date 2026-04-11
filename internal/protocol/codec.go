// Package protocol implements JSON-RPC 2.0 types and codec for the MCP server.
package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// IncomingMessage is a union type for messages arriving on stdin. It
// distinguishes client requests/notifications (which have a Method field)
// from client responses to server-initiated requests (which have Result or
// Error but no Method).
type IncomingMessage struct {
	IsResponse bool
	Request    Request
	Response   Response
}

// Decode reads one JSON-RPC 2.0 message from the decoder. It detects batch
// arrays (returning a parse error), normalizes absent or null params to {},
// and returns the decoded Request.
func Decode(dec *json.Decoder) (Request, error) {
	msg, err := DecodeMessage(dec)
	if err != nil {
		return Request{}, err
	}
	if msg.IsResponse {
		return Request{}, errors.New("unexpected response message")
	}
	return msg.Request, nil
}

// DecodeMessage reads one JSON-RPC 2.0 message from the decoder and classifies
// it as either a request/notification (has "method") or a response to a
// server-initiated request (has "result" or "error", no "method").
func DecodeMessage(dec *json.Decoder) (IncomingMessage, error) {
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return IncomingMessage{}, fmt.Errorf("decode message: %w", err)
	}

	// Batch detection: JSON array at top level is not supported.
	if len(raw) > 0 && raw[0] == '[' {
		return IncomingMessage{}, errors.New("batch requests not supported")
	}

	// Classify: a response has "result" or "error" (no "method"); a request
	// has "method". Check for response indicators to handle case-insensitive
	// field matching correctly (encoding/json v1 is case-insensitive).
	var probe struct {
		Method string          `json:"method"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	_ = json.Unmarshal(raw, &probe)

	isResponse := (len(probe.Result) > 0 || len(probe.Error) > 0) && probe.Method == ""
	if !isResponse {
		// It's a request or notification.
		var msg Request
		if err := json.Unmarshal(raw, &msg); err != nil {
			return IncomingMessage{}, fmt.Errorf("decode message: %w", err)
		}
		if len(msg.Params) == 0 || bytes.Equal(msg.Params, []byte("null")) {
			msg.Params = json.RawMessage("{}")
		}
		return IncomingMessage{Request: msg}, nil
	}

	// It's a response to a server-initiated request.
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return IncomingMessage{}, fmt.Errorf("decode message: %w", err)
	}
	return IncomingMessage{IsResponse: true, Response: resp}, nil
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
			// valid: string, integer, null
			// JSON-RPC 2.0: "Fractional parts MUST NOT be used"
			if bytes.ContainsAny(req.ID, ".eE") {
				return ErrInvalidRequest("id must not contain fractional or exponent parts")
			}
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
