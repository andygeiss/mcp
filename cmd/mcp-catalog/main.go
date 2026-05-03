// Package main is the catalog binary that backs `make catalog` (FR11).
//
// The binary invokes the local mcp server in `--inspect-only` mode, parses
// the resulting JSON via internal/inspect, and renders a Markdown catalog
// to stdout via internal/catalog.Render. The Makefile target redirects this
// stdout into `docs/TOOLS.md` (with a fail-on-drift compare so commits stay
// in sync with the registry).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/andygeiss/mcp/internal/catalog"
	"github.com/andygeiss/mcp/internal/inspect"
)

const inspectTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mcp-catalog:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), inspectTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/mcp/", "--inspect-only")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("invoke mcp --inspect-only: %w", err)
	}

	var got inspect.Output
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		return fmt.Errorf("parse inspect output: %w", err)
	}

	return catalog.Render(got, os.Stdout)
}
