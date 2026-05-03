package protocol_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
)

// fixtureFiller is a placeholder string used for required Section/Summary
// fields in throwaway test fixtures. Hoisted so the goconst linter does not
// flag the same literal across many fixture builders.
const fixtureFiller = "test"

func Test_Clauses_Should_HaveConsistentMapKeys(t *testing.T) {
	t.Parallel()

	// Act + Assert — map storage trivially guarantees uniqueness; the
	// invariant worth pinning is that the map key always equals Clause.ID,
	// so iterators (e.g. Render) never disagree on the canonical ID.
	for id, c := range protocol.Clauses {
		assert.That(t, "map key matches Clause.ID for "+id, c.ID, id)
	}
}

func Test_Clauses_Should_HaveAtLeastThreeBootstrapEntries(t *testing.T) {
	t.Parallel()

	// Assert — bootstrap requirement: registry must not ship empty.
	assert.That(t, "at least 3 bootstrap clauses", len(protocol.Clauses) >= 3, true)
}

func Test_Clauses_Should_HaveNonEmptyTestsSlice(t *testing.T) {
	t.Parallel()

	// Act + Assert — the whole point of the registry is the function-pointer
	// reference; an empty Tests slice means the clause is unverified.
	for _, c := range protocol.Clauses {
		assert.That(t, "non-empty Tests for "+c.ID, len(c.Tests) > 0, true)
	}
}

func Test_Clauses_Should_HaveValidLevel(t *testing.T) {
	t.Parallel()

	// Arrange — RFC 2119 levels accepted by the registry.
	allowed := map[string]struct{}{
		"MAY":    {},
		"MUST":   {},
		"SHOULD": {},
	}

	// Act + Assert
	for _, c := range protocol.Clauses {
		_, ok := allowed[c.Level]
		assert.That(t, "valid Level for "+c.ID+" ("+c.Level+")", ok, true)
	}
}

func Test_Clauses_Should_ResolveAllFunctionNames(t *testing.T) {
	t.Parallel()

	// Act + Assert — runtime.FuncForPC must yield a non-empty fully-qualified
	// name for every registered test function pointer. A blank name signals a
	// stripped binary or a compiler bug; the audit story is uninterpretable
	// without these names.
	for _, c := range protocol.Clauses {
		for i, fn := range c.Tests {
			ptr := reflect.ValueOf(fn).Pointer()
			info := runtime.FuncForPC(ptr)
			assert.That(t, "FuncForPC non-nil for "+c.ID, info != nil, true)
			if info != nil {
				assert.That(t, "non-empty name for "+c.ID, info.Name() != "", true)
				_ = i
			}
		}
	}
}

func Test_Register_With_DuplicateID_Should_PanicWithBothSites(t *testing.T) {
	t.Parallel()

	// Arrange — capture an arbitrary already-registered ID. Re-registering
	// it must panic with a message naming BOTH the first registration site
	// and the duplicate site so the conflict is mechanical to resolve (FR5).
	var existingID string
	for id := range protocol.Clauses {
		existingID = id
		break
	}
	if existingID == "" {
		t.Skip("no bootstrap clauses available for duplicate-ID check")
	}

	dup := protocol.Clause{
		ID:      existingID,
		Level:   protocol.LevelMUST,
		Section: fixtureFiller,
		Summary: fixtureFiller,
		Tests:   []func(*testing.T){func(_ *testing.T) {}},
	}

	// Act + Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate Register, got none")
		}
		msg := fmt.Sprint(r)
		// The panic must name the conflicting ID (quoted), the words
		// "first registered at" and "duplicate at", and contain a Go source
		// path with a line number for at least one site (the originating
		// init() block lives in *_test.go, so ".go:" is the load-bearing
		// substring).
		assert.That(t, "names ID quoted", strings.Contains(msg, `"`+existingID+`"`), true)
		assert.That(t, "names first-site phrase", strings.Contains(msg, "first registered at"), true)
		assert.That(t, "names duplicate-site phrase", strings.Contains(msg, "duplicate at"), true)
		assert.That(t, "contains a source-location marker", strings.Contains(msg, ".go:"), true)
	}()
	protocol.Register(dup)
}

// expectPanicAndCleanup wraps a Register call that is expected to panic. If
// the guard regresses (no panic), it both marks the test failed AND deletes
// the leaked entry so the global registry stays clean for parallel readers.
// Cleanup is conditional — when the guard fires correctly, no map mutation
// happens, so this stays race-free against parallel iterators.
func expectPanicAndCleanup(t *testing.T, leakedID string, msg string, fn func()) {
	t.Helper()
	leaked := false
	defer func() {
		if leaked {
			delete(protocol.Clauses, leakedID)
		}
	}()
	defer func() {
		if r := recover(); r == nil {
			t.Error(msg)
			leaked = true
		}
	}()
	fn()
}

func Test_Register_With_EmptyID_Should_Panic(t *testing.T) {
	t.Parallel()

	expectPanicAndCleanup(t, "", "expected panic on empty ID", func() {
		protocol.Register(protocol.Clause{
			ID:      "",
			Level:   protocol.LevelMUST,
			Section: fixtureFiller,
			Summary: fixtureFiller,
			Tests:   []func(*testing.T){func(_ *testing.T) {}},
		})
	})
}

func Test_Register_With_InvalidLevel_Should_Panic(t *testing.T) {
	t.Parallel()

	expectPanicAndCleanup(t, "test/invalid-level", "expected panic on invalid Level", func() {
		protocol.Register(protocol.Clause{
			ID:      "test/invalid-level",
			Level:   "REQUIRED",
			Section: fixtureFiller,
			Summary: fixtureFiller,
			Tests:   []func(*testing.T){func(_ *testing.T) {}},
		})
	})
}

