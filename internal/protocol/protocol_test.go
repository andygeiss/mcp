package protocol_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
)

// Spec-coverage registry bootstrap: see docs/development-guide.md
// "Adding a spec clause" for the canonical pattern.

func init() {
	protocol.Register(protocol.Clause{
		ID:      "MCP-2025-11-25/jsonrpc/MUST-echo-id",
		Level:   protocol.LevelMUST,
		Section: "JSON-RPC 2.0 §5 Response object",
		Summary: "Decoder preserves request id exactly (string and number forms) for echo back to the client.",
		Tests: []func(*testing.T){
			Test_Decode_With_StringID_Should_PreserveExactly,
			Test_Decode_With_NumberID_Should_PreserveExactly,
		},
	})
	protocol.Register(protocol.Clause{
		ID:      "MCP-2025-11-25/decode/MUST-reject-deep-nesting",
		Level:   protocol.LevelMUST,
		Section: "M1a decode-time structural limits",
		Summary: "Decoder rejects payloads whose JSON nesting exceeds MaxJSONDepth before unmarshal.",
		Tests: []func(*testing.T){
			Test_Decode_With_DeeplyNestedJSON_Should_ReturnError,
		},
	})
}

// --- Decode tests ---

func Test_Decode_With_ValidRequest_Should_PreserveAllFields(t *testing.T) {
	t.Parallel()

	// Arrange
	input := `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	msg, err := protocol.Decode(dec)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "jsonrpc", msg.JSONRPC, "2.0")
	assert.That(t, "method", msg.Method, "initialize")
	assert.That(t, "id", string(msg.ID), "1")
	assert.That(t, "params", string(msg.Params), `{"capabilities":{}}`)
}

func Test_Decode_With_StringID_Should_PreserveExactly(t *testing.T) {
	t.Parallel()

	// Arrange
	input := `{"jsonrpc":"2.0","method":"ping","id":"abc-123","params":{}}` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	msg, err := protocol.Decode(dec)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "id", string(msg.ID), `"abc-123"`)
}

func Test_Decode_With_NumberID_Should_PreserveExactly(t *testing.T) {
	t.Parallel()

	// Arrange
	input := `{"jsonrpc":"2.0","method":"ping","id":42,"params":{}}` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	msg, err := protocol.Decode(dec)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "id", string(msg.ID), "42")
}

func Test_Decode_With_NullID_Should_PreserveAsNullBytes(t *testing.T) {
	t.Parallel()

	// Arrange
	input := `{"jsonrpc":"2.0","method":"ping","id":null,"params":{}}` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	msg, err := protocol.Decode(dec)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "id", string(msg.ID), "null")
}

func Test_Decode_With_AbsentParams_Should_DefaultToEmptyObject(t *testing.T) {
	t.Parallel()

	// Arrange
	input := `{"jsonrpc":"2.0","method":"ping","id":1}` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	msg, err := protocol.Decode(dec)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "params", string(msg.Params), "{}")
}

func Test_Decode_With_NullParams_Should_DefaultToEmptyObject(t *testing.T) {
	t.Parallel()

	// Arrange
	input := `{"jsonrpc":"2.0","method":"ping","id":1,"params":null}` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	msg, err := protocol.Decode(dec)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "params", string(msg.Params), "{}")
}

func Test_Decode_With_BatchArray_Should_ReturnParseError(t *testing.T) {
	t.Parallel()

	// Arrange
	input := `[{"jsonrpc":"2.0","method":"ping","id":1}]` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	_, err := protocol.Decode(dec)

	// Assert
	if err == nil {
		t.Fatal("expected error for batch array")
	}
	assert.That(t, "error message", err.Error(), "batch requests not supported")
}

func Test_Decode_With_DeeplyNestedJSON_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — 200 nested arrays inside a valid jsonrpc envelope; the
	// envelope is shallow so only the params depth triggers the guard.
	var sb strings.Builder
	sb.WriteString(`{"jsonrpc":"2.0","method":"x","params":{"a":`)
	for range 200 {
		sb.WriteByte('[')
	}
	for range 200 {
		sb.WriteByte(']')
	}
	sb.WriteString(`}}` + "\n")
	dec := json.NewDecoder(strings.NewReader(sb.String()))

	// Act
	_, err := protocol.Decode(dec)

	// Assert
	if err == nil {
		t.Fatal("expected depth-guard error for deeply nested JSON")
	}
	if !strings.Contains(err.Error(), "nesting exceeds max depth") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_Decode_With_ResponseHavingBothResultAndError_Should_Reject(t *testing.T) {
	t.Parallel()

	// Arrange — malformed response per JSON-RPC 2.0 §5.
	input := `{"jsonrpc":"2.0","id":1,"result":{"ok":true},"error":{"code":-1,"message":"x"}}` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	_, err := protocol.DecodeMessage(dec)

	// Assert
	if err == nil {
		t.Fatal("expected error when response carries both result and error")
	}
	assert.That(t, "error message", err.Error(), "response has both result and error")
}

func Test_Decode_With_NullResultField_Should_NotClassifyAsResponse(t *testing.T) {
	t.Parallel()

	// Arrange — "result":null alone used to be misclassified as a response.
	input := `{"jsonrpc":"2.0","id":1,"method":"ping","result":null}` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	msg, err := protocol.DecodeMessage(dec)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "classified as request", msg.IsResponse, false)
	assert.That(t, "method", msg.Request.Method, "ping")
}

func Test_Decode_With_Notification_Should_HaveZeroLengthID(t *testing.T) {
	t.Parallel()

	// Arrange
	input := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	msg, err := protocol.Decode(dec)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "id length", len(msg.ID), 0)
	assert.That(t, "method", msg.Method, "notifications/initialized")
}

// --- encoding/json v1 accepted behavior ---
// These tests document behaviors of encoding/json that differ from json/v2's
// strict defaults. Each behavior is accepted for MCP: well-behaved clients
// send correctly-cased fields, never duplicate keys, and valid UTF-8.

func Test_Decode_With_CaseInsensitiveField_Should_MatchStructField(t *testing.T) {
	t.Parallel()

	// Arrange — "Method" instead of "method"
	input := `{"jsonrpc":"2.0","Method":"ping","id":1}` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	msg, err := protocol.Decode(dec)

	// Assert — encoding/json matches case-insensitively
	assert.That(t, "error", err, nil)
	assert.That(t, "method", msg.Method, "ping")
}

func Test_Decode_With_DuplicateKeys_Should_UseLastValue(t *testing.T) {
	t.Parallel()

	// Arrange — "method" appears twice
	input := `{"jsonrpc":"2.0","method":"initialize","method":"ping","id":1}` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	msg, err := protocol.Decode(dec)

	// Assert — encoding/json takes last value
	assert.That(t, "error", err, nil)
	assert.That(t, "method", msg.Method, "ping")
}

func Test_Decode_With_InvalidUTF8InParams_Should_NotReject(t *testing.T) {
	t.Parallel()

	// Arrange — invalid UTF-8 byte sequence in params value
	input := "{\"jsonrpc\":\"2.0\",\"method\":\"tools/call\",\"id\":1,\"params\":{\"name\":\"\xff\"}}\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act
	msg, err := protocol.Decode(dec)

	// Assert — encoding/json passes through invalid UTF-8
	assert.That(t, "error", err, nil)
	assert.That(t, "method", msg.Method, "tools/call")
	if len(msg.Params) == 0 {
		t.Fatal("expected non-empty params")
	}
}

// --- Encode tests ---

func Test_Encode_With_SuccessResponse_Should_ProduceValidJSON(t *testing.T) {
	t.Parallel()

	// Arrange
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	resp := protocol.Response{
		ID:      json.RawMessage("1"),
		JSONRPC: protocol.Version,
		Result:  json.RawMessage(`{"status":"ok"}`),
	}

	// Act
	err := protocol.Encode(enc, resp)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "output", buf.String(), "{\"id\":1,\"jsonrpc\":\"2.0\",\"result\":{\"status\":\"ok\"}}\n")
}

func Test_Encode_With_ErrorResponse_Should_IncludeErrorObject(t *testing.T) {
	t.Parallel()

	// Arrange
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	resp := protocol.NewErrorResponse(json.RawMessage("1"), protocol.MethodNotFound, "method not found")

	// Act
	err := protocol.Encode(enc, resp)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "output", buf.String(), "{\"error\":{\"code\":-32601,\"message\":\"method not found\"},\"id\":1,\"jsonrpc\":\"2.0\"}\n")
}

// --- Golden tests: byte-for-byte comparison ---

func Test_Encode_With_NullIDErrorResponse_Should_ProduceExactJSON(t *testing.T) {
	t.Parallel()

	// Arrange
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	resp := protocol.NewErrorResponse(protocol.NullID(), protocol.ParseError, "parse error")

	// Act
	err := protocol.Encode(enc, resp)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "golden output", buf.String(), "{\"error\":{\"code\":-32700,\"message\":\"parse error\"},\"id\":null,\"jsonrpc\":\"2.0\"}\n")
}

// --- Helper constructor tests ---

func Test_NewResultResponse_Should_MarshalResult(t *testing.T) {
	t.Parallel()

	// Arrange
	type result struct {
		Content string `json:"content"`
	}

	// Act
	resp, err := protocol.NewResultResponse(json.RawMessage("42"), result{Content: "hello"})

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "jsonrpc", resp.JSONRPC, "2.0")
	assert.That(t, "id", string(resp.ID), "42")
	assert.That(t, "result", string(resp.Result), `{"content":"hello"}`)
}

// --- CodeError tests ---

// errMsgUnknownToolFoo is a fixture error message reused across CodeError
// tests; hoisted to satisfy the goconst linter (3+ occurrences in this file).
const errMsgUnknownToolFoo = "unknown tool: foo"

func Test_CodeError_Should_ImplementErrorInterface(t *testing.T) {
	t.Parallel()

	// Arrange
	var err error = &protocol.CodeError{Code: protocol.InvalidParams, Message: errMsgUnknownToolFoo}

	// Assert
	assert.That(t, "error message", err.Error(), errMsgUnknownToolFoo)
}

func Test_CodeError_With_ErrorsAsType_Should_Unwrap(t *testing.T) {
	t.Parallel()

	// Arrange
	err := fmt.Errorf("dispatch: %w", &protocol.CodeError{Code: protocol.InvalidParams, Message: errMsgUnknownToolFoo})

	// Act
	pe, ok := errors.AsType[*protocol.CodeError](err)

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "code", pe.Code, protocol.InvalidParams)
	assert.That(t, "message", pe.Message, errMsgUnknownToolFoo)
}

func Test_CodeError_With_NonCodeError_Should_NotMatch(t *testing.T) {
	t.Parallel()

	// Arrange
	err := errors.New("some random error")

	// Act
	_, ok := errors.AsType[*protocol.CodeError](err)

	// Assert
	assert.That(t, "found", ok, false)
}

// --- Constructor tests ---

func Test_ErrInternalError_Should_ReturnCorrectCode(t *testing.T) {
	t.Parallel()

	// Act
	err := protocol.ErrInternalError("something broke")

	// Assert
	assert.That(t, "code", err.Code, protocol.InternalError)
	assert.That(t, "message", err.Message, "something broke")
}

func Test_ErrInvalidParams_Should_ReturnCorrectCode(t *testing.T) {
	t.Parallel()

	// Act
	err := protocol.ErrInvalidParams("missing field: name")

	// Assert
	assert.That(t, "code", err.Code, protocol.InvalidParams)
	assert.That(t, "message", err.Message, "missing field: name")
}

func Test_ErrInvalidRequest_Should_ReturnCorrectCode(t *testing.T) {
	t.Parallel()

	// Act
	err := protocol.ErrInvalidRequest("server not initialized")

	// Assert
	assert.That(t, "code", err.Code, protocol.InvalidRequest)
	assert.That(t, "message", err.Message, "server not initialized")
}

func Test_ErrMethodNotFound_Should_ReturnCorrectCode(t *testing.T) {
	t.Parallel()

	// Act
	err := protocol.ErrMethodNotFound("unknown method: foo")

	// Assert
	assert.That(t, "code", err.Code, protocol.MethodNotFound)
	assert.That(t, "message", err.Message, "unknown method: foo")
}

func Test_ErrParseError_Should_ReturnCorrectCode(t *testing.T) {
	t.Parallel()

	// Act
	err := protocol.ErrParseError("invalid JSON")

	// Assert
	assert.That(t, "code", err.Code, protocol.ParseError)
	assert.That(t, "message", err.Message, "invalid JSON")
}

func Test_ErrConstructor_With_Wrapping_Should_ExtractViaErrorsAsType(t *testing.T) {
	t.Parallel()

	// Arrange
	wrapped := fmt.Errorf("context: %w", protocol.ErrInvalidParams("bad input"))

	// Act
	pe, ok := errors.AsType[*protocol.CodeError](wrapped)

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "code", pe.Code, protocol.InvalidParams)
	assert.That(t, "message", pe.Message, "bad input")
}

// --- NewErrorResponse with Data tests ---

func Test_NewErrorResponse_With_Data_Should_IncludeDataField(t *testing.T) {
	t.Parallel()

	// Arrange
	data := json.RawMessage(`{"detail":"something broke"}`)
	resp := protocol.NewErrorResponse(json.RawMessage("1"), protocol.InvalidParams, "bad input")
	resp.Error.Data = data

	// Act
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err := protocol.Encode(enc, resp)

	// Assert
	assert.That(t, "error", err, nil)
	if !strings.Contains(buf.String(), `"data"`) {
		t.Fatalf("expected JSON to contain \"data\", got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"detail":"something broke"`) {
		t.Fatalf("expected JSON to contain data payload, got: %s", buf.String())
	}
}

