// Package main is the entry point for the mcp-repl binary — the
// Postman-for-MCP interactive client that backs `make repl` (FR9).
//
// The binary spawns the local `mcp` server via `go run ./cmd/mcp/` so the
// REPL works against the in-tree build without a separate install step.
// Operators wanting to point the REPL at a pre-built binary can override the
// MCP_REPL_SERVER environment variable with a path to an mcp executable.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/andygeiss/mcp/internal/repl"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mcp-repl:", err)
		os.Exit(1)
	}
}

func run() error {
	// Go 1.26: NotifyContext propagates the signal as the context cause so
	// downstream wait loops can surface the originating signal name.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	args := serverArgs()
	server := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // hardcoded server invocation, optional path via env
	return repl.Run(ctx, os.Stdin, os.Stdout, os.Stderr, server)
}

// serverArgs returns the argv to spawn for the mcp server. Defaults to
// `go run ./cmd/mcp/` so `make repl` works against the in-tree build with no
// additional setup; an operator pointing the REPL at an installed binary can
// set MCP_REPL_SERVER to that path.
func serverArgs() []string {
	if path := os.Getenv("MCP_REPL_SERVER"); path != "" {
		return []string{path}
	}
	return []string{"go", "run", "./cmd/mcp/"}
}
