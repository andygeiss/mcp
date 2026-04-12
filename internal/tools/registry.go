// Package tools provides the tool registry and handler types for the MCP server.
package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
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

// InputSchema aliases schema.InputSchema so tools and prompts share the same
// JSON Schema vocabulary.
type InputSchema = schema.InputSchema

// OutputSchema aliases schema.OutputSchema for structured tool output.
type OutputSchema = schema.OutputSchema

// Property aliases schema.Property.
type Property = schema.Property

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
	Annotations  *Annotations  `json:"annotations,omitempty"`
	Description  string        `json:"description"`
	Handler      toolHandler   `json:"-"`
	InputSchema  InputSchema   `json:"inputSchema"`
	Name         string        `json:"name"`
	OutputSchema *OutputSchema `json:"outputSchema,omitempty"`
	Title        string        `json:"title,omitempty"`
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

// Register adds a tool to the registry. The generic type parameter T defines
// the tool's input struct — its exported fields (with json tags) are reflected
// to build the JSON Schema returned in tools/list. Fields without an
// "omitempty" option in their json tag are marked as required.
//
// Register returns an error if a tool with the same name is already registered
// or if the input type contains unsupported field types (channels, funcs,
// etc.). The registry is kept sorted alphabetically after each registration so
// that tools/list responses are deterministic.
//
// The handler receives a typed input T after the server unmarshals the raw
// JSON arguments. Return [TextResult] for success or [ErrorResult] for
// tool-level failures.
func Register[T any](r *Registry, name, description string, handler func(ctx context.Context, input T) Result, opts ...Option) error {
	if _, exists := r.index[name]; exists {
		return fmt.Errorf("duplicate tool name: %s", name)
	}

	inputSchema, err := schema.DeriveInputSchema[T]()
	if err != nil {
		return fmt.Errorf("derive schema for tool %q: %w", name, err)
	}

	wrapped := func(ctx context.Context, params json.RawMessage) (Result, error) {
		var input T
		if err := unmarshalAndValidate(params, &input, inputSchema.Required); err != nil {
			return Result{}, &protocol.CodeError{
				Code:    protocol.InvalidParams,
				Message: fmt.Sprintf("invalid arguments for tool %q: %v", name, err),
			}
		}
		return handler(ctx, input), nil
	}

	tool := Tool{
		Description: description,
		Handler:     wrapped,
		InputSchema: inputSchema,
		Name:        name,
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
