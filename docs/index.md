# Project Documentation Index

**Project:** github.com/andygeiss/mcp
**Generated:** 2026-04-05
**Scan Level:** Exhaustive

## Project Overview

- **Type:** Monolith
- **Primary Language:** Go 1.26
- **Architecture:** Sequential message loop with three-state lifecycle
- **Protocol:** MCP 2024-11-05 over JSON-RPC 2.0
- **Dependencies:** Zero (standard library only)

## Quick Reference

- **Tech Stack:** Go 1.26, encoding/json, log/slog, testing/synctest
- **Entry Point:** `cmd/mcp/main.go`
- **Architecture Pattern:** Flat packages, strict one-way dependency graph
- **Transport:** stdin (requests) / stdout (responses) / stderr (diagnostics)
- **Registered Tools:** search (regex file content search)

## Generated Documentation

- [Project Overview](./project-overview.md) -- Executive summary, tech stack, key metrics
- [Architecture](./architecture.md) -- Package structure, state machine, protocol, tool system
- [Source Tree Analysis](./source-tree-analysis.md) -- Annotated directory tree, critical folders
- [API Contracts](./api-contracts.md) -- MCP methods, JSON-RPC interface, error codes, tool schemas
- [Development Guide](./development-guide.md) -- Setup, build, test, contribute, add tools
- [Deployment Guide](./deployment-guide.md) -- Release pipeline, CI/CD, OSS-Fuzz, security

## Existing Documentation

- [README.md](../README.md) -- Project overview, quickstart, architecture summary
- [CONTRIBUTING.md](../CONTRIBUTING.md) -- Dev setup, testing requirements, PR process
- [SECURITY.md](../SECURITY.md) -- Security policy, vulnerability reporting
- [CLAUDE.md](../CLAUDE.md) -- Engineering guide for AI agents (comprehensive)
- [LICENSE](../LICENSE) -- MIT license

## Getting Started

1. **Understand the project:** Start with [Project Overview](./project-overview.md) for scope and tech stack
2. **Understand the architecture:** Read [Architecture](./architecture.md) for the state machine, transport, and tool system
3. **Explore the code:** Use [Source Tree Analysis](./source-tree-analysis.md) to navigate the codebase
4. **Understand the protocol:** Reference [API Contracts](./api-contracts.md) for all MCP methods and error codes
5. **Set up development:** Follow [Development Guide](./development-guide.md) for build, test, and contribution workflow
6. **Release and deploy:** See [Deployment Guide](./deployment-guide.md) for CI/CD and release pipeline

## For Brownfield PRD

When planning new features for this project, provide this index as input to the PRD workflow:
- **UI-only features:** Not applicable (CLI binary, no UI)
- **Protocol features:** Reference [Architecture](./architecture.md) + [API Contracts](./api-contracts.md)
- **New tools:** Reference [Development Guide](./development-guide.md#adding-a-new-tool) + [Architecture](./architecture.md#tool-system)
- **Full-stack features:** Reference all architecture docs
