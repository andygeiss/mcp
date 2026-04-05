//go:build integration

package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var testBinary string

func TestMain(m *testing.M) {
	testBinary = filepath.Join(os.TempDir(), "mcp-test-binary")
	cmd := exec.Command("go", "build", "-o", testBinary, "./cmd/mcp/") //nolint:gosec // integration test: args are literals except temp path
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic("build test binary: " + err.Error() + "\n" + string(out))
	}

	code := m.Run()
	_ = os.Remove(testBinary)
	os.Exit(code)
}
