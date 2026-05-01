// Package tools provides the tool registry and handler types for the MCP server.
package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/schema"
)

// Content type constants for tool result content blocks.
const (
	ContentTypeAudio        = "audio"
	ContentTypeImage        = "image"
	ContentTypeResourceLink = "resource_link"
	ContentTypeText         = "text"
)

// ContentBlock represents a single content item in a tool result.
// Text content uses [Text] and [Type]. Image and audio content use [Data],
// [MimeType], and [Type]. Resource links use [URI] and [Type].
type ContentBlock struct {
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Type     string `json:"type"`
	URI      string `json:"uri,omitempty"`
}

// Registry holds registered tools sorted alphabetically by name.
// Not safe for concurrent use — register all tools before starting the server.
type Registry struct {
	index map[string]int // name → position in tools slice
	tools []Tool
}

// Result represents the outcome of a tool handler invocation.
type Result struct {
	Content           []ContentBlock  `json:"content"`
	IsError           bool            `json:"isError,omitempty"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
}

// Tool represents a registered MCP tool with its handler.
type Tool struct {
	Annotations  *Annotations         `json:"annotations,omitempty"`
	Description  string               `json:"description"`
	Handler      toolHandler          `json:"-"`
	InputSchema  schema.InputSchema   `json:"inputSchema"`
	Name         string               `json:"name"`
	OutputSchema *schema.OutputSchema `json:"outputSchema,omitempty"`
	Title        string               `json:"title,omitempty"`
}

// toolHandler is the low-level function signature used internally by the
// registry after JSON unmarshalling. Tool authors do not implement this
// directly — [Register] wraps a typed handler func(ctx, T) Result into a
// toolHandler automatically.
type toolHandler func(ctx context.Context, params json.RawMessage) (Result, error)

// ErrorResult creates a Result indicating a tool execution failure.
func ErrorResult(text string) Result {
	return Result{
		Content: []ContentBlock{{Text: text, Type: ContentTypeText}},
		IsError: true,
	}
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{index: make(map[string]int), tools: []Tool{}}
}

// Lookup finds a tool by name in O(1) via the index map.
func (r *Registry) Lookup(name string) (Tool, bool) {
	i, ok := r.index[name]
	if !ok {
		return Tool{}, false
	}
	return r.tools[i], true
}

// Names returns the names of all registered tools in alphabetical order.
func (r *Registry) Names() []string {
	names := make([]string, len(r.tools))
	for i, t := range r.tools {
		names[i] = t.Name
	}
	return names
}

// Register adds a tool to the registry. The generic type parameters describe
// the handler's typed input In and structured output Out:
//
//   - Exported fields of In (with json tags) are reflected to build the JSON
//     Schema returned in tools/list. Fields without an "omitempty" option and
//     non-pointer types are marked as required.
//   - Out drives the outputSchema advertised on the tool definition. Tools
//     without meaningful structured output should use a small named typed
//     wrapper rather than [any] or struct{} — concrete types document the
//     contract and let the schema engine produce a stable shape.
//
// The handler receives a typed In after the server unmarshals the raw JSON
// arguments and returns (Out, [Result]). The dispatch layer marshals a
// non-zero Out into Result.StructuredContent via [encoding/json] (v1) before
// the response leaves the server; zero-value Out (including nil pointer Out
// and a non-nil pointer to a zero struct) is skipped so omitempty stays
// honest. When the handler also sets Result.StructuredContent manually, the
// auto-marshaled Out wins — the Out-side path is the canonical surface; the
// manual escape hatch survives only as a no-op when Out is zero. When
// Result.IsError is true, the auto-marshal step is skipped entirely so error
// responses do not carry success-shaped structuredContent.
//
// Register returns an error if a tool with the same name is already registered
// or if either the input or output type contains unsupported field types
// (channels, funcs, etc.). The registry is kept sorted alphabetically after
// each registration so that tools/list responses are deterministic.
//
// The reflection-derived OutputSchema is set on the Tool BEFORE opts are
// applied, so [WithOutputSchema] (or any option that writes the same field)
// deliberately overrides the derived schema for callers who need a
// hand-authored shape.
func Register[In, Out any](
	r *Registry,
	name, description string,
	handler func(ctx context.Context, input In) (Out, Result),
	opts ...Option,
) error {
	if _, exists := r.index[name]; exists {
		return fmt.Errorf("duplicate tool name: %s", name)
	}

	inputSchema, err := schema.DeriveInputSchema[In]()
	if err != nil {
		return fmt.Errorf("derive input schema for tool %q: %w", name, err)
	}
	outputSchema, err := schema.DeriveOutputSchema[Out]()
	if err != nil {
		return fmt.Errorf("derive output schema for tool %q: %w", name, err)
	}

	wrapped := func(ctx context.Context, params json.RawMessage) (Result, error) {
		var input In
		if err := unmarshalAndValidate(params, &input, inputSchema.Required); err != nil {
			return Result{}, &protocol.CodeError{
				Code:    protocol.InvalidParams,
				Message: fmt.Sprintf("invalid arguments for tool %q: %v", name, err),
			}
		}
		out, result := handler(ctx, input)
		return finalizeResult(name, out, result)
	}

	tool := Tool{
		Description:  description,
		Handler:      wrapped,
		InputSchema:  inputSchema,
		Name:         name,
		OutputSchema: &outputSchema,
	}
	for _, opt := range opts {
		opt(&tool)
	}

	r.tools = append(r.tools, tool)
	slices.SortFunc(r.tools, func(a, b Tool) int {
		return cmp.Compare(a.Name, b.Name)
	})
	for i, t := range r.tools {
		r.index[t.Name] = i
	}
	return nil
}

// finalizeResult applies the dispatch-side post-processing common to every
// registered tool: short-circuit error responses, auto-marshal a non-zero Out
// into StructuredContent, and ensure Content serializes as a JSON array.
//
// The Content nil-guard is load-bearing: MCP CallToolResult requires content
// as an array, and a nil slice marshals to "null", which TS-side clients
// reject with an "expected: array, received: null" validation error.
func finalizeResult(toolName string, out any, result Result) (Result, error) {
	if result.IsError {
		return result, nil
	}
	if isStructuredOutputPresent(out) {
		raw, err := json.Marshal(out)
		if err != nil {
			return Result{}, &protocol.CodeError{
				Code:    protocol.InternalError,
				Message: fmt.Sprintf("marshal structured output for tool %q: %v", toolName, err),
			}
		}
		result.StructuredContent = raw
	}
	if result.Content == nil {
		result.Content = []ContentBlock{}
	}
	return result, nil
}

// isStructuredOutputPresent reports whether out should be marshaled into
// StructuredContent. Skipped cases (return false): zero value of a value
// type, nil pointer, non-nil pointer to a zero struct. Marshal-worthy cases
// (return true): non-zero value type, non-nil pointer to a non-zero struct.
// reflect.Indirect collapses the pointer/value distinction so callers don't
// need to special-case Out = *T versus Out = T at registration sites.
func isStructuredOutputPresent(out any) bool {
	v := reflect.Indirect(reflect.ValueOf(out))
	if !v.IsValid() {
		return false
	}
	return !v.IsZero()
}

// StructuredResult creates a successful Result with both a human-readable text
// content block and machine-readable structured content. The structured JSON
// should conform to the tool's OutputSchema.
func StructuredResult(text string, structured json.RawMessage) Result {
	return Result{
		Content:           []ContentBlock{{Text: text, Type: ContentTypeText}},
		StructuredContent: structured,
	}
}

// TextResult creates a successful Result with a text content block.
func TextResult(text string) Result {
	return Result{
		Content: []ContentBlock{{Text: text, Type: ContentTypeText}},
	}
}

// Tools returns a copy of all registered tools in alphabetical order.
func (r *Registry) Tools() []Tool {
	return slices.Clone(r.tools)
}
