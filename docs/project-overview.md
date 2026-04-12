# Project Overview

## Summary

**mcp** is a minimal, zero-dependency Go implementation of the [Model Context Protocol](https://modelcontextprotocol.io) (MCP) version `2025-11-25`. It produces a single CLI binary that communicates over stdin/stdout using JSON-RPC 2.0 with newline-delimited framing.

The project serves two purposes:

1. **Working MCP server** with tools, resources, prompts, progress, logging, and bidirectional transport.
2. **Template scaffold** -- run `go run ./cmd/init github.com/yourorg/yourproject` to bootstrap a new MCP server project.

## Key Properties

| Property | Value |
|---|---|
| Module path | `github.com/andygeiss/mcp` |
| Go version | 1.26 (Green Tea GC, `reflect.Type.Fields` iterators) |
| MCP version | `2025-11-25` |
| External dependencies | None (stdlib only) |
| Transport | stdin/stdout, JSON-RPC 2.0, newline-delimited |
| Architecture | Flat package structure, sequential dispatch |
| Test coverage | 90% minimum, enforced in CI |
| Linters | 54 rules via golangci-lint v2, zero suppression policy |

## Features

- **Tools, Resources (list/read), Prompts** -- registry-driven, with auto-derived schemas via reflection
- **Progress & Logging** -- context-injected notifications during tool execution
- **Bidirectional transport** -- generic server-to-client request primitive (basis for sampling, elicitation, roots; no built-in handlers)
- **Three-state lifecycle** (uninitialized / initializing / ready) per the MCP spec
- **Graceful shutdown** on SIGINT, SIGTERM, or EOF
- **Per-message size limits** (4 MB) and handler timeouts (30s default) with panic recovery
- **Structured logging** to stderr via `slog.JSONHandler`
- **Fuzz-tested** JSON decoder with 22-entry seed corpus
- **Template scaffold** via `cmd/init` for project bootstrapping

## Security

- OpenSSF Scorecard integration
- CodeQL analysis in CI
- govulncheck for known vulnerabilities
- cosign binary signing on release
- SBOM generation for release archives
- Input validation with path traversal protection
- No external dependencies reduces supply chain risk

## Protocol Compliance

MCP version `2025-11-25`, JSON-RPC 2.0:

- Newline-delimited JSON objects (no LSP framing)
- Batch requests rejected with `-32700`
- Missing `params` normalized to `{}`
- Request `id` preserved as `json.RawMessage`, echoed exactly
- Notifications never responded to; unknown notifications silently ignored
- Contextual error messages (e.g., `"unknown tool: foo"`)

## Quantitative Summary

| Metric | Value |
|---|---|
| Source files | 18 |
| Test files | 36 |
| Total Go lines | ~15,000 |
| Test functions | 419 |
| Fuzz targets | 4 |
| Benchmarks | 11 |
| Integration tests | 9 files |
| CI workflows | 5 |
| Linter rules | 54 |
| Coverage threshold | 90% |

---

*Generated: 2026-04-11 | Scan level: exhaustive | Project type: CLI*
