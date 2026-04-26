# Project Documentation Index

**Project:** `github.com/andygeiss/mcp`
**Generated:** 2026-04-26 (initial scan via `/bmad-document-project`, deep-scan level)
**Status:** v1.3.2 shipped (2026-04-25); v1.4.0 in prep

## Project Overview

- **Type:** Monolith (single Go module)
- **Project class:** CLI binary (`cmd/mcp/`) + scaffold/template for downstream MCP servers
- **Primary Language:** Go 1.26+
- **Architecture:** Layered with strict dependency direction; flat — no hexagonal/bounded-context layering

## Quick Reference

| Aspect | Detail |
|---|---|
| **Module** | `github.com/andygeiss/mcp` |
| **Entry point** | `cmd/mcp/main.go` (wiring only) |
| **Protocol** | MCP `2025-11-25` over JSON-RPC 2.0, NDJSON on stdin/stdout |
| **Dependencies** | Zero external `go.mod` deps — stdlib only |
| **Lint** | `golangci-lint`; must pass with zero issues |
| **Tests** | `go test -race ./...` mandatory; 90% coverage threshold (`make coverage`) |
| **Fuzz** | 5 targets; OSS-Fuzz integrated; `make fuzz` runs decoder for 30s |
| **Release** | goreleaser + cosign keyless + SBOM + SLSA L3 provenance |
| **Concurrency** | Sequential — `experimental.concurrency.maxInFlight: 1` advertised |
| **Per-message cap** | 4 MB (counting reader) |
| **Handler timeout** | 30 seconds → `-32001` |

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

ADRs live in `docs/adr/` and capture irreversible architectural decisions. **Not present in the working tree** at scan time — recently deleted (`git status` shows `D docs/adr/ADR-001-stdio-ndjson-transport.md`, `D docs/adr/ADR-002-internal-package-layout.md`, `D docs/adr/ADR-003-bidi-reader-split.md`). Restore via:

```bash
git restore docs/adr/ docs/SPEC_UPGRADE.md
```

If restored, the canonical references are:

- **ADR-001** — stdio + NDJSON transport rationale (referenced from README "Protocol compliance")
- **ADR-002** — internal package layout
- **ADR-003** — bidi reader-split design + `Peer` v1.x stability surface + four ratified invariants (AI7 cancel chain, AI8 typed errors, AI9 capability gate, AI10 no-progress-during-outbound)

ADRs are written **after code stabilizes** (retrospective style) and never cite gitignored paths (`_bmad-output/`, `_bmad/`).

## Supplementary references

- [`../_bmad-output/project-context.md`](../_bmad-output/project-context.md) — operational rule sheet for AI agents (gitignored). 173 bullets across 7 sections: Tech Stack, Language Rules, MCP Protocol Rules, Testing Rules, Code Quality, Development Workflow, Don't-Miss Rules.

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

## What's not implemented (rejected with `-32601`)

- `resources/subscribe`, `resources/unsubscribe`
- `completion/complete`
- `roots/list`
- Server-hosted `sampling/*`, `elicitation/*`
- `*/list_changed` notifications (planned for v1.4.0)

The bidi primitive (`protocol.SendRequest`) lets a tool handler call the *client* for sampling/elicitation/roots when invoked from a handler context — but the server does not host these as inbound methods.

## Source of truth on conflicts

When this documentation conflicts with the codebase, the codebase wins. Specific authorities:

| Topic | Authority |
|---|---|
| Go version | `go.mod` |
| Protocol version | `internal/protocol/constants.go` (`MCPVersion`) |
| Engineering philosophy | `CLAUDE.md` |
| Operational rules for AI agents | `_bmad-output/project-context.md` |
| Build/test/release commands | `Makefile`, `.github/workflows/release.yml`, `.goreleaser.yml` |
| Lint rules | `.golangci.yml` |
