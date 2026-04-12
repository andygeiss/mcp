# Architecture

## Overview

Flat and simple -- no hexagonal layers, no bounded contexts. A single CLI binary reads JSON-RPC 2.0 messages from stdin, dispatches them through a three-state lifecycle engine, and writes responses to stdout. Diagnostics go to stderr via `slog.JSONHandler`.

## Package Structure

```
cmd/mcp/                  main.go -- wiring only: flags, I/O injection, os.Exit
cmd/init/                 template rewriter -- rewrites the module path for scaffold consumers, self-deletes after use (binary dir remains cmd/mcp)
internal/
  assert/                 lightweight test assertion helpers (assert.That)
  prompts/                prompt registry, reflection-based argument derivation
  protocol/               JSON-RPC 2.0 types, message codec, constants
  resources/              resource registry, static resources, URI templates (RFC 6570 Level 1)
  schema/                 shared JSON Schema derivation engine via reflection
  server/                 lifecycle, dispatch, notifications, progress, logging, bidirectional transport
  tools/                  tool registry, schema derivation, handlers, input validation
```

## Dependency Direction

```
cmd/mcp/ ──> server/ ──> protocol/
                ├──> tools/    ──> protocol/, schema/
                ├──> resources/
                └──> prompts/  ──> schema/
```

- `protocol/` has zero internal dependencies
- `schema/` has zero internal dependencies
- `resources/` has zero internal dependencies
- `assert/` is test-only
- `tools/` and `prompts/` may import `protocol/` and `schema/` but never `server/`

## Transport

| Stream | Purpose | Constraint |
|---|---|---|
| **stdin** | Persistent `json.NewDecoder` | No `bufio.Scanner`. EOF = clean shutdown (exit 0) |
| **stdout** | Protocol-only | Every byte is a valid JSON-RPC message. No logs, no debug |
| **stderr** | `slog.JSONHandler` exclusively | Structured JSON diagnostics |

All constructors accept `io.Reader`/`io.Writer` (not `*os.File`) so tests inject buffers.

## Initialization State Machine

Three states: **uninitialized** -> **initializing** -> **ready**.

| State | Allowed | Rejected with |
|---|---|---|
| **Uninitialized** | `initialize`, `ping` | `-32000` ("server not initialized") |
| **Initializing** | `ping` | `-32000` ("server not initialized") |
| **Ready** | All methods | -- |

Transitions:
- `initialize` -> respond with capabilities, transition to **initializing**. Duplicate -> `-32000`.
- `notifications/initialized` in **initializing** -> transition to **ready**. Other states -> silently ignore.
- `ping` works in any state.

## Server Capabilities

Advertised during `initialize` response:

| Capability | Condition | Methods |
|---|---|---|
| **tools** | Always (when registry provided) | `tools/list`, `tools/call` |
| **resources** | When resources registry provided | `resources/list`, `resources/read` |
| **prompts** | When prompts registry provided | `prompts/list`, `prompts/get` |
| **logging** | Always | `logging/setLevel` |
| **experimental** | Always | `maxInFlight: 1` (sequential dispatch) |

## Dispatch Model

Sequential dispatch with one in-flight handler at a time.

While a handler is in flight:
- `ping` answered immediately
- Notifications handled normally (including `notifications/cancelled`)
- Other requests rejected with `-32000` ("server busy")

Tool calls are dispatched asynchronously (goroutine) with:
- Configurable handler timeout (default 30s)
- Safety margin (1s goroutine cleanup buffer)
- Panic recovery (logged, error returned with `-32603`)
- Context cancellation via `notifications/cancelled`
- Cancelled requests produce no response

## Size Limits

| Limit | Value | Enforcement |
|---|---|---|
| Per-message | 4 MB | Counting reader (streaming, not cumulative) |
| Per-tool result | 1 MB | Truncated with warning if exceeded |
| Per-input | 4,096 chars | Validated before handler dispatch |

## Error Codes

| Code | Meaning |
|---|---|
| `-32700` | Parse error -- malformed JSON, size limit exceeded |
| `-32600` | Invalid request -- bad structure, wrong jsonrpc version |
| `-32601` | Method not found -- unknown method, `rpc.*` reserved |
| `-32602` | Invalid params -- wrong types, missing required fields |
| `-32603` | Internal error -- should not happen in normal operation |
| `-32000` | Server error -- not initialized, already initialized, busy |
| `-32001` | Server timeout -- handler timed out or cancelled |

## Schema Derivation

Tools, prompts, and (optionally) output schemas are auto-derived from Go structs via reflection (`internal/schema/`):

- Exported fields with `json` tags become properties
- `description` struct tag provides property descriptions
- Fields without `omitempty` are required
- Supports: string, int, float, bool, slices, maps, nested structs, pointers
- Max recursion depth: 10
- Anonymous embedded structs promoted to parent (unless tagged)

## Progress & Logging

Tool handlers receive a `*Progress` via context (`server.ProgressFromContext(ctx)`):

- `p.Report(current, total)` -- progress notifications (requires client `_meta.progressToken`)
- `p.Log(level, logger, data)` -- `notifications/message` log events
- Both methods are nil-safe (no-op on nil receiver)

## Bidirectional Transport

`server.SendRequestFromContext(ctx, method, params)` is the primitive for server-to-client requests. No handlers for the following are built in -- consumers wire them as needed:
- Sampling (LLM completion requests)
- Elicitation (user input requests)
- Roots listing

Responses are correlated via a pending-request map with atomic ID generation.

## JSON-RPC 2.0

- Newline-delimited JSON objects (no LSP framing, no batch requests)
- `id` is `json.RawMessage` -- preserved and echoed exactly
- A message without `id` is a notification -- never respond
- `params` absent or null defaults to `{}` before unmarshaling
- `encoding/json` v1 with `omitempty` (no `omitzero`, no `GOEXPERIMENT=jsonv2`)

---

*Generated: 2026-04-11 | Scan level: exhaustive*
