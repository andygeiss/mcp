package protocol_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/pkg/assert"
	"github.com/andygeiss/mcp/internal/protocol"
)

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
		JSONRPC: "2.0",
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

func Test_CodeError_Should_ImplementErrorInterface(t *testing.T) {
	t.Parallel()

	// Arrange
	var err error = &protocol.CodeError{Code: protocol.InvalidParams, Message: "unknown tool: foo"}

	// Assert
	assert.That(t, "error message", err.Error(), "unknown tool: foo")
}

func Test_CodeError_With_ErrorsAsType_Should_Unwrap(t *testing.T) {
	t.Parallel()

	// Arrange
	err := fmt.Errorf("dispatch: %w", &protocol.CodeError{Code: protocol.InvalidParams, Message: "unknown tool: foo"})

	// Act
	pe, ok := errors.AsType[*protocol.CodeError](err)

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "code", pe.Code, protocol.InvalidParams)
	assert.That(t, "message", pe.Message, "unknown tool: foo")
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
		Message: "unknown tool: foo",
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
	assert.That(t, "error message", decoded.Error.Message, "unknown tool: foo")
	assert.That(t, "error data", string(decoded.Error.Data), `{"trace_id":"abc-123"}`)
}

// --- Validate tests ---

func Test_Validate_With_WrongVersion_Should_ReturnInvalidRequest(t *testing.T) {
	t.Parallel()

	// Arrange
	req := protocol.Request{
		JSONRPC: "1.0",
		Method:  "ping",
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
		JSONRPC: "2.0",
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
		JSONRPC: "2.0",
		Method:  "ping",
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
		JSONRPC: "2.0",
		Method:  "ping",
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
		JSONRPC: "2.0",
		Method:  "ping",
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
