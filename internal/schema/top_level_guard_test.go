package schema_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/schema"
)

type guardFoo struct {
	Name string `json:"name"`
}

// The guard rejects each non-struct top-level type with a typed error naming
// the offending type. The cases below cover the four shapes called out by
// FR3 (Story 1.3): primitives, slices, maps, and pointer-to-pointer-to-struct.

func Test_DeriveInputSchema_With_IntTopLevel_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := schema.DeriveInputSchema[int]()

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names offending type", strings.Contains(err.Error(), "int"), true)
	assert.That(t, "names guard", strings.Contains(err.Error(), "not a struct"), true)
}

func Test_DeriveInputSchema_With_SliceTopLevel_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := schema.DeriveInputSchema[[]string]()

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names slice type", strings.Contains(err.Error(), "[]string"), true)
}

func Test_DeriveInputSchema_With_MapTopLevel_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := schema.DeriveInputSchema[map[string]string]()

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names map type", strings.Contains(err.Error(), "map[string]string"), true)
}

func Test_DeriveInputSchema_With_DoublePointerToStruct_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act — **guardFoo unwraps once to *guardFoo (still pointer), so the
	// guard catches it instead of silently succeeding with an empty schema.
	_, err := schema.DeriveInputSchema[**guardFoo]()

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names pointer type", strings.Contains(err.Error(), "*"), true)
}

// time.Time and json.RawMessage are accepted as top-level documented special
// cases. Both produce an empty schema (no properties / no required); callers
// wanting a strictly-typed wire shape wrap them in a single-field struct.

func Test_DeriveInputSchema_With_TimeTimeTopLevel_Should_Accept(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[time.Time]()

	// Assert
	assert.That(t, "no error", err, nil)
	assert.That(t, "type is object", s.Type, schema.TypeObject)
	assert.That(t, "no properties", len(s.Properties), 0)
}

func Test_DeriveInputSchema_With_RawMessageTopLevel_Should_Accept(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveInputSchema[json.RawMessage]()

	// Assert
	assert.That(t, "no error", err, nil)
	assert.That(t, "type is object", s.Type, schema.TypeObject)
	assert.That(t, "no properties", len(s.Properties), 0)
}

// Mirror tests on DeriveOutputSchema so the guard covers both positions.

func Test_DeriveOutputSchema_With_IntTopLevel_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := schema.DeriveOutputSchema[int]()

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names offending type", strings.Contains(err.Error(), "int"), true)
}

func Test_DeriveOutputSchema_With_DoublePointerToStruct_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := schema.DeriveOutputSchema[**guardFoo]()

	// Assert
	assert.That(t, "error returned", err != nil, true)
}

func Test_DeriveOutputSchema_With_TimeTimeTopLevel_Should_Accept(t *testing.T) {
	t.Parallel()

	// Act
	s, err := schema.DeriveOutputSchema[time.Time]()

	// Assert
	assert.That(t, "no error", err, nil)
	assert.That(t, "type is object", s.Type, schema.TypeObject)
}
