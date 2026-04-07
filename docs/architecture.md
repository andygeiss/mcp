# Architecture

**Project:** mcp
**Generated:** 2026-04-07

## Overview

Flat, simple architecture. Single CLI binary communicating over stdin/stdout via JSON-RPC 2.0. No HTTP, no WebSocket, no external dependencies. Complexity is added only when the code demands it.

## Package Structure

```
cmd/
  mcp/                  Entry point — wiring only: flags, I/O injection, os.Exit
  init/                 Template rewriter — rewrites module path, renames binary, self-deletes
internal/
  pkg/
    assert/             Lightweight test assertion helper (assert.That)
  protocol/             JSON-RPC 2.0 types, codec, constants
  server/               MCP lifecycle, dispatch, capability negotiation, resilience
  tools/                Tool registry, reflection-based schema derivation, handlers
```

## Dependency Direction

```
cmd/mcp/ ──→ internal/server/ ──→ internal/protocol/
                    │
                    └──→ internal/tools/ ──→ internal/protocol/
```

- `protocol/` has zero internal dependencies -- it is the foundation layer
- `tools/` may import `protocol` but never `server`
- `cmd/mcp/` wires everything together

## Transport Layer

| Stream | Purpose | Rules |
|---|---|---|
| **stdin** | Incoming JSON-RPC messages | Persistent `json.NewDecoder`. No `bufio.Scanner`. |
| **stdout** | Outgoing JSON-RPC responses | Protocol-only. Every byte must be valid JSON-RPC. |
| **stderr** | Diagnostics | `slog.JSONHandler` exclusively. |

All constructors accept `io.Reader`/`io.Writer` (not `*os.File`) so tests inject `bytes.Buffer`.

## Server Lifecycle (State Machine)

Three states: **uninitialized** -> **initializing** -> **ready**.

```
                 initialize           notifications/initialized
  UNINITIALIZED ──────────→ INITIALIZING ───────────────────→ READY
       │                         │                              │
       │ ping ✓                  │ ping ✓                      │ all methods ✓
       │ other → -32600          │ other → -32600              │
```

- `initialize` transitions to **initializing** and returns capabilities
- `notifications/initialized` transitions to **ready**
- `ping` is always accepted regardless of state
- Unknown notifications are silently ignored

## Dispatch Architecture

The dispatch loop uses two modes:

### Idle Mode
```
stdin decode → validate → route:
  ├── notification → handle silently (no response)
  ├── ping → immediate response
  ├── initialize → state machine transition
  ├── tools/call → spawn async handler
  └── other method → synchronous dispatch
```

### In-Flight Mode (tool handler running)
```
priority: check handler completion first
then select:
  ├── handler result → send response, return to idle
  ├── stdin message:
  │     ├── ping → immediate response
  │     ├── notifications/cancelled → cancel handler
  │     ├── other notification → handle silently
  │     └── other request → reject -32600 (busy)
  └── context done → cancel handler, shutdown
```

## Tool System

### Registration
Tools are registered in `cmd/mcp/main.go` via `tools.Register[T]()`. The generic type parameter `T` defines the input struct. Struct fields with `json` tags are reflected to build the JSON Schema returned in `tools/list`. Fields without `omitempty` are marked required.

### Schema Derivation
`tools/schema.go` uses `reflect.Type.Fields` (Go 1.26 iterator) to walk struct fields. Supports: bool, int/uint variants, float, string, slice, map (string keys), nested structs, pointers. Panics on unsupported types (channels, funcs).

### Handler Execution
1. Validate params, unmarshal arguments
2. Spawn handler goroutine with `context.WithTimeout` (30s default)
3. Safety timer at timeout + 1s margin
4. Panic recovery with stack trace logging (trace excluded from response)
5. Result size limit: 1MB (truncated with warning)

### Annotations
Tools can declare behavioral hints via `WithAnnotations()`:
- `destructiveHint`, `idempotentHint`, `openWorldHint`, `readOnlyHint`, `title`

## Error Handling

### JSON-RPC Error Codes

| Code | Meaning | When |
|---|---|---|
| `-32700` | Parse error | Malformed JSON, size limit exceeded, batch arrays |
| `-32600` | Invalid request | Bad structure, not initialized, already initialized, busy |
| `-32602` | Invalid params | Wrong types, missing fields, unknown tool name |
| `-32601` | Method not found | Unknown method, `rpc.*` reserved, unsupported capability |
| `-32603` | Internal error | Handler panic, timeout, marshal failure |

### Error Propagation
- Handlers return `*protocol.CodeError` with specific codes
- Non-CodeError errors fall back to -32603 (internal error)
- `errors.AsType[*protocol.CodeError]` (Go 1.26) for type-safe unwrapping

### Unsupported Capabilities
Methods in `completion/`, `elicitation/`, `prompts/`, `resources/` namespaces return -32601 with structured `capabilityGuidance` data pointing to `tools` as the only supported capability.

## Resilience

### Size Limits
- Per-message: 4MB via `countingReader` (reset between messages)
- Tool result: 1MB (truncated with warning, not fatal)

### Handler Timeout
- Default: 30s per handler
- Safety margin: 1s additional before force-fail
- Configurable via `WithHandlerTimeout()` and `WithSafetyMargin()`

