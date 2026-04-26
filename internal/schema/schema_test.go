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

func Test_DeriveInputSchema_With_SimpleStruct_Should_ReturnCorrectSchema(t *testing.T) {
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

func Test_DeriveInputSchema_With_NestedStruct_Should_ReturnNestedProperties(t *testing.T) {
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

func Test_DeriveInputSchema_With_Slice_Should_ReturnArrayType(t *testing.T) {
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

func Test_DeriveInputSchema_With_Map_Should_ReturnObjectType(t *testing.T) {
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

func Test_DeriveInputSchema_With_EmbeddedStruct_Should_PromoteFields(t *testing.T) {
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

func Test_DeriveInputSchema_With_Pointer_Should_DereferenceType(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[pointerInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "value type", s.Properties["value"].Type, schema.TypeString)
}

type taggedEmbedBase struct {
	ID string `json:"id" description:"Identifier"`
}

type taggedEmbedInput struct {
	taggedEmbedBase `json:"base"`
	Name            string `json:"name"`
}

func Test_DeriveInputSchema_With_TaggedEmbed_Should_NotPromote(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[taggedEmbedInput]()

	// Assert
	assert.That(t, "error", err, nil)
	// The tagged embed should appear as a nested object, not promoted
	assert.That(t, "base type", s.Properties["base"].Type, schema.TypeObject)
	assert.That(t, "name type", s.Properties["name"].Type, schema.TypeString)
}

type pointerEmbedInput struct {
	*EmbeddedBase
	Name string `json:"name"`
}

func Test_DeriveInputSchema_With_PointerEmbed_Should_PromoteFields(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[pointerEmbedInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "property count", len(s.Properties), 2)
	assert.That(t, "id type", s.Properties["id"].Type, schema.TypeString)
	assert.That(t, "name type", s.Properties["name"].Type, schema.TypeString)
}

type nonStringMapInput struct {
	Data map[int]string `json:"data"`
}

func Test_DeriveInputSchema_With_NonStringMapKey_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := schema.DeriveInputSchema[nonStringMapInput]()

	// Assert
	if err == nil {
		t.Fatal("expected error for non-string map key")
	}
}

type chanInput struct {
	Ch chan string `json:"ch"`
}

func Test_DeriveInputSchema_With_UnsupportedType_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := schema.DeriveInputSchema[chanInput]()

	// Assert
	if err == nil {
		t.Fatal("expected error for unsupported chan type")
	}
}

type untaggedFieldInput struct {
	Tagged   string `json:"tagged"`
	Untagged string
}

func Test_DeriveInputSchema_With_UntaggedField_Should_SkipIt(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[untaggedFieldInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "property count", len(s.Properties), 1)
	assert.That(t, "tagged type", s.Properties["tagged"].Type, schema.TypeString)
}

type ignoredFieldInput struct {
	Kept    string `json:"kept"`
	Ignored string `json:"-"`
}

func Test_DeriveInputSchema_With_IgnoredField_Should_SkipIt(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[ignoredFieldInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "property count", len(s.Properties), 1)
	assert.That(t, "kept type", s.Properties["kept"].Type, schema.TypeString)
}

func Test_DeriveInputSchema_With_PointerTypeParam_Should_Dereference(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[*simpleInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "type", s.Type, schema.TypeObject)
	assert.That(t, "property count", len(s.Properties), 4)
}

type sliceChanInput struct {
	Items []chan int `json:"items"`
}

func Test_DeriveInputSchema_With_SliceOfUnsupportedType_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := schema.DeriveInputSchema[sliceChanInput]()

	// Assert
	if err == nil {
		t.Fatal("expected error for slice of unsupported type")
	}
}

type mapChanInput struct {
	Data map[string]chan int `json:"data"`
}

func Test_DeriveInputSchema_With_MapOfUnsupportedValueType_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := schema.DeriveInputSchema[mapChanInput]()

	// Assert
	if err == nil {
		t.Fatal("expected error for map with unsupported value type")
	}
}

type allOptionalNested struct {
	Inner struct {
		A string `json:"a,omitempty"`
		B string `json:"b,omitempty"`
	} `json:"inner"`
}

func Test_DeriveInputSchema_With_AllOptionalNestedStruct_Should_OmitRequired(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[allOptionalNested]()

	// Assert
	assert.That(t, "error", err, nil)
	inner := s.Properties["inner"]
	assert.That(t, "inner type", inner.Type, schema.TypeObject)
	assert.That(t, "inner required count", len(inner.Required), 0)
}

type unexportedBase struct {
	ID string `json:"id"`
}

type unexportedEmbedInput struct {
	unexportedBase
	Name string `json:"name"`
}

func Test_DeriveInputSchema_With_UnexportedEmbed_Should_NotPromote(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[unexportedEmbedInput]()

	// Assert
	assert.That(t, "error", err, nil)
	// The unexported embed should be skipped entirely
	assert.That(t, "property count", len(s.Properties), 1)
	assert.That(t, "name type", s.Properties["name"].Type, schema.TypeString)
}

type depth0 struct {
	A struct {
		B struct {
			C struct {
				D struct {
					E struct {
						F struct {
							G struct {
								H struct {
									I struct {
										J struct {
											K struct {
												L string `json:"l"`
											} `json:"k"`
										} `json:"j"`
									} `json:"i"`
								} `json:"h"`
							} `json:"g"`
						} `json:"f"`
					} `json:"e"`
				} `json:"d"`
			} `json:"c"`
		} `json:"b"`
	} `json:"a"`
}

func Test_DeriveInputSchema_With_ExcessiveDepth_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := schema.DeriveInputSchema[depth0]()

	// Assert
	if err == nil {
		t.Fatal("expected error for exceeding max schema depth")
	}
}

type uintInput struct {
	Val uint `json:"val"`
}

func Test_DeriveInputSchema_With_UintType_Should_ReturnInteger(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[uintInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "val type", s.Properties["val"].Type, schema.TypeInteger)
}

type pointerOptionalInput struct {
	Required string  `json:"required"`
	Optional *string `json:"optional"`
}

func Test_DeriveInputSchema_With_PointerField_Should_BeOptional(t *testing.T) {
	t.Parallel()

	// Act: *T is the idiomatic Go "nullable" marker and must map to a
	// non-required JSON Schema property even without omitempty.
	s, err := schema.DeriveInputSchema[pointerOptionalInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "optional unwrapped type", s.Properties["optional"].Type, schema.TypeString)
	assert.That(t, "only non-pointer is required", s.Required, []string{"required"})
}

type anyInput struct {
	Meta any `json:"meta"`
}

func Test_DeriveInputSchema_With_InterfaceField_Should_ReturnOpenSchema(t *testing.T) {
	t.Parallel()

	// Act: `any` accepts any JSON value; the derived property carries no
	// type constraint (open schema).
	s, err := schema.DeriveInputSchema[anyInput]()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "meta has no type constraint", s.Properties["meta"].Type, "")
}
