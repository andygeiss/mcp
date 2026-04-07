//go:build integration

package main_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/andygeiss/mcp/internal/pkg/assert"
)

func buildBinary(t *testing.T, ldflags string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mcp")
	args := []string{"build", "-o", bin}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "./cmd/mcp/")
	cmd := exec.Command("go", args...) //nolint:gosec // test helper builds known cmd/mcp package
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func Test_Integration_With_VersionFlagAndLdflags_Should_PrintVersionAndExit(t *testing.T) {
	t.Parallel()

	// Arrange
	bin := buildBinary(t, `-X main.version=v1.2.3`)

	// Act
	cmd := exec.Command(bin, "--version") //nolint:gosec // executing binary we just built
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "stdout", string(stdout), "v1.2.3\n")
	assert.That(t, "stderr", stderr.String(), "")
}

func Test_Integration_With_VersionFlagNoLdflags_Should_PrintDevAndExit(t *testing.T) {
	t.Parallel()

	// Arrange
	bin := buildBinary(t, "")

	// Act
	cmd := exec.Command(bin, "--version") //nolint:gosec // executing binary we just built
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "stdout", string(stdout), "dev\n")
	assert.That(t, "stderr", stderr.String(), "")
}
