package tools_test

import (
	"context"
	"testing"

	"github.com/andygeiss/mcp/internal/pkg/assert"
	"github.com/andygeiss/mcp/internal/tools"
)

type stubInput struct {
	Message string `json:"message" description:"test message"`
}

func Test_Register_With_DuplicateName_Should_Panic(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "dup", "desc", func(_ context.Context, _ stubInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Act / Assert
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("expected panic for duplicate tool name")
		}
		assert.That(t, "panic message", rec, "duplicate tool name: dup")
	}()
	tools.Register(r, "dup", "dup", func(_ context.Context, _ stubInput) tools.Result {
		return tools.TextResult("dup")
	})
}

func Test_Tools_With_MultipleTools_Should_ReturnAlphabetically(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "zeta", "z desc", func(_ context.Context, _ stubInput) tools.Result {
		return tools.TextResult("z")
	})
	tools.Register(r, "alpha", "a desc", func(_ context.Context, _ stubInput) tools.Result {
		return tools.TextResult("a")
	})

	// Act
	result := r.Tools()

	// Assert
	assert.That(t, "count", len(result), 2)
	assert.That(t, "first", result[0].Name, "alpha")
	assert.That(t, "second", result[1].Name, "zeta")
}

func Test_Lookup_With_ExistingTool_Should_ReturnTool(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "stub", "stub tool", func(_ context.Context, input stubInput) tools.Result {
		return tools.TextResult(input.Message)
	})

	// Act
	tool, ok := r.Lookup("stub")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "name", tool.Name, "stub")
}

func Test_Lookup_With_UnknownTool_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act
	_, ok := r.Lookup("nonexistent")

	// Assert
	assert.That(t, "found", ok, false)
}

func Test_Names_With_MultipleTools_Should_ReturnAlphabetical(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "beta", "b desc", func(_ context.Context, _ stubInput) tools.Result {
		return tools.TextResult("b")
	})
	tools.Register(r, "alpha", "a desc", func(_ context.Context, _ stubInput) tools.Result {
		return tools.TextResult("a")
	})

	// Act
	names := r.Names()

	// Assert
	assert.That(t, "count", len(names), 2)
	assert.That(t, "first", names[0], "alpha")
	assert.That(t, "second", names[1], "beta")
}

func Test_Names_With_EmptyRegistry_Should_ReturnEmptySlice(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act
	names := r.Names()

	// Assert
	assert.That(t, "length", len(names), 0)
	if names == nil {
		t.Fatal("expected empty slice, got nil")
	}
}

func Test_TextResult_Should_SetContentType(t *testing.T) {
	t.Parallel()

	// Act
	result := tools.TextResult("hello")

	// Assert
	assert.That(t, "content length", len(result.Content), 1)
	assert.That(t, "type", result.Content[0].Type, "text")
	assert.That(t, "text", result.Content[0].Text, "hello")
	assert.That(t, "isError", result.IsError, false)
}

func Test_ErrorResult_Should_SetIsError(t *testing.T) {
	t.Parallel()

	// Act
	result := tools.ErrorResult("something went wrong")

	// Assert
	assert.That(t, "content length", len(result.Content), 1)
	assert.That(t, "type", result.Content[0].Type, "text")
	assert.That(t, "text", result.Content[0].Text, "something went wrong")
	assert.That(t, "isError", result.IsError, true)
}
