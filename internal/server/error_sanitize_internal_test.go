package server

import (
	"bytes"
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
	resp := srv.errorResponse(json.RawMessage(`1`), errors.New("open /etc/shadow: permission denied"))

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