func Test_Register_With_EmptySection_Should_Panic(t *testing.T) {
	t.Parallel()

	expectPanicAndCleanup(t, "test/no-section", "expected panic on empty Section", func() {
		protocol.Register(protocol.Clause{
			ID:      "test/no-section",
			Level:   protocol.LevelMUST,
			Section: "",
			Summary: fixtureFiller,
			Tests:   []func(*testing.T){func(_ *testing.T) {}},
		})
	})
}

func Test_Register_With_EmptySummary_Should_Panic(t *testing.T) {
	t.Parallel()

	expectPanicAndCleanup(t, "test/no-summary", "expected panic on empty Summary", func() {
		protocol.Register(protocol.Clause{
			ID:      "test/no-summary",
			Level:   protocol.LevelMUST,
			Section: fixtureFiller,
			Summary: "",
			Tests:   []func(*testing.T){func(_ *testing.T) {}},
		})
	})
}

func Test_Register_With_EmptyTests_Should_Panic(t *testing.T) {
	t.Parallel()

	expectPanicAndCleanup(t, "test/no-tests", "expected panic on empty Tests slice", func() {
		protocol.Register(protocol.Clause{
			ID:      "test/no-tests",
			Level:   protocol.LevelMUST,
			Section: fixtureFiller,
			Summary: fixtureFiller,
			Tests:   nil,
		})
	})
}

func Test_Register_With_NilTestEntry_Should_Panic(t *testing.T) {
	t.Parallel()

	expectPanicAndCleanup(t, "test/nil-test", "expected panic on nil entry in Tests", func() {
		protocol.Register(protocol.Clause{
			ID:      "test/nil-test",
			Level:   protocol.LevelMUST,
			Section: fixtureFiller,
			Summary: fixtureFiller,
			Tests:   []func(*testing.T){nil},
		})
	})
}

type failingWriter struct{ failOn int }

func (f *failingWriter) Write(p []byte) (int, error) {
	f.failOn--
	if f.failOn < 0 {
		return 0, errFailingWriter
	}
	return len(p), nil
}

var errFailingWriter = &writeError{}

type writeError struct{}

func (*writeError) Error() string { return "synthetic write failure" }

func Test_Render_With_FailingHeader_Should_ReturnError(t *testing.T) {
	t.Parallel()

	w := &failingWriter{failOn: 0}
	err := protocol.Render(w)
	assert.That(t, "header error surfaces", err != nil, true)
}

func Test_Render_With_FailingRow_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange — first write (header) succeeds, second (row) fails.
	w := &failingWriter{failOn: 1}

	// Act
	err := protocol.Render(w)

	// Assert — render returns an error rather than silently truncating.
	assert.That(t, "row error surfaces", err != nil, true)
}

func Test_Render_With_PopulatedRegistry_Should_WriteHeaderAndSortedRows(t *testing.T) {
	t.Parallel()

	// Arrange
	var buf bytes.Buffer

	// Act
	err := protocol.Render(&buf)
	assert.That(t, "render error", err, nil)

	// Assert — header present.
	out := buf.String()
	assert.That(t, "header line",
		strings.HasPrefix(out, "ID\tLevel\tSection\tTests\n"), true)

	// Assert — rows sorted ascending by ID.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	assert.That(t, "at least header + one row", len(lines) >= 2, true)
	var lastID string
	for _, line := range lines[1:] {
		fields := strings.SplitN(line, "\t", 2)
		id := fields[0]
		if lastID != "" {
			assert.That(t, "rows sorted ("+lastID+" then "+id+")", id >= lastID, true)
		}
		lastID = id
	}
}

// Test_RenderSpecCoverage_Should_MatchProtocolFragment regenerates the
// per-package audit fragment at docs/spec-coverage.protocol.txt and
// verifies it matches what is currently committed. The test is the
// authoritative producer of that fragment: bootstrap init() blocks live
// in _test.go files and only fire under `go test`, so each test binary
// sees only its own package's registrations. The aggregator at
// cmd/spec-coverage merges this fragment with the per-package fragments
// produced by sibling tests in other packages into the canonical
// docs/spec-coverage.txt audit.
//
// On match: passes silently (no disk write).
// On drift: regenerates the fragment and fails with explicit guidance, so
// a developer who forgot to run `make spec-coverage` sees the failure and
// a ready-to-commit diff. CI runs this through `make check` and surfaces
// the drift, closing the loophole where stale audits could land unnoticed.
//
// No t.Parallel(): the test may write to a tracked file path and must not
// race other invocations under -count>1 or shared workspaces.
//
//nolint:paralleltest // intentionally sequential — mutates docs/spec-coverage.protocol.txt on drift
func Test_RenderSpecCoverage_Should_MatchProtocolFragment(t *testing.T) {
	// Arrange — locate repo root by walking up from this file until go.mod.
	repoRoot, err := findRepoRoot()
	assert.That(t, "find repo root", err, nil)
	target := filepath.Join(repoRoot, "docs", "spec-coverage.protocol.txt")

	// Act
	var buf bytes.Buffer
	assert.That(t, "render", protocol.Render(&buf), nil)

	existing, readErr := os.ReadFile(target) //nolint:gosec // known repo-relative path
	if readErr == nil && bytes.Equal(existing, buf.Bytes()) {
		return
	}

	if err := os.WriteFile(target, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write %s: %v", target, err)
	}
	t.Fatalf("docs/spec-coverage.protocol.txt drifted from the in-memory registry — the file has been regenerated, please review and commit it (run `make spec-coverage`)")
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
