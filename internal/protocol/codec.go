// Package protocol implements JSON-RPC 2.0 types and codec for the MCP server.
package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrBatchNotSupported is returned by DecodeMessage when the top-level JSON is
// an array. Exposed as a sentinel so the server can surface a specific wire
// message instead of a generic parse error.
var ErrBatchNotSupported = errors.New("batch requests not supported")

// ErrJSONDepthExceeded is returned by DecodeMessage when a JSON-RPC payload
// nests more than MaxJSONDepth levels deep.
var ErrJSONDepthExceeded = fmt.Errorf("json nesting exceeds max depth %d", MaxJSONDepth)

// errUnbalanced signals malformed close-bracket structure encountered during
// the byte-scan (e.g. a top-level `]`). Surfaced as a generic parse error to
// the wire — the JSON unmarshaler would catch the same input, but the byte-scan
// must reject it without underflowing its depth counter.
var errUnbalanced = errors.New("unbalanced json close bracket")

// StructuralLimitError signals that a decoded JSON-RPC payload exceeds one of
// the structural caps enforced by checkLimits (key-count or string-length).
// The fields carry the limit name, the observed value, and the configured
// maximum so the server can render the wire message and (in a future PR)
// serialize them into the `error.data` object.
type StructuralLimitError struct {
	Limit  string
	Actual int
	Max    int
}

// Error implements the error interface and produces the exact wire message
// surfaced to clients when a structural limit is breached.
func (e *StructuralLimitError) Error() string {
	return fmt.Sprintf("payload exceeds %s: %d > %d", e.Limit, e.Actual, e.Max)
}

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
		return IncomingMessage{}, ErrBatchNotSupported
	}

	if err := checkLimits(raw); err != nil {
		return IncomingMessage{}, fmt.Errorf("decode message: %w", err)
	}

	isResponse, err := classifyResponse(raw)
	if err != nil {
		return IncomingMessage{}, err
	}
	if isResponse {
		return decodeResponseMessage(raw)
	}
	return decodeRequestMessage(raw)
}

// classifyResponse decides whether raw is a JSON-RPC response (has result or
// error, no method) or a request/notification. Rejects messages that carry
// both result and error per JSON-RPC 2.0 §5.
func classifyResponse(raw json.RawMessage) (bool, error) {
	var probe struct {
		Method string          `json:"method"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	_ = json.Unmarshal(raw, &probe)

	hasResult := isPresent(probe.Result)
	hasError := isPresent(probe.Error)
	if hasResult && hasError {
		return false, errors.New("response has both result and error")
	}
	return (hasResult || hasError) && probe.Method == "", nil
}

// decodeRequestMessage unmarshals raw as a Request, normalizing absent/null
// params to {} for downstream handlers.
func decodeRequestMessage(raw json.RawMessage) (IncomingMessage, error) {
	var msg Request
	if err := json.Unmarshal(raw, &msg); err != nil {
		return IncomingMessage{}, fmt.Errorf("decode message: %w", err)
	}
	if len(msg.Params) == 0 || bytes.Equal(msg.Params, []byte("null")) {
		msg.Params = json.RawMessage("{}")
	}
	return IncomingMessage{Request: msg}, nil
}

// decodeResponseMessage unmarshals raw as a Response routed to the pending map.
func decodeResponseMessage(raw json.RawMessage) (IncomingMessage, error) {
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

// limitScanner is the single-pass byte-scan state that enforces depth, key,
// and string-length caps. Per-depth arrays are statically sized to
// MaxJSONDepth+1; the depth-overflow guard runs before any index write so
// malformed input can never panic, and `depth--` is guarded by `depth > 0`.
type limitScanner struct {
	depth          int
	escaped        bool
	inString       bool
	keyCount       [MaxJSONDepth + 1]int
	parentIsObject [MaxJSONDepth + 1]bool
	strBytes       int
}

// step advances the scanner one byte and returns a non-nil error on a
// structural-limit breach, depth overflow, or unbalanced close-bracket.
func (s *limitScanner) step(b byte) error {
	if s.escaped {
		s.escaped = false
		return s.countStringByte()
	}
	if s.inString {
		return s.stepInString(b)
	}
	switch b {
	case '"':
		s.inString = true
		s.strBytes = 0
	case '{':
		return s.push(true)
	case '[':
		return s.push(false)
	case '}', ']':
		return s.pop()
	case ':':
		// 0 <= s.depth < len(s.parentIsObject) is invariant: push() returns
		// before exceeding the bound and pop() never goes negative.
		if s.parentIsObject[s.depth] {
			return s.bumpKey()
		}
	}
	return nil
}

func (s *limitScanner) stepInString(b byte) error {
	switch b {
	case '\\':
		s.escaped = true
		return s.countStringByte()
	case '"':
		s.inString = false
		s.strBytes = 0
		return nil
	}
	return s.countStringByte()
}

func (s *limitScanner) countStringByte() error {
	s.strBytes++
	if s.strBytes > MaxJSONStringLen {
		return &StructuralLimitError{Limit: "maxStringLength", Actual: s.strBytes, Max: MaxJSONStringLen}
	}
	return nil
}

func (s *limitScanner) push(isObject bool) error {
	next := s.depth + 1
	if next >= len(s.parentIsObject) {
		return ErrJSONDepthExceeded
	}
	s.depth = next
	s.parentIsObject[next] = isObject
	s.keyCount[next] = 0
	return nil
}

func (s *limitScanner) pop() error {
	if s.depth == 0 {
		return errUnbalanced
	}
	s.depth--
	return nil
}

func (s *limitScanner) bumpKey() error {
	s.keyCount[s.depth]++
	if s.keyCount[s.depth] > MaxJSONKeysPerObject {
		return &StructuralLimitError{Limit: "maxKeysPerObject", Actual: s.keyCount[s.depth], Max: MaxJSONKeysPerObject}
	}
	return nil
}

// checkLimits scans raw JSON bytes once and enforces all structural caps:
// nesting depth (MaxJSONDepth), per-object key count (MaxJSONKeysPerObject),
// and per-string raw byte length (MaxJSONStringLen). Brackets and colons
// inside string literals are ignored via the inString state.
func checkLimits(raw json.RawMessage) error {
	var s limitScanner
	for _, b := range raw {
		if err := s.step(b); err != nil {
			return err
		}
	}
	return nil
}

// isPresent reports whether a json.RawMessage carries a non-null value. A raw
// value of "null" (or empty) is treated as absent for response classification.
func isPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && !bytes.Equal(raw, []byte("null"))
}
