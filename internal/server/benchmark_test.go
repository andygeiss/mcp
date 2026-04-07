package server_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/server"
)

func Benchmark_RequestResponse_EchoTool(b *testing.B) {
	registry := testRegistry()
	input := handshake() +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"test","arguments":{"message":"bench"}}}` + "\n"
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var out bytes.Buffer
		srv := server.NewServer("mcp", "test", registry, strings.NewReader(input), &out, io.Discard)
		_ = srv.Run(context.Background())
	}
}

func Benchmark_RequestResponse_MultipleToolCalls(b *testing.B) {
	registry := testRegistry()
	var sb strings.Builder
	sb.WriteString(handshake())
	for i := range 10 {
		fmt.Fprintf(&sb, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"test","arguments":{"message":"bench"}}}`, i+2)
		sb.WriteByte('\n')
	}
	input := sb.String()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var out bytes.Buffer
		srv := server.NewServer("mcp", "test", registry, strings.NewReader(input), &out, io.Discard)
		_ = srv.Run(context.Background())
	}
}

func Benchmark_RequestResponse_PingOnly(b *testing.B) {
	registry := testRegistry()
	input := handshake() +
		`{"jsonrpc":"2.0","id":2,"method":"ping","params":{}}` + "\n"
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var out bytes.Buffer
		srv := server.NewServer("mcp", "test", registry, strings.NewReader(input), &out, io.Discard)
		_ = srv.Run(context.Background())
	}
}
