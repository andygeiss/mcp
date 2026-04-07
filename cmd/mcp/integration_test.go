//go:build integration

package main_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

func Test_Server_With_EOFAfterInitialize_Should_ExitCleanly(t *testing.T) {
	t.Parallel()

	// Arrange
	cmd := exec.Command(testBinary)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	// Act
	if err := cmd.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Send initialization handshake then close stdin (EOF)
	_, _ = stdin.Write([]byte(`{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}` + "\n"))
	_, _ = stdin.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"))
	_ = stdin.Close()

	// Assert — wait for exit with timeout
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case waitErr := <-done:
		assert.That(t, "exit code", waitErr, nil)
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timed out waiting for process exit")
	}
}

func Test_Server_With_MalformedJSON_Should_ExitWithCode1(t *testing.T) {
	t.Parallel()

	// Arrange
	cmd := exec.Command(testBinary)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	// Act
	if err := cmd.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	_, _ = stdin.Write([]byte("NOT VALID JSON\n"))
	_ = stdin.Close()

	// Assert — wait for exit with timeout
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case waitErr := <-done:
		var exitErr *exec.ExitError
		if !errors.As(waitErr, &exitErr) {
			t.Fatalf("expected ExitError, got: %v", waitErr)
		}
		assert.That(t, "exit code", exitErr.ExitCode(), 1)
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timed out waiting for process exit")
	}
}
