# Project Documentation Index

**Project:** `github.com/andygeiss/mcp`
**Generated:** 2026-04-26 (initial scan via `/bmad-document-project`, deep-scan level)
**Status:** v1.3.2 shipped (2026-04-25); v1.4.0 in prep

## Project Overview

- **Type:** Monolith (single Go module)
- **Project class:** CLI binary (`cmd/mcp/`) + scaffold/template for downstream MCP servers
- **Primary Language:** Go 1.26+
- **Architecture:** Flat layered (no hexagonal / bounded-context structure), strict dependency direction

## Quick Reference

For module path, protocol version, dependency policy, build/test/release commands, and operational limits: see [Key facts](./project-overview.md#key-facts) in the Project Overview.

## Generated Documentation

- [Project Overview](./project-overview.md) — what, why, capabilities, tech stack, release history
- [Architecture](./architecture.md) — system design, lifecycle state machine, transport, bidi, schema derivation, error taxonomy
- [Source Tree Analysis](./source-tree-analysis.md) — annotated package map, file-by-file purpose, fuzz targets, CI workflows
- [Development Guide](./development-guide.md) — Make targets, test conventions, adding tools/resources/prompts, bidi handler pattern, commit conventions, PR process
- [Deployment Guide](./deployment-guide.md) — release pipeline, signing, verification (cosign + SLSA), supply-chain hardening, operational limits

## Authored References (project root)

- [README.md](../README.md) — user-facing introduction, install, "Your first tool" walkthrough
- [CLAUDE.md](../CLAUDE.md) — engineering philosophy + conventions (load-bearing for AI agents)
- [CHANGELOG.md](../CHANGELOG.md) — Keep a Changelog format
- [CONTRIBUTING.md](../CONTRIBUTING.md) — short-form contributor onboarding
- [SECURITY.md](../SECURITY.md) — security policy
- [VERSIONING.md](../VERSIONING.md) — SemVer policy

## Architecture Decision Records (ADRs)

ADRs in `docs/adr/` capture irreversible architectural decisions:

- [ADR-001](./adr/ADR-001-stdio-ndjson-transport.md) — stdio + NDJSON transport rationale (referenced from README "Protocol compliance")
- [ADR-002](./adr/ADR-002-internal-package-layout.md) — internal package layout
- [ADR-003](./adr/ADR-003-bidi-reader-split.md) — bidi reader-split design + `Peer` v1.x stability surface + four ratified invariants (AI7 cancel chain, AI8 typed errors, AI9 capability gate, AI10 no-progress-during-outbound)

ADRs are written **after code stabilizes** (retrospective style) and never cite gitignored paths (`_bmad-output/`, `_bmad/`).

## Supplementary references

- [Agent Rules](./agent-rules.md) — operational rule sheet for AI agents. Tech Stack, Language Rules, MCP Protocol Rules, Testing Rules, Code Quality, Development Workflow, Don't-Miss Rules.

## Getting Started

### As a user

```bash
go install github.com/andygeiss/mcp/cmd/mcp@latest
```

Point your MCP client at the installed binary (see `README.md`).

### As a contributor

```bash
git clone https://github.com/andygeiss/mcp.git
cd mcp
make setup        # one-time: configure pre-commit hooks
make check        # build + test + lint — your "everything green" gate
```

Read [CLAUDE.md](../CLAUDE.md) and [development-guide.md](./development-guide.md) before writing code.

### As a downstream scaffold consumer

```bash
git clone https://github.com/andygeiss/mcp.git yourproject
cd yourproject
make init MODULE=github.com/yourorg/yourproject
```

After `make init` succeeds, the welcome banner names three steps: **Edit** (`internal/tools/echo.go`) → **Wire** (`cmd/mcp/main.go`) → **Verify** (`make smoke`).

## What's not implemented

Server-hosted `sampling/*`, `elicitation/*`, `completion/complete`, `roots/list`, `resources/{subscribe,unsubscribe}`, and `*/list_changed` notifications all return `-32601`. The bidi primitive (`protocol.SendRequest`) lets a tool handler call the *client* for sampling/elicitation/roots, but the server does not host these as inbound methods.

Full list and rationale: see [Out of scope](./architecture.md#out-of-scope-deliberate-non-goals) in the architecture doc.

## Source of truth on conflicts

When this documentation conflicts with the codebase, the codebase wins. Specific authorities:

| Topic | Authority |
|---|---|
| Go version | `go.mod` |
| Protocol version | `internal/protocol/constants.go` (`MCPVersion`) |
| Engineering philosophy | `CLAUDE.md` |
| Operational rules for AI agents | [`docs/agent-rules.md`](./agent-rules.md) |
| Build/test/release commands | `Makefile`, `.github/workflows/release.yml`, `.goreleaser.yml` |
| Lint rules | `.golangci.yml` |
