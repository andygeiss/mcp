package server_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test_Architecture_With_ImportGraph_Should_MatchDocumentation verifies the
// package dependency direction documented in CLAUDE.md:
//   - protocol imports zero internal packages
//   - tools never imports server
//   - server never imports cmd
//   - assert imports zero internal packages
func Test_Architecture_With_ImportGraph_Should_MatchDocumentation(t *testing.T) {
	t.Parallel()

	const module = "github.com/andygeiss/mcp/internal/"

	rules := []struct {
		pkg        string
		disallowed []string
	}{
		{"internal/protocol", []string{module}},                        // imports nothing internal
		{"internal/tools", []string{module + "server"}},                // never imports server
		{"internal/server", []string{"github.com/andygeiss/mcp/cmd/"}}, // never imports cmd
		{"internal/pkg/assert", []string{module}},                      // imports nothing internal
	}

	for _, rule := range rules {
		pkgDir := filepath.Join("..", "..", rule.pkg)
		imports := extractImports(t, pkgDir)
		for _, imp := range imports {
			for _, disallowed := range rule.disallowed {
				if strings.HasPrefix(imp, disallowed) {
					t.Errorf("%s imports %s (violates dependency rule: must not import %s*)", rule.pkg, imp, disallowed)
				}
			}
		}
	}
}

func extractImports(t *testing.T, dir string) []string {
	t.Helper()

	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		t.Fatalf("read dir %s: %v", absDir, err)
	}

	fset := token.NewFileSet()
	seen := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(absDir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			seen[strings.Trim(imp.Path.Value, `"`)] = true
		}
	}

	result := make([]string, 0, len(seen))
	for path := range seen {
		result = append(result, path)
	}
	return result
}
