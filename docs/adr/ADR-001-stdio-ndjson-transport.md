# ADR-001: Stdio Transport with Newline-Delimited JSON

## Status

Accepted — 2026-04-14

## Context

MCP 2025-11-25 permits multiple transports (stdio, Streamable HTTP, experimental variants) and, for any stream-oriented transport, two on-wire framings: newline-delimited JSON (NDJSON) or LSP-style `Content-Length` headers. Every implementation must pick a subset.

This project targets a minimal, zero-dependency CLI binary suitable as both a production MCP server and a fork-and-modify template. Design surface that can be removed reduces the cognitive load carried by every downstream consumer.

## Decision

- **Transport:** stdio only. Stdin carries requests; stdout carries responses. Stderr is reserved for diagnostics via `slog.JSONHandler`.
- **Framing:** newline-delimited JSON. One JSON object per line. Batch arrays are rejected with `-32700`.
- **Explicitly out of scope:** HTTP, Streamable HTTP, WebSocket, LSP `Content-Length` framing, SSE, named pipes.

## Consequences

- Zero network surface. No listening port, no TLS, no authentication layer. The server runs under the parent process's trust boundary, which matches how Claude Desktop, VS Code, and other MCP clients spawn stdio subprocesses today.
- Startup is trivial: `exec` the binary, wire stdin/stdout, send `initialize`. No readiness probe, no handshake beyond the protocol itself.
- Debugging stays line-oriented: `MCP_TRACE=1` emits every framed message to stderr, human-readable under `tail -f`. LSP framing would require a reader to un-frame before inspection.
- One server process serves one client. Multi-client multiplexing is a non-goal.
- Revisiting this decision (adding HTTP, for example) requires a new ADR that supersedes this one and a MAJOR version bump per [VERSIONING.md](../../VERSIONING.md), because the binary contract would change.

## Alternatives considered

- **LSP `Content-Length` framing** — rejected: adds a framing codec that buys nothing for stdio (there is no stream interleaving to disambiguate) and breaks naive line-based debugging.
- **HTTP + SSE / Streamable HTTP** — rejected for v1.x: expands attack surface, forces TLS/auth decisions, multiplies transport code paths, and has no shipped client demand for this project.
- **WebSocket** — rejected for v1.x: same reasons as HTTP plus an additional framing layer.
