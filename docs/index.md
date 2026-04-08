# Project Documentation Index

## Project Overview

- **Type:** Monolith CLI
- **Primary Language:** Go 1.26
- **Architecture:** Flat dependency chain, single binary, stdin/stdout transport
- **MCP Version:** 2025-06-18
- **Dependencies:** None (stdlib only)

## Quick Reference

- **Tech Stack:** Go 1.26, encoding/json, log/slog, testing (fuzz + race)
- **Entry Point:** `cmd/mcp/main.go`
- **Architecture Pattern:** Sequential dispatch, three-state lifecycle, JSON-RPC 2.0

## Generated Documentation

- [Project Overview](./project-overview.md)
- [Architecture](./architecture.md)
- [Source Tree Analysis](./source-tree-analysis.md)
- [Development Guide](./development-guide.md)
- [Deployment Guide](./deployment-guide.md)

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

*Generated: 2026-04-08 | Scan level: exhaustive | Project type: CLI*
