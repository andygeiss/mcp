//go:build ignore

// TOOL TEMPLATE — copy this file to a new .go file (lowercase, no leading
// underscore), rename the struct and handler, and delete the //go:build
// ignore line at the top. Then register it in cmd/mcp/main.go.
//
// Why the leading underscore AND the build tag? Belt and suspenders: the
// underscore keeps most editors from including the file in the build graph;
// the build tag guarantees `go build` and `go test` skip it even if an editor
// or tool forgets the filename convention.

package tools

import "context"

// YourToolInput describes the arguments your tool accepts. Each exported
// field becomes a JSON Schema property. The `description` struct tag is
// what the agent sees — write descriptions that teach the agent when to
// call your tool.
type YourToolInput struct {
	// Limit is optional because it is a pointer. Use pointer types for
	// optional fields; a missing field decodes to nil, not zero.
	Limit *int `json:"limit,omitempty" description:"Maximum number of results. Defaults to 10 when omitted."`

	// Query is a required field. Without an explicit default or pointer
	// type, the reflection engine marks it required in the JSON Schema.
	Query string `json:"query" description:"What the user is asking. Keep it one sentence."`
}

// YourTool is the handler. It runs in a context-bounded, sequentially-
// dispatched goroutine. Return a Result from the tools package — most tools
// use TextResult for free-form string output.
func YourTool(ctx context.Context, input YourToolInput) Result {
	_ = ctx
	_ = input
	return TextResult("replace me")
}

// Register in cmd/mcp/main.go:
//
//	tools.Register[YourToolInput](reg, "your-tool", YourTool)
