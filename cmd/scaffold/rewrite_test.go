package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
)

// repoShortFixture is a fixture short-form repo path used by test tables;
// hoisted so the goconst linter does not flag the same literal across rows.
const repoShortFixture = "myorg/mytool"

// isolateGit neutralizes the developer's global and system git configuration
// and sets a throwaway identity so `git commit` succeeds deterministically in
// tests and on a clean CI runner. Callers must not use t.Parallel — t.Setenv
// is incompatible with parallel tests.
func isolateGit(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
}

func Test_RepoShortForm_With_VariousPaths_Should_ReturnOwnerRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "github owner/repo", input: "github.com/" + repoShortFixture, expected: repoShortFixture},
		{name: "github with v2 suffix", input: "github.com/" + repoShortFixture + "/v2", expected: repoShortFixture},
		{name: "gitlab host", input: "gitlab.com/group/project", expected: "group/project"},
		{name: "custom host with subpath", input: "example.com/a/b/c", expected: "a/b"},
		{name: "trailing slash", input: "github.com/" + repoShortFixture + "/", expected: repoShortFixture},
		{name: "two segments only", input: "foo/bar", expected: ""},
		{name: "single segment", input: "single", expected: ""},
		{name: "empty string", input: "", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.That(t, "short form", repoShortForm(tt.input), tt.expected)
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

	// Read state after first run
	goModAfterFirst, _ := readFile(filepath.Join(dir, "go.mod"))
	mainGoAfterFirst, _ := readFile(filepath.Join(dir, "cmd", "mcp", "main.go"))

	// Act — second rewrite
	err = rewriteGoMod(dir, newPath)
	assert.That(t, "second gomod", err, nil)
	err = rewriteGoFiles(dir, newPath)
	assert.That(t, "second gofiles", err, nil)

	// Assert — identical after second run
	goModAfterSecond, _ := readFile(filepath.Join(dir, "go.mod"))
	mainGoAfterSecond, _ := readFile(filepath.Join(dir, "cmd", "mcp", "main.go"))

	assert.That(t, "go.mod identical", string(goModAfterSecond), string(goModAfterFirst))
	assert.That(t, "main.go identical", string(mainGoAfterSecond), string(mainGoAfterFirst))
}

func Test_RewriteProject_With_ExtendedTemplatePath_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	err := rewriteProject(t.TempDir(), "github.com/andygeiss/mcp-extended", false)

	// Assert
	if err == nil {
		t.Fatal("expected error for extended template path")
	}
}

func Test_IsTextFile_With_KnownExtensions_Should_ReturnTrue(t *testing.T) {
	t.Parallel()

	textExtensions := []string{".go", ".md", ".mod", ".sum", ".yml", ".yaml", ".json", ".toml", ".txt", ".cfg"}
	for _, ext := range textExtensions {
		t.Run(ext, func(t *testing.T) {
			t.Parallel()
			assert.That(t, "is text", isTextFile("file"+ext), true)
		})
	}
}

func Test_IsTextFile_With_BinaryExtensions_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	nonTextExtensions := []string{".png", ".jpg", ".exe", ".bin", ".pdf", ""}
	for _, ext := range nonTextExtensions {
		name := ext
		if name == "" {
			name = "no_extension"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.That(t, "is text", isTextFile("file"+ext), false)
		})
	}
}

