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

## Architecture

Flat, simple dependency chain — no hexagonal layers:

```
cmd/mcp/ → internal/server/ → internal/protocol/
                            → internal/tools/
```

| Package | Responsibility |
|---|---|
| `cmd/mcp/` | Binary entry point — wiring only |
| `cmd/init/` | Template rewriter — self-deleting |
| `internal/protocol/` | JSON-RPC 2.0 codec, types, constants |
| `internal/server/` | Lifecycle, dispatch, capability negotiation |
| `internal/tools/` | Tool registry, reflection-based schema derivation |
| `internal/pkg/assert/` | Test assertion helpers |

## Features

- **JSON-RPC 2.0** over stdin/stdout — newline-delimited, no LSP framing
- **Automatic input schema** derived from Go struct tags via reflection
- **Three-state lifecycle** (uninitialized / initializing / ready) per MCP spec
- **Graceful shutdown** on SIGINT, SIGTERM, or EOF
- **Async tool dispatch** with cancellation support (`notifications/cancelled`)
- **Per-message size limits** (4 MB) and handler timeouts (30s) with panic recovery
- **Structured logging** to stderr via `slog.JSONHandler`
- **Fuzz-tested** JSON decoder with OSS-Fuzz integration
- **60+ linter rules** enforced via golangci-lint with zero suppression policy

## Quality Gates

- Race detector mandatory on all test runs
- 75% code coverage threshold enforced in CI
- Benchmark regression detection (20% threshold)
- Nightly fuzz testing (5 minutes per target)
- CodeQL and OpenSSF Scorecard for security
- Pre-commit hook runs full quality pipeline

## Repository Structure

| Path | Purpose |
|---|---|
| `cmd/` | Binary entry points |
| `internal/` | Core packages (not importable externally) |
| `oss-fuzz/` | Google OSS-Fuzz integration |
| `testdata/` | Test fixtures and benchmark baselines |
| `.github/workflows/` | CI/CD pipelines (5 workflows) |
| `docs/` | Project documentation |

## Quick Start

```bash
# Build
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/

# Run
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}' | ./mcp

# Use as template
go run ./cmd/init github.com/yourorg/yourproject

# Full quality check
make check
```
