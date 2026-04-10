# Project Overview

## Summary

**mcp** is a minimal, zero-dependency Go implementation of the [Model Context Protocol](https://modelcontextprotocol.io) (MCP). It provides a fully compliant MCP server as a single CLI binary communicating over stdin/stdout using JSON-RPC 2.0.

The project serves dual purposes:
1. **Working MCP server** — Ready to use with tools like Claude Code
2. **Template repository** — Scaffold your own MCP server via `make init MODULE=github.com/yourorg/yourproject`

## Key Facts

| Property | Value |
|---|---|
| **Module Path** | `github.com/andygeiss/mcp` |
| **Language** | Go 1.26 |
| **MCP Version** | `2025-06-18` |
| **Transport** | stdin/stdout (JSON-RPC 2.0, newline-delimited) |
| **External Dependencies** | None (stdlib only) |
| **License** | MIT |
| **Author** | Andreas Geiß |

### Why Go 1.26

Go 1.26 was chosen for specific stdlib improvements:

- **Green Tea GC** — 10-40% GC overhead reduction
- **`reflect.Type.Fields`** iterators — used by schema derivation
- **`signal.NotifyContext`** cancel cause — used by signal handling
- **`errors.AsType[T]`** — used for typed error extraction at dispatch boundary
- **`testing/synctest`** — deterministic concurrency testing with virtual time

### Why No Dependencies

MCP servers don't need HTTP frameworks, routers, or dependency trees. This project proves it: a fully compliant MCP server in pure Go, with automatic tool schema derivation and a three-state initialization handshake — all backed by the standard library alone. Zero external dependencies also eliminates supply chain risk.

`GOEXPERIMENT=jsonv2` is **not supported** — the protocol codec relies on `encoding/json` v1 behavior (case-insensitive field matching, duplicate key last-wins, invalid UTF-8 passthrough).

## Architecture

Flat, simple dependency chain — no hexagonal layers, no bounded contexts:

```
cmd/mcp/ ──→ internal/server/ ──→ internal/protocol/
                    │
                    └──→ internal/tools/ ──→ internal/protocol/

internal/assert/    (test-only, zero internal deps)
internal/protocol/  (zero internal deps — foundation layer)
```

| Package | Responsibility | Key Exports |
|---|---|---|
| `cmd/mcp/` | Binary entry point — wiring only | `run()`, `version` var |
| `cmd/init/` | Template rewriter — self-deleting | `rewriteProject()`, `deriveProjectName()` |
| `internal/protocol/` | JSON-RPC 2.0 codec, types, constants | `Decode`, `Encode`, `Validate`, `Request`, `Response`, `CodeError` |
| `internal/server/` | Lifecycle, dispatch, capability negotiation | `Server`, `NewServer`, `Run`, `WithHandlerTimeout`, `WithTrace` |
| `internal/tools/` | Tool registry, reflection-based schema derivation | `Registry`, `Register[T]`, `Tool`, `Result`, `ValidatePath`, `ValidateInput` |
| `internal/assert/` | Test assertion helpers | `That[T]` |

### Dependency Direction Enforcement

The import graph is enforced by automated tests in `architecture_test.go`:
- `protocol` imports zero internal packages
- `tools` never imports `server`
- `server` never imports `cmd`
- `assert` imports zero internal packages

## Features

- **JSON-RPC 2.0** over stdin/stdout — newline-delimited, no LSP framing, no batch requests
- **Automatic input schema** derived from Go struct tags via reflection — supports primitives, slices, maps, nested structs, embedded structs, pointers (max depth: 10)
- **Three-state lifecycle** (uninitialized / initializing / ready) per MCP spec
- **Graceful shutdown** on SIGINT, SIGTERM, or EOF
- **Async tool dispatch** with cancellation support (`notifications/cancelled`) and sequential dispatch (`maxInFlight: 1`)
- **Per-message size limits** (4 MB) and per-result limits (1 MB) with handler timeouts (30s default) and panic recovery
- **Structured logging** to stderr via `slog.JSONHandler` with `snake_case` keys
- **Protocol trace mode** via `MCP_TRACE=1` environment variable — logs all request/response messages
- **Fuzz-tested** JSON decoder, server pipeline, and input validators with OSS-Fuzz integration
- **45+ linter rules** enforced via golangci-lint with zero suppression policy
- **Tool annotations** — behavioral hints (read-only, destructive, idempotent, open-world) for MCP clients
- **Input validation** — path traversal prevention, null byte detection, length limits (4096 chars)
- **Unsupported capability guidance** — methods in `completion/`, `elicitation/`, `prompts/`, `resources/` return `-32601` with structured guidance pointing to `tools/list` and `tools/call`

## Protocol Compliance

MCP version `2025-06-18`. JSON-RPC 2.0 with these specifics:

| Behavior | Implementation |
|---|---|
| Framing | Newline-delimited JSON objects |
| Batch requests | Rejected with `-32700` |
| Missing `params` | Normalized to `{}` |
| Null `params` | Normalized to `{}` |
| Request `id` | Preserved as `json.RawMessage`, echoed exactly |
| Notifications (no `id`) | Never responded to |
| Unknown notifications | Silently ignored — never respond, never log |
| Reserved methods (`rpc.*`) | Rejected with `-32601` |
| Error messages | Contextual (e.g., `"unknown tool: foo"`, not `"invalid params"`) |
| Cancelled requests | Response suppressed per MCP spec |
| Unsupported capabilities | `-32601` with guidance data listing available capabilities |

### Error Codes

| Code | Meaning |
|---|---|
| `-32700` | Parse error — malformed JSON, size limit exceeded, batch array |
| `-32600` | Invalid request — bad structure, wrong jsonrpc version, non-object params |
| `-32601` | Method not found — unknown method, reserved `rpc.*`, unsupported capabilities |
| `-32602` | Invalid params — wrong types, missing required fields, unknown tool name |
| `-32603` | Internal error — should not happen in normal operation |
| `-32000` | Server error — not initialized, already initialized, server busy |
| `-32001` | Server timeout — tool handler timed out or was cancelled |

## Quality Gates

| Gate | Enforcement | Threshold |
|---|---|---|
| Race detector | Mandatory on all test runs (`-race`) | Zero races |
| Code coverage | CI + `make coverage` | 90% |
| Linting | golangci-lint v2, 45+ linters | Zero issues |
| Benchmark regression | CI compares against `testdata/benchmarks/baseline.txt` | 20% |
| Nightly fuzzing | 4 fuzz targets, 5 minutes each | No crashes |
| Static security | CodeQL weekly + on PR/push | Zero findings |
| Supply chain | OpenSSF Scorecard weekly | Continuous assessment |
| Vulnerability scan | govulncheck in CI | Zero known vulns |
| Pre-commit hook | `.githooks/pre-commit` runs `make check` | Must pass |

## Testing Infrastructure

| Category | Count | Description |
|---|---|---|
| Unit tests | 170+ | Black-box tests across all packages |
| Integration tests | 8+ | Full pipeline through compiled binary (`//go:build integration`) |
| Conformance scenarios | 33 | Protocol compliance data-driven tests |
| Fuzz targets | 4 | Decoder, server pipeline, path validation, input validation |
| Benchmarks | 11 | Codec (6), server (3), schema derivation (2) |
| Examples | 3 | Server wiring (2), tool registration (1) |
| Architecture tests | 2 | Import graph enforcement, CLAUDE.md claims verification |

## Security Measures

- **Zero external dependencies** — eliminates supply chain risk
- **GitHub Actions pinned to full commit SHAs** — prevents tag-based supply chain attacks
- **Cosign keyless signing** of release checksums via Sigstore
- **SBOM generation** via Syft for every release
- **OpenSSF Scorecard** for continuous supply chain assessment
- **CodeQL** for Go static analysis
- **govulncheck** for known vulnerability detection
- **Per-message size limits** (4 MB) prevent memory exhaustion attacks
- **Per-result size limits** (1 MB) prevent oversized responses
- **Input validation** for tool parameters — path traversal, null bytes, length limits
- **Panic recovery** in tool handlers — diagnostics logged to stderr, never sent to client
- **Error sanitization** — non-CodeError messages replaced with generic "internal error"
- **OSS-Fuzz** with libFuzzer and address sanitizer for continuous fuzzing
- **depguard rules** — `encoding/json/v2` and `log` (old) explicitly denied

## Repository Structure

| Path | Purpose |
|---|---|
| `cmd/mcp/` | MCP server binary — flags, I/O injection, signal handling |
| `cmd/init/` | Template rewriter — module path rewriting, self-cleanup |
| `internal/protocol/` | JSON-RPC 2.0 codec (3 source files) |
| `internal/server/` | Server lifecycle and dispatch (2 source files, 13 test files) |
| `internal/tools/` | Tool registry and schema (5 source files, 8 test files) |
| `internal/assert/` | Generic test assertion helper |
| `oss-fuzz/` | Google OSS-Fuzz integration (Dockerfile, build.sh, project.yaml) |
| `testdata/benchmarks/` | Benchmark baseline for regression detection |
| `.github/workflows/` | 5 CI/CD pipelines (ci, codeql, scorecard, release, fuzz) |
| `.github/ISSUE_TEMPLATE/` | Bug report and feature request templates |
| `docs/` | Generated project documentation |

## Quick Start

```bash
# Clone and setup
git clone https://github.com/andygeiss/mcp.git
cd mcp && make setup

# Build with version
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/

# Run the server
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}' | ./mcp

# Use as template
go run ./cmd/init github.com/yourorg/yourproject

# Full quality check
make check

# Run all tests including integration
go test -race -tags=integration ./...

# Fuzz the decoder
make fuzz FUZZTIME=2m
```
