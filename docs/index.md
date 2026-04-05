# Project Documentation Index

**Project:** mcp
**Generated:** 2026-04-05
**Scan Level:** Exhaustive

## Project Overview

- **Type:** Monolith
- **Primary Language:** Go 1.26
- **Architecture:** Sequential dispatch loop (stdin/stdout JSON-RPC 2.0)
- **Dependencies:** Zero external (stdlib only)
- **MCP Version:** 2024-11-05

## Quick Reference

- **Tech Stack:** Go 1.26, JSON-RPC 2.0, slog, encoding/json
- **Entry Point:** `cmd/mcp/main.go`
- **Architecture Pattern:** Decode -> validate -> dispatch -> encode loop
- **Build:** `go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/`
- **Test:** `go test -race ./...`
- **Lint:** `golangci-lint run ./...`

## Generated Documentation

- [Project Overview](./project-overview.md)
- [Architecture](./architecture.md)
- [Source Tree Analysis](./source-tree-analysis.md)
- [API Contracts](./api-contracts.md)
- [Development Guide](./development-guide.md)
- [Deployment Guide](./deployment-guide.md)

## Existing Documentation

- [README](../README.md) -- Project overview, quickstart, architecture summary
- [CLAUDE.md](../CLAUDE.md) -- AI agent engineering instructions and guardrails
- [CONTRIBUTING.md](../CONTRIBUTING.md) -- Contributor guide: prerequisites, testing, PR process
- [SECURITY.md](../SECURITY.md) -- Security policy and vulnerability reporting
- [LICENSE](../LICENSE) -- MIT License

## Getting Started

1. Install Go 1.26+
2. Clone the repository
3. Run `make check` to verify everything builds, tests pass, and lint is clean
4. Read `docs/architecture.md` for the full technical deep-dive
5. Read `docs/api-contracts.md` for the JSON-RPC protocol reference
6. To add a new tool, follow the guide in `docs/development-guide.md`
