package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/andygeiss/mcp/internal/pkg/assert"
)

func Test_matchFile_With_EmptyFile_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	// Arrange
	dir, err := os.MkdirTemp(".", "test-matchfile-*") //nolint:usetesting // t.TempDir creates dirs outside cwd, failing path validation
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	re := regexp.MustCompile("anything")

	// Act
	result := matchFile(empty, dir, re, 10)

	// Assert
	assert.That(t, "empty file returns nil", result, []string(nil))
}
