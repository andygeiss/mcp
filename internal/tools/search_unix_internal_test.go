//go:build unix

package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/andygeiss/mcp/internal/pkg/assert"
)

func Test_matchFile_With_Symlink_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	// Arrange
	dir, err := os.MkdirTemp(".", "test-matchfile-*") //nolint:usetesting // t.TempDir creates dirs outside cwd, failing path validation
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	regular := filepath.Join(dir, "regular.txt")
	if err := os.WriteFile(regular, []byte("hello world\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	absRegular, err := filepath.Abs(regular)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(absRegular, link); err != nil {
		t.Fatal(err)
	}

	re := regexp.MustCompile("hello")

	// Act
	result := matchFile(link, dir, re, 10)

	// Assert
	assert.That(t, "symlink returns nil", result, []string(nil))
}

func Test_matchFile_With_PermissionDenied_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	// Arrange
	dir, err := os.MkdirTemp(".", "test-matchfile-*") //nolint:usetesting // t.TempDir creates dirs outside cwd, failing path validation
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	noread := filepath.Join(dir, "noread.txt")
	if err := os.WriteFile(noread, []byte("secret data\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(noread, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(noread, 0o600) })

	re := regexp.MustCompile("secret")

	// Act
	result := matchFile(noread, dir, re, 10)

	// Assert
	assert.That(t, "permission denied returns nil", result, []string(nil))
}
