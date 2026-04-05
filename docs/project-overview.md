# Project Overview

**Project:** mcp
**Generated:** 2026-04-05
**Module:** `github.com/andygeiss/mcp`

## Executive Summary

A minimal, zero-dependency Go implementation of the [Model Context Protocol](https://modelcontextprotocol.io) (MCP). The project delivers a single CLI binary that acts as a fully compliant MCP server communicating over stdin/stdout using JSON-RPC 2.0. It serves as both a production-ready MCP server and a template repository for scaffolding new MCP server projects.

## Purpose

MCP servers don't need HTTP frameworks, routers, or dependency trees. This project proves it: a fully compliant MCP server in pure Go, with automatic tool schema derivation from struct tags via reflection and a three-state initialization handshake -- all backed by the standard library alone.

## Tech Stack Summary

| Category | Technology | Version | Notes |
|---|---|---|---|
| Language | Go | 1.26 | Green Tea GC, `reflect.Type.Fields` iterators, `errors.AsType[T]` |
| Protocol | JSON-RPC 2.0 | - | Newline-delimited, no LSP framing |
| MCP Version | MCP | 2024-11-05 | Three-state lifecycle |
| Dependencies | stdlib only | - | Zero external dependencies |
| Linter | golangci-lint | v2 config | 50+ linters enabled |
| Release | GoReleaser | - | Cross-platform (darwin/linux, amd64/arm64) |
| Signing | Cosign | - | Keyless signing via Sigstore |
| SBOM | Syft | - | Software Bill of Materials for releases |
| Fuzzing | Go native + OSS-Fuzz | - | Continuous fuzzing via Google OSS-Fuzz |
| SAST | CodeQL | - | Weekly static analysis |
| Supply Chain | OpenSSF Scorecard | - | Security posture scoring |
| CI/CD | GitHub Actions | - | Build, test, lint, fuzz, release |

## Architecture Classification

- **Repository Type:** Monolith
- **Architecture Pattern:** Sequential dispatch loop (decode -> dispatch -> encode)
- **Transport:** stdin/stdout (JSON-RPC 2.0), stderr (structured logging via `slog.JSONHandler`)
- **Entry Point:** `cmd/mcp/main.go`

## Key Characteristics

- **Zero external dependencies** -- standard library only, enforced by `depguard` linter and tests
- **Three-state initialization** -- uninitialized -> initializing -> ready, per MCP spec
- **Automatic tool schema derivation** -- input JSON Schema generated from Go struct tags via reflection
- **Per-message size limits** -- 4MB per incoming message, 1MB per tool result
- **Handler timeout with panic recovery** -- configurable timeout (default 30s) with safety margin
- **Fuzz-tested** -- protocol decoder fuzzed nightly + via OSS-Fuzz
- **Self-documenting tests** -- CLAUDE.md claims verified by automated test coverage checks

## Links to Detailed Documentation

- [Architecture](./architecture.md)
- [Source Tree Analysis](./source-tree-analysis.md)
- [API Contracts](./api-contracts.md)
- [Development Guide](./development-guide.md)
- [Deployment Guide](./deployment-guide.md)