func Test_NewErrorResponse_With_NilData_Should_OmitDataField(t *testing.T) {
	t.Parallel()

	// Arrange
	resp := protocol.NewErrorResponse(json.RawMessage("1"), protocol.InvalidParams, "bad input")

	// Act
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err := protocol.Encode(enc, resp)

	// Assert
	assert.That(t, "error", err, nil)
	if strings.Contains(buf.String(), `"data"`) {
		t.Fatalf("expected JSON to NOT contain \"data\", got: %s", buf.String())
	}
}

func Test_NewErrorResponse_With_Data_Should_RoundTrip(t *testing.T) {
	t.Parallel()

	// Arrange
	data := json.RawMessage(`{"trace_id":"abc-123"}`)
	ce := &protocol.CodeError{
		Code:    protocol.InvalidParams,
		Data:    data,
		Message: errMsgUnknownToolFoo,
	}
	resp := protocol.NewErrorResponseFromCodeError(json.RawMessage("7"), ce)

	// Act — encode
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err := protocol.Encode(enc, resp)
	assert.That(t, "encode error", err, nil)

	// Act — decode
	var decoded protocol.Response
	err = json.Unmarshal(buf.Bytes(), &decoded)

	// Assert
	assert.That(t, "decode error", err, nil)
	assert.That(t, "error code", decoded.Error.Code, protocol.InvalidParams)
	assert.That(t, "error message", decoded.Error.Message, errMsgUnknownToolFoo)
	assert.That(t, "error data", string(decoded.Error.Data), `{"trace_id":"abc-123"}`)
}

