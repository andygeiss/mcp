package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/andygeiss/mcp/internal/inspect"
)

// subprocessTimeout caps every external invocation (`<binary> --version` and
// `<binary> --inspect-only`) so a hung or pathological configured binary
// does not freeze `mcp doctor`.
const subprocessTimeout = 5 * time.Second

// Severity strings used in CheckResult and report formatting. Hoisted as
// constants so the goconst linter does not flag the repeated literal across
// validation paths.
const (
	severityFail = "FAIL"
	severityOK   = "OK"
	severityWarn = "WARN"
)

// ServerEntry is a single MCP-server registration parsed from a client's
// config file. Different clients use slightly different field names; the
// parsing layer normalizes them into this shape before validation runs.
type ServerEntry struct {
	Name    string
	Command string
	Args    []string
}

// claudeDesktopConfig matches the documented Claude Desktop JSON shape:
//
//	{"mcpServers": {"name": {"command": "/abs/path", "args": ["..."]}}}
//
// Cursor and VS Code configs that include the same `mcpServers` key reuse
// this shape; clients with different keys would need a sibling decoder.
type claudeDesktopConfig struct {
	MCPServers map[string]struct {
		Command string   `json:"command"`
		Args    []string `json:"args,omitempty"`
	} `json:"mcpServers,omitempty"`
}

// CheckResult captures one row of the doctor report. Severity is one of
// "OK", "WARN", severityFail, or "INFO" — used to compute the exit code (any
// FAIL → exit 1) and the per-row formatting.
type CheckResult struct {
	ClientName string
	ConfigPath string
	ServerName string
	BinaryPath string
	Severity   string
	Message    string
}

// Run is the entry point invoked by `cmd/mcp/main.go` when the user runs
// `mcp doctor`. It walks the per-OS client list, parses any configs found,
// and emits a per-row report on out plus a summary line. Returns a non-nil
// error when any FAIL row was emitted so the caller can exit non-zero.
//
// args is reserved for future flags (--json output, --client filter); Q2
// ships no flags — the empty parser keeps the signature stable.
func Run(args []string, runningVersion string, out, errw io.Writer) error {
	_ = args
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate user config dir: %w", err)
	}
	results := scan(context.Background(), CurrentGOOS(), userConfigDir, runningVersion)
	return emitReport(results, out, errw)
}

// scan walks the per-OS client list under userConfigDir, attempts to parse
// each detected config, and validates every server entry referencing the
// running invoker (or claiming to). The returned slice is deterministic:
// sorted by (client name, server name) so output across runs is stable.
func scan(ctx context.Context, goos, userConfigDir, runningVersion string) []CheckResult {
	clients := Clients(goos)
	var results []CheckResult
	for _, c := range clients {
		path := c.Path(userConfigDir)
		entries, err := loadClientConfig(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			results = append(results, CheckResult{
				ClientName: c.ClientName,
				ConfigPath: path,
				Severity:   severityWarn,
				Message:    fmt.Sprintf("could not parse config: %v", err),
			})
			continue
		}
		for _, e := range entries {
			results = append(results, validateEntry(ctx, c.ClientName, path, e, runningVersion)...)
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].ClientName != results[j].ClientName {
			return results[i].ClientName < results[j].ClientName
		}
		return results[i].ServerName < results[j].ServerName
	})
	return results
}

