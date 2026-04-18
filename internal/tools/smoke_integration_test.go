//go:build integration

package tools_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func Test_MakeSmoke_With_BaseTemplate_Should_ExitZero_AndEmitBanner(t *testing.T) {
	t.Parallel()

	// Arrange — locate repo root from this test file's path.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	// Act
	cmd := exec.Command("make", "smoke")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	// Assert — exit 0 and the success banner.
	if err != nil {
		t.Fatalf("make smoke failed: %v\n--- output ---\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("Your server works.")) {
		t.Fatalf("missing success banner in output: %s", out)
	}
	if !bytes.Contains(out, []byte("tool(s).")) {
		t.Fatalf("missing tool count suffix in output: %s", out)
	}
}
