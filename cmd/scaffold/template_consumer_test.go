//go:build integration

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
)

func Test_Integration_With_TemplateConsumer_Should_PassAllQualityGates(t *testing.T) {
	t.Parallel()

	// Arrange — copy project to temp dir
	srcDir, err := filepath.Abs("../..")
	assert.That(t, "abs error", err, nil)

	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")

	err = copyDir(srcDir, projectDir)
	assert.That(t, "copy error", err, nil)

	newModule := "github.com/testorg/testserver"

	// Act 1 — run cmd/scaffold
	t.Log("running cmd/scaffold...")
	initCmd := exec.Command("go", "run", "./cmd/scaffold", newModule)
	initCmd.Dir = projectDir
	initCmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.invalid",
	)
	initOut, err := initCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cmd/scaffold failed: %v\noutput: %s", err, initOut)
	}

	// Assert — fresh git history was created.
	_, err = os.Stat(filepath.Join(projectDir, ".git", "HEAD"))
	assert.That(t, ".git/HEAD exists after init", err, nil)

	// Assert — template-only content is stripped.
	for _, name := range []string{claudeDirName, "CLAUDE.md", "_bmad", "_bmad-output"} {
		_, statErr := os.Stat(filepath.Join(projectDir, name))
		assert.That(t, name+" removed from consumer project", os.IsNotExist(statErr), true)
	}

	// Assert 1 — go build succeeds
	t.Log("running go build...")
	runInDir(t, projectDir, "go", "build", "./...")

	// Assert 2 — go test -race succeeds
	t.Log("running go test -race...")
	runInDir(t, projectDir, "go", "test", "-race", "./...")

	// Assert 3 — golangci-lint succeeds (skip if not installed)
	if _, lookErr := exec.LookPath("golangci-lint"); lookErr != nil {
		t.Log("golangci-lint not found, skipping lint check")
	} else {
		t.Log("running golangci-lint...")
		runInDir(t, projectDir, "golangci-lint", "run", "./...")
	}

	// Act 2 — add greet tool (copy fixture + register)
	t.Log("adding greet tool to scaffold...")
	fixtureData, err := os.ReadFile(filepath.Clean(filepath.Join(srcDir, "cmd", "scaffold", "testdata", "greet.go.fixture")))
	assert.That(t, "read fixture", err, nil)

	toolsDir := filepath.Join(projectDir, "internal", "tools")
	err = os.WriteFile(filepath.Join(toolsDir, "greet.go"), fixtureData, 0o600) //nolint:gosec // test-only: writing to t.TempDir()
	assert.That(t, "write greet.go", err, nil)

	// Register greet tool in main.go (after echo, alphabetical order).
	// The binary directory stays at cmd/mcp regardless of module path.
	mainGoPath := filepath.Join(projectDir, "cmd", "mcp", "main.go")
	mainData, err := os.ReadFile(filepath.Clean(mainGoPath))
	assert.That(t, "read main.go", err, nil)

	echoBlock := []byte(`if err := tools.Register[tools.EchoInput, tools.EchoOutput](registry, "echo", "Echoes the input message", tools.Echo); err != nil {
		return fmt.Errorf("register echo: %w", err)
	}`)
	greetReg := []byte("\n\tif err := tools.Register[tools.GreetInput, tools.GreetOutput](registry, \"greet\", \"Greets a person by name\", tools.Greet); err != nil {\n\t\treturn fmt.Errorf(\"register greet: %w\", err)\n\t}")
	echoAndGreet := make([]byte, 0, len(echoBlock)+len(greetReg))
	echoAndGreet = append(echoAndGreet, echoBlock...)
	echoAndGreet = append(echoAndGreet, greetReg...)
	mainData = bytes.Replace(mainData, echoBlock, echoAndGreet, 1)
	err = os.WriteFile(mainGoPath, mainData, 0o600) //nolint:gosec // test-only: writing to t.TempDir()
	assert.That(t, "write main.go", err, nil)

	// Assert 4 — go build succeeds after extension
	t.Log("running go build after extension...")
	runInDir(t, projectDir, "go", "build", "./...")

	// Assert 5 — go test -race succeeds after extension
	t.Log("running go test -race after extension...")
	runInDir(t, projectDir, "go", "test", "-race", "./...")

	// Assert 6 — golangci-lint succeeds after extension
	if _, lookErr := exec.LookPath("golangci-lint"); lookErr != nil {
		t.Log("golangci-lint not found, skipping post-extension lint check")
	} else {
		t.Log("running golangci-lint after extension...")
		runInDir(t, projectDir, "golangci-lint", "run", "./...")
	}
}

func runInDir(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...) //nolint:gosec // test helper: args from test code
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\noutput: %s", name, args, err, out)
	}
}
