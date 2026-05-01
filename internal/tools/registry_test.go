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
	if err := tools.Register(r, "dup", "desc", func(_ context.Context, _ stubInput) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult("ok")
	}); err != nil {
		t.Fatal(err)
	}

	// Act
	err := tools.Register(r, "dup", "dup", func(_ context.Context, _ stubInput) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult("dup")
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
	if err := tools.Register(r, "zeta", "z desc", func(_ context.Context, _ stubInput) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult("z")
	}); err != nil {
		t.Fatal(err)
	}
	if err := tools.Register(r, "alpha", "a desc", func(_ context.Context, _ stubInput) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult("a")
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
	if err := tools.Register(r, "stub", "stub tool", func(_ context.Context, input stubInput) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult(input.Message)
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
	if err := tools.Register(r, "beta", "b desc", func(_ context.Context, _ stubInput) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult("b")
	}); err != nil {
		t.Fatal(err)
	}
	if err := tools.Register(r, "alpha", "a desc", func(_ context.Context, _ stubInput) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult("a")
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
	if err := tools.Register(r, "greet", "greets", func(_ context.Context, input stubInput) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult("hello " + input.Message)
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
	if err := tools.Register(r, "greet", "greets", func(_ context.Context, input stubInput) (struct{}, tools.Result) {
		return struct{}{}, tools.TextResult("hello " + input.Message)
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
	if err := tools.Register(r, "opt", "optional", func(_ context.Context, input optionalInput) (struct{}, tools.Result) {
		if input.Name == "" {
			return struct{}{}, tools.TextResult("anonymous")
		}
		return struct{}{}, tools.TextResult(input.Name)
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

func Test_ContentBlock_With_TextType_Should_OmitDataAndMimeType(t *testing.T) {
	t.Parallel()

	// Arrange
	block := tools.ContentBlock{Text: "hello", Type: "text"}

	// Act
	data, err := json.Marshal(block)

	// Assert
	assert.That(t, "error", err, nil)
	s := string(data)
	if strings.Contains(s, "data") {
		t.Fatalf("expected no data key in text block, got: %s", s)
	}
	if strings.Contains(s, "mimeType") {
		t.Fatalf("expected no mimeType key in text block, got: %s", s)
	}
	if strings.Contains(s, "uri") {
		t.Fatalf("expected no uri key in text block, got: %s", s)
	}
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

func Test_TextResult_With_Success_Should_OmitIsErrorFromJSON(t *testing.T) {
	t.Parallel()

	// Act
	data, err := json.Marshal(tools.TextResult("ok"))

	// Assert
	assert.That(t, "error", err, nil)
	if strings.Contains(string(data), "isError") {
		t.Fatalf("expected no isError key in success result, got: %s", data)
	}
}

func Test_StructuredResult_With_ValidJSON_Should_IncludeStructuredContent(t *testing.T) {
	t.Parallel()

	// Act
	result := tools.StructuredResult("summary", json.RawMessage(`{"count":42}`))
	data, err := json.Marshal(result)

	// Assert
	assert.That(t, "error", err, nil)
	s := string(data)
	if !strings.Contains(s, `"structuredContent":{"count":42}`) {
		t.Fatalf("expected structuredContent in JSON, got: %s", s)
	}
	if !strings.Contains(s, `"text":"summary"`) {
		t.Fatalf("expected text content in JSON, got: %s", s)
	}
}

func Test_TextResult_With_Success_Should_OmitStructuredContent(t *testing.T) {
	t.Parallel()

	// Act
	data, err := json.Marshal(tools.TextResult("ok"))

	// Assert
	assert.That(t, "error", err, nil)
	if strings.Contains(string(data), "structuredContent") {
		t.Fatalf("expected no structuredContent key, got: %s", data)
	}
}

func Test_ErrorResult_With_Failure_Should_IncludeIsErrorInJSON(t *testing.T) {
	t.Parallel()

	// Act
	data, err := json.Marshal(tools.ErrorResult("fail"))

	// Assert
	assert.That(t, "error", err, nil)
	if !strings.Contains(string(data), `"isError":true`) {
		t.Fatalf("expected isError:true in error result, got: %s", data)
	}
}

// --- Q5 dispatch-side semantics: precedence, IsError gate, pointer Out ---

type structuredOutInput struct {
	Echo string `json:"echo" description:"input string"`
}

type structuredOutOutput struct {
	Echoed string `json:"echoed" description:"echoed value"`
}

func Test_Register_With_NonZeroOut_Should_OverrideManualStructuredContent(t *testing.T) {
	t.Parallel()

	// Arrange — handler sets StructuredContent manually AND returns a
	// non-zero Out. Per the documented precedence in Register's godoc, the
	// auto-marshaled Out wins; the manual escape hatch only survives when
	// Out is zero.
	r := tools.NewRegistry()
	if err := tools.Register(r, "precedence", "out wins",
		func(_ context.Context, in structuredOutInput) (structuredOutOutput, tools.Result) {
			manual := tools.Result{StructuredContent: json.RawMessage(`{"manual":true}`)}
			return structuredOutOutput{Echoed: in.Echo}, manual
		},
	); err != nil {
		t.Fatal(err)
	}
	tool, _ := r.Lookup("precedence")

	// Act
	result, err := tool.Handler(context.Background(), json.RawMessage(`{"echo":"x"}`))

	// Assert — auto-marshaled Out replaces the handler's manual payload.
	assert.That(t, "error", err, nil)
	assert.That(t, "structuredContent matches Out, not manual",
		string(result.StructuredContent), `{"echoed":"x"}`)
}

func Test_Register_With_IsErrorResult_Should_SkipStructuredOutMarshal(t *testing.T) {
	t.Parallel()

	// Arrange — handler returns IsError=true and a non-zero Out. The
	// dispatch wrapper must skip the auto-marshal so failure responses do
	// not carry success-shaped structuredContent.
	r := tools.NewRegistry()
	if err := tools.Register(r, "errorpath", "skip marshal",
		func(_ context.Context, in structuredOutInput) (structuredOutOutput, tools.Result) {
			return structuredOutOutput{Echoed: in.Echo}, tools.ErrorResult("failed")
		},
	); err != nil {
		t.Fatal(err)
	}
	tool, _ := r.Lookup("errorpath")

	// Act
	result, err := tool.Handler(context.Background(), json.RawMessage(`{"echo":"x"}`))

	// Assert — error result preserved, no structuredContent leaked.
	assert.That(t, "error", err, nil)
	assert.That(t, "isError", result.IsError, true)
	assert.That(t, "structuredContent omitted", len(result.StructuredContent), 0)
}

func Test_Register_With_PointerOut_Nil_Should_SkipMarshal(t *testing.T) {
	t.Parallel()

	// Arrange — Out is *T; handler returns nil pointer. reflect.Indirect
	// on a nil pointer yields an invalid Value; the wrapper treats this as
	// "no structured output."
	r := tools.NewRegistry()
	if err := tools.Register(r, "ptrnil", "nil ptr",
		func(_ context.Context, _ structuredOutInput) (*structuredOutOutput, tools.Result) {
			return nil, tools.Result{}
		},
	); err != nil {
		t.Fatal(err)
	}
	tool, _ := r.Lookup("ptrnil")

	// Act
	result, err := tool.Handler(context.Background(), json.RawMessage(`{"echo":"x"}`))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "structuredContent omitted", len(result.StructuredContent), 0)
}

func Test_Register_With_PointerOut_NonNilZero_Should_SkipMarshal(t *testing.T) {
	t.Parallel()

	// Arrange — Out is *T; handler returns &T{} (non-nil pointer to a zero
	// struct). Without reflect.Indirect, this would be considered non-zero
	// (the pointer itself is non-nil) and emit `{}`. With Indirect, the
	// wrapper sees the underlying zero T and skips the marshal — keeping
	// omitempty honest.
	r := tools.NewRegistry()
	if err := tools.Register(r, "ptrzero", "non-nil zero ptr",
		func(_ context.Context, _ structuredOutInput) (*structuredOutOutput, tools.Result) {
			return &structuredOutOutput{}, tools.Result{}
		},
	); err != nil {
		t.Fatal(err)
	}
	tool, _ := r.Lookup("ptrzero")

	// Act
	result, err := tool.Handler(context.Background(), json.RawMessage(`{"echo":"x"}`))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "structuredContent omitted", len(result.StructuredContent), 0)
}

func Test_Register_With_PointerOut_NonNilNonZero_Should_Marshal(t *testing.T) {
	t.Parallel()

	// Arrange — Out is *T; handler returns a pointer to a populated value.
	r := tools.NewRegistry()
	if err := tools.Register(r, "ptrnonzero", "non-nil non-zero ptr",
		func(_ context.Context, in structuredOutInput) (*structuredOutOutput, tools.Result) {
			return &structuredOutOutput{Echoed: in.Echo}, tools.Result{}
		},
	); err != nil {
		t.Fatal(err)
	}
	tool, _ := r.Lookup("ptrnonzero")

	// Act
	result, err := tool.Handler(context.Background(), json.RawMessage(`{"echo":"x"}`))

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "structuredContent emitted",
		string(result.StructuredContent), `{"echoed":"x"}`)
}

// unsupportedOut has a chan field — DeriveOutputSchema fails at registration,
// covering the err-return branch at registry.go's outputSchema derivation.
type unsupportedOut struct {
	Ch chan int `json:"ch" description:"unsupported channel"`
}

func Test_Register_With_UnsupportedOutType_Should_ReturnDeriveError(t *testing.T) {
	t.Parallel()

	r := tools.NewRegistry()
	err := tools.Register(r, "bad-out", "out type rejected by schema engine",
		func(_ context.Context, _ structuredOutInput) (unsupportedOut, tools.Result) {
			return unsupportedOut{}, tools.Result{}
		},
	)
	if err == nil {
		t.Fatal("expected error for unsupported Out type, got nil")
	}
	if !strings.Contains(err.Error(), "derive output schema") {
		t.Errorf("error should mention output schema derivation, got: %s", err.Error())
	}
}

// outWithInterface has an interface{} field — schema derivation accepts it
// (open schema), but at runtime json.Marshal fails when the interface holds
// an unmarshalable value (chan, func, cyclic). Covers the marshal-error
// branch of the dispatch wrapper.
type outWithInterface struct {
	Value any `json:"value" description:"any value"`
}

func Test_Register_With_UnmarshalableOutAtRuntime_Should_ReturnInternalError(t *testing.T) {
	t.Parallel()

	r := tools.NewRegistry()
	if err := tools.Register(r, "marshal-err", "marshalable schema, unmarshalable runtime",
		func(_ context.Context, _ structuredOutInput) (outWithInterface, tools.Result) {
			return outWithInterface{Value: make(chan int)}, tools.Result{}
		},
	); err != nil {
		t.Fatal(err)
	}
	tool, _ := r.Lookup("marshal-err")

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"echo":"x"}`))
	if err == nil {
		t.Fatal("expected internal error from json.Marshal on chan, got nil")
	}
	if !strings.Contains(err.Error(), "marshal structured output") {
		t.Errorf("error should mention marshal failure, got: %s", err.Error())
	}
}
