# Versioning Policy

This project follows [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

## Public API

The public API is deliberately narrow. Only the following are covered by semver guarantees:

1. **Binary CLI** (`cmd/mcp`). Current surface:
   - Flags: `--version`
   - Environment variables: `MCP_TRACE=1` (protocol trace to stderr)
   - Exit codes: `0` on clean shutdown (EOF, SIGINT, SIGTERM); `1` on fatal error
2. **Stdin/stdout protocol**: conformance with the MCP methods listed under [`Scope`](README.md#scope) in the README, plus the project-specific capability advertised during `initialize`:
   - `experimental.concurrency.maxInFlight: 1` (sequential dispatch)
3. **Default runtime limits**: 4 MB per-message cap, 30 s handler timeout, 4 096-char tool input cap, 10-level schema recursion. Raising any of these is a breaking change; tightening them for a security fix is allowed under `MINOR`.
4. **Scaffold contract** (`cmd/scaffold`, invoked via `make init MODULE=...`): the module-path argument and the rewrite rules that transform this repository into a new MCP server project.

## Not covered

- **Go packages under `internal/`** — not importable from outside the module. They change freely at any version.
- **Build, test, lint, fuzz, and benchmark infrastructure** — `Makefile` targets, CI workflows, linter configuration, coverage thresholds, and benchmark baselines are maintainer tooling.
- **Documentation wording** unless it changes a normative contract.
- **`MCP_TRACE=1` log line format** — diagnostic only.

## Compatibility guarantees

- **Patch (`v1.0.x`)**: bug fixes only. No new methods, no changed defaults, no schema changes.
- **Minor (`v1.x.0`)**: backwards-compatible additions. New methods (e.g., `resources/subscribe`, `completion/complete`) may be added provided existing capability negotiation continues to work. Defaults may be tightened for a security fix; otherwise stable.
- **Major (`v2.0.0`)**: breaking changes allowed. A `v2.0.0` will be preceded by at least one `v1.x.0` that marks the relevant surface as deprecated in documentation.

## Support window

- `v1.x`: the latest `v1.x.y` receives bug and security fixes.
- `v0.x`: unsupported once `v1.0.0` ships. Users should upgrade.
- After `v2.0.0` ships: `v1.x` receives security fixes only for at least 60 days; no feature work.

## Go version

The minimum required Go version is declared in `go.mod`. Raising it is not a breaking change under this policy — `internal/` is not a public Go API, and release binaries are distributed pre-built. Users building from source should track `go.mod`.

## MCP spec version

The implemented MCP protocol version is declared in `internal/protocol/constants.go` as `MCPVersion`. An update to a newer MCP spec is handled under the normal semver rules: new methods and capabilities are `MINOR`; incompatible protocol changes (e.g., mandatory framing change) are `MAJOR`.
