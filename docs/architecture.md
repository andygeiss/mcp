# Architecture

## Overview

Flat and simple -- no hexagonal layers, no bounded contexts. A single CLI binary reads JSON-RPC 2.0 messages from stdin, dispatches them through a three-state lifecycle engine, and writes responses to stdout. Diagnostics go to stderr via `slog.JSONHandler`. v1.3.0 extends the transport to bidirectional: a single reader goroutine demuxes inbound requests, notifications, and responses to server-initiated requests (sampling, elicitation, roots) -- see [ADR-003](adr/ADR-003-bidi-reader-split.md).

## Package Structure

```
cmd/mcp/                  main.go -- wiring only: flags, I/O injection, os.Exit
cmd/scaffold/             template rewriter -- rewrites the module path for scaffold consumers, self-deletes after use (binary dir remains cmd/mcp)
internal/
  assert/                 lightweight test assertion helpers (assert.That)
  prompts/                prompt registry, reflection-based argument derivation
  protocol/               JSON-RPC 2.0 types, message codec, constants, Peer interface, capability types, typed errors
  resources/              resource registry, static resources, URI templates (RFC 6570 Level 1)
  schema/                 shared JSON Schema derivation engine via reflection
  server/                 lifecycle, dispatch, notifications, progress, logging, bidirectional transport
  tools/                  tool registry, schema derivation, handlers, input validation
```

`internal/server/` is file-partitioned by concern: `server.go` (lifecycle + outbound `Peer.SendRequest`), `dispatch.go` (routing, notifications, error responses), `decode.go` (read loop, in-flight vs idle arms), `inflight.go` (async tool dispatch, timeout, cancellation), `handlers_*.go` (per-namespace request handlers), `progress.go` (`Progress` notifier), `counting_reader.go` (per-message size enforcement). See [ADR-002](adr/ADR-002-internal-package-layout.md) for the layout rationale.

## Dependency Direction

```
cmd/mcp/ ──> server/ ──> protocol/
                ├──> tools/    ──> protocol/, schema/
                ├──> resources/
                └──> prompts/  ──> protocol/, schema/
```

- `protocol/` has zero internal dependencies
- `schema/` has zero internal dependencies
- `resources/` has zero internal dependencies
- `assert/` is test-only
- `tools/` and `prompts/` may import `protocol/` and `schema/` but never `server/`
- `*Server` satisfies `protocol.Peer` — tool and prompt handlers reach the outbound path via `protocol.SendRequest(ctx, method, params)` without importing `internal/server` (Invariant I1)

## Transport

| Stream | Purpose | Constraint |
|---|---|---|
| **stdin** | Persistent `json.NewDecoder` | No `bufio.Scanner`. EOF = clean shutdown (exit 0) |
| **stdout** | Protocol-only, single writer mutex | Every byte is a valid JSON-RPC message. Responses, notifications, and outbound requests all serialize through `stdoutMu` |
| **stderr** | `slog.JSONHandler` exclusively | Structured JSON diagnostics |

All constructors accept `io.Reader`/`io.Writer` (not `*os.File`) so tests inject buffers.

### Bidirectional Reader (ADR-003)

A single reader goroutine consumes stdin and classifies each framed message:

| Shape | Condition | Routed to |
|---|---|---|
| **Inbound request** | `id` + `method` | Existing dispatch path (sync or async tool call) |
| **Inbound response** | `id` only, no `method` | Pending-request map, delivered to awaiting handler via buffer-1 `respChan` |
| **Inbound notification** | no `id` | `handleNotification` (initialized, cancelled, or silent drop) |

The pending map is guarded by `sync.Mutex`. The sole insert site is `registerPending`; cleanup is `delete(map, id)` and the unreferenced `respChan` is GC'd. The reader **never closes** `respChan` (Invariant I3) — cancellation is signalled via `ctx.Done()` and `s.done`.

## Initialization State Machine

Three states: **uninitialized** -> **initializing** -> **ready**.

| State | Allowed | Rejected with |
|---|---|---|
| **Uninitialized** | `initialize`, `ping` | `-32000` ("server not initialized") |
| **Initializing** | `ping` | `-32000` ("server not initialized") |
| **Ready** | All methods | -- |

Transitions:
- `initialize` -> snapshot `params.Capabilities` into `clientCaps` (atomic.Pointer), respond with capabilities, transition to **initializing**. Duplicate -> `-32000`.
- `notifications/initialized` in **initializing** -> transition to **ready**. Other states -> silently ignore.
- `ping` works in any state.

## Server Capabilities

Advertised during `initialize` response:

| Capability | Condition | Methods |
|---|---|---|
| **tools** | Always (when registry provided) | `tools/list`, `tools/call` |
| **resources** | When resources registry provided | `resources/list`, `resources/read`, `resources/templates/list` |
| **prompts** | When prompts registry provided | `prompts/list`, `prompts/get` |
| **logging** | Always | `logging/setLevel` |
| **experimental** | Always | `concurrency.maxInFlight: 1` (sequential inbound dispatch) |

Note: `maxInFlight: 1` governs **inbound** dispatch only. The outbound axis is independent — a single in-flight handler may issue multiple sequential `Peer.SendRequest` calls, each correlated by ID.

## Client Capabilities (v1.3.0)

The client advertises capabilities during `initialize`; the server snapshots them and gates server-initiated outbound requests (AI9):

| Capability | Required for | Method |
|---|---|---|
| `elicitation` | `Peer.SendRequest` | `elicitation/create` |
| `roots` | `Peer.SendRequest` | `roots/list` |
| `sampling` | `Peer.SendRequest` | `sampling/createMessage` |

