# Project Documentation Index

## Project Overview

- **Type:** Monolith CLI
- **Primary Language:** Go 1.26
- **Architecture:** Flat dependency chain, single binary, stdin/stdout transport
- **MCP Version:** 2025-06-18
- **Dependencies:** None (stdlib only)

## Quick Reference

- **Tech Stack:** Go 1.26, encoding/json v1, log/slog, testing (fuzz + race)
- **Entry Point:** `cmd/mcp/main.go`
- **Architecture Pattern:** Sequential dispatch, three-state lifecycle, JSON-RPC 2.0
- **Coverage Threshold:** 90%
- **Linters:** 45+ via golangci-lint v2, zero suppression policy
- **Test Count:** 170+ unit, 8+ integration, 33 conformance, 4 fuzz, 11 benchmarks

## Generated Documentation

- [Project Overview](./project-overview.md) — Executive summary, features, protocol compliance, security measures
- [Architecture](./architecture.md) — Full technical architecture: types, functions, state machine, dispatch model, schema derivation, linter config
- [Source Tree Analysis](./source-tree-analysis.md) — Annotated directory tree, per-file exports, conformance scenarios, fuzz targets, benchmarks
- [Development Guide](./development-guide.md) — Setup, testing (fuzz, conformance, synctest, architecture tests), tool authoring, code conventions, PR process
- [Deployment Guide](./deployment-guide.md) — Build, release pipeline, CI/CD details, security pipelines, MCP client config, platform support

## Existing Documentation

- [README](../README.md) — Project overview, quickstart, feature summary
- [CONTRIBUTING](../CONTRIBUTING.md) — Prerequisites, dev setup, testing requirements, PR process
- [SECURITY](../SECURITY.md) — Vulnerability reporting and response timeline
- [CLAUDE.md](../CLAUDE.md) — AI agent instructions and engineering conventions
- [LICENSE](../LICENSE) — MIT license

## Getting Started

1. **Clone and setup:**
   ```bash
   git clone https://github.com/andygeiss/mcp.git
   cd mcp && make setup
   ```

2. **Build and test:**
   ```bash
   make check   # build + test + lint
   ```

3. **Run the server:**
   ```bash
   echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}' | go run ./cmd/mcp/
   ```

4. **Use as a template:**
   ```bash
   go run ./cmd/init github.com/yourorg/yourproject
   ```

---

*Generated: 2026-04-10 | Scan level: exhaustive | Project type: CLI*
