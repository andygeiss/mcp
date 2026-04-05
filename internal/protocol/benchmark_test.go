package protocol_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/protocol"
)

func Benchmark_Decode_With_ValidRequest(b *testing.B) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"message":"hello"}}}` + "\n"
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		dec := json.NewDecoder(strings.NewReader(input))
		_, _ = protocol.Decode(dec)
	}
}

func Benchmark_Decode_With_LargeParams(b *testing.B) {
	payload := strings.Repeat("x", 1024)
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"message":"` + payload + `"}}}` + "\n"
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		dec := json.NewDecoder(strings.NewReader(input))
		_, _ = protocol.Decode(dec)
	}
}

func Benchmark_Encode_With_SuccessResponse(b *testing.B) {
	resp := protocol.Response{
		ID:      json.RawMessage("1"),
		JSONRPC: "2.0",
		Result:  json.RawMessage(`{"content":[{"type":"text","text":"hello"}]}`),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		_ = protocol.Encode(enc, resp)
	}
}

func Benchmark_Encode_With_ErrorResponse(b *testing.B) {
	resp := protocol.NewErrorResponse(json.RawMessage("1"), protocol.InvalidParams, "unknown tool: nonexistent")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		_ = protocol.Encode(enc, resp)
	}
}
