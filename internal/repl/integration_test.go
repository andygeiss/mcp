//go:build integration

package repl_test

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/repl"
)

// safeWriter serializes Write calls so the test buffer is safe under
// concurrent writers. In production the err writer is typically *os.File
// which is already safe; tests targeting bytes.Buffer need explicit
// synchronization.
type safeWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *safeWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

func (s *safeWriter) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if buf, ok := s.w.(*bytes.Buffer); ok {
		return buf.String()
	}
	return ""
}

// Test_Run_With_LiveMCPServer_Should_HandshakeAndCallEcho drives the REPL
// against an actual `go run ./cmd/mcp/` subprocess. The script sends one
// `tools call echo {...}` request and a `quit`; the test asserts the
// response echoes the input field on stdout. This is the FR9 wire-shape
// pinning that the unit tests cannot cover (pure parser tests don't touch
// the subprocess pipes or the initialize handshake).
//
// Skipped when `go` is not on PATH (CI sandboxes occasionally lack it).
//
//nolint:paralleltest // spawns a child process; serial execution avoids stdout interleaving with sibling tests
func Test_Run_With_LiveMCPServer_Should_HandshakeAndCallEcho(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; skipping live-server integration test")
	}

	repoRoot := mustFindRepoRootForREPLIntegration(t)

	// Build the server binary first so we exec a single child process
	// (signal-handling-friendly) rather than `go run`, which wraps the
	// build in a parent that does not always propagate SIGINT cleanly.
	binPath := buildMCPServer(t, repoRoot)

	script := `tools call echo {"message":"yolo"}` + "\n" + "quit\n"

	ctx, cancel := context.WithTimeout(context.Background(), 30*1000*1000*1000) // 30s
	defer cancel()

	server := exec.CommandContext(ctx, binPath) //nolint:gosec // built test artifact
	server.Dir = repoRoot

	var stdoutBuf, stderrBuf bytes.Buffer
	stderr := &safeWriter{w: &stderrBuf}
	if err := repl.Run(ctx, strings.NewReader(script), &stdoutBuf, stderr, server); err != nil {
		t.Fatalf("repl.Run: %v\nstdout=%s\nstderr=%s", err, stdoutBuf.String(), stderr.String())
	}

	// AC #9 wire-shape pinning: the tool call response must carry the typed
	// echo output as structuredContent. We don't pin the full byte shape
	// (handshake responses include dynamic serverInfo), only the load-
	// bearing field that proves the request/response loop completed end-
	// to-end against the live binary.
	out := stdoutBuf.String()
	assert.That(t, "echoes input field", strings.Contains(out, `"echoed": "yolo"`), true)
	assert.That(t, "structuredContent present", strings.Contains(out, "structuredContent"), true)
}

func buildMCPServer(t *testing.T, repoRoot string) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "mcp-test-server")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/mcp/") //nolint:gosec // hardcoded
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mcp server: %v\n%s", err, out)
	}
	return binPath
}

func mustFindRepoRootForREPLIntegration(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("locate repo root via git: %v", err)
	}
	return strings.TrimSpace(string(out))
}
