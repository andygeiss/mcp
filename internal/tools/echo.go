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

// Echo returns the input message as a text result.
func Echo(_ context.Context, input EchoInput) Result {
	return TextResult(input.Message)
}

// Register this tool in cmd/mcp/main.go with tools.Register[EchoInput](reg, "echo", Echo).