### Panic Recovery
Panics in tool handlers are caught, logged with stack trace, and returned as -32603 with `panicDiag` data. Stack traces are excluded from the wire response (security).

### Cancellation
`notifications/cancelled` with matching `requestId` cancels the in-flight handler's context. The response is suppressed per MCP spec.

### EOF Handling
- `io.EOF` / `io.ErrUnexpectedEOF` -> clean shutdown (exit 0)
- All other decode errors -> -32700 response, then fatal (exit 1)

## Input Validation

### Tool Inputs
- `ValidatePath()`: rejects paths > 4096 chars, null bytes, `..` traversal
- `ValidateInput()`: rejects inputs > 4096 chars, null bytes
- Search tool: path must be within working directory (symlink-resolved)
- File opens with `O_NOFOLLOW` on Unix to prevent symlink attacks

### Protocol Level
- JSON-RPC version must be "2.0"
- Method must be non-empty
- Params must be a JSON object (not array/primitive)
- ID must be string, number, or null

## Concurrency Model

### Sequential Dispatch
The server advertises `experimental.concurrency.maxInFlight: 1`. Only one tool handler runs at a time. Additional `tools/call` requests while a handler is in flight are rejected with `-32600` ("server busy: request in flight").

### Async Tool Dispatch
Despite sequential dispatch, tool calls are executed asynchronously from the decode loop:

```
Main goroutine (decode loop)          Handler goroutine
─────────────────────────────         ─────────────────
tools/call arrives
  ├── validate params
  ├── spawn handler goroutine  ──→    context.WithTimeout(ctx, 30s)
  ├── switch to in-flight mode        execute tool handler
  ├── continue reading stdin          ...
  │   (accept ping, cancel)           handler returns result
  ├── receive result  ←───────────    send to inFlightCh
  ├── encode response
  └── return to idle mode
```

This design enables:
- **Ping responsiveness** during long-running tools
- **Cancellation** via `notifications/cancelled` while handler is executing
- **Graceful shutdown** can cancel in-flight handler and wait for completion

### Goroutine Lifecycle
Handlers run in a nested goroutine structure:
1. **Outer goroutine** (started by `startToolCallAsync`): owns `inFlightCh`, calls `dispatchToolCall`
2. **Inner goroutine** (started by `dispatchToolCall`): runs the actual handler with `defer recover()` for panic protection, owns the handler timeout `context.WithTimeout`

If a handler ignores `ctx.Done()` and exceeds the budget, the goroutine is abandoned after the safety timer fires. The server logs a warning and proceeds to the next request.

### Decode Error During In-Flight
When a decode error occurs while a handler is running:
1. Send `-32700` parse error response
2. Wait for the handler to complete (with 2x budget timeout)
3. Send the handler's response if it completes normally
4. Then shut down with the fatal decode error

This ensures no response is lost even when the transport fails.

## Protocol Codec Details

### Decode Pipeline
```
json.Decoder.Decode → raw json.RawMessage
  ├── batch detection: raw[0] == '[' → error "batch requests not supported"
  ├── json.Unmarshal into Request struct
  └── params normalization: absent or null → {}
```

### encoding/json v1 Behaviors (Accepted)
The protocol codec relies on `encoding/json` v1 semantics. These behaviors differ from `json/v2` strict defaults but are accepted for MCP:
- **Case-insensitive field matching**: `"Method"` matches `method` struct field
- **Last-value-wins for duplicate keys**: `"method":"a","method":"b"` → method is "b"
- **Invalid UTF-8 passthrough**: Non-UTF-8 bytes in params are not rejected

These are documented as accepted trade-offs. Well-behaved MCP clients send correctly-cased fields, no duplicate keys, and valid UTF-8.

### ID Validation
The `Validate` function checks ID type by inspecting the first byte of `json.RawMessage`:
- `"` → string (valid)
- `0`-`9`, `-` → number (valid)
- `n` → null (valid)
- Anything else (e.g., `t`/`f` for booleans, `[` for arrays, `{` for objects) → `-32600`

## Wire Format Examples

### Initialize Handshake
```json
→ {"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}
← {"id":1,"jsonrpc":"2.0","result":{"capabilities":{"experimental":{"concurrency":{"maxInFlight":1}},"tools":{}},"protocolVersion":"2025-06-18","serverInfo":{"name":"mcp","version":"..."}}}
→ {"jsonrpc":"2.0","method":"notifications/initialized"}
(no response — notification)
```

### Tool Call
```json
→ {"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"echo","arguments":{"message":"hello"}}}
← {"id":2,"jsonrpc":"2.0","result":{"content":[{"text":"hello","type":"text"}]}}
```

### Error Response with Data
```json
→ {"jsonrpc":"2.0","method":"resources/list","id":3,"params":{}}
← {"error":{"code":-32601,"data":{"hint":"this server supports tools only; use tools/list and tools/call","supportedCapabilities":["tools"]},"message":"method not found: resources/list"},"id":3,"jsonrpc":"2.0"}
```

### Cancellation
```json
→ {"jsonrpc":"2.0","method":"tools/call","id":4,"params":{"name":"search","arguments":{"path":".","pattern":"TODO"}}}
→ {"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":4,"reason":"user cancelled"}}
(response for id:4 is suppressed)
```
