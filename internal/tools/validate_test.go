package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/tools"
)

func Test_ValidatePath_With_TraversalSegment_Should_ReturnError(t *testing.T) {
	t.Parallel()

	err := tools.ValidatePath("../../etc/passwd")

	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	assert.That(t, "message", err.Error(), "path traversal not allowed")
}

func Test_ValidatePath_With_DotDotOnly_Should_ReturnError(t *testing.T) {
	t.Parallel()

	err := tools.ValidatePath("..")

	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	assert.That(t, "message", err.Error(), "path traversal not allowed")
}

func Test_ValidatePath_With_NormalizedTraversal_Should_ReturnError(t *testing.T) {
	t.Parallel()

	err := tools.ValidatePath("foo/../../etc/passwd")

	if err == nil {
		t.Fatal("expected error for normalized path traversal")
	}
	assert.That(t, "message", err.Error(), "path traversal not allowed")
}

func Test_ValidatePath_With_NullByte_Should_ReturnError(t *testing.T) {
	t.Parallel()

	err := tools.ValidatePath("foo\x00bar")

	if err == nil {
		t.Fatal("expected error for null byte")
	}
	assert.That(t, "message", err.Error(), "path contains null byte")
}

func Test_ValidatePath_With_ExcessiveLength_Should_ReturnError(t *testing.T) {
	t.Parallel()

	err := tools.ValidatePath(strings.Repeat("a", tools.MaxInputLength+1))

	if err == nil {
		t.Fatal("expected error for excessive length")
	}
}

func Test_ValidatePath_With_ValidPath_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	err := tools.ValidatePath("src/main.go")

	assert.That(t, "error", err, nil)
}

func Test_ValidatePath_With_DotDotInFilename_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	// A file literally named "foo..bar" is not traversal.
	err := tools.ValidatePath("foo..bar")

	assert.That(t, "error", err, nil)
}

func Test_ValidateInput_With_NullByte_Should_ReturnError(t *testing.T) {
	t.Parallel()

	err := tools.ValidateInput("hello\x00world")

	if err == nil {
		t.Fatal("expected error for null byte")
	}
}

func Test_ValidateInput_With_ExcessiveLength_Should_ReturnError(t *testing.T) {
	t.Parallel()

	err := tools.ValidateInput(strings.Repeat("x", tools.MaxInputLength+1))

	if err == nil {
		t.Fatal("expected error for excessive length")
	}
}

func Test_ValidateInput_With_ValidInput_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	err := tools.ValidateInput("func main()")

	assert.That(t, "error", err, nil)
}

type decodeErrorInput struct {
	Value string `json:"value"`
}

func Test_UnmarshalAndValidate_With_InvalidJSON_Should_ReturnDecodeError(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "decode-err", "decode error tool", func(_ context.Context, _ decodeErrorInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}
	tool, ok := r.Lookup("decode-err")
	if !ok {
		t.Fatal("tool not found")
	}

	// Act
	_, err := tool.Handler(context.Background(), json.RawMessage(`not json`))

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid JSON params")
	}
	if !strings.Contains(err.Error(), "invalid arguments") {
		t.Errorf("error message should contain \"invalid arguments\", got: %s", err.Error())
	}
}