// --- Validate tests ---

func Test_Validate_With_WrongVersion_Should_ReturnInvalidRequest(t *testing.T) {
	t.Parallel()

	// Arrange
	req := protocol.Request{
		JSONRPC: "1.0",
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("{}"),
	}

	// Act
	got := protocol.Validate(req)

	// Assert
	if got == nil {
		t.Fatal("expected error, got nil")
	}
	assert.That(t, "code", got.Code, protocol.InvalidRequest)
	assert.That(t, "message", got.Message, "unsupported jsonrpc version")
}

func Test_Validate_With_EmptyMethod_Should_ReturnInvalidRequest(t *testing.T) {
	t.Parallel()

	// Arrange
	req := protocol.Request{
		JSONRPC: protocol.Version,
		Method:  "",
		Params:  json.RawMessage("{}"),
	}

	// Act
	got := protocol.Validate(req)

	// Assert
	if got == nil {
		t.Fatal("expected error, got nil")
	}
	assert.That(t, "code", got.Code, protocol.InvalidRequest)
	assert.That(t, "message", got.Message, "method is required")
}

func Test_Validate_With_ArrayParams_Should_ReturnInvalidRequest(t *testing.T) {
	t.Parallel()

	// Arrange
	req := protocol.Request{
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("[1,2]"),
	}

	// Act
	got := protocol.Validate(req)

	// Assert
	if got == nil {
		t.Fatal("expected error, got nil")
	}
	assert.That(t, "code", got.Code, protocol.InvalidRequest)
	assert.That(t, "message", got.Message, "params must be an object")
}

