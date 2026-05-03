package schema_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
)

// init registers the FR3 (Story 1.3) clause that pins the schema engine's
// top-level non-struct guard. Registration happens at test-binary load time
// so the renderer below sees the entry and writes it into the per-package
// fragment, which the cmd/spec-coverage aggregator merges into the canonical
// docs/spec-coverage.txt audit.
func init() {
	protocol.Register(protocol.Clause{
		ID:      "MCP-2025-11-25/schema/MUST-reject-non-struct-top-level",
		Level:   protocol.LevelMUST,
		Section: "FR3 schema engine top-level guard",
		Summary: "DeriveInputSchema and DeriveOutputSchema return a typed error naming the offending type when the top-level type is not a struct, *struct, time.Time, or json.RawMessage.",
		Tests: []func(*testing.T){
			Test_DeriveInputSchema_With_IntTopLevel_Should_ReturnError,
			Test_DeriveInputSchema_With_SliceTopLevel_Should_ReturnError,
			Test_DeriveInputSchema_With_MapTopLevel_Should_ReturnError,
			Test_DeriveInputSchema_With_DoublePointerToStruct_Should_ReturnError,
			Test_DeriveInputSchema_With_TimeTimeTopLevel_Should_Accept,
			Test_DeriveInputSchema_With_RawMessageTopLevel_Should_Accept,
			Test_DeriveOutputSchema_With_IntTopLevel_Should_ReturnError,
			Test_DeriveOutputSchema_With_DoublePointerToStruct_Should_ReturnError,
			Test_DeriveOutputSchema_With_TimeTimeTopLevel_Should_Accept,
		},
	})
}

// Test_RenderSpecCoverage_Should_MatchSchemaFragment regenerates the
// per-package audit fragment at docs/spec-coverage.schema.txt and verifies
// it matches what is currently committed. Mirrors the pattern documented at
// docs/development-guide.md (cross-package clause aggregation, post-5689f0e):
// each package whose tests register clauses owns a single fragment file; the
// cmd/spec-coverage aggregator merges all fragments into the canonical
// docs/spec-coverage.txt.
//
// On match: passes silently.
// On drift: regenerates the fragment and fails with explicit guidance, so a
// developer who forgot to run `make spec-coverage` sees the failure and a
// ready-to-commit diff.
//
// No t.Parallel(): may write to a tracked file on drift.
//
//nolint:paralleltest // intentionally sequential — mutates docs/spec-coverage.schema.txt on drift
func Test_RenderSpecCoverage_Should_MatchSchemaFragment(t *testing.T) {
	// Arrange — locate repo root by walking up until go.mod.
	repoRoot, err := findRepoRootForSchemaCoverage()
	assert.That(t, "find repo root", err, nil)
	target := filepath.Join(repoRoot, "docs", "spec-coverage.schema.txt")

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
	t.Fatalf("docs/spec-coverage.schema.txt drifted from the in-memory registry — the file has been regenerated, please review and commit it (run `make spec-coverage`)")
}

func findRepoRootForSchemaCoverage() (string, error) {
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
