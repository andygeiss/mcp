package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/schema"
	"github.com/andygeiss/mcp/internal/tools"
)

// Test fixture filler strings hoisted so the goconst linter does not flag the
// same literal across tools_test fixture builders.
const (
	testFixture    = "test"
	countFieldName = "count"
)

func Test_WithAnnotations_With_ReadOnlyHint_Should_SetAnnotation(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act
	if err := tools.Register(r, "annotated", testFixture, func(_ context.Context, _ struct{}) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult("ok")
	}, tools.WithAnnotations(tools.Annotations{ReadOnlyHint: true})); err != nil {
		t.Fatal(err)
	}

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
	if err := tools.Register(r, "plain", testFixture, func(_ context.Context, _ struct{}) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Assert
	tool, ok := r.Lookup("plain")
	assert.That(t, "found", ok, true)
	assert.That(t, "annotations", tool.Annotations == nil, true)
}

func Test_Annotations_With_NilPointer_Should_OmitFromJSON(t *testing.T) {
	t.Parallel()

	// Arrange
	tool := tools.Tool{
		Description: testFixture,
		Name:        testFixture,
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
		Description: testFixture,
		Name:        testFixture,
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

func Test_WithOutputSchema_With_Schema_Should_SetOutputSchema(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	out := schema.OutputSchema{
		Type: "object",
		Properties: map[string]schema.Property{
			countFieldName: {Type: "integer", Description: "item count"},
		},
		Required: []string{countFieldName},
	}

	// Act
	if err := tools.Register(r, "structured", testFixture, func(_ context.Context, _ struct{}) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult("ok")
	}, tools.WithOutputSchema(out)); err != nil {
		t.Fatal(err)
	}

	// Assert
	tool, ok := r.Lookup("structured")
	assert.That(t, "found", ok, true)
	if tool.OutputSchema == nil {
		t.Fatal("expected non-nil outputSchema")
	}
	assert.That(t, "type", tool.OutputSchema.Type, "object")
	assert.That(t, "required", len(tool.OutputSchema.Required), 1)
}

type derivableOut struct {
	Auto string `json:"auto" description:"reflection-derived field"`
}

func Test_WithOutputSchema_Should_OverrideAutoDerivedSchema(t *testing.T) {
	t.Parallel()

	// Arrange — register with both an Out type that yields a non-empty
	// reflection schema AND an explicit WithOutputSchema option carrying a
	// distinct hand-authored schema. The option must win — that's the
	// documented escape hatch in WithOutputSchema's godoc.
	r := tools.NewRegistry()
	override := schema.OutputSchema{
		Type: "object",
		Properties: map[string]schema.Property{
			"manual": {Type: "string", Description: "hand-authored field"},
		},
		Required: []string{"manual"},
	}
	if err := tools.Register(r, "override", testFixture,
		func(_ context.Context, _ struct{}) (derivableOut, tools.Result) {
			return derivableOut{Auto: "x"}, tools.Result{}
		},
		tools.WithOutputSchema(override),
	); err != nil {
		t.Fatal(err)
	}

	// Assert — the option-supplied schema wins; the derivable Out's "auto"
	// property is gone, replaced by the override's "manual" property.
	tool, _ := r.Lookup("override")
	if tool.OutputSchema == nil {
		t.Fatal("expected non-nil outputSchema")
	}
	_, hasManual := tool.OutputSchema.Properties["manual"]
	_, hasAuto := tool.OutputSchema.Properties["auto"]
	assert.That(t, "override schema has 'manual' property", hasManual, true)
	assert.That(t, "override schema does NOT carry derived 'auto' property", hasAuto, false)
}

func Test_Tool_With_NoOutputSchema_Should_OmitFromJSON(t *testing.T) {
	t.Parallel()

	// Arrange
	tool := tools.Tool{Description: testFixture, Name: testFixture}

	// Act
	data, err := json.Marshal(tool)

	// Assert
	assert.That(t, "error", err, nil)
	if strings.Contains(string(data), "outputSchema") {
		t.Fatalf("expected no outputSchema key, got: %s", data)
	}
}

func Test_WithTitle_With_DisplayName_Should_SetToolTitle(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act
	if err := tools.Register(r, "titled", testFixture, func(_ context.Context, _ struct{}) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult("ok")
	}, tools.WithTitle("My Display Name")); err != nil {
		t.Fatal(err)
	}

	// Assert
	tool, ok := r.Lookup("titled")
	assert.That(t, "found", ok, true)
	assert.That(t, "title", tool.Title, "My Display Name")
}

func Test_Tool_With_NoTitle_Should_OmitTitleFromJSON(t *testing.T) {
	t.Parallel()

	// Arrange
	tool := tools.Tool{Description: testFixture, Name: testFixture}

	// Act
	data, err := json.Marshal(tool)

	// Assert
	assert.That(t, "error", err, nil)
	if strings.Contains(string(data), "title") {
		t.Fatalf("expected no title key, got: %s", data)
	}
}

func Test_Tool_With_Title_Should_IncludeTitleInJSON(t *testing.T) {
	t.Parallel()

	// Arrange
	tool := tools.Tool{Description: testFixture, Name: testFixture, Title: "Display Name"}

	// Act
	data, err := json.Marshal(tool)

	// Assert
	assert.That(t, "error", err, nil)
	if !strings.Contains(string(data), `"title":"Display Name"`) {
		t.Fatalf("expected title in JSON, got: %s", data)
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
