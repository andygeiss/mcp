# Project Documentation Index

## Project Overview

- **Type:** Monolith CLI
- **Primary Language:** Go 1.26
- **Architecture:** Flat dependency chain, single binary, stdin/stdout transport (bidirectional)
- **MCP Version:** 2025-11-25
- **Latest release:** v1.3.0 (2026-04-18) -- bidi trio (sampling/elicitation/roots via `Peer`)
- **Dependencies:** None (stdlib only)

## Quick Reference

- **Tech Stack:** Go 1.26, encoding/json v1, log/slog, testing (fuzz + race)
- **Entry Point:** `cmd/mcp/main.go`
- **Architecture Pattern:** Sequential inbound dispatch, three-state lifecycle, single reader goroutine, `Peer` outbound
- **Coverage Threshold:** 90%
- **Source/Test files:** 29 / 42
- **Test count:** 463 unit, 10 integration files, 37 conformance, 5 fuzz, 11 benchmarks

## Generated Documentation

- [Project Overview](./project-overview.md) -- Executive summary, features, protocol compliance, security, metrics, versioning
- [Architecture](./architecture.md) -- Package structure, dependency direction, state machine, bidi reader, Peer interface, capability gate, error codes
- [Source Tree Analysis](./source-tree-analysis.md) -- Annotated directory tree, server file split, per-package exports, test inventory, conformance corpus
- [Development Guide](./development-guide.md) -- Setup, testing (fuzz, conformance, benchmarks), tool/resource/prompt authoring, outbound (`Peer`) usage, code conventions
- [Deployment Guide](./deployment-guide.md) -- Build, release pipeline, CI/CD workflows, security posture, MCP client config, platform support

## Architecture Decision Records

- [ADR-001 -- stdio + NDJSON transport](./adr/ADR-001-stdio-ndjson-transport.md)
- [ADR-002 -- internal package layout](./adr/ADR-002-internal-package-layout.md)
- [ADR-003 -- bidi reader-split + Peer stability surface](./adr/ADR-003-bidi-reader-split.md)

## Existing Documentation

- [README](../README.md) -- Project overview, quickstart, feature summary
- [CONTRIBUTING](../CONTRIBUTING.md) -- Prerequisites, dev setup, testing requirements, PR process
- [SECURITY](../SECURITY.md) -- Vulnerability reporting and response timeline
- [VERSIONING](../VERSIONING.md) -- SemVer policy and `Peer` stability commitment
- [CLAUDE.md](../CLAUDE.md) -- AI agent instructions and engineering conventions
- [SPEC_UPGRADE](./SPEC_UPGRADE.md) -- Playbook for future MCP spec revisions
- [LICENSE](../LICENSE) -- MIT license

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

3. **Smoke-test the server:**

   ```bash
   make smoke   # initialize + tools/list round-trip
   ```

4. **Use as a template:**

   ```bash
   make init MODULE=github.com/yourorg/yourproject
   ```

   Rewrites the module path and self-deletes `cmd/scaffold/`. The binary directory stays at `cmd/mcp/` -- every scaffold produces a binary named `mcp`. Refuses to run with a dirty working tree; pass `--force` to override.

---

*Generated: 2026-04-18 | Scan level: deep | Reflects v1.3.0*
