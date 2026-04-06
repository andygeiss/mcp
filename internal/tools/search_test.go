package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/pkg/assert"
	"github.com/andygeiss/mcp/internal/tools"
)

func localTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", "test-search-*") //nolint:usetesting // t.TempDir creates dirs outside cwd, failing path validation
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func Test_Search_With_ValidPattern_Should_ReturnMatches(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := localTempDir(t)
	writeFile(t, dir, "hello.txt", "hello world\ngoodbye world\nhello again")

	input := tools.SearchInput{Pattern: "hello", Path: dir}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, false)
	text := result.Content[0].Text
	if !strings.Contains(text, "hello.txt:1:") {
		t.Errorf("expected match at line 1, got: %s", text)
	}
	if !strings.Contains(text, "hello.txt:3:") {
		t.Errorf("expected match at line 3, got: %s", text)
	}
}

func Test_Search_With_NoMatches_Should_ReturnEmptyResult(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := localTempDir(t)
	writeFile(t, dir, "data.txt", "alpha\nbeta\ngamma")

	input := tools.SearchInput{Pattern: "zzz", Path: dir}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, false)
	assert.That(t, "text", result.Content[0].Text, "no matches found")
}

func Test_Search_With_InvalidPattern_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	input := tools.SearchInput{Pattern: "[invalid", Path: localTempDir(t)}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, true)
	if !strings.Contains(result.Content[0].Text, "invalid pattern") {
		t.Errorf("expected 'invalid pattern' in error, got: %s", result.Content[0].Text)
	}
}

func Test_Search_With_NonexistentPath_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	input := tools.SearchInput{Pattern: "x", Path: "/nonexistent/path/12345"}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, true)
}

func Test_Search_With_CancelledContext_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := localTempDir(t)
	writeFile(t, dir, "data.txt", "content")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	input := tools.SearchInput{Pattern: "content", Path: dir}

	// Act
	result := tools.Search(ctx, input)

	// Assert
	assert.That(t, "isError", result.IsError, true)
}

func Test_Search_With_MaxResults_Should_LimitOutput(t *testing.T) {
	t.Parallel()

	// Arrange — file with 10 matching lines
	dir := localTempDir(t)
	lines := strings.Repeat("match\n", 10)
	writeFile(t, dir, "many.txt", lines)

	input := tools.SearchInput{Pattern: "match", Path: dir, MaxResults: 3}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, false)
	count := strings.Count(result.Content[0].Text, "\n") + 1
	if count > 3 {
		t.Errorf("expected at most 3 results, got %d", count)
	}
}

func Test_Search_With_Extensions_Should_FilterFiles(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := localTempDir(t)
	writeFile(t, dir, "code.go", "func main() {}")
	writeFile(t, dir, "notes.md", "func notes")
	writeFile(t, dir, "data.txt", "func data")

	input := tools.SearchInput{Pattern: "func", Path: dir, Extensions: []string{".go"}}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, false)
	text := result.Content[0].Text
	if !strings.Contains(text, "code.go") {
		t.Error("expected code.go in results")
	}
	if strings.Contains(text, "notes.md") {
		t.Error("notes.md should be filtered out")
	}
	if strings.Contains(text, "data.txt") {
		t.Error("data.txt should be filtered out")
	}
}

func Test_Search_With_PathOutsideWorkingDir_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — OS temp dir is outside the working directory
	input := tools.SearchInput{Pattern: "x", Path: os.TempDir()}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, true)
	if !strings.Contains(result.Content[0].Text, "path must be within working directory") {
		t.Errorf("expected path validation error, got: %s", result.Content[0].Text)
	}
}

func Test_Search_With_SymlinkEscape_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := localTempDir(t)
	target := os.TempDir()
	link := filepath.Join(dir, "escape")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	input := tools.SearchInput{Pattern: "x", Path: link}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, true)
	if !strings.Contains(result.Content[0].Text, "path must be within working directory") {
		t.Errorf("expected path validation error, got: %s", result.Content[0].Text)
	}
}

func Test_Search_With_SymlinkInsideWorkDir_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := localTempDir(t)
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sub, "data.txt", "findme here\n")

	absSub, err := filepath.Abs(sub)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(absSub, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	input := tools.SearchInput{Pattern: "findme", Path: link}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, false)
	if !strings.Contains(result.Content[0].Text, "findme") {
		t.Errorf("expected match containing 'findme', got: %s", result.Content[0].Text)
	}
}

func Test_Search_With_DanglingSymlink_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := localTempDir(t)
	link := filepath.Join(dir, "dangling")
	if err := os.Symlink("/nonexistent/target/abc123", link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	input := tools.SearchInput{Pattern: "x", Path: link}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, true)
}

func Test_Search_With_SymlinkFileInWalk_Should_SkipFile(t *testing.T) {
	t.Parallel()

	// Arrange — directory with a regular file and a symlink to an outside file
	dir := localTempDir(t)
	writeFile(t, dir, "regular.txt", "findme regular\n")

	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("findme outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "symlink.txt")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	input := tools.SearchInput{Pattern: "findme", Path: dir}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, false)
	text := result.Content[0].Text
	if !strings.Contains(text, "regular.txt") {
		t.Error("expected regular.txt in results")
	}
	if strings.Contains(text, "symlink.txt") {
		t.Error("symlink file should be skipped")
	}
}

func Test_Search_With_BinaryFile_Should_SkipFile(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := localTempDir(t)
	writeFile(t, dir, "binary.dat", "match\x00binary")
	writeFile(t, dir, "text.txt", "match here\n")

	input := tools.SearchInput{Pattern: "match", Path: dir}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, false)
	text := result.Content[0].Text
	if strings.Contains(text, "binary.dat") {
		t.Error("binary file should be skipped")
	}
	if !strings.Contains(text, "text.txt") {
		t.Error("text file should be included")
	}
}

func Test_Search_With_LargeFile_Should_SkipFile(t *testing.T) {
	t.Parallel()

	// Arrange — file exceeds 1MB limit
	dir := localTempDir(t)
	large := "match this line\n" + strings.Repeat("x", 1<<20)
	writeFile(t, dir, "large.txt", large)
	writeFile(t, dir, "small.txt", "match this line\n")

	input := tools.SearchInput{Pattern: "match", Path: dir}

	// Act
	result := tools.Search(context.Background(), input)

	// Assert
	assert.That(t, "isError", result.IsError, false)
	text := result.Content[0].Text
	if strings.Contains(text, "large.txt") {
		t.Error("large file should be skipped")
	}
	if !strings.Contains(text, "small.txt") {
		t.Error("small file should be included")
	}
}
