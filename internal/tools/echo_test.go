package tools_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/tools"
)

func Test_Echo_With_ValidMessage_Should_ReturnStructuredOutput(t *testing.T) {
	t.Parallel()

	// Arrange
	input := tools.EchoInput{Message: "hello"}

	// Act — Echo emits structured Out only; the dispatch wrapper auto-
	// marshals it into StructuredContent. No legacy text content.
	out, result := tools.Echo(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, false)
	assert.That(t, "no legacy content blocks", len(result.Content), 0)
	assert.That(t, "echoed", out.Echoed, "hello")
}

func Test_Echo_With_EmptyMessage_Should_ReturnEmptyStructuredOutput(t *testing.T) {
	t.Parallel()

	// Arrange
	input := tools.EchoInput{Message: ""}

	// Act
	out, result := tools.Echo(context.Background(), input)

	// Assert — empty message still yields a zero-value EchoOutput. The Out
	// value is the zero Out, which dispatch will skip (omitempty).
	assert.That(t, "isError", result.IsError, false)
	assert.That(t, "no legacy content blocks", len(result.Content), 0)
	assert.That(t, "echoed", out.Echoed, "")
}

func Test_EchoTool_Should_Contain_StartHere_Anchor(t *testing.T) {
	t.Parallel()

	// Arrange
	data, err := os.ReadFile("echo.go")
	if err != nil {
		t.Fatalf("read echo.go: %v", err)
	}

	// Act / Assert — the anchor MUST persist across refactors so new
	// scaffold-forkers always have a labeled entry point.
	if !bytes.Contains(data, []byte("// START HERE")) {
		t.Fatal("echo.go is missing the // START HERE anchor (Story 3.1 / FR5)")
	}
}
