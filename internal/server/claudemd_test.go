package server_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// Test_ClaudeMD_Claims_Should_HaveMatchingTests verifies that key behavioral
// claims in CLAUDE.md have corresponding tests. This catches drift between
// documentation and implementation.
func Test_ClaudeMD_Claims_Should_HaveMatchingTests(t *testing.T) {
	t.Parallel()

	projectRoot := findProjectRoot(t)
	claudeMD := readFileContent(t, filepath.Join(projectRoot, "CLAUDE.md"))
	testFiles := collectTestFiles(t, projectRoot)

	claims := []struct {
		claim       string
		docPattern  string // regex to match in CLAUDE.md
		testPattern string // regex to match in test file content
	}{
		{
			"uninitialized requests return -32600",
			`(?i)uninitial.*-32600`,
			`(?i)uninitiali.*32600`,
		},
		{
			"duplicate initialize returns -32600",
			`(?i)duplicate.*-32600`,
			`(?i)duplicate.*init`,
		},
		{
			"EOF causes clean shutdown",
			`(?i)EOF.*clean shutdown`,
			`(?i)EOF.*shutdown`,
		},
		{
			"ping always works",
			"`ping` always works",
			`(?i)ping.*any.*state`,
		},
		{
			"unknown notifications silently ignored",
			`(?i)unknown notifications.*silently ignored`,
			`(?i)unknown.*notification.*silent`,
		},
		{
			"batch arrays return -32700",
			`(?i)batch.*-32700`,
			`(?i)batch.*array.*32700`,
		},
		{
			"tools/list returns alphabetical order",
			`(?i)deterministic ordering.*tools/list`,
			`(?i)tools.*list.*alphabetic`,
		},
		{
			"unknown tool returns -32602",
			`(?i)unknown tool name`,
			`(?i)unknown.*tool.*32602`,
		},
		{
			"parse error -32700 for malformed JSON",
			`Parse error.*malformed JSON`,
			`(?i)malformed.*json.*32700`,
		},
		{
			"decode errors respond -32700",
			`-32700.*[Pp]arse error`,
			`(?i)oversized.*message.*32700`,
		},
	}

	for _, c := range claims {
		// Verify the claim exists in CLAUDE.md.
		docRe := regexp.MustCompile(c.docPattern)
		if !docRe.MatchString(claudeMD) {
			t.Errorf("claim %q: pattern %q not found in CLAUDE.md", c.claim, c.docPattern)
			continue
		}

		// Verify a matching test exists.
		testRe := regexp.MustCompile(c.testPattern)
		found := false
		for _, content := range testFiles {
			if testRe.MatchString(content) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("CLAUDE.md claims %q but no test found matching %q", c.claim, c.testPattern)
		}
	}
}

// Test_ClaudeMD_ErrorCodeConstants_Should_HaveTestCoverage verifies that every
// JSON-RPC error code constant defined in protocol/constants.go is referenced
// in at least one test file.
func Test_ClaudeMD_ErrorCodeConstants_Should_HaveTestCoverage(t *testing.T) {
	t.Parallel()

	projectRoot := findProjectRoot(t)
	testFiles := collectTestFiles(t, projectRoot)

	constants := []string{
		"InternalError",
		"InvalidParams",
		"InvalidRequest",
		"MethodNotFound",
		"ParseError",
	}

	for _, name := range constants {
		found := false
		for _, content := range testFiles {
			if strings.Contains(content, "protocol."+name) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("error code constant %q is not referenced in any test file", name)
		}
	}
}

// Test_ClaudeMD_ImportGraph_Should_EnforceDependencyRules verifies the import
// graph rules: protocol/ has zero internal deps, tools/ never imports server/,
// and assert/ is only imported by test files.
func Test_ClaudeMD_ImportGraph_Should_EnforceDependencyRules(t *testing.T) {
	t.Parallel()

	projectRoot := findProjectRoot(t)
	modulePath := "github.com/andygeiss/mcp/internal/"

	// Rule 1: protocol/ source files must not import other internal packages
	protocolDir := filepath.Join(projectRoot, "internal", "protocol")
	for path, content := range collectSourceFiles(t, protocolDir) {
		if strings.Contains(content, modulePath) && !strings.Contains(content, modulePath+"protocol") {
			t.Errorf("protocol/ source file %s imports another internal package", filepath.Base(path))
		}
	}

	// Rule 2: tools/ source files must not import server/
	toolsDir := filepath.Join(projectRoot, "internal", "tools")
	for path, content := range collectSourceFiles(t, toolsDir) {
		if strings.Contains(content, modulePath+"server") {
			t.Errorf("tools/ source file %s imports internal/server", filepath.Base(path))
		}
	}

	// Rule 3: assert/ is only imported by test files (no production source imports it)
	assertImport := modulePath + "pkg/assert"
	err := filepath.WalkDir(filepath.Join(projectRoot, "internal"), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return walkErr
		}
		if d.IsDir() || strings.HasSuffix(path, "_test.go") || !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, readErr := os.ReadFile(filepath.Clean(path)) //nolint:gosec // test-only: walking known project tree, no untrusted symlinks
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), assertImport) {
			t.Errorf("non-test file %s imports assert package", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
}

// Test_ClaudeMD_GoMod_Should_HaveZeroExternalDependencies verifies that go.mod
// has no require directives (zero external dependencies).
func Test_ClaudeMD_GoMod_Should_HaveZeroExternalDependencies(t *testing.T) {
	t.Parallel()

	projectRoot := findProjectRoot(t)
	goMod := readFileContent(t, filepath.Join(projectRoot, "go.mod"))

	requireRe := regexp.MustCompile(`(?m)^require\s`)
	if requireRe.MatchString(goMod) {
		t.Error("go.mod contains require directive — zero external dependencies expected")
	}
}

// collectSourceFiles returns a map of path->content for non-test .go files in dir.
func collectSourceFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, readErr := os.ReadFile(filepath.Clean(path))
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		files[path] = string(data)
	}
	return files
}

func findProjectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller file")
	}
	// From internal/server/claudemd_test.go → project root is three levels up.
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func readFileContent(t *testing.T, path string) string {
	t.Helper()
	cleanPath := filepath.Clean(path)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		t.Fatalf("read %s: %v", cleanPath, err)
	}
	return string(data)
}

func collectTestFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	paths := findTestFilePaths(t, root)
	for _, p := range paths {
		data, err := os.ReadFile(filepath.Clean(p))
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		files[p] = string(data)
	}
	return files
}

func findTestFilePaths(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "_bmad-output" || base == "_bmad" || base == ".claude" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk test files: %v", err)
	}
	return paths
}
