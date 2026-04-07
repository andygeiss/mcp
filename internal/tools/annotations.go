package tools

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

// WithAnnotations returns an Option that attaches annotations to a tool.
func WithAnnotations(a Annotations) Option {
	return func(t *Tool) {
		t.Annotations = &a
	}
}
