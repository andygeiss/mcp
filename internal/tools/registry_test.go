package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/tools"
)

type stubInput struct {
	Message string `json:"message" description:"test message"`
}

func Test_Register_With_DuplicateName_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "dup", "desc", func(_ context.Context, _ stubInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	err := tools.Register(r, "dup", "dup", func(_ context.Context, _ stubInput) tools.Result {
		return tools.TextResult("dup")
	})

	// Assert
	if err == nil {
		t.Fatal("expected error for duplicate tool name")
	}
	if !strings.Contains(err.Error(), "duplicate tool name: dup") {
		t.Errorf("error should mention duplicate, got: %s", err.Error())
	}
}

func Test_Tools_With_MultipleTools_Should_ReturnAlphabetically(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "zeta", "z desc", func(_ context.Context, _ stubInput) tools.Result {
		return tools.TextResult("z")
	}); err != nil {
		t.Fatal(err)
	}
	if err := tools.Register(r, "alpha", "a desc", func(_ context.Context, _ stubInput) tools.Result {
		return tools.TextResult("a")
	}); err != nil {
		t.Fatal(err)
	}

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
	if err := tools.Register(r, "stub", "stub tool", func(_ context.Context, input stubInput) tools.Result {
		return tools.TextResult(input.Message)
	}); err != nil {
		t.Fatal(err)
	}

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
	if err := tools.Register(r, "beta", "b desc", func(_ context.Context, _ stubInput) tools.Result {
		return tools.TextResult("b")
	}); err != nil {
		t.Fatal(err)
	}
	if err := tools.Register(r, "alpha", "a desc", func(_ context.Context, _ stubInput) tools.Result {
		return tools.TextResult("a")
	}); err != nil {
		t.Fatal(err)
	}

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

func Test_Handler_With_MissingRequiredField_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "greet", "greets", func(_ context.Context, input stubInput) tools.Result {
		return tools.TextResult("hello " + input.Message)
	}); err != nil {
		t.Fatal(err)
	}
	tool, _ := r.Lookup("greet")

	// Act — "message" is required (no omitempty) but missing from params
	_, err := tool.Handler(context.Background(), json.RawMessage(`{}`))

	// Assert
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	if !strings.Contains(err.Error(), "missing required field") {
		t.Errorf("error should mention missing required field, got: %s", err.Error())
	}
}

func Test_Handler_With_AllRequiredFields_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "greet", "greets", func(_ context.Context, input stubInput) tools.Result {
		return tools.TextResult("hello " + input.Message)
	}); err != nil {
		t.Fatal(err)
	}
	tool, _ := r.Lookup("greet")

	// Act
	result, err := tool.Handler(context.Background(), json.RawMessage(`{"message":"world"}`))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "text", result.Content[0].Text, "hello world")
}

type optionalInput struct {
	Name string `json:"name,omitempty" description:"optional name"`
}

func Test_Handler_With_MissingOptionalField_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "opt", "optional", func(_ context.Context, input optionalInput) tools.Result {
		if input.Name == "" {
			return tools.TextResult("anonymous")
		}
		return tools.TextResult(input.Name)
	}); err != nil {
		t.Fatal(err)
	}
	tool, _ := r.Lookup("opt")

	// Act — "name" is optional (has omitempty), so {} should succeed
	result, err := tool.Handler(context.Background(), json.RawMessage(`{}`))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "text", result.Content[0].Text, "anonymous")
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