func Test_Validate_With_ValidRequest_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	// Arrange
	req := protocol.Request{
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("{}"),
	}

	// Act
	got := protocol.Validate(req)

	// Assert
	assert.That(t, "error", got, (*protocol.CodeError)(nil))
}

func Test_Validate_With_EmptyParams_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	// Arrange
	req := protocol.Request{
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
	}

	// Act
	got := protocol.Validate(req)

	// Assert
	assert.That(t, "error", got, (*protocol.CodeError)(nil))
}

// --- Round-trip golden test ---

func Test_DecodeEncode_With_ValidRequest_Should_RoundTrip(t *testing.T) {
	t.Parallel()

	// Arrange
	input := `{"jsonrpc":"2.0","method":"tools/call","id":"req-1","params":{"name":"echo","arguments":{"text":"hello"}}}` + "\n"
	dec := json.NewDecoder(strings.NewReader(input))

	// Act — decode
	msg, err := protocol.Decode(dec)
	assert.That(t, "decode error", err, nil)

	// Act — build response and encode
	resp, err := protocol.NewResultResponse(msg.ID, struct {
		Content []struct {
			Text string `json:"text"`
			Type string `json:"type"`
		} `json:"content"`
	}{
		Content: []struct {
			Text string `json:"text"`
			Type string `json:"type"`
		}{{Text: "hello", Type: "text"}},
	})
	assert.That(t, "marshal error", err, nil)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err = protocol.Encode(enc, resp)

	// Assert
	assert.That(t, "encode error", err, nil)

	// Verify the response is valid JSON by unmarshaling
	var check protocol.Response
	if uerr := json.Unmarshal(buf.Bytes(), &check); uerr != nil {
		t.Fatalf("output is not valid JSON: %v", uerr)
	}
	assert.That(t, "response id", string(check.ID), `"req-1"`)
	assert.That(t, "response jsonrpc", check.JSONRPC, "2.0")
}

