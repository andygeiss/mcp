package tools_test

import (
	"context"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/tools"
)

func Test_Echo_With_ValidMessage_Should_ReturnTextResult(t *testing.T) {
	t.Parallel()

	// Arrange
	input := tools.EchoInput{Message: "hello"}

	// Act
	result := tools.Echo(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, false)
	assert.That(t, "text", result.Content[0].Text, "hello")
}

func Test_Echo_With_EmptyMessage_Should_ReturnEmptyTextResult(t *testing.T) {
	t.Parallel()

	// Arrange
	input := tools.EchoInput{Message: ""}

	// Act
	result := tools.Echo(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, false)
	assert.That(t, "text", result.Content[0].Text, "")
}
