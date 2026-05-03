// White-box test file: tests unexported helpers (loadClientConfig, validateEntry,
// emitReport, isExecutable, scan) directly. Black-box surface (Run + Clients) is
// covered by paths_test.go.
//
//nolint:testpackage // intentionally same-package for unexported helper tests
package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
)

// Test_loadClientConfig_With_ValidJSON_Should_ParseEntries pins the parser
// against the documented Claude Desktop JSON shape.
func Test_loadClientConfig_With_ValidJSON_Should_ParseEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "claude.json")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"echo": map[string]any{"command": "/usr/local/bin/mcp", "args": []string{"--flag"}},
			"alt":  map[string]any{"command": "/opt/mcp/alt"},
		},
	}
	body, _ := json.Marshal(cfg)
	must(os.WriteFile(cfgPath, body, 0o600))

	entries, err := loadClientConfig(cfgPath)
	assert.That(t, "no error", err, nil)
	assert.That(t, "two entries", len(entries), 2)
	assert.That(t, "alt name (sorted first)", entries[0].Name, "alt")
	assert.That(t, "echo args preserved", entries[1].Args[0], "--flag")
}

func Test_loadClientConfig_With_MissingFile_Should_ReturnNotExist(t *testing.T) {
	t.Parallel()

	_, err := loadClientConfig(filepath.Join(t.TempDir(), "nope.json"))
	assert.That(t, "ErrNotExist", os.IsNotExist(err), true)
}

func Test_loadClientConfig_With_InvalidJSON_Should_ReturnError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadClientConfig(cfgPath)
	assert.That(t, "decode error", err != nil, true)
}

func Test_loadClientConfig_With_NoMCPServersKey_Should_ReturnEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(cfgPath, []byte(`{"unrelatedKey": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := loadClientConfig(cfgPath)
	assert.That(t, "no error", err, nil)
	assert.That(t, "zero entries", len(entries), 0)
}

func Test_validateEntry_With_MissingBinary_Should_ReportFAIL(t *testing.T) {
	t.Parallel()

	entry := ServerEntry{Name: "ghost", Command: "/nonexistent/path/to/binary"}
	results := validateEntry(context.Background(), "Claude Desktop", "/cfg", entry, "test")
	assert.That(t, "one row", len(results), 1)
	assert.That(t, "FAIL severity", results[0].Severity, "FAIL")
	assert.That(t, "names binary not found", strings.Contains(results[0].Message, "binary not found"), true)
}

func Test_validateEntry_With_DirectoryAsBinary_Should_ReportFAIL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	entry := ServerEntry{Name: "dir", Command: dir}
	results := validateEntry(context.Background(), "Cursor", "/cfg", entry, "test")
	assert.That(t, "FAIL severity", results[0].Severity, "FAIL")
	assert.That(t, "names not executable", strings.Contains(results[0].Message, "not executable"), true)
}

func Test_validateEntry_With_NonExecutableFile_Should_ReportFAIL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	entry := ServerEntry{Name: "noexec", Command: bin}
	results := validateEntry(context.Background(), "VS Code", "/cfg", entry, "test")
	assert.That(t, "FAIL severity", results[0].Severity, "FAIL")
	assert.That(t, "names not executable", strings.Contains(results[0].Message, "not executable"), true)
}

func Test_emitReport_With_NoResults_Should_PrintFriendlyMessage(t *testing.T) {
	t.Parallel()

	var out, errw bytes.Buffer
	err := emitReport(nil, &out, &errw)
	assert.That(t, "no error", err, nil)
	assert.That(t, "names absence", strings.Contains(out.String(), "no MCP client configurations"), true)
}

func Test_emitReport_With_FailRow_Should_ReturnErrorAndMirrorToStderr(t *testing.T) {
	t.Parallel()

	results := []CheckResult{
		{ClientName: "Claude Desktop", ServerName: "broken", BinaryPath: "/x", Severity: "FAIL", Message: "binary not found"},
	}
	var out, errw bytes.Buffer
	err := emitReport(results, &out, &errw)
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "stdout has row", strings.Contains(out.String(), "[FAIL]"), true)
	assert.That(t, "stderr mirrors row", strings.Contains(errw.String(), "[FAIL]"), true)
}

func Test_emitReport_With_AllPasses_Should_ReturnNil(t *testing.T) {
	t.Parallel()

	results := []CheckResult{
		{ClientName: "Cursor", ServerName: "ok", BinaryPath: "/x", Severity: "OK", Message: "binary exists and is executable"},
	}
	var out, errw bytes.Buffer
	err := emitReport(results, &out, &errw)
	assert.That(t, "no error", err, nil)
	assert.That(t, "summary present", strings.Contains(out.String(), "1 row(s); 0 FAIL"), true)
	assert.That(t, "stderr empty", errw.Len(), 0)
}

func Test_isExecutable_With_NonExecMode_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.WriteFile(bin, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	assert.That(t, "non-exec is false", isExecutable(info), false)
}

func Test_isExecutable_With_ExecMode_Should_ReturnTrue(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o600|0o111); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	assert.That(t, "exec is true", isExecutable(info), true)
}

func Test_scan_With_TempUserConfigDir_Should_DetectStubbedClient(t *testing.T) {
	t.Parallel()

	// Arrange — build a userConfigDir layout that mimics macOS for a single
	// Claude Desktop config file pointing at an executable in tmpdir.
	root := t.TempDir()
	bin := filepath.Join(root, "fake-mcp")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho fake\n"), 0o600|0o111); err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Join(root, "Claude")
	if err := os.MkdirAll(cfgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{
		"mcpServers": map[string]any{"fake": map[string]any{"command": bin}},
	}
	body, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(cfgDir, "claude_desktop_config.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}

	// Act
	results := scan(context.Background(), "darwin", root, "test")

	// Assert — expect at least an OK row for the Claude Desktop / fake entry.
	var ok bool
	for _, r := range results {
		if r.ClientName == "Claude Desktop" && r.ServerName == "fake" && r.Severity == "OK" {
			ok = true
		}
	}
	assert.That(t, "OK row for fake server", ok, true)
}
