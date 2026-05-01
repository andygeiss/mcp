// START HERE — your first tool. Edit, copy, rename. It's yours.

// Package tools holds the registered MCP tools and the registry primitives
// that wire them into the server.
package tools

import "context"

// EchoInput defines the parameters for the echo tool. This file is the starter
// tool — the first point of contact for new scaffold-forkers. Use it as a
// copy-target for your own tools.
type EchoInput struct {
	// The description tag drives the JSON Schema the agent sees. A wrong
	// description produces a misused tool — the agent will call it for the
	// wrong reasons.
	Message string `json:"message" description:"The message to echo back"`
}

// EchoOutput is the structured output Echo returns. The reflection engine
// derives the tool's outputSchema from this type so clients can validate the
// structuredContent on the response without ad-hoc string parsing.
type EchoOutput struct {
	Echoed string `json:"echoed" description:"The message that was echoed back"`
}

// Echo returns the input message as a typed EchoOutput. The dispatch wrapper
// auto-marshals the non-zero Out into Result.StructuredContent — no legacy
// text content block is emitted. New tools should follow this single-surface
// pattern: clients read structuredContent, the typed schema teaches them how
// to parse it. If you need the legacy text path for a particular client, add
// a TextResult(...) explicitly via the returned Result; do not double-emit
// by default.
func Echo(_ context.Context, input EchoInput) (EchoOutput, Result) {
	return EchoOutput{Echoed: input.Message}, Result{}
}

// Register this tool in cmd/mcp/main.go with
//
//	tools.Register[tools.EchoInput, tools.EchoOutput](reg, "echo", "...", tools.Echo)
