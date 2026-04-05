package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/protocol"
)

// CI fuzzing strategy:
//
//	go test -fuzz Fuzz_Decoder ./internal/protocol -fuzztime=${FUZZ_TIME:-30s}
//
// Set FUZZ_TIME env var to control duration. Default 30s for CI, longer for nightly runs.
// Corpus is committed to testdata/fuzz/ — new findings are added automatically by go test.
func Fuzz_Decoder_With_ArbitraryInput(f *testing.F) {
	// Seed corpus
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1,"params":{}}`)
	f.Add(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	f.Add(`[{"jsonrpc":"2.0","method":"ping","id":1}]`)
	f.Add(``)
	f.Add(`null`)
	f.Add(`{"jsonrpc":"2.0","method":"pi`)
	f.Add(`{"jsonrpc":"2.0","id":null}`)
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":"str","params":null}`)
	f.Add(`42`)
	f.Add(`true`)
	f.Add(`"just a string"`)

	// Empty JSON object — valid JSON but missing required fields
	f.Add(`{}`)
	// Unicode method name — non-ASCII characters in method field
	f.Add(`{"jsonrpc":"2.0","method":"\u00e9cho","id":1,"params":{}}`)
	// Extra unknown fields — decoder should ignore gracefully
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1,"params":{},"extra":"field","another":123}`)
	// Large integer ID — tests numeric precision in json.RawMessage
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":999999999999999999,"params":{}}`)
	// Negative ID — unusual but valid JSON number
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":-1,"params":{}}`)
	// Float ID — valid JSON number, unusual for JSON-RPC
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1.5,"params":{}}`)
	// Leading whitespace before JSON object
	f.Add(`  {"jsonrpc":"2.0","method":"ping","id":1,"params":{}}`)
	// Trailing whitespace after JSON object
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1,"params":{}}  `)
	// Newlines and tabs within JSON (pretty-printed)
	f.Add("{\n\t\"jsonrpc\":\"2.0\",\n\t\"method\":\"ping\",\n\t\"id\":1,\n\t\"params\":{}\n}")
	// Deeply nested params
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1,"params":{"a":{"b":{"c":{"d":{}}}}}}`)
	// Empty string method
	f.Add(`{"jsonrpc":"2.0","method":"","id":1,"params":{}}`)
	// Very long method name
	f.Add(`{"jsonrpc":"2.0","method":"` + strings.Repeat("a", 1000) + `","id":1,"params":{}}`)
	// Zero ID — boundary for numeric IDs
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":0,"params":{}}`)
	// Response with result field — decoder should handle gracefully
	f.Add(`{"jsonrpc":"2.0","result":{"success":true},"id":1}`)
	// Error response object — decoder should handle gracefully
	f.Add(`{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":1}`)
	// Scientific notation ID — must be rejected
	f.Add(`{"jsonrpc":"2.0","method":"ping","id":1e308,"params":{}}`)

	f.Fuzz(func(_ *testing.T, input string) {
		dec := json.NewDecoder(strings.NewReader(input))
		// Must not panic — errors are acceptable
		_, _ = protocol.Decode(dec)
	})
}