func Test_RewriteTextFile_With_TemplateReferences_Should_ReplaceAll(t *testing.T) {
	t.Parallel()

	// Arrange — cover full-form, short-form (badge URL), and cmd/mcp path.
	dir := t.TempDir()
	content := "module: github.com/andygeiss/mcp\n" +
		"badge: https://img.shields.io/github/v/release/andygeiss/mcp\n" +
		"codecov: https://codecov.io/gh/andygeiss/mcp\n" +
		"binary: cmd/mcp/ and cmd/mcp run\n"
	path := filepath.Join(dir, "README.md")
	err := os.WriteFile(path, []byte(content), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = rewriteTextFile(path, "github.com/test-org/test-tool")

	// Assert — module path and bare owner/repo are replaced; cmd/mcp is untouched.
	assert.That(t, "rewrite error", err, nil)
	data, _ := readFile(path)
	expected := "module: github.com/test-org/test-tool\n" +
		"badge: https://img.shields.io/github/v/release/test-org/test-tool\n" +
		"codecov: https://codecov.io/gh/test-org/test-tool\n" +
		"binary: cmd/mcp/ and cmd/mcp run\n"
	assert.That(t, "content", string(data), expected)
}

func Test_RewriteTextFile_With_NonGitHubModule_Should_StillReplaceShortForm(t *testing.T) {
	t.Parallel()

	// Arrange — module path not on github.com still yields an owner/repo short form.
	dir := t.TempDir()
	content := "see github.com/andygeiss/mcp and badge release/andygeiss/mcp\n"
	path := filepath.Join(dir, "README.md")
	err := os.WriteFile(path, []byte(content), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = rewriteTextFile(path, "gitlab.com/group/project")

	// Assert
	assert.That(t, "rewrite error", err, nil)
	data, _ := readFile(path)
	expected := "see gitlab.com/group/project and badge release/group/project\n"
	assert.That(t, "content", string(data), expected)
}

func Test_RewriteTextFile_With_GoreleaserCmdPath_Should_PreserveBytes(t *testing.T) {
	t.Parallel()

	// Arrange — realistic .goreleaser.yml fragment: cmd/mcp paths present,
	// but no template fingerprint. rewriteTextFile must be a no-op so
	// goreleaser keeps building ./cmd/mcp/ and emitting a binary named mcp.
	dir := t.TempDir()
	content := "project_name: mcp\nbuilds:\n  - main: ./cmd/mcp/\n    binary: mcp\n"
	path := filepath.Join(dir, ".goreleaser.yml")
	err := os.WriteFile(path, []byte(content), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = rewriteTextFile(path, "github.com/test-org/test-tool")

	// Assert — byte-identical; no cmd/mcp substitution snuck in.
	assert.That(t, "rewrite error", err, nil)
	data, _ := readFile(path)
	assert.That(t, "content unchanged", string(data), content)
}

func Test_RewriteTextFile_With_NoTemplateReferences_Should_BeIdempotent(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	content := "nothing to replace here\n"
	path := filepath.Join(dir, "notes.txt")
	err := os.WriteFile(path, []byte(content), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = rewriteTextFile(path, "github.com/test-org/test-tool")

	// Assert
	assert.That(t, "rewrite error", err, nil)
	data, _ := readFile(path)
	assert.That(t, "content unchanged", string(data), content)
}

func Test_RewriteTextFile_With_MissingFile_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	// Act
	err := rewriteTextFile(filepath.Join(t.TempDir(), "nonexistent.md"), "github.com/x/y")

	// Assert
	assert.That(t, "error", err, nil)
}

func Test_RewriteTextFiles_With_MixedFiles_Should_RewriteTextOnly(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	// Create a text file with template references
	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("github.com/andygeiss/mcp is great"), 0o600)
	assert.That(t, "write md", err, nil)
	// Create a .go file (should be skipped by rewriteTextFiles)
	err = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main // github.com/andygeiss/mcp"), 0o600)
	assert.That(t, "write go", err, nil)
	// Create a binary file (should be skipped)
	err = os.WriteFile(filepath.Join(dir, "image.png"), []byte("github.com/andygeiss/mcp"), 0o600)
	assert.That(t, "write png", err, nil)
	// Create an empty file (should be skipped)
	err = os.WriteFile(filepath.Join(dir, "empty.md"), []byte(""), 0o600)
	assert.That(t, "write empty", err, nil)

	// Act
	err = rewriteTextFiles(dir, "github.com/new/mod")

	// Assert
	assert.That(t, "error", err, nil)
	mdData, _ := readFile(filepath.Join(dir, "README.md"))
	assert.That(t, "md rewritten", string(mdData), "github.com/new/mod is great")
	goData, _ := readFile(filepath.Join(dir, "main.go"))
	assert.That(t, "go untouched", string(goData), "package main // github.com/andygeiss/mcp")
	pngData, _ := readFile(filepath.Join(dir, "image.png"))
	assert.That(t, "png untouched", string(pngData), "github.com/andygeiss/mcp")
}

func Test_SelfCleanup_With_ExistingDir_Should_RemoveIt(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	scaffoldDir := filepath.Join(dir, "cmd", "scaffold")
	err := os.MkdirAll(scaffoldDir, 0o750)
	assert.That(t, "mkdir error", err, nil)
	err = os.WriteFile(filepath.Join(scaffoldDir, "main.go"), []byte("package main\n"), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = selfCleanup(dir)

	// Assert
	assert.That(t, "cleanup error", err, nil)
	_, err = os.Stat(scaffoldDir)
	assert.That(t, "dir gone", os.IsNotExist(err), true)
}

func Test_SelfCleanup_With_MissingDir_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	// Act
	err := selfCleanup(t.TempDir())

	// Assert
	assert.That(t, "error", err, nil)
}

func Test_RemoveTemplateOnlyContent_With_AllPaths_Should_RemoveThem(t *testing.T) {
	t.Parallel()

	// Arrange — populate every template-only path with realistic content.
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Andy's conventions\n"), 0o600)
	assert.That(t, "write CLAUDE.md", err, nil)
	err = os.MkdirAll(filepath.Join(dir, "_bmad", "sub"), 0o750)
	assert.That(t, "mkdir _bmad", err, nil)
	err = os.WriteFile(filepath.Join(dir, "_bmad", "sub", "config.yaml"), []byte("key: value\n"), 0o600)
	assert.That(t, "write bmad file", err, nil)
	err = os.MkdirAll(filepath.Join(dir, "_bmad-output", "run"), 0o750)
	assert.That(t, "mkdir _bmad-output", err, nil)
	err = os.MkdirAll(filepath.Join(dir, claudeDirName, "skills"), 0o750)
	assert.That(t, "mkdir .claude", err, nil)

	// Act
	err = removeTemplateOnlyContent(dir)

	// Assert
	assert.That(t, "remove error", err, nil)
	for _, name := range []string{claudeDirName, "CLAUDE.md", "_bmad", "_bmad-output"} {
		_, statErr := os.Stat(filepath.Join(dir, name))
		assert.That(t, name+" gone", os.IsNotExist(statErr), true)
	}
}

func Test_RemoveTemplateOnlyContent_With_MissingPaths_Should_BeIdempotent(t *testing.T) {
	t.Parallel()

	// Act — removing from an empty dir must not error.
	err := removeTemplateOnlyContent(t.TempDir())

	// Assert
	assert.That(t, "error", err, nil)
}

func Test_RemoveBuildArtifacts_With_Binaries_Should_RemoveThem(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "mcp"), []byte("binary"), 0o600)
	assert.That(t, "write mcp", err, nil)
	err = os.WriteFile(filepath.Join(dir, "scaffold"), []byte("binary"), 0o600)
	assert.That(t, "write scaffold", err, nil)

	// Act
	err = removeBuildArtifacts(dir)

	// Assert
	assert.That(t, "remove error", err, nil)
	_, err = os.Stat(filepath.Join(dir, "mcp"))
	assert.That(t, "mcp gone", os.IsNotExist(err), true)
	_, err = os.Stat(filepath.Join(dir, "scaffold"))
	assert.That(t, "scaffold gone", os.IsNotExist(err), true)
}

