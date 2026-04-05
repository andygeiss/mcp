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

func run() error {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/init <module-path>")
		return errors.New("missing module path argument")
	}

	modulePath := strings.TrimRight(os.Args[1], "/")
	if !strings.Contains(modulePath, "/") {
		return errors.New("invalid module path: must contain at least one '/'")
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	fmt.Fprintf(os.Stderr, "initializing project: %s\n", modulePath)
	if err := rewriteProject(dir, modulePath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "project initialized successfully\n")
	return nil
}
