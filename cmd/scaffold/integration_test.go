//go:build integration

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

func Test_Integration_With_FullInit_Should_ProduceWorkingProject(t *testing.T) {
	t.Parallel()

	// Arrange — copy project to temp dir (exclude .git and _bmad-output)
	srcDir, err := filepath.Abs("../..")
	assert.That(t, "abs error", err, nil)

	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")

	err = copyDir(srcDir, projectDir)
	assert.That(t, "copy error", err, nil)

	newModule := "github.com/test-org/test-tool"

	// Act — run init
	cmd := exec.Command("go", "run", "./cmd/scaffold", newModule)
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.invalid",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		t.Fatalf("init failed: %v\nstderr: %s", err, stderr.String())
	}

	// Assert — go mod tidy already ran as part of init, verify go.mod is correct
	goMod, err := readFile(filepath.Join(projectDir, "go.mod"))
	assert.That(t, "read go.mod", err, nil)
	if !bytes.Contains(goMod, []byte("module "+newModule)) {
		t.Fatalf("go.mod does not contain new module path: %s", goMod)
	}

	// Assert — go test passes
	testCmd := exec.Command("go", "test", "-race", "./...")
	testCmd.Dir = projectDir
	testCmd.Env = os.Environ()
	testOut, err := testCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test failed: %v\noutput: %s", err, testOut)
	}

	// Assert — cmd/mcp/ is preserved (init no longer renames the binary dir)
	_, err = os.Stat(filepath.Join(projectDir, "cmd", "mcp", "main.go"))
	assert.That(t, "mcp binary dir preserved", err, nil)

	// Assert — no directory derived from the module suffix was created
	_, err = os.Stat(filepath.Join(projectDir, "cmd", "test-tool"))
	assert.That(t, "test-tool binary dir absent", os.IsNotExist(err), true)

	// Assert — cmd/scaffold/ was removed (self-cleanup)
	_, err = os.Stat(filepath.Join(projectDir, "cmd", "scaffold"))
	assert.That(t, "scaffold dir gone", os.IsNotExist(err), true)

	// Assert — zero fingerprint
	err = verifyZeroFingerprint(projectDir)
	assert.That(t, "zero fingerprint", err, nil)

	// Assert — README badges now point at the consumer's repo
	readme, err := readFile(filepath.Join(projectDir, "README.md"))
	assert.That(t, "read README", err, nil)
	if bytes.Contains(readme, []byte("andygeiss/mcp")) {
		t.Errorf("README still contains template slug: %s", readme)
	}
	if !bytes.Contains(readme, []byte("test-org/test-tool")) {
		t.Errorf("README missing rewritten owner/repo short form: %s", readme)
	}

	// Assert — files without template fingerprint survive byte-identical.
	// Guards against accidental reintroduction of cmd/mcp path substitution.
	for _, name := range []string{".goreleaser.yml", ".mcp.json"} {
		orig, origErr := readFile(filepath.Join(srcDir, name))
		assert.That(t, name+" read orig", origErr, nil)
		got, gotErr := readFile(filepath.Join(projectDir, name))
		assert.That(t, name+" read rewritten", gotErr, nil)
		assert.That(t, name+" byte-identical", string(got), string(orig))
	}

	// Assert — fresh git history with a single initial commit on main.
	_, err = os.Stat(filepath.Join(projectDir, ".git", "HEAD"))
	assert.That(t, ".git/HEAD exists", err, nil)
	assert.That(t, "commit count", gitOutput(t, projectDir, "rev-list", "--count", "HEAD"), "1")
	assert.That(t, "commit subject", gitOutput(t, projectDir, "log", "-1", "--format=%s"), "feat: initial version")
	assert.That(t, "branch", gitOutput(t, projectDir, "branch", "--show-current"), "main")
	assert.That(t, "clean tree", gitOutput(t, projectDir, "status", "--porcelain"), "")
}

// gitOutput runs `git <args>` in dir and returns stdout with surrounding
// whitespace stripped. Fails the test on error.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // test helper: args from test code
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v failed: %v\noutput: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// copyDir copies a directory recursively, skipping .git and _bmad-output.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Skip directories that should not be copied.
		if info.IsDir() {
			base := filepath.Base(rel)
			if base == ".git" || base == "_bmad-output" || base == "_bmad" || base == ".claude" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), info.Mode())
		}

		// Skip binary files and large files.
		if strings.HasSuffix(path, ".exe") || strings.HasSuffix(path, ".test") {
			return nil
		}

		data, readErr := readFile(path)
		if readErr != nil {
			return readErr
		}
		return writeFile(filepath.Join(dst, rel), data)
	})
}
