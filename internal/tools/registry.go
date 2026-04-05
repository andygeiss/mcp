// Package tools provides the tool registry and handler types for the MCP server.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
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
	tools []Tool
}

// Result represents the outcome of a tool handler invocation.
type Result struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// Tool represents a registered MCP tool with its handler.
type Tool struct {
	Description string      `json:"description"`
	Handler     ToolHandler `json:"-"`
	InputSchema InputSchema `json:"inputSchema"`
	Name        string      `json:"name"`
}

// ToolHandler is the low-level function signature used internally by the
// registry after JSON unmarshalling. Tool authors do not implement this
// directly — [Register] wraps a typed handler func(ctx, T) Result into a
// ToolHandler automatically.
type ToolHandler func(ctx context.Context, params json.RawMessage) Result

// ErrorResult creates a Result indicating a tool execution failure.
func ErrorResult(text string) Result {
	return Result{
		Content: []ContentBlock{{Text: text, Type: "text"}},
		IsError: true,
	}
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: []Tool{}}
}

// Names returns the names of all registered tools in alphabetical order.
func (r *Registry) Names() []string {
	names := make([]string, len(r.tools))
	for i, t := range r.tools {
		names[i] = t.Name
	}
	return names
}

// Lookup finds a tool by name.
func (r *Registry) Lookup(name string) (Tool, bool) {
	for _, t := range r.tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

// Register adds a tool to the registry. The generic type parameter T defines
// the tool's input struct — its exported fields (with json tags) are reflected
// to build the JSON Schema returned in tools/list. Fields without an
// "omitempty" option in their json tag are marked as required.
//
// Register panics if a tool with the same name is already registered. The
// registry is kept sorted alphabetically after each registration so that
// tools/list responses are deterministic.
//
// The handler receives a typed input T after the server unmarshals the raw
// JSON arguments. Return [TextResult] for success or [ErrorResult] for
// tool-level failures.
func Register[T any](r *Registry, name, description string, handler func(ctx context.Context, input T) Result) {
	for _, t := range r.tools {
		if t.Name == name {
			panic("duplicate tool name: " + name)
		}
	}

	schema := deriveSchema[T]()

	wrapped := func(ctx context.Context, params json.RawMessage) Result {
		var input T
		if err := json.Unmarshal(params, &input); err != nil {
			return ErrorResult(fmt.Sprintf("invalid arguments for tool %q: %v", name, err))
		}
		return handler(ctx, input)
	}

	tool := Tool{
		Description: description,
		Handler:     wrapped,
		InputSchema: schema,
		Name:        name,
	}

	r.tools = append(r.tools, tool)
	slices.SortFunc(r.tools, func(a, b Tool) int {
		return strings.Compare(a.Name, b.Name)
	})
}

// TextResult creates a successful Result with a text content block.
func TextResult(text string) Result {
	return Result{
		Content: []ContentBlock{{Text: text, Type: "text"}},
	}
}

// Tools returns a copy of all registered tools in alphabetical order.
func (r *Registry) Tools() []Tool {
	return slices.Clone(r.tools)
}
