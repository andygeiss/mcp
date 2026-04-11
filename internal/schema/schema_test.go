package schema_test

import (
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/schema"
)

type simpleInput struct {
	Name   string  `json:"name" description:"The name"`
	Age    int     `json:"age,omitempty" description:"The age"`
	Active bool    `json:"active" description:"Is active"`
	Score  float64 `json:"score,omitempty"`
}

func TestDeriveInputSchema_With_SimpleStruct_Should_ReturnCorrectSchema(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[simpleInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "type", s.Type, schema.TypeObject)
	assert.That(t, "property count", len(s.Properties), 4)
	assert.That(t, "name type", s.Properties["name"].Type, schema.TypeString)
	assert.That(t, "name desc", s.Properties["name"].Description, "The name")
	assert.That(t, "age type", s.Properties["age"].Type, schema.TypeInteger)
	assert.That(t, "active type", s.Properties["active"].Type, schema.TypeBoolean)
	assert.That(t, "score type", s.Properties["score"].Type, schema.TypeNumber)
	// name and active are required (no omitempty), age and score are not
	assert.That(t, "required count", len(s.Required), 2)
}

type nestedInput struct {
	Inner struct {
		Value string `json:"value" description:"Inner value"`
	} `json:"inner"`
}

func TestDeriveInputSchema_With_NestedStruct_Should_ReturnNestedProperties(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[nestedInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "inner type", s.Properties["inner"].Type, schema.TypeObject)
	assert.That(t, "inner.value type", s.Properties["inner"].Properties["value"].Type, schema.TypeString)
}

type sliceInput struct {
	Tags []string `json:"tags" description:"List of tags"`
}

func TestDeriveInputSchema_With_Slice_Should_ReturnArrayType(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[sliceInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "tags type", s.Properties["tags"].Type, schema.TypeArray)
	assert.That(t, "tags items", s.Properties["tags"].Items.Type, schema.TypeString)
}

type mapInput struct {
	Labels map[string]string `json:"labels,omitempty"`
}

func TestDeriveInputSchema_With_Map_Should_ReturnObjectType(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[mapInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "labels type", s.Properties["labels"].Type, schema.TypeObject)
	assert.That(t, "labels additionalProperties", s.Properties["labels"].AdditionalProperties.Type, schema.TypeString)
}

// EmbeddedBase is exported so the anonymous embed is promoted.
type EmbeddedBase struct {
	ID string `json:"id" description:"Identifier"`
}

type embeddedInput struct {
	EmbeddedBase
	Name string `json:"name"`
}

func TestDeriveInputSchema_With_EmbeddedStruct_Should_PromoteFields(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[embeddedInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "property count", len(s.Properties), 2)
	assert.That(t, "id type", s.Properties["id"].Type, schema.TypeString)
	assert.That(t, "name type", s.Properties["name"].Type, schema.TypeString)
}

type pointerInput struct {
	Value *string `json:"value,omitempty"`
}

func TestDeriveInputSchema_With_Pointer_Should_DereferenceType(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[pointerInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "value type", s.Properties["value"].Type, schema.TypeString)
}
