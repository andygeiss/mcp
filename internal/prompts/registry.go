// Package prompts provides the prompt registry for the MCP server.
package prompts

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/schema"
)

// Argument describes a prompt argument.
type Argument struct {
	Description string `json:"description,omitempty"`
	Name        string `json:"name"`
	Required    bool   `json:"required,omitempty"`
}

// ContentBlock represents content in a prompt message.
type ContentBlock struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

// Message represents a single message in a prompt result.
type Message struct {
	Content ContentBlock `json:"content"`
	Role    string       `json:"role"`
}

// Option configures a Prompt during registration.
type Option func(*Prompt)

// Prompt represents a registered MCP prompt.
type Prompt struct {
	Arguments   []Argument    `json:"arguments,omitempty"`
	Description string        `json:"description,omitempty"`
	Handler     promptHandler `json:"-"`
	Name        string        `json:"name"`
}

// Registry holds registered prompts sorted alphabetically by name.
// Not safe for concurrent use — register all prompts before starting the server.
type Registry struct {
	index   map[string]int
	prompts []Prompt
}

// Result is the outcome of getting a prompt.
type Result struct {
	Description string    `json:"description,omitempty"`
	Messages    []Message `json:"messages"`
}

// promptHandler is the low-level function signature used internally.
type promptHandler func(ctx context.Context, args map[string]string) (Result, error)

// NewRegistry creates an empty prompt registry.
func NewRegistry() *Registry {
	return &Registry{
		index:   make(map[string]int),
		prompts: []Prompt{},
	}
}

// Lookup finds a prompt by name in O(1) via the index map.
func (r *Registry) Lookup(name string) (Prompt, bool) {
	i, ok := r.index[name]
	if !ok {
		return Prompt{}, false
	}
	return r.prompts[i], true
}

// Prompts returns a copy of all registered prompts in alphabetical order.
func (r *Registry) Prompts() []Prompt {
	return slices.Clone(r.prompts)
}

// Register adds a prompt to the registry. The generic type parameter T defines
// the prompt's argument struct — its exported fields (with json and description
// tags) are reflected to build the prompt arguments list.
func Register[T any](r *Registry, name, description string, handler func(ctx context.Context, input T) Result, opts ...Option) error {
	if _, exists := r.index[name]; exists {
		return fmt.Errorf("duplicate prompt name: %s", name)
	}

	arguments, err := deriveArguments[T]()
	if err != nil {
		return fmt.Errorf("derive arguments for prompt %q: %w", name, err)
	}

	wrapped := func(ctx context.Context, args map[string]string) (Result, error) {
		var input T
		raw, mErr := json.Marshal(args)
		if mErr != nil {
			return Result{}, &protocol.CodeError{
				Code:    protocol.InvalidParams,
				Message: fmt.Sprintf("invalid arguments for prompt %q: %v", name, mErr),
			}
		}
		if uErr := json.Unmarshal(raw, &input); uErr != nil {
			return Result{}, &protocol.CodeError{
				Code:    protocol.InvalidParams,
				Message: fmt.Sprintf("invalid arguments for prompt %q: %v", name, uErr),
			}
		}
		return handler(ctx, input), nil
	}

	prompt := Prompt{
		Arguments:   arguments,
		Description: description,
		Handler:     wrapped,
		Name:        name,
	}
	for _, opt := range opts {
		opt(&prompt)
	}

	r.prompts = append(r.prompts, prompt)
	slices.SortFunc(r.prompts, func(a, b Prompt) int {
		return cmp.Compare(a.Name, b.Name)
	})
	for i, p := range r.prompts {
		r.index[p.Name] = i
	}
	return nil
}

// UserMessage creates a Message with the user role.
func UserMessage(text string) Message {
	return Message{Content: ContentBlock{Text: text, Type: "text"}, Role: "user"}
}

// AssistantMessage creates a Message with the assistant role.
func AssistantMessage(text string) Message {
	return Message{Content: ContentBlock{Text: text, Type: "text"}, Role: "assistant"}
}

// deriveArguments reflects over struct T to build the prompt arguments list.
func deriveArguments[T any]() ([]Argument, error) {
	s, err := schema.DeriveInputSchema[T]()
	if err != nil {
		return nil, err
	}

	requiredSet := make(map[string]bool, len(s.Required))
	for _, r := range s.Required {
		requiredSet[r] = true
	}

	var args []Argument
	for name, prop := range s.Properties {
		args = append(args, Argument{
			Description: prop.Description,
			Name:        name,
			Required:    requiredSet[name],
		})
	}
	slices.SortFunc(args, func(a, b Argument) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return args, nil
}
