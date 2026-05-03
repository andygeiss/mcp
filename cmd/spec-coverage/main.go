// Package main is the spec-coverage aggregator: it reads the per-package
// audit fragments committed under docs/, deduplicates rows by clause ID,
// sorts ascending by ID, and writes the unified table to stdout.
//
// Per-package fragments are produced by the renderer tests colocated with
// each package whose _test.go init() blocks register protocol.Clause
// entries (e.g. internal/protocol → docs/spec-coverage.protocol.txt;
// internal/server → docs/spec-coverage.server.txt). Those tests fail with
// regen guidance on drift, the same UX as the aggregate step that runs
// after this binary writes its output.
//
// To extend: when a new package starts registering clauses, (1) add a
// Test_RenderSpecCoverage_Should_Match{Pkg}Fragment test in that package,
// (2) commit a docs/spec-coverage.{pkg}.txt fragment, (3) append the
// fragment path to fragmentPaths below.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// fragmentPaths lists the per-package fragment files relative to the
// repo root. Order is insignificant — rows are merged and sorted by
// clause ID before emission.
var fragmentPaths = []string{
	"docs/spec-coverage.inspect.txt",
	"docs/spec-coverage.protocol.txt",
	"docs/spec-coverage.schema.txt",
	"docs/spec-coverage.server.txt",
}

const header = "ID\tLevel\tSection\tTests\n"

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "spec-coverage:", err)
		os.Exit(1)
	}
}

func run(out io.Writer) error {
	root, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("locate repo root: %w", err)
	}

	rows, err := collectRows(root, fragmentPaths)
	if err != nil {
		return err
	}

	sort.Strings(rows)

	w := bufio.NewWriter(out)
	if _, err := w.WriteString(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, row := range rows {
		if _, err := w.WriteString(row + "\n"); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	return w.Flush()
}

// collectRows reads each fragment under root, drops the header line of
// each, and returns the union of remaining rows. Duplicate clause IDs
// (first column) are an author error — fail loudly. Intra-fragment
// duplicates and cross-fragment duplicates produce distinct error
// messages so the offending edit is unambiguous.
func collectRows(root string, paths []string) ([]string, error) {
	seen := map[string]string{} // clause ID → source fragment path
	var rows []string
	for _, rel := range paths {
		fragmentRows, err := readFragment(filepath.Join(root, rel), rel)
		if err != nil {
			return nil, err
		}
		for _, row := range fragmentRows {
			id, _, _ := strings.Cut(row, "\t")
			if prev, dup := seen[id]; dup {
				if prev == rel {
					return nil, fmt.Errorf("intra-fragment duplicate clause ID %q in %s", id, rel)
				}
				return nil, fmt.Errorf("cross-fragment duplicate clause ID %q: appears in %s and %s", id, prev, rel)
			}
			seen[id] = rel
			rows = append(rows, row)
		}
	}
	return rows, nil
}

// readFragment loads a fragment file, drops its header line, and returns
// non-empty rows. Each row must have exactly four tab-separated columns
// (ID, Level, Section, Tests) — anything else is a hand-edit mistake or
// a Render-format change that needs to surface here, not silently flow
// into the canonical aggregate. An empty fragment (header only) is valid
// and returns nil.
func readFragment(abs, rel string) ([]string, error) {
	data, err := os.ReadFile(abs) //nolint:gosec // known repo-relative fragment paths
	if err != nil {
		return nil, fmt.Errorf("read fragment %s: %w", rel, err)
	}
	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte{'\n'})
	if len(lines) <= 1 {
		return nil, nil
	}
	rows := make([]string, 0, len(lines)-1)
	for i, line := range lines[1:] {
		row := string(line)
		if row == "" {
			continue
		}
		cols := strings.Split(row, "\t")
		if len(cols) != 4 || cols[0] == "" {
			return nil, fmt.Errorf("fragment %s row %d: expected 4 tab-separated columns (ID, Level, Section, Tests), got %d", rel, i+1, len(cols))
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
