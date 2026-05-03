package inspect_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
)

// init registers the FR7 (Story 2.1) clause that pins the inspection-mode
// contract: deterministic JSON shape, empty arrays for unwired registries,
// fields stable across runs (verified by golden test).
func init() {
	protocol.Register(protocol.Clause{
		ID:      "MCP-2025-11-25/inspect/MUST-emit-deterministic-shape",
		Level:   protocol.LevelMUST,
		Section: "FR7 inspection-mode contract",
		Summary: "inspect.Inspect emits a deterministic JSON document for the registered surface — fields stable across runs, lists sorted by identity key, empty (not omitted) for unwired registries — so mcp --inspect-only, mcp doctor, and make catalog can rely on a stable consumer contract.",
		Tests: []func(*testing.T){
			Test_Inspect_With_PopulatedRegistries_Should_EmitDeterministicShape,
			Test_Inspect_With_NilRegistries_Should_EmitEmptyArrays,
			Test_Inspect_With_FixtureRegistries_Should_MatchGolden,
		},
	})
}

// Test_RenderSpecCoverage_Should_MatchInspectFragment regenerates the
// per-package audit fragment at docs/spec-coverage.inspect.txt and verifies
// it matches what is currently committed. Mirrors the cross-package
// aggregation pattern documented in docs/development-guide.md (post-5689f0e).
//
// On match: passes silently.
// On drift: regenerates and fails with explicit guidance.
//
//nolint:paralleltest // intentionally sequential — mutates docs/spec-coverage.inspect.txt on drift
func Test_RenderSpecCoverage_Should_MatchInspectFragment(t *testing.T) {
	repoRoot, err := findRepoRootForInspectCoverage()
	assert.That(t, "find repo root", err, nil)
	target := filepath.Join(repoRoot, "docs", "spec-coverage.inspect.txt")

	var buf bytes.Buffer
	assert.That(t, "render", protocol.Render(&buf), nil)

	existing, readErr := os.ReadFile(target) //nolint:gosec // known repo-relative path
	if readErr == nil && bytes.Equal(existing, buf.Bytes()) {
		return
	}
	if err := os.WriteFile(target, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write %s: %v", target, err)
	}
	t.Fatalf("docs/spec-coverage.inspect.txt drifted from the in-memory registry — file regenerated, please review and commit it (run `make spec-coverage`)")
}

func findRepoRootForInspectCoverage() (string, error) {
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