func Test_Validate_With_BooleanID_Should_ReturnInvalidRequest(t *testing.T) {
	t.Parallel()
	// Arrange
	req := protocol.Request{
		ID:      json.RawMessage("true"),
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("{}"),
	}
	// Act
	got := protocol.Validate(req)
	// Assert
	if got == nil {
		t.Fatal("expected error, got nil")
	}
	assert.That(t, "code", got.Code, protocol.InvalidRequest)
	assert.That(t, "message", got.Message, "id must be a string, number, or null")
}

func Test_Validate_With_BooleanFalseID_Should_ReturnInvalidRequest(t *testing.T) {
	t.Parallel()
	// Arrange
	req := protocol.Request{
		ID:      json.RawMessage("false"),
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("{}"),
	}
	// Act
	got := protocol.Validate(req)
	// Assert
	if got == nil {
		t.Fatal("expected error, got nil")
	}
	assert.That(t, "code", got.Code, protocol.InvalidRequest)
	assert.That(t, "message", got.Message, "id must be a string, number, or null")
}

func Test_Validate_With_ArrayID_Should_ReturnInvalidRequest(t *testing.T) {
	t.Parallel()
	// Arrange
	req := protocol.Request{
		ID:      json.RawMessage("[1]"),
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("{}"),
	}
	// Act
	got := protocol.Validate(req)
	// Assert
	if got == nil {
		t.Fatal("expected error, got nil")
	}
	assert.That(t, "code", got.Code, protocol.InvalidRequest)
	assert.That(t, "message", got.Message, "id must be a string, number, or null")
}

func Test_Validate_With_ObjectID_Should_ReturnInvalidRequest(t *testing.T) {
	t.Parallel()
	// Arrange
	req := protocol.Request{
		ID:      json.RawMessage(`{"a":1}`),
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("{}"),
	}
	// Act
	got := protocol.Validate(req)
	// Assert
	if got == nil {
		t.Fatal("expected error, got nil")
	}
	assert.That(t, "code", got.Code, protocol.InvalidRequest)
	assert.That(t, "message", got.Message, "id must be a string, number, or null")
}

func Test_Validate_With_FloatID_Should_ReturnInvalidRequest(t *testing.T) {
	t.Parallel()
	// Arrange
	req := protocol.Request{
		ID:      json.RawMessage("1.5"),
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("{}"),
	}
	// Act
	got := protocol.Validate(req)
	// Assert
	if got == nil {
		t.Fatal("expected error, got nil")
	}
	assert.That(t, "code", got.Code, protocol.InvalidRequest)
	assert.That(t, "message", got.Message, "id must not contain fractional or exponent parts")
}

func Test_Validate_With_ScientificNotationID_Should_ReturnInvalidRequest(t *testing.T) {
	t.Parallel()
	// Arrange
	req := protocol.Request{
		ID:      json.RawMessage("1e308"),
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("{}"),
	}
	// Act
	got := protocol.Validate(req)
	// Assert
	if got == nil {
		t.Fatal("expected error, got nil")
	}
	assert.That(t, "code", got.Code, protocol.InvalidRequest)
	assert.That(t, "message", got.Message, "id must not contain fractional or exponent parts")
}

func Test_Validate_With_NullID_Should_ReturnNil(t *testing.T) {
	t.Parallel()
	// Arrange
	req := protocol.Request{
		ID:      json.RawMessage("null"),
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("{}"),
	}
	// Act
	got := protocol.Validate(req)
	// Assert
	assert.That(t, "error", got, (*protocol.CodeError)(nil))
}

func Test_Validate_With_ZeroID_Should_ReturnNil(t *testing.T) {
	t.Parallel()
	// Arrange
	req := protocol.Request{
		ID:      json.RawMessage("0"),
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("{}"),
	}
	// Act
	got := protocol.Validate(req)
	// Assert
	assert.That(t, "error", got, (*protocol.CodeError)(nil))
}

