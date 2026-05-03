package schema_test

import (
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/schema"
)

type marshalCleanOut struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type marshalChanOut struct {
	Bad chan int `json:"bad"`
}

type marshalFuncOut struct {
	Bad func() `json:"bad"`
}

type marshalNestedChan struct {
	Outer struct {
		Inner chan int `json:"inner"`
	} `json:"outer"`
}

type marshalCyclic struct {
	Self *marshalCyclic `json:"self,omitempty"`
}

type marshalListOfChans struct {
	Items []struct {
		Bad chan int `json:"bad"`
	} `json:"items"`
}

func Test_FindUnmarshalable_With_CleanStruct_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Act
	path, ok := schema.FindUnmarshalable(marshalCleanOut{Name: "x", Age: 1})

	// Assert
	assert.That(t, "ok", ok, false)
	assert.That(t, "empty path", path, "")
}

func Test_FindUnmarshalable_With_ChanField_Should_ReturnFieldName(t *testing.T) {
	t.Parallel()

	// Act
	path, ok := schema.FindUnmarshalable(marshalChanOut{Bad: make(chan int)})

	// Assert
	assert.That(t, "ok", ok, true)
	assert.That(t, "path is bad", path, "bad")
}

func Test_FindUnmarshalable_With_FuncField_Should_ReturnFieldName(t *testing.T) {
	t.Parallel()

	// Act
	path, ok := schema.FindUnmarshalable(marshalFuncOut{Bad: func() {}})

	// Assert
	assert.That(t, "ok", ok, true)
	assert.That(t, "path is bad", path, "bad")
}

func Test_FindUnmarshalable_With_NestedChan_Should_ReturnDottedPath(t *testing.T) {
	t.Parallel()

	// Arrange
	v := marshalNestedChan{}
	v.Outer.Inner = make(chan int)

	// Act
	path, ok := schema.FindUnmarshalable(v)

	// Assert
	assert.That(t, "ok", ok, true)
	assert.That(t, "dotted path", path, "outer.inner")
}

func Test_FindUnmarshalable_With_CyclicPointer_Should_ReturnFieldName(t *testing.T) {
	t.Parallel()

	// Arrange — cyclic self-reference.
	v := &marshalCyclic{}
	v.Self = v

	// Act
	path, ok := schema.FindUnmarshalable(v)

	// Assert
	assert.That(t, "ok", ok, true)
	assert.That(t, "path is self", path, "self")
}

func Test_FindUnmarshalable_With_SliceOfStructWithChan_Should_ReturnElementFieldPath(t *testing.T) {
	t.Parallel()

	// Arrange
	v := marshalListOfChans{Items: []struct {
		Bad chan int `json:"bad"`
	}{{Bad: make(chan int)}}}

	// Act
	path, ok := schema.FindUnmarshalable(v)

	// Assert — slice path strips the index; the path is "items.bad".
	assert.That(t, "ok", ok, true)
	assert.That(t, "stripped-index path", path, "items.bad")
}

func Test_FindUnmarshalable_With_NilPointer_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Arrange — nil pointer marshals as "null", which is fine.
	var v *marshalChanOut

	// Act
	path, ok := schema.FindUnmarshalable(v)

	// Assert
	assert.That(t, "ok", ok, false)
	assert.That(t, "empty path", path, "")
}

func Test_FindUnmarshalable_With_NilInterface_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Act
	path, ok := schema.FindUnmarshalable(nil)

	// Assert
	assert.That(t, "ok", ok, false)
	assert.That(t, "empty path", path, "")
}

func Test_FindUnmarshalable_With_TopLevelChan_Should_ReturnTrueWithEmptyPath(t *testing.T) {
	t.Parallel()

	// Act — top-level chan reports ok=true, empty path (root).
	path, ok := schema.FindUnmarshalable(make(chan int))

	// Assert
	assert.That(t, "ok", ok, true)
	assert.That(t, "empty path", path, "")
}

type marshalAnyOut struct {
	Payload any `json:"payload"`
}

func Test_FindUnmarshalable_With_InterfaceHoldingChan_Should_ReportFieldName(t *testing.T) {
	t.Parallel()

	// Act
	path, ok := schema.FindUnmarshalable(marshalAnyOut{Payload: make(chan int)})

	// Assert
	assert.That(t, "ok", ok, true)
	assert.That(t, "path is payload", path, "payload")
}

func Test_FindUnmarshalable_With_NilInterfaceField_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Act — nil any payload marshals as "null".
	path, ok := schema.FindUnmarshalable(marshalAnyOut{})

	// Assert
	assert.That(t, "ok", ok, false)
	assert.That(t, "empty path", path, "")
}

type marshalEmptyAny struct {
	Items []any `json:"items"`
}

func Test_FindUnmarshalable_With_EmptySlice_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Act — empty slices marshal cleanly regardless of element type.
	path, ok := schema.FindUnmarshalable(marshalEmptyAny{Items: nil})

	// Assert
	assert.That(t, "ok", ok, false)
	assert.That(t, "empty path", path, "")
}

type marshalMapOut struct {
	Lookup map[string]any `json:"lookup"`
}

func Test_FindUnmarshalable_With_MapOfChans_Should_ReportFieldName(t *testing.T) {
	t.Parallel()

	// Act
	path, ok := schema.FindUnmarshalable(marshalMapOut{Lookup: map[string]any{"k": make(chan int)}})

	// Assert
	assert.That(t, "ok", ok, true)
	assert.That(t, "path is lookup", path, "lookup")
}

func Test_FindUnmarshalable_With_EmptyMap_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Act
	path, ok := schema.FindUnmarshalable(marshalMapOut{Lookup: map[string]any{}})

	// Assert
	assert.That(t, "ok", ok, false)
	assert.That(t, "empty path", path, "")
}

type unexportedField struct {
	hidden chan int //nolint:unused // present to ensure the walker skips unexported fields
	Name   string   `json:"name"`
}

func Test_FindUnmarshalable_With_UnexportedFieldChan_Should_SkipAndReturnFalse(t *testing.T) {
	t.Parallel()

	// Arrange — `hidden` is unexported; json.Marshal skips it. The walker must
	// match that visibility rule and not flag it.
	v := unexportedField{Name: "x"}

	// Act
	path, ok := schema.FindUnmarshalable(v)

	// Assert
	assert.That(t, "ok", ok, false)
	assert.That(t, "empty path", path, "")
}

type goNamedField struct {
	Untagged chan int
}

func Test_FindUnmarshalable_With_UntaggedField_Should_UseGoFieldName(t *testing.T) {
	t.Parallel()

	// Act
	path, ok := schema.FindUnmarshalable(goNamedField{Untagged: make(chan int)})

	// Assert — falls back to the Go field name when no json tag is present.
	assert.That(t, "ok", ok, true)
	assert.That(t, "path is Go name", path, "Untagged")
}
