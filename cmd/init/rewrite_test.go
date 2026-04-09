package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
)

func Test_DeriveProjectName_With_ModulePath_Should_ReturnLastSegment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected string
		input    string
		name     string
	}{
		{name: "three segments", input: "github.com/org/mytool", expected: "mytool"},
		{name: "two segments", input: "example.com/tool", expected: "tool"},
		{name: "no slash", input: "standalone", expected: "standalone"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.That(t, "project name", deriveProjectName(tt.input), tt.expected)
		})
	}
}

func Test_RewriteGoMod_With_ValidPath_Should_ReplaceModuleLine(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	goMod := "module github.com/andygeiss/mcp\n\ngo 1.26.1\n"
	err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = rewriteGoMod(dir, "github.com/test-org/test-tool")

	// Assert
	assert.That(t, "rewrite error", err, nil)
	data, _ := readFile(filepath.Join(dir, "go.mod"))
	assert.That(t, "content", string(data), "module github.com/test-org/test-tool\n\ngo 1.26.1\n")
}

func Test_RewriteImportsInFile_With_TemplateImports_Should_ReplaceAll(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	src := `package main

import (
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)
`
	path := filepath.Join(dir, "main.go")
	err := os.WriteFile(path, []byte(src), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = rewriteImportsInFile(path, "github.com/test-org/test-tool")

	// Assert
	assert.That(t, "rewrite error", err, nil)
	data, _ := readFile(path)
	expected := `package main

import (
	"github.com/test-org/test-tool/internal/server"
	"github.com/test-org/test-tool/internal/tools"
)
`
	assert.That(t, "content", string(data), expected)
}

func Test_RewriteImportsInFile_With_AliasedImport_Should_ReplaceCorrectly(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	src := `package main

import (
	json "encoding/json"

	"github.com/andygeiss/mcp/internal/protocol"
)
`
	path := filepath.Join(dir, "main.go")
	err := os.WriteFile(path, []byte(src), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = rewriteImportsInFile(path, "github.com/test-org/test-tool")

	// Assert
	assert.That(t, "rewrite error", err, nil)
	data, _ := readFile(path)
	expected := `package main

import (
	json "encoding/json"

	"github.com/test-org/test-tool/internal/protocol"
)
`
	assert.That(t, "content", string(data), expected)
}

func Test_RenameBinaryDir_With_NewName_Should_MoveDirectory(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	oldDir := filepath.Join(dir, "cmd", "mcp")
	err := os.MkdirAll(oldDir, 0o750)
	assert.That(t, "mkdir error", err, nil)
	err = os.WriteFile(filepath.Join(oldDir, "main.go"), []byte("package main\n"), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = renameBinaryDir(dir, "mytool")

	// Assert
	assert.That(t, "rename error", err, nil)
	_, err = os.Stat(filepath.Join(dir, "cmd", "mytool", "main.go"))
	assert.That(t, "new file exists", err, nil)
	_, err = os.Stat(oldDir)
	assert.That(t, "old dir gone", os.IsNotExist(err), true)
}

func Test_RenameBinaryDir_With_SameName_Should_BeIdempotent(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	existingDir := filepath.Join(dir, "cmd", "mcp")
	err := os.MkdirAll(existingDir, 0o750)
	assert.That(t, "mkdir error", err, nil)

	// Act — rename to same name
	err = renameBinaryDir(dir, "mcp")

	// Assert — no error
	assert.That(t, "rename error", err, nil)
}

func Test_Init_With_SamePathTwice_Should_ProduceIdenticalResults(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()

	// Create minimal project structure
	err := os.MkdirAll(filepath.Join(dir, "cmd", "mcp"), 0o750)
	assert.That(t, "mkdir cmd", err, nil)

	goMod := "module github.com/andygeiss/mcp\n\ngo 1.26.1\n"
	err = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600)
	assert.That(t, "write go.mod", err, nil)

	mainGo := `package main

import "github.com/andygeiss/mcp/internal/server"

var _ = server.NewServer
`
	err = os.WriteFile(filepath.Join(dir, "cmd", "mcp", "main.go"), []byte(mainGo), 0o600)
	assert.That(t, "write main.go", err, nil)

	newPath := "github.com/test-org/test-tool"

	// Act — first rewrite (without go mod tidy, fingerprint check, or self-cleanup)
	err = rewriteGoMod(dir, newPath)
	assert.That(t, "first gomod", err, nil)
	err = rewriteGoFiles(dir, newPath)
	assert.That(t, "first gofiles", err, nil)
	err = renameBinaryDir(dir, "test-tool")
	assert.That(t, "first rename", err, nil)

	// Read state after first run
	goModAfterFirst, _ := readFile(filepath.Join(dir, "go.mod"))
	mainGoAfterFirst, _ := readFile(filepath.Join(dir, "cmd", "test-tool", "main.go"))

	// Act — second rewrite
	err = rewriteGoMod(dir, newPath)
	assert.That(t, "second gomod", err, nil)
	err = rewriteGoFiles(dir, newPath)
	assert.That(t, "second gofiles", err, nil)
	err = renameBinaryDir(dir, "test-tool")
	assert.That(t, "second rename", err, nil)

	// Assert — identical after second run
	goModAfterSecond, _ := readFile(filepath.Join(dir, "go.mod"))
	mainGoAfterSecond, _ := readFile(filepath.Join(dir, "cmd", "test-tool", "main.go"))

	assert.That(t, "go.mod identical", string(goModAfterSecond), string(goModAfterFirst))
	assert.That(t, "main.go identical", string(mainGoAfterSecond), string(mainGoAfterFirst))
}

func Test_DeriveProjectName_With_TrailingSlash_Should_TrimAndReturn(t *testing.T) {
	t.Parallel()

	// Act / Assert
	assert.That(t, "trailing slash", deriveProjectName("github.com/org/tool/"), "tool")
	assert.That(t, "multiple slashes", deriveProjectName("github.com/org/tool///"), "tool")
}

func Test_RewriteProject_With_ExtendedTemplatePath_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	err := rewriteProject(t.TempDir(), "github.com/andygeiss/mcp-extended")

	// Assert
	if err == nil {
		t.Fatal("expected error for extended template path")
	}
}
