// Package main provides the entry point for the MCP server binary.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	registry := tools.NewRegistry()
	// Register tools here. The search tool is included as a reference
	// implementation — replace or extend with your own tools.
	// See internal/tools/search.go for the implementation pattern.
	tools.Register(registry, "search", "Searches files for a pattern", tools.Search)

	srv := server.NewServer("mcp", version, registry, os.Stdin, os.Stdout, os.Stderr,
		server.WithTrace(os.Getenv("MCP_TRACE") == "1"),
	)
	return srv.Run(ctx)
}