`Peer.SendRequest` rejects synchronously with `*protocol.CapabilityNotAdvertisedError` if the method's required capability was not advertised. The check uses an `atomic.Pointer[ClientCapabilities]` for lock-free reads from handler goroutines.

## Dispatch Model

Sequential inbound dispatch with one in-flight handler at a time.

While a handler is in flight:
- `ping` answered immediately
- Notifications handled normally (including `notifications/cancelled`)
- Other requests rejected with `-32000` ("server busy: request in flight (maxInFlight is 1)")

Tool calls are dispatched asynchronously (goroutine) with:
- Configurable handler timeout (default 30s, `WithHandlerTimeout`)
- Safety margin (default 1s, `WithSafetyMargin`) before force-failing
- Panic recovery (logged, error returned with `-32603`; panic value never sent to client)
- Context cancellation via `notifications/cancelled` (inbound) — response suppressed per MCP spec
- Tool and prompt handlers receive a `*Progress` and a `protocol.Peer` via context

## Peer Interface (ADR-003)

`protocol.Peer` is the stable surface tool and prompt handlers use to send JSON-RPC 2.0 requests back to the client:

```go
type Peer interface {
    SendRequest(ctx context.Context, method string, params any) (*Response, error)
}
```

Stability commitment per ADR-003 §Peer Stability Surface — any change (adding/removing methods, changing signatures) is a MAJOR version bump. Signatures are restricted to stdlib types and protocol-native types; no `internal/server`, no `internal/tools`, no third-party types.

Handlers invoke the outbound path via the convenience wrapper:

```go
resp, err := protocol.SendRequest(ctx, "sampling/createMessage", params)
```

`*Server` implements `Peer` and is injected into the handler context during tool/prompt dispatch. A compile-time `var _ protocol.Peer = (*Server)(nil)` guards signature drift.

## Outbound Cancellation (A7 Symmetry)

When a handler's context is cancelled mid-outbound (e.g., client sent `notifications/cancelled` for the inbound request that spawned the outbound), the server emits a **symmetric outbound cancel**:

1. Priority check: if `s.done` is already closed (shutdown), return `ErrServerShutdown` — never emit cancel to a client we are no longer talking to.
2. Otherwise emit `notifications/cancelled` with the **outbound** `requestId` BEFORE removing the pending-map entry (preserves "entry still present while cancel is in flight" invariant).
3. Delete the pending entry; return `ctx.Err()` to the handler.

Documented in `dispatch.go`'s `emitOutboundCancel` and exercised by the bidi synctest scenarios.

## Size Limits

| Limit | Value | Enforcement |
|---|---|---|
| Per-message | 4 MB | Counting reader (streaming, per-message — not cumulative) |
| Per-tool result | 1 MB | Truncated with warning if exceeded |
| Per-input string | 4,096 chars | Validated before handler dispatch |
| JSON nesting depth | 64 | Pre-unmarshal brace/bracket scan |
| Pending outbound requests | 1,024 | `ErrPendingRequestsFull` back-pressure |

## Error Codes

| Code | Meaning |
|---|---|
| `-32700` | Parse error -- malformed JSON, size limit exceeded, batch rejected |
| `-32600` | Invalid request -- bad structure, wrong jsonrpc version |
| `-32601` | Method not found -- unknown method, `rpc.*` reserved, unsupported capability namespace |
| `-32602` | Invalid params -- wrong types, missing required fields, unknown tool/prompt name |
| `-32603` | Internal error -- should not happen in normal operation |
| `-32000` | Server error -- state prevents processing (not initialized, already initialized, server busy) |
| `-32001` | Server timeout -- tool/prompt handler timed out or was cancelled |
| `-32002` | Resource not found -- `resources/read` URI matched no resource or template |

Error codes map from `*protocol.CodeError` at the dispatch boundary via `errors.AsType[*protocol.CodeError]`. Handlers returning a plain error fall back to `-32603`.

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

**AI10 invariant (convention-only):** a handler parked in `protocol.SendRequest` MUST NOT interleave `Report` calls with the awaited reply. The per-handler timeout (`-32001`) is the sole slow-client recovery mechanism.

## Typed Errors (AI8)

Errors crossing the reader ↔ pending-map boundary are typed sentinels in `internal/protocol`:

| Error | Returned by | Meaning |
|---|---|---|
| `ErrServerShutdown` | `Peer.SendRequest` | `s.done` closed before response |
| `ErrPendingRequestsFull` | `Peer.SendRequest` | Correlation map at capacity (1,024) |
| `ErrNoPeerInContext` | `protocol.SendRequest` | No `Peer` attached to ctx (handler outside tool/prompt scope) |
| `*CapabilityNotAdvertisedError` | `Peer.SendRequest` | Capability gate rejected the method |
| `*ClientRejectedError` | — | Wraps a client-returned JSON-RPC error object (Code/Message/Data) |

Handlers match via `errors.Is` (sentinels) or `errors.AsType[T]` (structured).

## JSON-RPC 2.0

- Newline-delimited JSON objects (no LSP framing, no batch requests)
- `id` is `json.RawMessage` -- preserved and echoed exactly
- A message without `id` is a notification -- never respond
- `params` absent or null defaults to `{}` before unmarshaling
- `encoding/json` v1 with `omitempty` (no `omitzero`, no `GOEXPERIMENT=jsonv2`)

---

*Generated: 2026-04-18 | Scan level: deep | Reflects v1.3.0*
