# Architecture

**Project:** github.com/andygeiss/mcp
**Generated:** 2026-04-05

## Executive Summary

The MCP server is a single-binary Go application that implements the Model Context Protocol over stdin/stdout using JSON-RPC 2.0. The architecture is intentionally flat: four internal packages with a strict one-way dependency graph, no external dependencies, and no abstraction ahead of need.

## Architecture Pattern

**Sequential message loop** with a three-state lifecycle state machine. No goroutine pools, no worker queues. One message decoded, dispatched, responded, repeat. Tool handlers run with timeout and panic recovery via a dedicated goroutine per call.

## Package Structure and Dependency Graph

```
cmd/mcp/main.go
    |
    v
internal/server/
    |         \
    v          v
internal/     internal/
protocol/     tools/
               |
               v
          internal/
          protocol/
```

**Rules enforced by tests (claudemd_test.go):**
- `protocol/` has zero dependencies on other internal packages
- `tools/` may import `protocol/` but never `server/`
- `assert/` is test-only (never imported by non-test files)

## Initialization State Machine

Three states: **uninitialized** -> **initializing** -> **ready**.

```
                    initialize
  [uninitialized] ────────────> [initializing]
        |                            |
        | ping (always OK)           | notifications/initialized
        |                            |
        v                            v
  [uninitialized]              [ready]
                                     |
                                     | All methods available
                                     v
                               [ready]
```

| State | Allowed | Rejected with |
|-------|---------|---------------|
| Uninitialized | `initialize`, `ping` | `-32600` ("server not initialized") |
| Initializing | `ping` | `-32600` ("server not initialized") |
| Ready | All methods | -- |

- `initialize` -> respond with capabilities, transition to initializing. Duplicate -> `-32600`.
- `notifications/initialized` in initializing -> transition to ready. Other states -> silently ignore.
- `ping` always works in any state.
- Unknown notifications are silently ignored -- never respond, never log.

## Transport Layer

| Stream | Purpose | Implementation |
|--------|---------|----------------|
| stdin | JSON-RPC requests | Persistent `json.NewDecoder`, no `bufio.Scanner` |
| stdout | JSON-RPC responses only | `json.NewEncoder`, every byte is valid JSON-RPC |
| stderr | Diagnostics | `slog.JSONHandler` exclusively |

**I/O injection:** Constructors accept `io.Reader`/`io.Writer` -- not `*os.File` -- so tests inject `bytes.Buffer`.

**EOF handling:** `io.EOF` / `io.ErrUnexpectedEOF` -> clean shutdown (exit 0). All other decode errors -> fatal (exit 1).

**Size limits:** Per-message via `countingReader` (4 MB limit). Not cumulative -- counter resets between messages.

**Signals:** `SIGINT`/`SIGTERM` cancel the server context for graceful shutdown. No drain -- exit promptly.

## JSON-RPC 2.0 Protocol

- Newline-delimited JSON objects. No LSP framing. No batch requests (JSON array -> `-32700`).
- `id` is `json.RawMessage` -- preserve original type, echo back exactly.
- A message without `id` is a notification -- never respond to it.
- `params`: when absent or `null`, default to `{}` before unmarshaling.
- Error `message` must be contextual (e.g., `"unknown tool: foo"`, not `"invalid params"`).

### Error Codes

| Code | Meaning | Example |
|------|---------|---------|
| `-32700` | Parse error | Malformed JSON, size limit exceeded, batch array |
| `-32600` | Invalid request | Bad structure, not initialized, already initialized |
| `-32601` | Method not found | Unknown method, `rpc.*` reserved, unsupported namespaces |
| `-32602` | Invalid params | Wrong types, missing fields, unknown tool name |
| `-32603` | Internal error | Handler panic (includes panic value + stack in Data) |

### Unsupported Capability Namespaces

`prompts/*` and `resources/*` methods return `-32601` with a `capabilityGuidance` data payload indicating the namespace is not supported.

## Tool System

### Registry

`tools.Registry` manages named tools with:
- Thread-unsafe registration (done at startup)
- Alphabetically sorted tool list (deterministic ordering)
- Lookup by name for dispatch
- Duplicate name detection (panics at registration)

### Schema Derivation

Input schemas are auto-derived from Go struct tags via reflection (`tools/schema.go`):
- `json:"name"` tag -> property name
- `description:"..."` tag -> property description
- Fields without `omitempty` -> required
- Supports: string, int, float, bool, slices, maps (string keys), nested structs, pointers

### Tool Handler Execution

1. Server dispatches `tools/call` to the registered handler
2. Handler runs in a goroutine with `context.WithTimeout` (default 30s)
3. **Panic recovery:** Panics are caught, logged with stack trace to stderr, returned as `-32603` with `panicDiag` data
4. **Timeout:** If handler exceeds timeout, context cancels. Safety timer (default +1s) ensures the goroutine is not leaked
5. **Cancellation:** If parent context cancels (SIGINT/SIGTERM), handler receives cancellation via context

### Built-in Tools

**search** -- Recursive file content search with regex:
- Path validation (must be within working directory)
- Symlink protection (`O_NOFOLLOW` on Unix)
- Binary file detection (null byte check)
- 1 MB file size limit
- Extension filtering
- Case-insensitive regex support
- Context-aware cancellation
- MaxResults limiting (default 100)

## Server Core (server.go)

460 lines implementing:
- Three-state lifecycle management
- Sequential message dispatch loop (decode -> validate -> route -> respond)
- Method routing: `initialize`, `ping`, `tools/list`, `tools/call`
- Notification handling: `notifications/initialized` (silently ignore unknown)
- Error response construction with contextual messages
- Handler timeout with safety timer
- Panic recovery with structured diagnostics

## Counting Reader (counting_reader.go)

Per-message size enforcement:
- Wraps `io.Reader`, tracks bytes since last `Reset()`
- Returns `errMessageTooLarge` when 4 MB limit exceeded
- Accounts for json.Decoder internal buffering (4-64 KB)
- Reset between messages for per-message (not cumulative) enforcement

## Template Rewriter (cmd/init/)

One-time project initialization tool:
1. Rewrites `go.mod` module path
2. Replaces all import statements in `.go` files
3. Replaces references in text files (README, Makefile, etc.)
4. Renames `cmd/mcp/` to `cmd/<projectName>/`
5. Runs `go mod tidy`
6. Self-deletes `cmd/init/`
7. Verifies zero template fingerprint remains

Uses `bytes.ReplaceAll` for simplicity (not AST parsing). Path-sanitized with `filepath.Clean`. Idempotent.

## Testing Strategy

| Category | Approach |
|----------|----------|
| Unit tests | Black-box (`package foo_test`), parallel, table-driven |
| Integration tests | `//go:build integration`, full pipeline through server |
| Fuzz tests | Native Go fuzz for protocol decoder |
| Benchmarks | Codec encode/decode, schema derivation |
| I/O tests | Slow readers, partial reads, closed writers |
| Concurrency | `testing/synctest` with virtual time (no wall-clock flakiness) |
| Documentation | `claudemd_test.go` validates CLAUDE.md claims match tests |
| Dependency | Tests enforce import graph rules |
| External | OSS-Fuzz with AddressSanitizer |

**Naming:** `Test_<Unit>_With_<Condition>_Should_<Outcome>`
**Assertions:** `assert.That(t, "description", got, expected)`
**Structure:** `// Arrange` / `// Act` / `// Assert`
