package main

import (
	"os"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
)

func Test_Run_With_VersionFlag_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	// Arrange — override os.Args and os.Stdout
	origArgs := os.Args
	origStdout := os.Stdout
	t.Cleanup(func() {
		os.Args = origArgs
		os.Stdout = origStdout
	})

	r, w, err := os.Pipe()
	assert.That(t, "pipe error", err, nil)
	os.Stdout = w
	os.Args = []string{"mcp", "--version"}

	// Act
	runErr := run()

	// Assert
	assert.That(t, "run error", runErr, nil)
	_ = w.Close()
	buf := make([]byte, 256)
	n, _ := r.Read(buf)
	assert.That(t, "output", string(buf[:n]), "dev\n")
}

func Test_Run_With_EOFStdin_Should_ReturnNil(t *testing.T) { //nolint:paralleltest // mutates os.Args, os.Stdin, os.Stdout, os.Stderr
	// Arrange — override os.Args and os.Stdin with a pipe that sends
	// a handshake then EOF.
	origArgs := os.Args
	origStdin := os.Stdin
	origStdout := os.Stdout
	origStderr := os.Stderr
	t.Cleanup(func() {
		os.Args = origArgs
		os.Stdin = origStdin
		os.Stdout = origStdout
		os.Stderr = origStderr
	})

	// Stdin: handshake then close
	stdinR, stdinW, err := os.Pipe()
	assert.That(t, "stdin pipe error", err, nil)
	os.Stdin = stdinR

	// Stdout: discard
	_, stdoutW, err := os.Pipe()
	assert.That(t, "stdout pipe error", err, nil)
	os.Stdout = stdoutW

	// Stderr: discard
	_, stderrW, err := os.Pipe()
	assert.That(t, "stderr pipe error", err, nil)
	os.Stderr = stderrW

	os.Args = []string{"mcp"}

	handshake := `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
	_, _ = stdinW.WriteString(handshake)
	_ = stdinW.Close()

	// Act
	runErr := run()

	// Assert
	assert.That(t, "run error", runErr, nil)
	_ = stdoutW.Close()
	_ = stderrW.Close()
}
