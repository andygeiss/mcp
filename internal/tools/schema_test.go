package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/pkg/assert"
	"github.com/andygeiss/mcp/internal/tools"
)

type singleFieldInput struct {
	Message string `json:"message" description:"The message to process"`
}

func Test_DeriveSchema_With_SingleField_Should_ProduceCorrectSchema(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "stub", "stub tool", func(_ context.Context, input singleFieldInput) tools.Result {
		return tools.TextResult(input.Message)
	})

	// Act
	tool, ok := r.Lookup("stub")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "schema type", tool.InputSchema.Type, "object")
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 1)

	msgProp, exists := tool.InputSchema.Properties["message"]
	assert.That(t, "message property exists", exists, true)
	assert.That(t, "message type", msgProp.Type, "string")
	assert.That(t, "message description", msgProp.Description, "The message to process")
	assert.That(t, "required", tool.InputSchema.Required, []string{"message"})
}

type multiFieldInput struct {
	Count   int    `json:"count"`
	Name    string `json:"name"`
	Verbose bool   `json:"verbose,omitempty"`
}

func Test_DeriveSchema_With_MultipleFields_Should_DeriveAllTypes(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "multi", "multi-field tool", func(_ context.Context, _ multiFieldInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Act
	tool, ok := r.Lookup("multi")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 3)

	countProp := tool.InputSchema.Properties["count"]
	assert.That(t, "count type", countProp.Type, "integer")

	nameProp := tool.InputSchema.Properties["name"]
	assert.That(t, "name type", nameProp.Type, "string")

	verboseProp := tool.InputSchema.Properties["verbose"]
	assert.That(t, "verbose type", verboseProp.Type, "boolean")

	// verbose has omitempty, so only count and name should be required
	assert.That(t, "required count", len(tool.InputSchema.Required), 2)
}

type sliceFieldInput struct {
	Tags []string `json:"tags" description:"List of tags"`
}

func Test_DeriveSchema_With_SliceField_Should_ProduceArrayType(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "tagger", "tags tool", func(_ context.Context, _ sliceFieldInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Act
	tool, _ := r.Lookup("tagger")

	// Assert
	tagsProp := tool.InputSchema.Properties["tags"]
	assert.That(t, "tags type", tagsProp.Type, "array")
	if tagsProp.Items == nil {
		t.Fatal("expected items to be set for array type")
	}
	assert.That(t, "items type", tagsProp.Items.Type, "string")
}

type mapFieldInput struct {
	Metadata map[string]string `json:"metadata" description:"Key-value metadata"`
}

func Test_DeriveSchema_With_MapField_Should_ProduceObjectType(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "mapper", "map tool", func(_ context.Context, _ mapFieldInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Act
	tool, _ := r.Lookup("mapper")

	// Assert
	metaProp := tool.InputSchema.Properties["metadata"]
	assert.That(t, "metadata type", metaProp.Type, "object")
	if metaProp.AdditionalProperties == nil {
		t.Fatal("expected additionalProperties to be set for map type")
	}
	assert.That(t, "additionalProperties type", metaProp.AdditionalProperties.Type, "string")
}

type nestedConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type nestedStructInput struct {
	Config nestedConfig `json:"config" description:"Server config"`
}

func Test_DeriveSchema_With_NestedStruct_Should_ProduceNestedObject(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "nested", "nested tool", func(_ context.Context, _ nestedStructInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Act
	tool, _ := r.Lookup("nested")

	// Assert
	configProp := tool.InputSchema.Properties["config"]
	assert.That(t, "config type", configProp.Type, "object")
	assert.That(t, "config properties count", len(configProp.Properties), 2)

	hostProp := configProp.Properties["host"]
	assert.That(t, "host type", hostProp.Type, "string")

	portProp := configProp.Properties["port"]
	assert.That(t, "port type", portProp.Type, "integer")
}

type unsupportedInput struct {
	Ch chan int `json:"ch"`
}

func Test_DeriveSchema_With_UnsupportedType_Should_Panic(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act / Assert
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("expected panic for unsupported type")
		}
		msg, ok := rec.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T", rec)
		}
		if !strings.Contains(msg, "Ch") {
			t.Errorf("panic message should contain field name \"Ch\", got: %s", msg)
		}
		if !strings.Contains(msg, "chan") {
			t.Errorf("panic message should contain type \"chan\", got: %s", msg)
		}
	}()
	tools.Register(r, "bad", "bad tool", func(_ context.Context, _ unsupportedInput) tools.Result {
		return tools.TextResult("ok")
	})
}