func Test_RemoveBuildArtifacts_With_NoBinaries_Should_BeIdempotent(t *testing.T) {
	t.Parallel()

	// Act
	err := removeBuildArtifacts(t.TempDir())

	// Assert
	assert.That(t, "error", err, nil)
}

func Test_RemoveBuildArtifacts_With_Directory_Should_SkipIt(t *testing.T) {
	t.Parallel()

	// Arrange — create a directory named "mcp" (not a file)
	dir := t.TempDir()
	err := os.Mkdir(filepath.Join(dir, "mcp"), 0o750)
	assert.That(t, "mkdir error", err, nil)

	// Act
	err = removeBuildArtifacts(dir)

	// Assert — directory should not be removed
	assert.That(t, "remove error", err, nil)
	_, err = os.Stat(filepath.Join(dir, "mcp"))
	assert.That(t, "dir still exists", err, nil)
}

func Test_VerifyZeroFingerprint_With_NoTemplateRefs_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("clean project"), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = verifyZeroFingerprint(dir)

	// Assert
	assert.That(t, "error", err, nil)
}

func Test_VerifyZeroFingerprint_With_TemplateRefs_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("uses github.com/andygeiss/mcp"), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = verifyZeroFingerprint(dir)

	// Assert
	if err == nil {
		t.Fatal("expected error for remaining template references")
	}
}