func Test_Validate_With_EmptyStringID_Should_ReturnNil(t *testing.T) {
	t.Parallel()
	// Arrange
	req := protocol.Request{
		ID:      json.RawMessage(`""`),
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("{}"),
	}
	// Act
	got := protocol.Validate(req)
	// Assert
	assert.That(t, "error", got, (*protocol.CodeError)(nil))
}

func Test_Validate_With_NegativeNumberID_Should_ReturnNil(t *testing.T) {
	t.Parallel()
	// Arrange
	req := protocol.Request{
		ID:      json.RawMessage("-1"),
		JSONRPC: protocol.Version,
		Method:  protocol.MethodPing,
		Params:  json.RawMessage("{}"),
	}
	// Act
	got := protocol.Validate(req)
	// Assert
	assert.That(t, "error", got, (*protocol.CodeError)(nil))
}

func Test_Validate_With_NotificationNoID_Should_SkipValidation(t *testing.T) {
	t.Parallel()
	// Arrange
	req := protocol.Request{
		JSONRPC: protocol.Version,
		Method:  "notifications/initialized",
		Params:  json.RawMessage("{}"),
	}
	// Act
	got := protocol.Validate(req)
	// Assert
	assert.That(t, "error", got, (*protocol.CodeError)(nil))
}

// --- Error branch tests ---

func Test_Encode_With_FailingWriter_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	enc := json.NewEncoder(&failWriter{})
	resp := protocol.Response{
		ID:      json.RawMessage("1"),
		JSONRPC: protocol.Version,
		Result:  json.RawMessage(`{}`),
	}

	// Act
	err := protocol.Encode(enc, resp)

	// Assert
	if err == nil {
		t.Fatal("expected error from failing writer, got nil")
	}
}

func Test_NewResultResponse_With_UnmarshalableValue_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	ch := make(chan int)

	// Act
	_, err := protocol.NewResultResponse(json.RawMessage("1"), ch)

	// Assert
	if err == nil {
		t.Fatal("expected marshal error for channel type, got nil")
	}
}

func Test_ErrServerError_Should_ReturnCorrectCode(t *testing.T) {
	t.Parallel()

	// Act
	err := protocol.ErrServerError("server not initialized")

	// Assert
	assert.That(t, "code", err.Code, protocol.ServerError)
	assert.That(t, "message", err.Message, "server not initialized")
}

func Test_ErrServerTimeout_Should_ReturnCorrectCode(t *testing.T) {
	t.Parallel()

	// Act
	err := protocol.ErrServerTimeout("tool handler timed out")

	// Assert
	assert.That(t, "code", err.Code, protocol.ServerTimeout)
	assert.That(t, "message", err.Message, "tool handler timed out")
}

// failWriter is a writer that always returns an error.
type failWriter struct{}

func (f *failWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write error")
}

// --- M1a structural-limit tests (key-count, string-length, no-panic) ---

// noPanic decodes input and fails the test on any panic, regardless of
// whether decode itself returned an error. Every malformed-input test must
// route through this wrapper to prove the byte-scan never panics.
func noPanic(t *testing.T, input string) error {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("decode panicked on input %q: %v", input, r)
		}
	}()
	dec := json.NewDecoder(strings.NewReader(input))
	_, err := protocol.DecodeMessage(dec)
	return err
}

func Test_Decode_With_TooManyKeys_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — params object with MaxJSONKeysPerObject + 1 keys.
	var sb strings.Builder
	sb.WriteString(`{"jsonrpc":"2.0","method":"x","id":1,"params":{`)
	for i := range protocol.MaxJSONKeysPerObject + 1 {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `"k%d":%d`, i, i)
	}
	sb.WriteString("}}\n")

	// Act
	err := noPanic(t, sb.String())

	// Assert
	if err == nil {
		t.Fatal("expected structural-limit error")
	}
	var sle *protocol.StructuralLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("expected *StructuralLimitError, got %T: %v", err, err)
	}
	assert.That(t, "limit", sle.Limit, "maxKeysPerObject")
	assert.That(t, "max", sle.Max, protocol.MaxJSONKeysPerObject)
	if sle.Actual <= protocol.MaxJSONKeysPerObject {
		t.Fatalf("expected actual > max, got actual=%d max=%d", sle.Actual, sle.Max)
	}
	want := fmt.Sprintf("payload exceeds maxKeysPerObject: %d > %d", sle.Actual, sle.Max)
	assert.That(t, "wire message", sle.Error(), want)
}

