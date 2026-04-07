# Project Documentation Index

**Project:** mcp
**Generated:** 2026-04-07
**Scan Level:** Exhaustive

## Project Overview

- **Type:** Monolith (single CLI binary)
- **Primary Language:** Go 1.26
- **Architecture:** Flat — `cmd/mcp/ -> server/ -> protocol/`, `server/ -> tools/`
- **Protocol:** MCP 2025-06-18 over JSON-RPC 2.0 (stdin/stdout)
- **Dependencies:** None (standard library only)

## Quick Reference

- **Tech Stack:** Go 1.26, encoding/json, log/slog
- **Entry Point:** `cmd/mcp/main.go`
- **Architecture Pattern:** Flat, sequential dispatch, three-state lifecycle
- **Tools:** echo, search
- **Build:** `make check` (build + test + lint)
- **Test:** `go test -race ./...`

## Generated Documentation

- [Project Overview](./project-overview.md)
- [Architecture](./architecture.md)
- [Source Tree Analysis](./source-tree-analysis.md)
- [Development Guide](./development-guide.md)
- [Deployment Guide](./deployment-guide.md)

## Existing Documentation

- [README.md](../README.md) — Project introduction, quickstart, feature list
- [CONTRIBUTING.md](../CONTRIBUTING.md) — Dev setup, testing requirements, PR process
- [SECURITY.md](../SECURITY.md) — Vulnerability reporting policy
- [CLAUDE.md](../CLAUDE.md) — AI-facing engineering instructions and guardrails
- [LICENSE](../LICENSE) — MIT License

## Getting Started

### Build and Run

```bash
make check                    # Full quality pipeline
make build                    # Compile only
go run ./cmd/mcp/             # Run the MCP server
```

### Add a New Tool

1. Create input struct + handler in `internal/tools/`
2. Register in `cmd/mcp/main.go` via `tools.Register[T]()`
3. Schema is auto-derived from struct tags
4. Write tests (unit + integration)

### Use as Template

```bash
git clone https://github.com/andygeiss/mcp.git my-server
cd my-server
make init MODULE=github.com/myorg/my-server
```
