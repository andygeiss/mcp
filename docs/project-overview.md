# Project Overview

**Project:** github.com/andygeiss/mcp
**Generated:** 2026-04-05
**Scan Level:** Exhaustive

## Executive Summary

A minimal, zero-dependency Go implementation of the [Model Context Protocol](https://modelcontextprotocol.io) (MCP). Single CLI binary communicating over stdin/stdout using JSON-RPC 2.0. No HTTP, no WebSocket, no external dependencies -- standard library only.

The project serves dual purposes:
1. **Production MCP server** with a built-in `search` tool for file content searching
2. **Template repository** that can be forked and rewritten into a new MCP server via `cmd/init`

## Technology Stack

| Category | Technology | Version | Notes |
|----------|-----------|---------|-------|
| Language | Go | 1.26 | Green Tea GC, reflect.Type.Fields iterators |
| Protocol | JSON-RPC 2.0 | - | Newline-delimited, no LSP framing |
| MCP Version | MCP | 2024-11-05 | Three-state lifecycle handshake |
| Transport | stdin/stdout | - | Protocol-only stdout, slog to stderr |
| Logging | log/slog | stdlib | JSONHandler to stderr only |
| JSON | encoding/json | v1 | omitempty (not omitzero), no jsonv2 |
| Testing | testing, testing/synctest | stdlib | Race detector, fuzz, benchmarks |
| Linting | golangci-lint | - | 48 linters enabled |
| CI/CD | GitHub Actions | - | Build, test, fuzz, lint, release, scorecard |
| Release | GoReleaser | - | Cross-platform, cosign-signed, SBOM |
| Fuzzing | OSS-Fuzz + native Go fuzz | - | Continuous + nightly + CI |

## Architecture Type

**Monolith** -- single cohesive codebase with flat package structure.

No hexagonal layers, no bounded contexts. Complexity added only when code demands it.

## Repository Structure

```
cmd/mcp/       Entry point -- wiring only: flags, I/O injection, os.Exit
cmd/init/      Template rewriter -- not part of normal builds
internal/
  protocol/    JSON-RPC 2.0 codec, types, constants
  server/      MCP lifecycle, dispatch, capability negotiation
  tools/       Tool registry, schema derivation, tool handlers
  pkg/assert/  Lightweight test assertion helpers
oss-fuzz/      OSS-Fuzz integration harness
```

## Key Metrics

| Metric | Value |
|--------|-------|
| Go source files | 34 |
| Packages | 7 (2 cmd, 4 internal, 1 test helper) |
| Total source lines | ~1,400 (non-test) |
| Total test lines | ~3,800 |
| Test-to-source ratio | ~2.7:1 |
| External dependencies | 0 |
| Registered tools | 1 (search) |
| MCP methods | 4 (initialize, ping, tools/list, tools/call) |
| JSON-RPC error codes | 5 (-32700, -32600, -32601, -32602, -32603) |

## Links to Detailed Documentation

- [Architecture](./architecture.md) -- Full architecture documentation
- [Source Tree Analysis](./source-tree-analysis.md) -- Annotated directory tree
- [API Contracts](./api-contracts.md) -- MCP protocol methods and JSON-RPC interface
- [Development Guide](./development-guide.md) -- Setup, build, test, contribute
- [Deployment Guide](./deployment-guide.md) -- Release pipeline and distribution
