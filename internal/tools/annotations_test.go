package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/pkg/assert"
	"github.com/andygeiss/mcp/internal/tools"
)

func Test_WithAnnotations_With_ReadOnlyHint_Should_SetAnnotation(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act
	tools.Register(r, "annotated", "test", func(_ context.Context, _ struct{}) tools.Result {
		return tools.TextResult("ok")
	}, tools.WithAnnotations(tools.Annotations{ReadOnlyHint: true}))

	// Assert
	tool, ok := r.Lookup("annotated")
	assert.That(t, "found", ok, true)
	if tool.Annotations == nil {
		t.Fatal("expected non-nil annotations")
	}
	assert.That(t, "readOnlyHint", tool.Annotations.ReadOnlyHint, true)
}

func Test_Register_With_NoOptions_Should_HaveNilAnnotations(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act
	tools.Register(r, "plain", "test", func(_ context.Context, _ struct{}) tools.Result {
		return tools.TextResult("ok")
	})

	// Assert
	tool, ok := r.Lookup("plain")
	assert.That(t, "found", ok, true)
	assert.That(t, "annotations", tool.Annotations == nil, true)
}

func Test_Annotations_With_NilPointer_Should_OmitFromJSON(t *testing.T) {
	t.Parallel()

	// Arrange
	tool := tools.Tool{
		Description: "test",
		Name:        "test",
	}

	// Act
	data, err := json.Marshal(tool)

	// Assert
	assert.That(t, "error", err, nil)
	if strings.Contains(string(data), "annotations") {
		t.Fatalf("expected no annotations key, got: %s", data)
	}
}

func Test_Annotations_With_NonNilPointer_Should_IncludeInJSON(t *testing.T) {
	t.Parallel()

	// Arrange
	tool := tools.Tool{
		Annotations: &tools.Annotations{ReadOnlyHint: true},
		Description: "test",
		Name:        "test",
	}

	// Act
	data, err := json.Marshal(tool)

	// Assert
	assert.That(t, "error", err, nil)
	s := string(data)
	if !strings.Contains(s, `"annotations":{"readOnlyHint":true}`) {
		t.Fatalf("expected readOnlyHint annotation, got: %s", s)
	}
	if strings.Contains(s, "destructiveHint") {
		t.Fatalf("expected no destructiveHint, got: %s", s)
	}
}

func Test_Annotations_With_AllFields_Should_SerializeCorrectly(t *testing.T) {
	t.Parallel()

	// Arrange
	a := tools.Annotations{
		DestructiveHint: true,
		IdempotentHint:  true,
		OpenWorldHint:   true,
		ReadOnlyHint:    true,
		Title:           "My Tool",
	}

	// Act
	data, err := json.Marshal(a)

	// Assert
	assert.That(t, "error", err, nil)
	s := string(data)
	for _, field := range []string{"destructiveHint", "idempotentHint", "openWorldHint", "readOnlyHint", "title"} {
		if !strings.Contains(s, field) {
			t.Errorf("expected %s in JSON, got: %s", field, s)
		}
	}
}
