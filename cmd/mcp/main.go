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
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		_, _ = os.Stdout.WriteString(version + "\n")
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	registry := tools.NewRegistry()
	// Register tools here. The echo tool is a minimal reference (~5 lines);
	// the search tool is a full-featured example. Replace or extend with your own.
	// See internal/tools/echo.go and internal/tools/search.go for patterns.
	tools.Register(registry, "echo", "Echoes the input message", tools.Echo)
	tools.Register(registry, "search", "Searches files for a pattern", tools.Search,
		tools.WithAnnotations(tools.Annotations{ReadOnlyHint: true}),
	)

	srv := server.NewServer("mcp", version, registry, os.Stdin, os.Stdout, os.Stderr,
		server.WithTrace(os.Getenv("MCP_TRACE") == "1"),
	)
	return srv.Run(ctx)
}
