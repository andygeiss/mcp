package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/tools"
)

type singleFieldInput struct {
	Message string `json:"message" description:"The message to process"`
}

func Test_DeriveSchema_With_SingleField_Should_ProduceCorrectSchema(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "stub", "stub tool", func(_ context.Context, input singleFieldInput) tools.Result {
		return tools.TextResult(input.Message)
	}); err != nil {
		t.Fatal(err)
	}

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
	if err := tools.Register(r, "multi", "multi-field tool", func(_ context.Context, _ multiFieldInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

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
	if err := tools.Register(r, "tagger", "tags tool", func(_ context.Context, _ sliceFieldInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

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
	if err := tools.Register(r, "mapper", "map tool", func(_ context.Context, _ mapFieldInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

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
	if err := tools.Register(r, "nested", "nested tool", func(_ context.Context, _ nestedStructInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

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

func Test_DeriveSchema_With_UnsupportedType_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act
	err := tools.Register(r, "bad", "bad tool", func(_ context.Context, _ unsupportedInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Assert
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
	if !strings.Contains(err.Error(), "Ch") {
		t.Errorf("error message should contain field name \"Ch\", got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "chan") {
		t.Errorf("error message should contain type \"chan\", got: %s", err.Error())
	}
}

type pointerFieldInput struct {
	Value *string `json:"value"`
}

func Test_DeriveSchema_With_PointerField_Should_UnwrapToUnderlyingType(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "ptr", "pointer tool", func(_ context.Context, _ pointerFieldInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

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
	if err := tools.Register(r, "matrix", "nested slice", func(_ context.Context, _ nestedSliceInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

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
	if err := tools.Register(r, "dash", "dash tag", func(_ context.Context, _ dashTagInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

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
	if err := tools.Register(r, "comma", "bare comma", func(_ context.Context, _ bareCommaTagInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	tool, _ := r.Lookup("comma")

	// Assert — only "named" should appear, not "Internal" (empty name skipped)
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 1)
	_, hasNamed := tool.InputSchema.Properties["named"]
	assert.That(t, "named present", hasNamed, true)
}

// Depth guard test types

type depth1 struct {
	F depth2 `json:"f"`
}
type depth2 struct {
	F depth3 `json:"f"`
}
type depth3 struct {
	F depth4 `json:"f"`
}
type depth4 struct {
	F depth5 `json:"f"`
}
type depth5 struct {
	F depth6 `json:"f"`
}
type depth6 struct {
	F depth7 `json:"f"`
}
type depth7 struct {
	F depth8 `json:"f"`
}
type depth8 struct {
	F depth9 `json:"f"`
}
type depth9 struct {
	F depth10 `json:"f"`
}
type depth10 struct {
	F depth11 `json:"f"`
}
type depth11 struct {
	F string `json:"f"`
}

type excessiveDepthInput struct {
	Root depth1 `json:"root"`
}

func Test_DeriveSchema_With_ExcessiveDepth_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act
	err := tools.Register(r, "deep", "deep tool", func(_ context.Context, _ excessiveDepthInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Assert
	if err == nil {
		t.Fatal("expected error for excessive depth")
	}
	if !strings.Contains(err.Error(), "exceeded max depth") {
		t.Errorf("error message should contain \"exceeded max depth\", got: %s", err.Error())
	}
}

type selfRef struct {
	F *selfRef `json:"f"`
}

type selfRefInput struct {
	Root selfRef `json:"root"`
}

func Test_DeriveSchema_With_SelfReferentialType_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act
	err := tools.Register(r, "selfref", "self-ref tool", func(_ context.Context, _ selfRefInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Assert
	if err == nil {
		t.Fatal("expected error for self-referential type")
	}
	if !strings.Contains(err.Error(), "exceeded max depth") {
		t.Errorf("error message should contain \"exceeded max depth\", got: %s", err.Error())
	}
}

// Exact depth limit types (10 levels: exact1 -> exact2 -> ... -> exact10 with a string leaf)

type exact1 struct {
	F exact2 `json:"f"`
}
type exact2 struct {
	F exact3 `json:"f"`
}
type exact3 struct {
	F exact4 `json:"f"`
}
type exact4 struct {
	F exact5 `json:"f"`
}
type exact5 struct {
	F exact6 `json:"f"`
}
type exact6 struct {
	F exact7 `json:"f"`
}
type exact7 struct {
	F exact8 `json:"f"`
}
type exact8 struct {
	F exact9 `json:"f"`
}
type exact9 struct {
	F exact10 `json:"f"`
}
type exact10 struct {
	F string `json:"f"`
}

type exactDepthInput struct {
	Root exact1 `json:"root"`
}

func Test_DeriveSchema_With_ExactDepthLimit_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act — should not error
	if err := tools.Register(r, "exact", "exact depth tool", func(_ context.Context, _ exactDepthInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Assert
	tool, ok := r.Lookup("exact")
	assert.That(t, "found", ok, true)
	assert.That(t, "schema type", tool.InputSchema.Type, "object")
}

type mixedLeaf struct {
	V string `json:"v"`
}

type mixedDepthInput struct {
	Data []map[string][]map[string][]map[string][]map[string][]map[string]mixedLeaf `json:"data"`
}

func Test_DeriveSchema_With_MixedNestingExceedingDepth_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act
	err := tools.Register(r, "mixed", "mixed depth tool", func(_ context.Context, _ mixedDepthInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Assert
	if err == nil {
		t.Fatal("expected error for mixed nesting exceeding depth")
	}
	if !strings.Contains(err.Error(), "exceeded max depth") {
		t.Errorf("error message should contain \"exceeded max depth\", got: %s", err.Error())
	}
}

// Embedding test types

type EmbeddedBase struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type embeddedInput struct {
	EmbeddedBase
	Name string `json:"name"`
}

func Test_DeriveSchema_With_EmbeddedStruct_Should_PromoteFields(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "embed", "embed tool", func(_ context.Context, _ embeddedInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	tool, ok := r.Lookup("embed")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 3)

	hostProp := tool.InputSchema.Properties["host"]
	assert.That(t, "host type", hostProp.Type, "string")

	portProp := tool.InputSchema.Properties["port"]
	assert.That(t, "port type", portProp.Type, "integer")

	nameProp := tool.InputSchema.Properties["name"]
	assert.That(t, "name type", nameProp.Type, "string")

	assert.That(t, "required count", len(tool.InputSchema.Required), 3)
}

type TaggedEmbeddedBase struct {
	X string `json:"x"`
	Y string `json:"y"`
}

type taggedEmbeddedInput struct {
	TaggedEmbeddedBase `json:"base"`
	Label              string `json:"label"`
}

func Test_DeriveSchema_With_TaggedEmbeddedStruct_Should_NestFields(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "tagged-embed", "tagged embed tool", func(_ context.Context, _ taggedEmbeddedInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	tool, ok := r.Lookup("tagged-embed")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 2)

	baseProp := tool.InputSchema.Properties["base"]
	assert.That(t, "base type", baseProp.Type, "object")
	assert.That(t, "base nested count", len(baseProp.Properties), 2)

	labelProp := tool.InputSchema.Properties["label"]
	assert.That(t, "label type", labelProp.Type, "string")
}

type DeepEmbedC struct {
	Z string `json:"z"`
}

type DeepEmbedB struct {
	DeepEmbedC
	Y string `json:"y"`
}

type DeepEmbedA struct {
	DeepEmbedB
	X string `json:"x"`
}

type deepEmbeddedInput struct {
	DeepEmbedA
	W string `json:"w"`
}

func Test_DeriveSchema_With_DeepEmbedding_Should_PromoteAllLevels(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "deep-embed", "deep embed tool", func(_ context.Context, _ deepEmbeddedInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	tool, ok := r.Lookup("deep-embed")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 4)
	assert.That(t, "w type", tool.InputSchema.Properties["w"].Type, "string")
	assert.That(t, "x type", tool.InputSchema.Properties["x"].Type, "string")
	assert.That(t, "y type", tool.InputSchema.Properties["y"].Type, "string")
	assert.That(t, "z type", tool.InputSchema.Properties["z"].Type, "string")
}

type PtrEmbedBase struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ptrEmbeddedInput struct {
	*PtrEmbedBase
	Extra string `json:"extra"`
}

func Test_DeriveSchema_With_EmbeddedPointerStruct_Should_PromoteFields(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "ptr-embed", "ptr embed tool", func(_ context.Context, _ ptrEmbeddedInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	tool, ok := r.Lookup("ptr-embed")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 3)
	assert.That(t, "id type", tool.InputSchema.Properties["id"].Type, "integer")
	assert.That(t, "name type", tool.InputSchema.Properties["name"].Type, "string")
	assert.That(t, "extra type", tool.InputSchema.Properties["extra"].Type, "string")
}

type ShadowBase struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type shadowInput struct {
	ShadowBase
	Name string `json:"name"` // shadows embedded Name
}

func Test_DeriveSchema_With_ShadowedField_Should_PreferParent(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "shadow", "shadow tool", func(_ context.Context, _ shadowInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	tool, ok := r.Lookup("shadow")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 2)
	assert.That(t, "name type", tool.InputSchema.Properties["name"].Type, "string")
	assert.That(t, "age type", tool.InputSchema.Properties["age"].Type, "integer")
}

type unexportedBase struct {
	Secret string `json:"secret"`
}

type unexportedEmbedInput struct {
	unexportedBase
	Public string `json:"public"`
}

func Test_DeriveSchema_With_UnexportedEmbeddedStruct_Should_NotPromote(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "unexported", "unexported tool", func(_ context.Context, _ unexportedEmbedInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	tool, ok := r.Lookup("unexported")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 1)
	_, hasSecret := tool.InputSchema.Properties["secret"]
	assert.That(t, "secret not promoted", hasSecret, false)
	_, hasPublic := tool.InputSchema.Properties["public"]
	assert.That(t, "public present", hasPublic, true)
}

type MyString string

type nonStructEmbedInput struct {
	MyString `json:"value"`
	Label    string `json:"label"`
}

func Test_DeriveSchema_With_EmbeddedNonStructType_Should_NotPanic(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "nonstruct", "nonstruct tool", func(_ context.Context, _ nonStructEmbedInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	tool, ok := r.Lookup("nonstruct")

	// Assert
	assert.That(t, "found", ok, true)
	_, hasLabel := tool.InputSchema.Properties["label"]
	assert.That(t, "label present", hasLabel, true)
}

type float64FieldInput struct {
	Score float64 `json:"score" description:"A floating point score"`
}

func Test_DeriveSchema_With_Float64Field_Should_ProduceNumberType(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "floater", "float tool", func(_ context.Context, _ float64FieldInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	tool, ok := r.Lookup("floater")

	// Assert
	assert.That(t, "found", ok, true)
	scoreProp := tool.InputSchema.Properties["score"]
	assert.That(t, "score type", scoreProp.Type, "number")
	assert.That(t, "score description", scoreProp.Description, "A floating point score")
	assert.That(t, "required", tool.InputSchema.Required, []string{"score"})
}

type uintFieldInput struct {
	Count uint `json:"count" description:"An unsigned integer count"`
}

func Test_DeriveSchema_With_UintField_Should_ProduceIntegerType(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "uinter", "uint tool", func(_ context.Context, _ uintFieldInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	tool, ok := r.Lookup("uinter")

	// Assert
	assert.That(t, "found", ok, true)
	countProp := tool.InputSchema.Properties["count"]
	assert.That(t, "count type", countProp.Type, "integer")
	assert.That(t, "count description", countProp.Description, "An unsigned integer count")
	assert.That(t, "required", tool.InputSchema.Required, []string{"count"})
}

type emptyStructInput struct{}

func Test_DeriveSchema_With_EmptyStruct_Should_ProduceEmptyProperties(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "empty", "empty tool", func(_ context.Context, _ emptyStructInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	tool, ok := r.Lookup("empty")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "schema type", tool.InputSchema.Type, "object")
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 0)
	assert.That(t, "required count", len(tool.InputSchema.Required), 0)
}

type allOmitemptyInput struct {
	Foo string `json:"foo,omitempty"`
	Bar int    `json:"bar,omitempty"`
}

func Test_DeriveSchema_With_AllOmitemptyFields_Should_ProduceEmptyRequired(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()
	if err := tools.Register(r, "allopt", "all optional tool", func(_ context.Context, _ allOmitemptyInput) tools.Result {
		return tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	tool, ok := r.Lookup("allopt")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "properties count", len(tool.InputSchema.Properties), 2)
	assert.That(t, "required count", len(tool.InputSchema.Required), 0)
	fooProp := tool.InputSchema.Properties["foo"]
	assert.That(t, "foo type", fooProp.Type, "string")
	barProp := tool.InputSchema.Properties["bar"]
	assert.That(t, "bar type", barProp.Type, "integer")
}

type nonStringMapKeyInput struct {
	Data map[int]string `json:"data"`
}

func Test_DeriveSchema_With_NonStringMapKey_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	r := tools.NewRegistry()

	// Act
	err := tools.Register(r, "badmap", "bad map", func(_ context.Context, _ nonStringMapKeyInput) tools.Result {
		return tools.TextResult("ok")
	})

	// Assert
	if err == nil {
		t.Fatal("expected error for non-string map key")
	}
	if !strings.Contains(err.Error(), "Data") {
		t.Errorf("error message should contain field name \"Data\", got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "must be string") {
		t.Errorf("error message should contain \"must be string\", got: %s", err.Error())
	}
}