func Test_Decode_With_OversizedString_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — single string value just over MaxJSONStringLen raw bytes.
	value := strings.Repeat("a", protocol.MaxJSONStringLen+1)
	input := `{"jsonrpc":"2.0","method":"x","id":1,"params":{"data":"` + value + `"}}` + "\n"

	// Act
	err := noPanic(t, input)

	// Assert
	if err == nil {
		t.Fatal("expected structural-limit error")
	}
	var sle *protocol.StructuralLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("expected *StructuralLimitError, got %T: %v", err, err)
	}
	assert.That(t, "limit", sle.Limit, "maxStringLength")
	assert.That(t, "max", sle.Max, protocol.MaxJSONStringLen)
	if sle.Actual <= protocol.MaxJSONStringLen {
		t.Fatalf("expected actual > max, got actual=%d max=%d", sle.Actual, sle.Max)
	}
}

func Test_Decode_With_OversizedKeyName_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — key name itself exceeds MaxJSONStringLen.
	key := strings.Repeat("k", protocol.MaxJSONStringLen+1)
	input := `{"jsonrpc":"2.0","method":"x","id":1,"params":{"` + key + `":1}}` + "\n"

	// Act
	err := noPanic(t, input)

	// Assert
	if err == nil {
		t.Fatal("expected structural-limit error")
	}
	var sle *protocol.StructuralLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("expected *StructuralLimitError, got %T: %v", err, err)
	}
	assert.That(t, "limit", sle.Limit, "maxStringLength")
}

func Test_Decode_With_SiblingObjects_Should_ResetKeyCount(t *testing.T) {
	t.Parallel()

	// Arrange — first object inside an array has the limit, second has limit+1.
	// The per-object key counter must reset on each `{`.
	var sb strings.Builder
	sb.WriteString(`{"jsonrpc":"2.0","method":"x","id":1,"params":{"items":[{`)
	for i := range 5 {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `"a%d":%d`, i, i)
	}
	sb.WriteString(`},{`)
	for i := range protocol.MaxJSONKeysPerObject + 1 {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `"b%d":%d`, i, i)
	}
	sb.WriteString("}]}}\n")

	// Act
	err := noPanic(t, sb.String())

	// Assert
	if err == nil {
		t.Fatal("expected structural-limit error from second sibling")
	}
	var sle *protocol.StructuralLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("expected *StructuralLimitError, got %T: %v", err, err)
	}
	assert.That(t, "limit", sle.Limit, "maxKeysPerObject")
}

func Test_Decode_With_NestedObjectsUnderLimit_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Arrange — outer object with 5 keys, each holding a nested object with 5 keys.
	input := `{"jsonrpc":"2.0","method":"x","id":1,"params":{"a":{"x":1,"y":2,"z":3,"w":4,"v":5},"b":{"x":1,"y":2,"z":3,"w":4,"v":5},"c":{"x":1,"y":2,"z":3,"w":4,"v":5},"d":{"x":1,"y":2,"z":3,"w":4,"v":5},"e":{"x":1,"y":2,"z":3,"w":4,"v":5}}}` + "\n"

	// Act
	err := noPanic(t, input)

	// Assert
	assert.That(t, "error", err, nil)
}

func Test_Decode_With_BracketsInString_Should_NotMiscount(t *testing.T) {
	t.Parallel()

	// Arrange — string values contain `:`, `{`, `}`, `[`, `]`. These bytes
	// inside string literals must not affect key-count or depth tracking.
	input := `{"jsonrpc":"2.0","method":"x","id":1,"params":{"a":"x:y","b":"}","c":"{","d":"[","e":"]","f":":::::"}}` + "\n"

	// Act
	err := noPanic(t, input)

	// Assert
	assert.That(t, "error", err, nil)
}

func Test_Decode_With_UnbalancedCloseBracket_Should_ReturnErrorNotPanic(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		`]` + "\n",
		`]]]]` + "\n",
		`}` + "\n",
		`}}}}` + "\n",
	} {
		err := noPanic(t, input)
		if err == nil {
			t.Fatalf("expected error for unbalanced input %q", input)
		}
	}
}

func Test_Decode_With_MixedDelimiters_Should_ReturnErrorNotPanic(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		`[}` + "\n",
		`{]` + "\n",
		`{[}` + "\n",
		`[{]}` + "\n",
	} {
		err := noPanic(t, input)
		if err == nil {
			t.Fatalf("expected error for mixed delimiter input %q", input)
		}
	}
}