func Test_VerifyZeroFingerprint_With_OnlyShortForm_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — only the bare owner/repo form is present (e.g., missed badge URL).
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("badge: release/andygeiss/mcp\n"), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = verifyZeroFingerprint(dir)

	// Assert
	if err == nil {
		t.Fatal("expected error for remaining short-form reference")
	}
}

func Test_VerifyZeroFingerprint_With_SkippedDirs_Should_IgnoreThem(t *testing.T) {
	t.Parallel()

	// Arrange — template reference inside .git dir should be ignored
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	err := os.MkdirAll(gitDir, 0o750)
	assert.That(t, "mkdir error", err, nil)
	err = os.WriteFile(filepath.Join(gitDir, "config.txt"), []byte("github.com/andygeiss/mcp"), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = verifyZeroFingerprint(dir)

	// Assert — should pass since .git is skipped
	assert.That(t, "error", err, nil)
}

func Test_ShouldSkip_With_NonDirectory_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Arrange — create a file (not a dir)
	dir := t.TempDir()
	path := filepath.Join(dir, ".git")
	err := os.WriteFile(path, []byte("file"), 0o600)
	assert.That(t, "write error", err, nil)
	info, err := os.Stat(path)
	assert.That(t, "stat error", err, nil)

	// Act / Assert — files named .git are not skipped
	assert.That(t, "should skip", shouldSkip(dir, path, info), false)
}

func Test_WriteFile_With_ValidPath_Should_CreateFile(t *testing.T) {
	t.Parallel()

	// Arrange
	path := filepath.Join(t.TempDir(), "test.txt")

	// Act
	err := writeFile(path, []byte("hello"))

	// Assert
	assert.That(t, "write error", err, nil)
	data, err := readFile(path)
	assert.That(t, "read error", err, nil)
	assert.That(t, "content", string(data), "hello")
}

func Test_RewriteGoFiles_With_NestedGoFiles_Should_RewriteAll(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	subDir := filepath.Join(dir, "internal", "pkg")
	err := os.MkdirAll(subDir, 0o750)
	assert.That(t, "mkdir error", err, nil)
	src := `package pkg

import "github.com/andygeiss/mcp/internal/protocol"
`
	err = os.WriteFile(filepath.Join(subDir, "foo.go"), []byte(src), 0o600)
	assert.That(t, "write error", err, nil)

	// Act
	err = rewriteGoFiles(dir, "github.com/new/mod")

	// Assert
	assert.That(t, "error", err, nil)
	data, _ := readFile(filepath.Join(subDir, "foo.go"))
	expected := `package pkg

import "github.com/new/mod/internal/protocol"
`
	assert.That(t, "content", string(data), expected)
}