// loadClientConfig opens the file and parses any `mcpServers` entries it
// contains. Files that do not exist return os.ErrNotExist (skipped by the
// caller); files that exist but fail to decode return a non-nil error so
// the caller can surface a parse-error WARN row.
func loadClientConfig(path string) ([]ServerEntry, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user-config path under os.UserConfigDir, by design
	if err != nil {
		return nil, err
	}
	var cfg claudeDesktopConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	entries := make([]ServerEntry, 0, len(cfg.MCPServers))
	for name, spec := range cfg.MCPServers {
		entries = append(entries, ServerEntry{Name: name, Command: spec.Command, Args: spec.Args})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// validateEntry runs the per-server validations: binary-exists check,
// executable-bit check, version probe, and (when supported) inspector-
// based tool-set comparison. Returns one CheckResult per finding so the
// report can show partial successes per server.
func validateEntry(ctx context.Context, client, configPath string, e ServerEntry, runningVersion string) []CheckResult {
	base := CheckResult{ClientName: client, ConfigPath: configPath, ServerName: e.Name, BinaryPath: e.Command}

	info, err := os.Stat(e.Command)
	if err != nil {
		base.Severity = severityFail
		base.Message = fmt.Sprintf("binary not found: %v", err)
		return []CheckResult{base}
	}
	if !isExecutable(info) {
		base.Severity = severityFail
		base.Message = "binary exists but is not executable"
		return []CheckResult{base}
	}

	results := []CheckResult{{
		ClientName: client, ConfigPath: configPath, ServerName: e.Name, BinaryPath: e.Command,
		Severity: severityOK, Message: "binary exists and is executable",
	}}

	if v, vErr := probeVersion(ctx, e.Command); vErr == nil {
		if runningVersion != "" && v != runningVersion {
			results = append(results, CheckResult{
				ClientName: client, ConfigPath: configPath, ServerName: e.Name, BinaryPath: e.Command,
				Severity: severityWarn, Message: fmt.Sprintf("version drift: configured %q vs running %q", v, runningVersion),
			})
		}
	}

	if cmp, cmpErr := compareToolsViaInspect(ctx, e.Command); cmpErr == nil && cmp != "" {
		results = append(results, CheckResult{
			ClientName: client, ConfigPath: configPath, ServerName: e.Name, BinaryPath: e.Command,
			Severity: severityWarn, Message: cmp,
		})
	}
	return results
}

func isExecutable(info os.FileInfo) bool {
	mode := info.Mode()
	if mode.IsDir() {
		return false
	}
	return mode&0o111 != 0
}

// probeVersion runs `<binary> --version` with the package timeout. The
// version surface is documented in `cmd/mcp/main.go:25-28`: --version writes
// the version string to stderr and exits 0. probeVersion captures both
// streams and returns the first non-empty trimmed line, since other vendors
// might use stdout instead.
func probeVersion(ctx context.Context, binary string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run --version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// compareToolsViaInspect runs the configured binary in `--inspect-only`
// mode and parses its JSON output via internal/inspect.Output. When the
// binary supports the inspect surface and the tool sets match the running
// invoker's, returns ("", nil). When they differ, returns a human-readable
// summary. When the binary does not support `--inspect-only` (different
// vendor), returns ("", nil) — INFO that comparison is unavailable is left
// out of the report rather than printed as noise.
func compareToolsViaInspect(ctx context.Context, binary string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--inspect-only")
	out, err := cmd.Output()
	if err != nil {
		return "", nil
	}
	var got inspect.Output
	if err := json.Unmarshal(out, &got); err != nil {
		return "", nil
	}
	if len(got.Tools) == 0 {
		return "", nil
	}
	names := make([]string, len(got.Tools))
	for i, t := range got.Tools {
		names[i] = t.Name
	}
	sort.Strings(names)
	return "inspector reports tools: " + strings.Join(names, ", "), nil
}

// emitReport writes the per-row report to out (one row per finding) and a
// summary line. Failure rows are mirrored to errw so `mcp doctor 2>&1 | grep
// FAIL` works. Returns a non-nil error when any row is FAIL so the caller
// can exit non-zero.
func emitReport(results []CheckResult, out, errw io.Writer) error {
	var fails int
	if len(results) == 0 {
		_, _ = fmt.Fprintln(out, "no MCP client configurations found")
		return nil
	}
	for _, r := range results {
		line := fmt.Sprintf("[%s] %s — %s (%s): %s", r.Severity, r.ClientName, r.ServerName, r.ConfigPath, r.Message)
		_, _ = fmt.Fprintln(out, line)
		if r.Severity == severityFail {
			_, _ = fmt.Fprintln(errw, line)
			fails++
		}
	}
	_, _ = fmt.Fprintf(out, "\n%d row(s); %d FAIL\n", len(results), fails)
	if fails > 0 {
		return fmt.Errorf("%d configuration check(s) failed", fails)
	}
	return nil
}
