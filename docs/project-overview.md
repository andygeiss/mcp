# Project Overview

**Project:** `mcp` — minimal, stdlib-only Go implementation of the [Model Context Protocol](https://modelcontextprotocol.io)
**Module:** `github.com/andygeiss/mcp`
**Type:** CLI binary (single executable) + scaffold/template for custom MCP servers
**Status:** v1.3.2 shipped (2026-04-25); v1.4.0 in prep
**License:** [MIT](../LICENSE)

---

## Purpose

A production-ready MCP server in pure Go that communicates over stdin/stdout using JSON-RPC 2.0 — no HTTP, no WebSocket, no router, no external dependencies. The repository is also a scaffold: `make init MODULE=github.com/yourorg/yourproject` rewrites the module path and resets git history so anyone can fork it and ship their own MCP server in minutes.

## Key facts

| Aspect | Detail |
|---|---|
| **Language** | Go 1.26+ (`go.mod` is source of truth) |
| **Protocol** | MCP `2025-11-25` over JSON-RPC 2.0 |
| **Transport** | Newline-delimited JSON on stdin/stdout |
| **Dependencies** | Zero external `go.mod` dependencies — standard library only |
| **JSON codec** | `encoding/json` v1 (`GOEXPERIMENT=jsonv2` is unsupported) |
| **Concurrency** | Sequential dispatch — `experimental.concurrency.maxInFlight: 1` advertised |
| **Per-message cap** | 4 MB |
| **Handler timeout** | 30s |
| **Coverage threshold** | 90% (enforced via `make coverage`) |
| **Logging** | `slog.JSONHandler` to stderr; stdout stays protocol-only |
| **Repository type** | Monolith (single Go module) |
| **Architecture pattern** | Layered with strict dependency direction (`cmd → server → protocol`) |

## Capabilities

**Implemented (callable):**
- `tools/list`, `tools/call`
- `resources/list`, `resources/read`, `resources/templates/list`
- `prompts/list`, `prompts/get`
- `logging/setLevel`
- `notifications/initialized`, `notifications/cancelled`
- `notifications/progress`, `notifications/message` (server → client)
- `ping` (always allowed, all states)
- Generic server-to-client request primitive (enables bidirectional methods like sampling, elicitation, roots when invoked from a tool handler)

**Not implemented (rejected with `-32601`):**
- `resources/subscribe`, `resources/unsubscribe`
- `completion/complete`
- `roots/list`
- `sampling/*`, `elicitation/*` as server-hosted methods
- `*/list_changed` notifications (planned for v1.4.0)

## Tech stack

| Category | Choice | Why |
|---|---|---|
| Language | Go 1.26 | Green Tea GC, `reflect.Type.Fields` iterators, `signal.NotifyContext` cancel cause, `errors.AsType[T]` |
| Dependencies | stdlib only | Smallest possible attack surface, supply-chain simplicity |
| Transport | stdin/stdout (NDJSON) | Protocol-mandated; no framework overhead |
| JSON | `encoding/json` v1 | Codec relies on v1 behavior; `jsonv2` is unsupported |
| Schema derivation | reflection in `internal/schema/` | Tool/prompt schemas derived from struct tags — no manual JSON Schema |
| Lint | `golangci-lint` | Must pass with zero issues; `.golangci.yml` is read-only |
| Release | `goreleaser` (cosign-signed, SLSA L3, SBOMs) | Supply-chain hardening |
| Fuzzing | OSS-Fuzz integration + local `make fuzz` | Continuous corpus + per-PR smoke |
| CI | GitHub Actions (`ci`, `codeql`, `fuzz`, `release`, `scorecard`) | Multi-OS test matrix, security analysis, supply-chain scoring |

## Repository structure

```
.
├── cmd/
│   ├── mcp/           # Server binary entry point (wiring only)
│   └── scaffold/      # Template rewriter for `make init`
├── internal/
│   ├── assert/        # Test assertion helpers (test-only)
│   ├── prompts/       # Prompt registry, argument derivation
│   ├── protocol/      # JSON-RPC 2.0 codec, types, constants, Peer interface
│   ├── resources/     # Resource registry, URI templates
│   ├── schema/        # Reflection-based JSON Schema derivation
│   ├── server/        # Lifecycle, dispatch, bidi transport, notifications
│   └── tools/         # Tool registry, schema derivation, handlers
├── docs/              # Project documentation (this folder)
├── oss-fuzz/          # OSS-Fuzz build harness
├── testdata/
│   └── benchmarks/    # benchstat baseline
├── .github/workflows/ # CI/CD pipelines
├── CLAUDE.md          # AI agent engineering philosophy + conventions
├── CHANGELOG.md       # Keep a Changelog format
├── CONTRIBUTING.md    # Contribution guidelines
├── README.md          # User-facing introduction
├── SECURITY.md        # Security policy
├── VERSIONING.md      # Versioning policy
├── Makefile           # Build, test, fuzz, smoke, init targets
├── go.mod             # Single source of truth for Go version
└── _bmad/             # BMad agent workflow (planning + implementation)
```

**Source breakdown:** ~29 production `.go` files, ~42 test files, 5 fuzz targets, ~11 benchmarks, 37 conformance fixtures, ~10 integration tests.

## Release history

| Version | Date | Highlight |
|---|---|---|
| v1.0.0 | 2026-04-12 | Initial narrow-scope release: tools, resources, prompts |
| v1.1.0–v1.2.0 | 2026-04-12..04-16 | Iterative hardening; held narrow scope |
| v1.3.0 | 2026-04-18 | "Bidi trio" — `protocol.Peer` surface, AI9 capability gate, A7 cancel symmetry, scaffold UX, ADR-003 reader-split |
| v1.3.1 | 2026-04-18 | Documentation refresh for v1.3.0 |
| v1.3.2 | 2026-04-25 | ADR-003 self-contained (dropped gitignored citations) |
| v1.4.0 | (in prep) | Likely scope: `notifications/list_changed` |

## Documentation

Generated documentation in this folder:

- [Architecture](./architecture.md) — system design, dispatch, bidi transport
- [Source Tree Analysis](./source-tree-analysis.md) — annotated package map
- [Development Guide](./development-guide.md) — setup, commands, test conventions
- [Deployment Guide](./deployment-guide.md) — release artifacts, verification, supply chain
- [Index](./index.md) — master navigation

Authored references (root):

- [README.md](../README.md) — user-facing introduction
- [CLAUDE.md](../CLAUDE.md) — engineering philosophy for AI agents
- [CHANGELOG.md](../CHANGELOG.md) — version history
- [CONTRIBUTING.md](../CONTRIBUTING.md) — contributor onboarding
- [SECURITY.md](../SECURITY.md) — security policy
- [VERSIONING.md](../VERSIONING.md) — versioning policy

ADRs (in `docs/adr/`) capture irreversible architectural decisions — currently absent from the working tree (recently deleted; restore via `git restore docs/adr/` if needed).