func Test_RewriteGoFiles_With_SkippedDir_Should_SkipIt(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	// Create a .go file inside a .git directory (should be skipped)
	gitDir := filepath.Join(dir, ".git")
	err := os.MkdirAll(gitDir, 0o750)
	assert.That(t, "mkdir error", err, nil)
	err = os.WriteFile(filepath.Join(gitDir, "hooks.go"), []byte(`package hooks
import "github.com/andygeiss/mcp/internal/server"
`), 0o600)
	assert.That(t, "write error", err, nil)

	// Create a normal .go file
	err = os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main
import "github.com/andygeiss/mcp/internal/tools"
`), 0o600)
	assert.That(t, "write main", err, nil)

	// Act
	err = rewriteGoFiles(dir, "github.com/new/mod")

	// Assert
	assert.That(t, "error", err, nil)
	// main.go should be rewritten
	data, _ := readFile(filepath.Join(dir, "main.go"))
	assert.That(t, "main rewritten", string(data), `package main
import "github.com/new/mod/internal/tools"
`)
	// .git/hooks.go should NOT be rewritten
	gitData, _ := readFile(filepath.Join(gitDir, "hooks.go"))
	assert.That(t, "git untouched", string(gitData), `package hooks
import "github.com/andygeiss/mcp/internal/server"
`)
}

func Test_RewriteGoMod_With_MissingFile_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	err := rewriteGoMod(t.TempDir(), "github.com/x/y")

	// Assert
	if err == nil {
		t.Fatal("expected error for missing go.mod")
	}
}

func Test_RewriteImportsInFile_With_MissingFile_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	err := rewriteImportsInFile(filepath.Join(t.TempDir(), "missing.go"), "github.com/x/y")

	// Assert
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func Test_RewriteGoFiles_With_WalkError_Should_PropagateError(t *testing.T) {
	t.Parallel()

	// Act — walk a non-existent directory
	err := rewriteGoFiles(filepath.Join(t.TempDir(), "nonexistent"), "github.com/x/y")

	// Assert
	if err == nil {
		t.Fatal("expected walk error")
	}
}

func Test_RewriteTextFiles_With_WalkError_Should_PropagateError(t *testing.T) {
	t.Parallel()

	// Act
	err := rewriteTextFiles(filepath.Join(t.TempDir(), "nonexistent"), "github.com/x/y")

	// Assert
	if err == nil {
		t.Fatal("expected walk error")
	}
}

func Test_VerifyZeroFingerprint_With_WalkError_Should_PropagateError(t *testing.T) {
	t.Parallel()

	// Act
	err := verifyZeroFingerprint(filepath.Join(t.TempDir(), "nonexistent"))

	// Assert
	if err == nil {
		t.Fatal("expected walk error")
	}
}

func Test_ShouldSkip_With_BmadDir_Should_ReturnTrue(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	bmadDir := filepath.Join(dir, "_bmad")
	err := os.MkdirAll(bmadDir, 0o750)
	assert.That(t, "mkdir error", err, nil)
	info, err := os.Stat(bmadDir)
	assert.That(t, "stat error", err, nil)

	// Act / Assert
	assert.That(t, "should skip _bmad", shouldSkip(dir, bmadDir, info), true)
}

func Test_ShouldSkip_With_ClaudeDir_Should_ReturnTrue(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, claudeDirName)
	err := os.MkdirAll(claudeDir, 0o750)
	assert.That(t, "mkdir error", err, nil)
	info, err := os.Stat(claudeDir)
	assert.That(t, "stat error", err, nil)

	// Act / Assert
	assert.That(t, "should skip .claude", shouldSkip(dir, claudeDir, info), true)
}

func Test_ShouldSkip_With_RegularDir_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := t.TempDir()
	subDir := filepath.Join(dir, "internal")
	err := os.MkdirAll(subDir, 0o750)
	assert.That(t, "mkdir error", err, nil)
	info, err := os.Stat(subDir)
	assert.That(t, "stat error", err, nil)

	// Act / Assert
	assert.That(t, "should not skip internal", shouldSkip(dir, subDir, info), false)
}

//nolint:paralleltest // rewriteProject ends with resetGitHistory; isolateGit uses t.Setenv which is incompatible with t.Parallel.
func Test_RewriteProject_With_ValidProject_Should_RewriteEverything(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	isolateGit(t)

	// Arrange — create a minimal project structure in a temp dir
	dir := t.TempDir()

	// go.mod
	goMod := "module github.com/andygeiss/mcp\n\ngo 1.26\n"
	err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600)
	assert.That(t, "write go.mod", err, nil)

	// go.sum (empty is fine)
	err = os.WriteFile(filepath.Join(dir, "go.sum"), []byte(""), 0o600)
	assert.That(t, "write go.sum", err, nil)

	// cmd/mcp/main.go
	err = os.MkdirAll(filepath.Join(dir, "cmd", "mcp"), 0o750)
	assert.That(t, "mkdir cmd/mcp", err, nil)
	mainGo := "package main\n\nfunc main() {}\n"
	err = os.WriteFile(filepath.Join(dir, "cmd", "mcp", "main.go"), []byte(mainGo), 0o600)
	assert.That(t, "write main.go", err, nil)

	// cmd/scaffold/ (for self-cleanup)
	err = os.MkdirAll(filepath.Join(dir, "cmd", "scaffold"), 0o750)
	assert.That(t, "mkdir cmd/scaffold", err, nil)
	err = os.WriteFile(filepath.Join(dir, "cmd", "scaffold", "main.go"), []byte("package main\n"), 0o600)
	assert.That(t, "write scaffold main.go", err, nil)

	// README.md with template references
	readme := "# github.com/andygeiss/mcp\n\nbuild: cmd/mcp/ or cmd/mcp run\n"
	err = os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o600)
	assert.That(t, "write README.md", err, nil)

	// Template-only content that must not leak to consumers.
	err = os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# project conventions\n"), 0o600)
	assert.That(t, "write CLAUDE.md", err, nil)
	err = os.MkdirAll(filepath.Join(dir, "_bmad"), 0o750)
	assert.That(t, "mkdir _bmad", err, nil)
	err = os.WriteFile(filepath.Join(dir, "_bmad", "config.yaml"), []byte("key: value\n"), 0o600)
	assert.That(t, "write bmad config", err, nil)
	err = os.MkdirAll(filepath.Join(dir, claudeDirName), 0o750)
	assert.That(t, "mkdir .claude", err, nil)

	// Act
	err = rewriteProject(dir, "github.com/test-org/test-tool", false)

	// Assert
	assert.That(t, "rewrite error", err, nil)

	// Check go.mod was rewritten
	modData, _ := readFile(filepath.Join(dir, "go.mod"))
	if !bytes.Contains(modData, []byte("module github.com/test-org/test-tool")) {
		t.Errorf("go.mod not rewritten: %s", modData)
	}

	// cmd/mcp is preserved — init no longer renames the binary dir.
	_, err = os.Stat(filepath.Join(dir, "cmd", "mcp", "main.go"))
	assert.That(t, "mcp dir preserved", err, nil)
	_, err = os.Stat(filepath.Join(dir, "cmd", "test-tool"))
	assert.That(t, "test-tool dir absent", os.IsNotExist(err), true)

	// Check cmd/scaffold was removed
	_, err = os.Stat(filepath.Join(dir, "cmd", "scaffold"))
	assert.That(t, "scaffold dir gone", os.IsNotExist(err), true)

	// Check README.md was rewritten
	readmeData, _ := readFile(filepath.Join(dir, "README.md"))
	if bytes.Contains(readmeData, []byte(templateModulePath)) {
		t.Errorf("README still contains template path: %s", readmeData)
	}

	// Check template-only content was removed
	for _, name := range []string{claudeDirName, "CLAUDE.md", "_bmad"} {
		_, statErr := os.Stat(filepath.Join(dir, name))
		assert.That(t, name+" gone", os.IsNotExist(statErr), true)
	}
}

func Test_RunGoModTidy_With_ValidProject_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Arrange — create a minimal Go project
	dir := t.TempDir()
	goMod := "module example.com/test\n\ngo 1.26\n"
	err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600)
	assert.That(t, "write go.mod", err, nil)
	mainGo := "package main\n\nfunc main() {}\n"
	err = os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainGo), 0o600)
	assert.That(t, "write main.go", err, nil)

	// Act
	err = runGoModTidy(dir)

	// Assert
	assert.That(t, "tidy error", err, nil)
}

func Test_CheckCleanWorkingTree_With_DirtyTree_Should_ReturnError(t *testing.T) { //nolint:paralleltest // isolateGit uses t.Setenv
	isolateGit(t)

	// Arrange — real git repo with a committed file and an uncommitted edit.
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...) //nolint:gosec // test helper, fixed args
		cmd.Dir = dir
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			t.Fatalf("git %v: %v\n%s", args, runErr, out)
		}
	}
	runGit("init", "-b", "main")
	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("initial\n"), 0o600)
	assert.That(t, "write initial README", err, nil)
	runGit("add", ".")
	runGit("commit", "-m", "seed")
	// Dirty the tree.
	err = os.WriteFile(filepath.Join(dir, "README.md"), []byte("uncommitted edit\n"), 0o600)
	assert.That(t, "dirty the tree", err, nil)

	// Act
	err = checkCleanWorkingTree(dir, false)

	// Assert
	if err == nil {
		t.Fatal("expected error on dirty tree")
	}
	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("expected uncommitted-changes message, got: %v", err)
	}
}

func Test_CheckCleanWorkingTree_With_BrokenGitDir_Should_ReturnError(t *testing.T) { //nolint:paralleltest // isolateGit uses t.Setenv
	isolateGit(t)

	// Arrange — .git exists as a directory but lacks the metadata git status
	// needs, so `git status --porcelain` fails. Simulates a corrupted repo,
	// interrupted init, or permissions issue. Without the check, the scaffold
	// would proceed to resetGitHistory and wipe whatever is there.
	dir := t.TempDir()
	err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750)
	assert.That(t, "mkdir .git", err, nil)

	// Act
	err = checkCleanWorkingTree(dir, false)

	// Assert
	if err == nil {
		t.Fatal("expected error when git status fails")
	}
	if !strings.Contains(err.Error(), "git status") {
		t.Errorf("expected git-status error wrap, got: %v", err)
	}
}
