package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
)

func Test_loggerFromContext_With_NoLogger_Should_ReturnFallback(t *testing.T) {
	t.Parallel()

	// Arrange
	fallback := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	// Act
	got := loggerFromContext(context.Background(), fallback)

	// Assert
	assert.That(t, "logger", got, fallback)
}

func Test_withRequestLogger_With_NonEmptyID_Should_AttachRequestID(t *testing.T) {
	t.Parallel()

	// Arrange — capture log output to a buffer so we can assert the
	// request_id attribute is present.
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))
	ctx := withRequestLogger(context.Background(), base, json.RawMessage("42"))

	// Act
	loggerFromContext(ctx, base).Info("test_event", "field", "value")

	// Assert — the JSON log line carries both the user attribute and the
	// auto-injected request_id.
	if !strings.Contains(buf.String(), `"request_id":"42"`) {
		t.Fatalf("expected request_id in log output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"field":"value"`) {
		t.Fatalf("expected user attribute, got: %s", buf.String())
	}
}

func Test_withRequestLogger_With_EmptyID_Should_NotAttachRequestID(t *testing.T) {
	t.Parallel()

	// Arrange — notification path: id is empty, no request_id should attach.
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))
	ctx := withRequestLogger(context.Background(), base, nil)

	// Act
	loggerFromContext(ctx, base).Info("test_event")

	// Assert
	if strings.Contains(buf.String(), `"request_id"`) {
		t.Fatalf("expected NO request_id in log output, got: %s", buf.String())
	}
}