func Test_Decode_With_DepthAtBoundary_Should_RejectAtMaxPlusOne(t *testing.T) {
	t.Parallel()

	// Arrange — wrap MaxJSONDepth + 1 nested arrays inside a valid envelope so
	// the batch-detection path does not fire. The envelope adds three opening
	// brackets ({, params{, a:[) which alone stay under the limit; the deep
	// chain of `[` triggers the depth guard before any slice index write.
	var sb strings.Builder
	sb.WriteString(`{"jsonrpc":"2.0","method":"x","params":{"a":`)
	for range protocol.MaxJSONDepth + 1 {
		sb.WriteByte('[')
	}
	for range protocol.MaxJSONDepth + 1 {
		sb.WriteByte(']')
	}
	sb.WriteString("}}\n")

	// Act
	err := noPanic(t, sb.String())

	// Assert
	if err == nil {
		t.Fatal("expected depth error at MaxJSONDepth+1")
	}
	if !errors.Is(err, protocol.ErrJSONDepthExceeded) {
		t.Fatalf("expected ErrJSONDepthExceeded, got %v", err)
	}
}

func Test_Decode_With_EscapedBracketInString_Should_NotDecrementDepth(t *testing.T) {
	t.Parallel()

	// Arrange — escaped quotes and brackets inside a string. The escape state
	// must consume the next byte without affecting depth or key counts.
	input := `{"jsonrpc":"2.0","method":"x","id":1,"params":{"a":"\"]","b":"\"}","c":"\\"}}` + "\n"

	// Act
	err := noPanic(t, input)

	// Assert
	assert.That(t, "error", err, nil)
}

func Test_Decode_With_KeysAtLimit_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Arrange — exactly MaxJSONKeysPerObject keys (boundary case).
	var sb strings.Builder
	sb.WriteString(`{"jsonrpc":"2.0","method":"x","id":1,"params":{`)
	for i := range protocol.MaxJSONKeysPerObject {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `"k%d":%d`, i, i)
	}
	sb.WriteString("}}\n")

	// Act
	err := noPanic(t, sb.String())

	// Assert
	assert.That(t, "error", err, nil)
}

func Test_Decode_With_StringAtLimit_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Arrange — string of exactly MaxJSONStringLen raw bytes (boundary case).
	value := strings.Repeat("a", protocol.MaxJSONStringLen)
	input := `{"jsonrpc":"2.0","method":"x","id":1,"params":{"data":"` + value + `"}}` + "\n"

	// Act
	err := noPanic(t, input)

	// Assert
	assert.That(t, "error", err, nil)
}

func Test_StructuralLimitError_Should_FormatExactWireMessage(t *testing.T) {
	t.Parallel()

	// Arrange
	sle := &protocol.StructuralLimitError{
		Limit:  "maxKeysPerObject",
		Actual: 10_001,
		Max:    10_000,
	}

	// Assert
	assert.That(t, "error message", sle.Error(), "payload exceeds maxKeysPerObject: 10001 > 10000")
}

func Test_StructuralLimitError_With_ErrorsAs_Should_Match(t *testing.T) {
	t.Parallel()

	// Arrange
	wrapped := fmt.Errorf("decode message: %w", &protocol.StructuralLimitError{
		Limit:  "maxStringLength",
		Actual: 1_048_577,
		Max:    1_048_576,
	})

	// Act
	var sle *protocol.StructuralLimitError
	ok := errors.As(wrapped, &sle)

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "limit", sle.Limit, "maxStringLength")
	assert.That(t, "actual", sle.Actual, 1_048_577)
	assert.That(t, "max", sle.Max, 1_048_576)
}

// Regression: backslash bytes inside strings must count toward MaxJSONStringLen
// so an escape-heavy payload cannot bypass the cap. Pre-fix, `\\` counted as 1
// byte instead of 2, letting a 2 MiB string of `\\` slip past a 1 MiB cap.
func Test_Decode_With_EscapeHeavyString_Should_CountBackslashBytes(t *testing.T) {
	t.Parallel()

	// Arrange — string content of N pairs of `\\`. Raw byte count between the
	// quotes is 2N. Choose N so 2N > MaxJSONStringLen but N <= MaxJSONStringLen,
	// proving the cap trips on raw bytes (post-fix) and would have passed if
	// only logical characters were counted (pre-fix bug).
	pairs := protocol.MaxJSONStringLen/2 + 1
	value := strings.Repeat(`\\`, pairs)
	input := `{"jsonrpc":"2.0","method":"x","id":1,"params":{"data":"` + value + `"}}` + "\n"

	// Act
	err := noPanic(t, input)

	// Assert
	if err == nil {
		t.Fatal("expected structural-limit error for escape-heavy string")
	}
	var sle *protocol.StructuralLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("expected *StructuralLimitError, got %T: %v", err, err)
	}
	assert.That(t, "limit", sle.Limit, "maxStringLength")
}
