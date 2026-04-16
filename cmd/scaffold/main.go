// Package main provides the init tool for rewriting the MCP template project.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

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
	fmt.Fprintf(os.Stderr, "project initialized successfully\n")
	return nil
}
