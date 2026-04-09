// Package tools provides the tool registry and handler types for the MCP server.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/andygeiss/mcp/internal/protocol"
)

// ContentTypeText is the MIME content type for plain text tool results.
const ContentTypeText = "text"

const (
	// SchemaTypeArray is the JSON Schema type for array values.
	SchemaTypeArray = "array"
	// SchemaTypeBoolean is the JSON Schema type for boolean values.
	SchemaTypeBoolean = "boolean"
	// SchemaTypeInteger is the JSON Schema type for integer values.
	SchemaTypeInteger = "integer"
	// SchemaTypeNumber is the JSON Schema type for numeric values.
	SchemaTypeNumber = "number"
	// SchemaTypeObject is the JSON Schema type for object values.
	SchemaTypeObject = "object"
	// SchemaTypeString is the JSON Schema type for string values.
	SchemaTypeString = "string"
)

// ContentBlock represents a single content item in a tool result.
type ContentBlock struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

// InputSchema describes the JSON Schema for a tool's input parameters.
type InputSchema struct {
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
	Type       string              `json:"type"`
}

// Property describes a single property in a tool's input schema.
type Property struct {
	AdditionalProperties *Property           `json:"additionalProperties,omitempty"`
	Description          string              `json:"description,omitempty"`
	Items                *Property           `json:"items,omitempty"`
	Properties           map[string]Property `json:"properties,omitempty"`
	Required             []string            `json:"required,omitempty"`
	Type                 string              `json:"type"`
}

// Registry holds registered tools sorted alphabetically by name.
// Not safe for concurrent use — register all tools before starting the server.
type Registry struct {
	index map[string]int // name → position in tools slice
	tools []Tool
}

// Result represents the outcome of a tool handler invocation.
type Result struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

// Tool represents a registered MCP tool with its handler.
type Tool struct {
	Annotations *Annotations `json:"annotations,omitempty"`
	Description string       `json:"description"`
	Handler     toolHandler  `json:"-"`
	InputSchema InputSchema  `json:"inputSchema"`
	Name        string       `json:"name"`
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

// Names returns the names of all registered tools in alphabetical order.
func (r *Registry) Names() []string {
	names := make([]string, len(r.tools))
	for i, t := range r.tools {
		names[i] = t.Name
	}
	return names
}

// Lookup finds a tool by name in O(1) via the index map.
func (r *Registry) Lookup(name string) (Tool, bool) {
	i, ok := r.index[name]
	if !ok {
		return Tool{}, false
	}
	return r.tools[i], true
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

	schema, err := deriveSchema[T]()
	if err != nil {
		return fmt.Errorf("derive schema for tool %q: %w", name, err)
	}

	wrapped := func(ctx context.Context, params json.RawMessage) (Result, error) {
		var input T
		if err := unmarshalAndValidate(params, &input, schema.Required); err != nil {
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
		InputSchema: schema,
		Name:        name,
	}
	for _, opt := range opts {
		opt(&tool)
	}

	r.tools = append(r.tools, tool)
	slices.SortFunc(r.tools, func(a, b Tool) int {
		return strings.Compare(a.Name, b.Name)
	})
	for i, t := range r.tools {
		r.index[t.Name] = i
	}
	return nil
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
