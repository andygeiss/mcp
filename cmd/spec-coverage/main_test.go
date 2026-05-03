package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
)

//nolint:paralleltest // t.Chdir touches process-global cwd; cannot share with parallel siblings
func Test_run_With_PerPackageFragments_Should_EmitSortedUnionWithHeader(t *testing.T) {
	// Arrange — three synthetic fragments under a tempdir disguised as a
	// repo root (a go.mod sentinel triggers findRepoRoot). One per package
	// listed in fragmentPaths; the union is sorted by clause ID.
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module example.test\n")
	mustWrite(t, filepath.Join(root, "docs", "spec-coverage.inspect.txt"),
		"ID\tLevel\tSection\tTests\n"+
			"E/clause\tMUST\tInspect\tTestE\n")
	mustWrite(t, filepath.Join(root, "docs", "spec-coverage.protocol.txt"),
		"ID\tLevel\tSection\tTests\n"+
			"B/clause\tMUST\tProto\tTestB\n"+
			"A/clause\tMUST\tProto\tTestA\n")
	mustWrite(t, filepath.Join(root, "docs", "spec-coverage.schema.txt"),
		"ID\tLevel\tSection\tTests\n"+
			"D/clause\tMUST\tSchema\tTestD\n")
	mustWrite(t, filepath.Join(root, "docs", "spec-coverage.server.txt"),
		"ID\tLevel\tSection\tTests\n"+
			"C/clause\tSHOULD\tServer\tTestC\n")

	// Act — run from the tempdir so findRepoRoot lands on it.
	popDir := chdir(t, root)
	defer popDir()

	var buf bytes.Buffer
	err := run(&buf)

	// Assert
	assert.That(t, "run error", err, nil)
	got := buf.String()
	want := "ID\tLevel\tSection\tTests\n" +
		"A/clause\tMUST\tProto\tTestA\n" +
		"B/clause\tMUST\tProto\tTestB\n" +
		"C/clause\tSHOULD\tServer\tTestC\n" +
		"D/clause\tMUST\tSchema\tTestD\n" +
		"E/clause\tMUST\tInspect\tTestE\n"
	assert.That(t, "sorted union output", got, want)
}

func Test_collectRows_With_CrossFragmentDuplicateID_Should_FailLoudly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "a.txt"),
		"ID\tLevel\tSection\tTests\n"+
			"X/clause\tMUST\tA\tTestA\n")
	mustWrite(t, filepath.Join(root, "docs", "b.txt"),
		"ID\tLevel\tSection\tTests\n"+
			"X/clause\tMUST\tB\tTestB\n")

	_, err := collectRows(root, []string{"docs/a.txt", "docs/b.txt"})
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names cross-fragment in message",
		strings.Contains(err.Error(), "cross-fragment duplicate clause ID"), true)
}

func Test_collectRows_With_IntraFragmentDuplicateID_Should_FailWithIntraMessage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "a.txt"),
		"ID\tLevel\tSection\tTests\n"+
			"X/clause\tMUST\tA\tTestA\n"+
			"X/clause\tMUST\tA\tTestB\n")

	_, err := collectRows(root, []string{"docs/a.txt"})
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names intra-fragment in message",
		strings.Contains(err.Error(), "intra-fragment duplicate clause ID"), true)
}

func Test_readFragment_With_HeaderOnly_Should_ReturnNoRows(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "empty.txt")
	mustWrite(t, path, "ID\tLevel\tSection\tTests\n")

	rows, err := readFragment(path, "empty.txt")
	assert.That(t, "no error on header-only fragment", err, nil)
	assert.That(t, "zero rows", len(rows), 0)
}

func Test_readFragment_With_MalformedRow_Should_FailWithColumnCount(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "bad.txt")
	mustWrite(t, path,
		"ID\tLevel\tSection\tTests\n"+
			"X/clause\tMUST\tonly-three\n")

	_, err := readFragment(path, "bad.txt")
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names column count expectation",
		strings.Contains(err.Error(), "4 tab-separated columns"), true)
}

func Test_readFragment_With_EmptyClauseID_Should_Fail(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "blank.txt")
	mustWrite(t, path,
		"ID\tLevel\tSection\tTests\n"+
			"\tMUST\tSection\tTest\n")

	_, err := readFragment(path, "blank.txt")
	assert.That(t, "error returned", err != nil, true)
}

// Test_run_With_CommittedFragments_Should_MatchCommittedAggregate runs
// the aggregator against the real committed fragments and verifies its
// output matches docs/spec-coverage.txt. This closes the CI drift gate
// that `make spec-coverage` provides locally — under default `go test`
// the aggregate would otherwise be unguarded against fragment drift.
//
//nolint:paralleltest // chdir touches process-global state; cannot share with parallel siblings
func Test_run_With_CommittedFragments_Should_MatchCommittedAggregate(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	popDir := chdir(t, repoRoot)
	defer popDir()

	var buf bytes.Buffer
	if err := run(&buf); err != nil {
		t.Fatalf("aggregator run: %v", err)
	}

	committed, err := os.ReadFile(filepath.Join(repoRoot, "docs", "spec-coverage.txt")) //nolint:gosec // known repo-relative path
	if err != nil {
		t.Fatalf("read committed aggregate: %v", err)
	}
	if !bytes.Equal(committed, buf.Bytes()) {
		t.Fatalf("docs/spec-coverage.txt drifted from the aggregator's view of committed fragments — run `make spec-coverage` and commit the regenerated file")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func chdir(t *testing.T, dir string) func() {
	t.Helper()
	t.Chdir(dir)
	return func() {} // t.Chdir restores cwd at test end via t.Cleanup.
}

func mustFindRepoRoot(t *testing.T) string {
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
