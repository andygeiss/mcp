# Project Overview

**Project:** mcp
**Repository:** [github.com/andygeiss/mcp](https://github.com/andygeiss/mcp)
**License:** MIT (Andreas Geiss)
**Generated:** 2026-04-07

## Executive Summary

A minimal, zero-dependency Go implementation of the [Model Context Protocol](https://modelcontextprotocol.io) (MCP). The server communicates exclusively over stdin/stdout using JSON-RPC 2.0 -- no HTTP, no WebSocket. It serves as both a working MCP server and a template for scaffolding custom MCP servers.

## Purpose

Prove that MCP servers need nothing beyond the standard library: a fully compliant MCP server in pure Go with automatic tool schema derivation and a three-state initialization handshake. Fork or clone the repository, run `make init MODULE=...`, and get a clean project with all template references rewritten.

## Technology Stack

| Category | Technology | Version | Notes |
|---|---|---|---|
| Language | Go | 1.26 | Green Tea GC, `reflect.Type.Fields` iterators, `errors.AsType[T]` |
| Protocol | JSON-RPC 2.0 | — | Newline-delimited, no LSP framing |
| MCP Version | MCP | 2025-06-18 | Tools capability only |
| JSON | encoding/json | v1 | `GOEXPERIMENT=jsonv2` not supported |
| Logging | log/slog | — | `slog.JSONHandler` to stderr |
| Dependencies | None | — | Standard library only |

## Architecture

- **Type:** Monolith
- **Pattern:** Flat and simple -- no hexagonal layers, no bounded contexts
- **Entry Point:** `cmd/mcp/main.go`
- **Dependency Flow:** `cmd/mcp/ -> server/ -> protocol/`, `server/ -> tools/`
- **Transport:** stdin (persistent `json.Decoder`) / stdout (protocol-only JSON-RPC) / stderr (`slog.JSONHandler`)

## Key Features

- Automatic input schema derived from Go struct tags via reflection
- Three-state lifecycle: uninitialized -> initializing -> ready
- Graceful shutdown on SIGINT, SIGTERM, or EOF
- Per-message 4MB size limits and 30s handler timeouts with panic recovery
- Async tool dispatch with cancellation support (`notifications/cancelled`)
- Sequential dispatch (maxInFlight: 1)
- Fuzz-tested JSON decoder (in-repo + OSS-Fuzz)

## Registered Tools

| Tool | Description | Annotations |
|---|---|---|
| `echo` | Echoes the input message | — |
| `search` | Searches files for a pattern | `readOnlyHint: true` |

## Template System

The `cmd/init/` tool rewrites the project for new consumers:
1. Replaces module path in `go.mod` and all `.go` imports
2. Rewrites references in text files (Makefile, README, configs)
3. Renames `cmd/mcp/` to `cmd/<projectName>/`
4. Runs `go mod tidy`
5. Self-deletes `cmd/init/`
6. Verifies zero template fingerprint remains

## Links

- [Architecture](./architecture.md)
- [Source Tree Analysis](./source-tree-analysis.md)
- [Development Guide](./development-guide.md)
- [Deployment Guide](./deployment-guide.md)
