package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/andygeiss/mcp/internal/pkg/assert"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/server"
)

// closingWriter fails with io.ErrClosedPipe after failAt successful writes.
type closingWriter struct {
	buf    bytes.Buffer
	count  int
	failAt int
}

func (w *closingWriter) Write(p []byte) (int, error) {
	w.count++
	if w.count > w.failAt {
		return 0, io.ErrClosedPipe
	}
	return w.buf.Write(p)
}

// slowReader delivers one byte at a time from the underlying data.
type slowReader struct {
	data []byte
	pos  int
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	p[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}

// partialReader delivers data in small, arbitrary-sized chunks.
type partialReader struct {
	data      []byte
	pos       int
	chunkSize int
}

func (r *partialReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := r.chunkSize
	remaining := len(r.data) - r.pos
	if n > remaining {
		n = remaining
	}
	if n > len(p) {
		n = len(p)
	}
	copy(p, r.data[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}

func Test_Server_With_StdoutClosedMidWrite_Should_NotHang(t *testing.T) {
	t.Parallel()

	// Arrange — stdout fails after the first write (initialize response)
	input := handshake() +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"test","arguments":{"message":"test"}}}` + "\n"

	cw := &closingWriter{failAt: 1}
	var stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), strings.NewReader(input), cw, &stderr)

	// Act — must not hang; use timeout to guarantee
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := srv.Run(ctx)

	// Assert — server returns an error (encode failure), does not hang
	if err == nil {
		t.Fatal("expected error when stdout is closed")
	}
}

func Test_Server_With_SlowStdin_Should_CompleteCorrectly(t *testing.T) {
	t.Parallel()

	// Arrange — deliver bytes one at a time
	input := initRequest + initializedNotification +
		`{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n"

	sr := &slowReader{data: []byte(input)}
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), sr, &stdout, &stderr)

	// Act
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := srv.Run(ctx)

	// Assert
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}
	assert.That(t, "response count", len(responses), 2) // init + ping
	assert.That(t, "ping result", string(responses[1].Result), "{}")
}

func Test_Server_With_PartialReads_Should_DecodeCorrectly(t *testing.T) {
	t.Parallel()

	// Arrange — split messages into 3-byte chunks
	input := initRequest + initializedNotification +
		`{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"test","arguments":{"message":"partial"}}}` + "\n"

	pr := &partialReader{data: []byte(input), chunkSize: 3}
	var stdout, stderr bytes.Buffer
	srv := server.NewServer("mcp", "test", testRegistry(), pr, &stdout, &stderr)

	// Act
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := srv.Run(ctx)

	// Assert
	assert.That(t, "error", err, nil)

	var responses []protocol.Response
	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if uerr := dec.Decode(&resp); uerr != nil {
			break
		}
		responses = append(responses, resp)
	}
	assert.That(t, "response count", len(responses), 2) // init + echo

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	err = json.Unmarshal(responses[1].Result, &result)
	assert.That(t, "unmarshal", err, nil)
	assert.That(t, "text", result.Content[0].Text, "partial")
}