type pointerFieldInput struct {
	Value *string `json:"value"`
}

func Test_DeriveSchema_With_PointerField_Should_UnwrapToUnderlyingType(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "ptr", "pointer tool", func(_ context.Context, _ pointerFieldInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Act
	tool, _ := r.Lookup("ptr")

	// Assert
	valProp := tool.InputSchema.Properties["value"]
	assert.That(t, "value type", valProp.Type, "string")
	assert.That(t, "required", tool.InputSchema.Required, []string{"value"})
}

type nestedSliceInput struct {
	Matrix [][]string `json:"matrix"`
}

func Test_DeriveSchema_With_NestedSlice_Should_ProduceNestedArraySchema(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "matrix", "nested slice", func(_ context.Context, _ nestedSliceInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Act
	tool, _ := r.Lookup("matrix")

	// Assert
	matrixProp := tool.InputSchema.Properties["matrix"]
	assert.That(t, "matrix type", matrixProp.Type, "array")
	if matrixProp.Items == nil {
		t.Fatal("expected items for outer array")
	}
	assert.That(t, "inner array type", matrixProp.Items.Type, "array")
	if matrixProp.Items.Items == nil {
		t.Fatal("expected items for inner array")
	}
	assert.That(t, "leaf type", matrixProp.Items.Items.Type, "string")
}

type dashTagInput struct {
	Hidden  string `json:"-"`
	Visible string `json:"visible"`
}

func Test_DeriveSchema_With_DashTag_Should_ExcludeField(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	tools.Register(r, "dash", "dash tag", func(_ context.Context, _ dashTagInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Act
	tool, _ := r.Lookup("dash")

	// Assert
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 1)
	_, hasHidden := tool.InputSchema.Properties["hidden"]
	assert.That(t, "hidden excluded", hasHidden, false)
	_, hasVisible := tool.InputSchema.Properties["visible"]
	assert.That(t, "visible present", hasVisible, true)
}

type bareCommaTagInput struct {
	Internal string `json:",omitempty"` //nolint:tagliatelle // intentional: testing bare-comma tag behavior
	Named    string `json:"named"`
}

func Test_DeriveSchema_With_BareCommaTag_Should_ExcludeField(t *testing.T) {
	t.Parallel()

	// Arrange — field with tag ",omitempty" has empty name, which causes skip
	r := tools.NewRegistry()
	tools.Register(r, "comma", "bare comma", func(_ context.Context, _ bareCommaTagInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Act
	tool, _ := r.Lookup("comma")

	// Assert — only "named" should appear, not "Internal" (empty name skipped)
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 1)
	_, hasNamed := tool.InputSchema.Properties["named"]
	assert.That(t, "named present", hasNamed, true)
}

type nonStringMapKeyInput struct {
	Data map[int]string `json:"data"`
}

func Test_DeriveSchema_With_NonStringMapKey_Should_Panic(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act / Assert
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("expected panic for non-string map key")
		}
		msg, ok := rec.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T", rec)
		}
		if !strings.Contains(msg, "Data") {
			t.Errorf("panic message should contain field name \"Data\", got: %s", msg)
		}
		if !strings.Contains(msg, "must be string") {
			t.Errorf("panic message should contain \"must be string\", got: %s", msg)
		}
	}()
	tools.Register(r, "badmap", "bad map", func(_ context.Context, _ nonStringMapKeyInput) tools.Result {
		return tools.TextResult("ok")
	})
}
