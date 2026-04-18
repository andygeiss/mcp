// Package main provides the init tool for rewriting the MCP template project.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// welcomeBanner is the post-success orientation message emitted to stderr after
// `make init` completes. Per FR5c (PRD amendment Story 1.4) and architecture
// D4, the banner names the next imperative steps and points at the README.
// Suppressed when rewriteProject returns an error so the user is not misled
// about scaffold state.
const welcomeBanner = `Your MCP server is running.

  Edit:   internal/tools/echo.go
  Wire:   cmd/mcp/main.go
  Verify: make smoke

Full guide: README.md
`

// emitWelcome writes the welcome banner to the supplied stderr writer. The
// io.Writer seam lets tests capture and assert against it. A write error here
// is non-fatal — the scaffold has already succeeded; we just lose the banner.
func emitWelcome(stderr io.Writer) {
	_, _ = fmt.Fprint(stderr, welcomeBanner)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// validateModulePath enforces the canonical host/owner/repo form. The rewriter
// derives a bare "owner/repo" slug from the module path to substitute into
// badge URLs (shields.io, codecov, GitHub). Paths with fewer than three
// segments have no such slug and would leave the template fingerprint behind.
func validateModulePath(modulePath string) error {
	parts := strings.Split(modulePath, "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return fmt.Errorf("run: invalid module path %q: must be of the form host/owner/repo (e.g. github.com/myorg/myrepo); template badge URLs require an owner/repo slug", modulePath)
	}
	return nil
}

func run() error {
	args := os.Args[1:]
	force := false
	positional := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--force" {
			force = true
			continue
		}
		positional = append(positional, a)
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/scaffold [--force] <module-path>")
		return errors.New("run: missing module path argument")
	}

	modulePath := strings.TrimRight(positional[0], "/")
	if err := validateModulePath(modulePath); err != nil {
		return err
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	fmt.Fprintf(os.Stderr, "initializing project: %s\n", modulePath)
	if err := rewriteProject(dir, modulePath, force); err != nil {
		return err
	}
	emitWelcome(os.Stderr)
	return nil
}
