# Project Overview

## Summary

**mcp** is a minimal, zero-dependency Go implementation of the [Model Context Protocol](https://modelcontextprotocol.io) (MCP) version `2025-11-25`. It produces a single CLI binary that communicates over stdin/stdout using JSON-RPC 2.0 with newline-delimited framing.

The project serves two purposes:

1. **Working MCP server** with tools, resources, prompts, progress, logging, and bidirectional transport (`Peer` interface for sampling, elicitation, roots).
2. **Template scaffold** -- run `make init MODULE=github.com/yourorg/yourproject` to bootstrap a new MCP server project.

## Key Properties

| Property | Value |
|---|---|
| Module path | `github.com/andygeiss/mcp` |
| Go version | 1.26 (Green Tea GC, `reflect.Type.Fields` iterators, `errors.AsType[T]`) |
| MCP version | `2025-11-25` |
| External dependencies | None (stdlib only) |
| Transport | stdin/stdout, JSON-RPC 2.0, newline-delimited, bidirectional |
| Architecture | Flat package structure, sequential inbound dispatch, single reader goroutine |
| Test coverage | 90% minimum, enforced in CI |

## Features

- **Tools, Resources, Prompts** -- registry-driven, with auto-derived schemas via reflection
- **Bidirectional transport** -- `protocol.Peer` interface for server-to-client requests (sampling, elicitation, roots). Capability-gated per client advertisement (AI9).
- **Progress & Logging** -- context-injected notifications during tool execution
- **Three-state lifecycle** (uninitialized / initializing / ready) per the MCP spec
- **Graceful shutdown** on SIGINT, SIGTERM, or EOF (no drain)
- **Per-message size limits** (4 MB), handler timeouts (30s default), panic recovery
- **Structured logging** to stderr via `slog.JSONHandler`
- **Fuzz-tested** JSON decoder, path validator, input validator, server pipeline, resource template matcher (5 targets)
- **Template scaffold** via `cmd/scaffold` for project bootstrapping
- **Peer stability surface** -- v1.x commitment; any signature change is a MAJOR bump (see [ADR-003](adr/ADR-003-bidi-reader-split.md))

## Security

- OpenSSF Scorecard integration
- CodeQL analysis in CI
- govulncheck for known vulnerabilities
- cosign keyless signing on release, SLSA L3 provenance
- SBOM generation for release archives
- OSS-Fuzz integration
- Input validation with path traversal protection
- Panic values logged but never sent to clients
- No external dependencies reduces supply chain risk

## Protocol Compliance

MCP version `2025-11-25`, JSON-RPC 2.0:

- Newline-delimited JSON objects (no LSP framing)
- Batch requests rejected with `-32700`
- Missing `params` normalized to `{}`
- Request `id` preserved as `json.RawMessage`, echoed exactly
- Notifications never responded to; unknown notifications silently ignored
- Contextual error messages (e.g., `"unknown tool: foo"`)
- JSON nesting depth capped at 64 (pre-unmarshal scan)

### Implemented

`initialize`, `ping`, `tools/list`, `tools/call`, `resources/list`, `resources/read`, `resources/templates/list`, `prompts/list`, `prompts/get`, `logging/setLevel`, plus `notifications/initialized`, `notifications/cancelled`, `notifications/progress`, `notifications/message`. Server-initiated outbound via `protocol.Peer.SendRequest` (for sampling, elicitation, roots).

### Not implemented (reject with `-32601`)

`resources/subscribe`, `resources/unsubscribe`, `completion/complete`, `*/list_changed` notifications. `elicitation/*` and `completion/*` namespaces return structured guidance in `Error.Data`.

## Quantitative Summary

| Metric | Value |
|---|---|
| Source files (non-test) | 29 |
| Test files | 42 |
| Total Go LOC | ~16,800 (3,784 non-test + 13,029 test) |
| Test functions | 463 |
| Fuzz targets | 5 |
| Benchmarks | 11 |
| Integration files | 10 |
| Conformance scenarios | 37 |
| CI workflows | 5 |
| Coverage threshold | 90% |

## Versioning

Follows SemVer plus a v1.x stability commitment on the `protocol.Peer` interface. See [VERSIONING.md](../VERSIONING.md) and [ADR-003 §Peer Stability Surface](adr/ADR-003-bidi-reader-split.md#peer-stability-surface).

Shipped milestones:

- **v1.0.0 – v1.2.0** (2026-04-12 … 2026-04-16): narrow-scope baseline -- tools, resources, prompts, logging, progress
- **v1.3.0** (2026-04-18): bidi trio -- `Peer` interface, AI9 capability gate, A7 outbound cancel symmetry, reader-split (ADR-003), scaffold UX

---

*Generated: 2026-04-18 | Scan level: deep | Reflects v1.3.0*
