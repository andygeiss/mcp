// Package main provides the entry point for the MCP server binary.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/andygeiss/mcp/internal/doctor"
	"github.com/andygeiss/mcp/internal/inspect"
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		_, _ = os.Stderr.WriteString(version + "\n")
		return nil
	}

	// --inspect-only is a one-shot CLI mode: dump the registered surface as
	// JSON to stdout, then exit. Does NOT touch stdin, does NOT enter the
	// dispatch loop. Wire-discipline carve-out per NFR6 — server-mode and
	// inspect-mode never coexist within a single process invocation. Shared
	// inspector primitive lives in internal/inspect; reused by mcp doctor
	// (FR10) and make catalog (FR11).
	if len(os.Args) == 2 && os.Args[1] == "--inspect-only" {
		registry, err := buildToolRegistry()
		if err != nil {
			return err
		}
		return inspect.Inspect("mcp", version, registry, nil, nil, os.Stdout)
	}

	// `mcp doctor` — read-only configuration validator (FR10). CLI mode:
	// structured human output to stdout, FAIL rows mirrored to stderr,
	// non-zero exit when any check fails. Never modifies a client config.
	if len(os.Args) >= 2 && os.Args[1] == "doctor" {
		return doctor.Run(os.Args[2:], version, os.Stdout, os.Stderr)
	}

	// Go 1.26: NotifyContext propagates the signal as the context cause,
	// so context.Cause(ctx) inside the server can surface the signal name
	// in the shutdown log.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	registry, err := buildToolRegistry()
	if err != nil {
		return err
	}

	srv := server.NewServer("mcp", version, registry, os.Stdin, os.Stdout, os.Stderr,
		server.WithTrace(os.Getenv("MCP_TRACE") == "1"),
	)
	return srv.Run(ctx)
}

// buildToolRegistry constructs the production tool registry. Extracted into a
// helper so that --inspect-only and the server path register the identical
// set of tools — the inspector cannot lie about what the server exposes.
func buildToolRegistry() (*tools.Registry, error) {
	registry := tools.NewRegistry()
	// Register tools here. The echo tool is a minimal reference (~5 lines).
	// Replace or extend with your own. See internal/tools/echo.go for the pattern.
	if err := tools.Register[tools.EchoInput, tools.EchoOutput](registry, "echo", "Echoes the input message", tools.Echo); err != nil {
		return nil, fmt.Errorf("register echo: %w", err)
	}
	return registry, nil
}
