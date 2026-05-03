# ADR-004: `mcp doctor` config-drift detection semantics

**Status:** Accepted (2026-05-03)
**Deciders:** Andy
**Related:** the Q2 epic plan tracks this as FR10 (Story 3.3); see the PR description for cross-links. ADR index: [`docs/index.md`](../index.md).

## Context

Q2 ships `mcp doctor` (FR10) as the cross-OS configuration validator for MCP client integrations. Operators install the `mcp` binary, then point one or more clients (Claude Desktop, Cursor, VS Code) at it via per-client config files. The most common failure mode is not "the binary doesn't work" — it's "the client config points at the wrong path, or the version drifted." `mcp doctor` collapses the troubleshooting matrix to one command.

This ADR documents the **detection semantics** — what counts as drift, when to warn vs. fail, and why the tool stays read-only.

## Decision

### Detection scope (Q2)

The first release scans for three named clients on macOS, Linux, and Windows:

- **Claude Desktop** — config: `<userConfigDir>/Claude/claude_desktop_config.json`
- **Cursor** — config: `<userConfigDir>/Cursor/User/settings.json`
- **VS Code** — config: `<userConfigDir>/Code/User/settings.json`

Where `<userConfigDir>` is `os.UserConfigDir()`'s OS-specific result (`~/Library/Application Support` on macOS, `$XDG_CONFIG_HOME` or `~/.config` on Linux, `%APPDATA%` on Windows).

The parser handles only the `mcpServers` JSON shape (Claude Desktop's documented schema; Cursor and VS Code workspaces that adopt the same key reuse the same parser). Clients with bespoke schemas would need a sibling decoder — out of Q2 scope.

### Drift definitions

A configuration entry is checked along four axes:

1. **Binary present** — `os.Stat(command)` succeeds.
2. **Binary executable** — for the resolved file: `Mode()&0o111 != 0` (UNIX) or analogous on Windows.
3. **Version match** — `<binary> --version` output matches the running invoker's version. Different vendors produce different `--version` formats; doctor compares strings verbatim.
4. **Tool-set match** — when the configured binary supports `--inspect-only` (FR7), its reported tool list is rendered into the report so the operator can spot drift between what the client expects and what the binary advertises.

### Severity matrix

| Finding | Severity | Exit-code impact |
|---|---|---|
| Config file absent | (skipped — not reported) | — |
| Config file present but malformed | WARN | non-fatal |
| Server entry's binary missing | FAIL | non-zero exit |
| Binary present but not executable | FAIL | non-zero exit |
| Version probe fails (binary refuses `--version` or hangs past 5 s) | (silently skipped — not reported) | — |
| Version mismatch between configured binary and running invoker | WARN | non-fatal |
| Tool-set comparison fails (binary lacks `--inspect-only`) | (silently skipped) | — |
| Tool-set inventory available | WARN (informational row showing the configured binary's reported tools) | non-fatal |

Any FAIL row makes `mcp doctor` exit non-zero. WARN and INFO-style rows do not affect the exit code.

### Why no auto-remediation

`mcp doctor` is intentionally **read-only**. Auto-repair would:

- Race the client process — Claude Desktop and Cursor may hold the config open and expect to be the sole writer; concurrent edits risk corruption.
- Overwrite operator customizations — config files often contain entries for many MCP servers, not just this binary; a "fix" pass risks side-effects on unrelated entries.
- Hide root causes — the discipline of running `doctor`, reading the report, and editing intentionally is what teaches operators the layout.

When `doctor` finds drift, the report names the file path and the offending entry. The operator decides what to change. Auto-repair is not on the Q3 roadmap either.

### How to add new clients later

A future PR adding (e.g.) a new Anthropic client follows this pattern:

1. Add a `ClientConfig` entry to the per-OS lists in `internal/doctor/paths.go`.
2. If the client uses a non-`mcpServers` JSON schema, add a sibling decoder in `internal/doctor/doctor.go` and update `loadClientConfig` to dispatch on the client name.
3. Add per-OS path-resolution unit tests under `internal/doctor/paths_test.go`.
4. Update this ADR with the new client name and config path.

## Consequences

- **Positive**: scaffold consumers and operators get a one-command answer to "is my install healthy?", without needing to know the per-client config-file layout.
- **Positive**: the `mcp doctor` surface reuses Story 2.1's `internal/inspect` primitive. Tool-set comparison falls out for free; no separate registry-introspection mechanism.
- **Positive**: stdlib-only (NFR1). `os/exec`, `os`, `path/filepath`, `runtime`, `encoding/json`, `time`, `context`, `fmt`, `strings`, `errors`, `io`, `sort` — all from the standard library.
- **Negative**: paths under `os.UserConfigDir()` cover the common case but not relocated config trees. A `--config-dir` override flag is reasonable Q3 follow-up.
- **Negative**: the Claude/Cursor/VS Code path conventions reflect publicly-documented install layouts as of 2026-05-03. If an upstream vendor relocates a config, doctor needs a corresponding update.
- **Trade-off**: every extra client adds another disk read on every `doctor` run. The number of named clients stays small and the read is on a fast path; latency is not a concern.

## Alternatives considered

- **Vendor a config-loading library** (e.g., HCL, TOML, viper). Rejected: AR2 and NFR1 keep the project stdlib-only; current clients all use JSON.
- **Auto-discover MCP-server entries by parsing every JSON file under `userConfigDir`**. Rejected: too much surface area, false positives, hard to keep stable across releases. A named-client list is a smaller, predictable surface.
- **Background daemon that watches configs**. Rejected: feature creep. Operators can run `mcp doctor` on demand or wire it into their shell rc.

## References

- [`internal/doctor/`](../../internal/doctor) — implementation
- [`internal/inspect/`](../../internal/inspect) — Story 2.1's primitive that powers tool-set comparison
- [`docs/scaffold-consumer-guide.md`](../scaffold-consumer-guide.md) — operator-facing documentation
- [`docs/architecture.md#error-code-taxonomy`](../architecture.md#error-code-taxonomy) — exit-code conventions
- [ADR-003](./ADR-003-bidi-reader-split.md) — style template for this ADR
