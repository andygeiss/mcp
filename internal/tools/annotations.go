package tools

import "github.com/andygeiss/mcp/internal/schema"

// Annotations describes optional behavioral hints for a tool, enabling MCP
// clients to make informed decisions before invocation.
type Annotations struct {
	DestructiveHint bool   `json:"destructiveHint,omitempty"`
	IdempotentHint  bool   `json:"idempotentHint,omitempty"`
	OpenWorldHint   bool   `json:"openWorldHint,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint,omitempty"`
	Title           string `json:"title,omitempty"`
}

// Option configures a Tool during registration.
type Option func(*Tool)

// WithAnnotations returns an Option that attaches behavioral hint annotations to a tool.
func WithAnnotations(a Annotations) Option {
	return func(t *Tool) {
		t.Annotations = &a
	}
}

// WithOutputSchema returns an Option that declares a JSON Schema for the tool's
// structured output. Tools with an output schema may return [StructuredResult]
// to provide machine-readable content alongside the human-readable text.
func WithOutputSchema(out schema.OutputSchema) Option {
	return func(t *Tool) {
		t.OutputSchema = &out
	}
}

// WithTitle returns an Option that sets the tool's human-readable display name.
func WithTitle(title string) Option {
	return func(t *Tool) {
		t.Title = title
	}
}
