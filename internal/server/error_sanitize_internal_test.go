package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/tools"
)

func Test_errorResponse_With_NonCodeError_Should_SanitizeMessage(t *testing.T) {
	t.Parallel()

	// Arrange
	var stderr bytes.Buffer
	srv := NewServer("mcp", "test", tools.NewRegistry(), strings.NewReader(""), &bytes.Buffer{}, &stderr)

	// Act — pass a raw error with internal details
	resp := srv.errorResponse(context.Background(), json.RawMessage(`1`), errors.New("open /etc/shadow: permission denied"))

	// Assert — client should NOT see the raw error
	assert.That(t, "code", resp.Error.Code, protocol.InternalError)
	if strings.Contains(resp.Error.Message, "/etc/shadow") {
		t.Errorf("error message leaks internal details: %s", resp.Error.Message)
	}
	assert.That(t, "sanitized message", resp.Error.Message, "internal error")

	// Verify the real error was logged to stderr
	if !strings.Contains(stderr.String(), "/etc/shadow") {
		t.Error("expected internal error details to be logged to stderr")
	}
}

func Test_sanitizeErrorID_Should_NormalizeMalformedIDs(t *testing.T) {
	t.Parallel()

	// Arrange — table of (input, expected output) pairs covering valid types
	// (null, number, negative number, string) and structurally invalid types
	// (boolean, array, object). Per JSON-RPC 2.0 §5, invalid ids must surface
	// as null on the wire.
	const jsonNull = "null"
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty (notification)", "", ""},
		{"null", jsonNull, jsonNull},
		{"positive int", "42", "42"},
		{"negative int", "-7", "-7"},
		{"zero", "0", "0"},
		{"string", `"abc"`, `"abc"`},
		{"boolean true", "true", jsonNull},
		{"boolean false", "false", jsonNull},
		{"array", "[1,2]", jsonNull},
		{"object", `{"a":1}`, jsonNull},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := sanitizeErrorID(json.RawMessage(tc.in))

			// Assert
			assert.That(t, "sanitized id", string(got), tc.want)
		})
	}
}
