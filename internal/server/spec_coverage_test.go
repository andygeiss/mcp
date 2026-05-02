//go:build integration

// The integration tag is required because Story 2.2's Q5 clause init()
// blocks live in internal/server/integration_test.go (also tagged
// integration). Without the tag, Q5 clauses do not register and the
// server fragment is incomplete. The Makefile spec-coverage target
// invokes this test under -tags=integration; the CI integration job
// also runs it. Default `go test` skips this file entirely — that is
// intentional, drift detection is gated to runs that can see the full
// per-package registry.
package server_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
)

// Test_RenderSpecCoverage_Should_MatchServerFragment regenerates the
// per-package audit fragment at docs/spec-coverage.server.txt and
// verifies it matches what is currently committed. The test is the
// authoritative producer of that fragment for clauses registered from
// init() blocks in this package's _test.go files (e.g. Q5 structured-
// content, Q6 progress-token discipline, Q18 elicitation/create). The
// aggregator at cmd/spec-coverage merges this fragment with sibling
// per-package fragments into the canonical docs/spec-coverage.txt audit.
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
//nolint:paralleltest // intentionally sequential — mutates docs/spec-coverage.server.txt on drift
func Test_RenderSpecCoverage_Should_MatchServerFragment(t *testing.T) {
	// Arrange — locate repo root by walking up from this file until go.mod.
	repoRoot, err := findRepoRootForSpecCoverage()
	assert.That(t, "find repo root", err, nil)
	target := filepath.Join(repoRoot, "docs", "spec-coverage.server.txt")

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
	t.Fatalf("docs/spec-coverage.server.txt drifted from the in-memory registry — the file has been regenerated, please review and commit it (run `make spec-coverage`)")
}

func findRepoRootForSpecCoverage() (string, error) {
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
