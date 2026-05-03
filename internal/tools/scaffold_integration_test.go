//go:build integration

package tools_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Test_MakeNewTool_With_ValidIdentifier_Should_ScaffoldCompilableHandler runs
// `make new-tool TOOL=ScaffoldFixtureXyz` against the live Makefile and
// asserts the produced file is byte-correct (no template placeholders left,
// no //go:build ignore leak) AND that it compiles cleanly. The fixture name
// uses an "Xyz" suffix that no production tool uses, so the test is safe to
// run without colliding with checked-in handlers; the file is removed on
// cleanup.
//
//nolint:paralleltest // mutates internal/tools/scaffoldfixturexyz.go
func Test_MakeNewTool_With_ValidIdentifier_Should_ScaffoldCompilableHandler(t *testing.T) {
	repoRoot := mustFindRepoRootForScaffold(t)
	dest := filepath.Join(repoRoot, "internal", "tools", "scaffoldfixturexyz.go")
	t.Cleanup(func() { _ = os.Remove(dest) })

	// Defensive: clean up any prior leak from a failed run.
	_ = os.Remove(dest)

	out, err := runMake(repoRoot, "new-tool", "TOOL=ScaffoldFixtureXyz")
	if err != nil {
		t.Fatalf("make new-tool failed: %v\noutput:\n%s", err, out)
	}

	// Assert — registration line printed to stdout
	if !bytes.Contains(out, []byte(`tools.Register[tools.ScaffoldFixtureXyzInput, tools.ScaffoldFixtureXyzOutput]`)) {
		t.Fatalf("registration line missing from stdout:\n%s", out)
	}

	// Assert — file exists with substituted identifiers and no template leftovers
	body, err := os.ReadFile(dest) //nolint:gosec // test-controlled path under repo's internal/tools/
	if err != nil {
		t.Fatalf("read scaffolded file: %v", err)
	}
	bodyStr := string(body)
	if strings.Contains(bodyStr, "//go:build ignore") {
		t.Errorf("scaffolded file still carries //go:build ignore tag")
	}
	if strings.Contains(bodyStr, "YourTool") {
		t.Errorf("scaffolded file still contains template placeholder 'YourTool'")
	}
	for _, want := range []string{
		"type ScaffoldFixtureXyzInput struct",
		"type ScaffoldFixtureXyzOutput struct",
		"func ScaffoldFixtureXyz(ctx context.Context",
	} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("scaffolded file missing %q", want)
		}
	}

	// Assert — the package compiles after the new file lands
	buildOut, buildErr := runGo(repoRoot, "build", "./internal/tools/...")
	if buildErr != nil {
		t.Fatalf("go build ./internal/tools/... failed after scaffold: %v\noutput:\n%s", buildErr, buildOut)
	}
}

// Test_MakeNewTool_With_LowercaseName_Should_RefuseAndExitNonZero pins the
// CamelCase identifier guard. Other refusal paths (missing TOOL=, collision)
// are covered by reading the Makefile rule directly during dev review and
// would each require a Make subprocess invocation; this test pins the most
// common author mistake.
//
//nolint:paralleltest // not strictly necessary, but keeps these tests serial with the happy-path one
func Test_MakeNewTool_With_LowercaseName_Should_RefuseAndExitNonZero(t *testing.T) {
	repoRoot := mustFindRepoRootForScaffold(t)
	out, err := runMake(repoRoot, "new-tool", "TOOL=lowercase")
	if err == nil {
		t.Fatalf("expected non-zero exit on lowercase name; got success:\n%s", out)
	}
	if !bytes.Contains(out, []byte("CamelCase")) {
		t.Errorf("expected CamelCase guidance in output, got:\n%s", out)
	}
}

func runMake(dir string, args ...string) ([]byte, error) {
	// Test helper: program name is the hardcoded literal "make"; args come
	// from in-package fixture strings. Not a tainted-input subprocess.
	cmd := exec.Command("make", args...) //nolint:gosec // hardcoded program, fixture-string args
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func runGo(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("go", args...) //nolint:gosec // hardcoded program, fixture-string args
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func mustFindRepoRootForScaffold(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod walking up from %s", wd)
		}
		dir = parent
	}
}
