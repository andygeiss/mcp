# Architecture

**Project:** mcp
**Generated:** 2026-04-05

## Executive Summary

The MCP server is a single Go binary (`cmd/mcp/`) that implements the Model Context Protocol over stdin/stdout using JSON-RPC 2.0. The architecture is intentionally flat -- no hexagonal layers, no bounded contexts, no dependency injection frameworks. Complexity is added only when the code demands it.

## Architecture Pattern

**Sequential dispatch loop** -- the server reads one JSON-RPC message at a time from stdin, dispatches it to the appropriate handler, and writes the response to stdout. There are no goroutine pools for request handling; concurrency exists only within individual tool handler execution (timeout/panic recovery).

```
stdin -> json.Decoder -> Decode -> Validate -> Dispatch -> Encode -> json.Encoder -> stdout
                                                  |
                                            State Machine
                                         (uninitialized ->
                                          initializing ->
                                              ready)
```

## Package Structure and Dependency Direction

```
cmd/mcp/           -> internal/server/ -> internal/protocol/
                   -> internal/tools/  -> internal/protocol/

cmd/init/          (standalone, self-deleting template rewriter)

internal/pkg/assert/  (test-only, zero internal dependencies)
```

### Package Responsibilities

| Package | Responsibility |
|---|---|
| `cmd/mcp/` | Wiring only: parse flags, inject `os.Stdin`/`os.Stdout`/`os.Stderr`, register tools, call `server.Run`, `os.Exit` |
| `cmd/init/` | Template rewriter for consumers who fork/clone the repo. Rewrites module paths, renames binary dir, self-deletes. Not part of normal builds. |
| `internal/protocol/` | JSON-RPC 2.0 types (`Request`, `Response`, `Error`, `CodeError`), codec (`Decode`, `Validate`, `Encode`), and protocol constants. Zero internal dependencies. |
| `internal/server/` | MCP server lifecycle: initialization state machine, method dispatch, tool call execution with timeout/panic recovery, per-message size enforcement via counting reader. |
| `internal/tools/` | Tool registry, reflection-based schema derivation from struct tags, individual tool handler implementations (currently: `search`), input validation. |
| `internal/pkg/assert/` | Lightweight generic test assertion helper (`assert.That`). Test-only. |

## Initialization State Machine

Three states: **uninitialized** -> **initializing** -> **ready**.

| State | Allowed Methods | Rejected With |
|---|---|---|
| **Uninitialized** | `initialize`, `ping` | `-32600` ("server not initialized") |
| **Initializing** | `ping` | `-32600` ("server initializing, awaiting notifications/initialized") |
| **Ready** | All methods | -- |

- `initialize` -> respond with capabilities, transition to **initializing**. Duplicate -> `-32600` ("already initialized").
- `notifications/initialized` in **initializing** -> transition to **ready**. Other states -> silently ignore.
- `ping` always works in any state.
- Unknown notifications are silently ignored -- never respond, never log.

## Transport Constraints

| Channel | Purpose | Rules |
|---|---|---|
| **stdin** | JSON-RPC requests | Persistent `json.NewDecoder`. No `bufio.Scanner`. |
| **stdout** | JSON-RPC responses only | Every byte must be a valid JSON-RPC message. No logs, no debug output. |
| **stderr** | Structured diagnostics | `slog.JSONHandler` exclusively. `snake_case` log keys. |

- **I/O injection**: Constructors accept `io.Reader`/`io.Writer` (not `*os.File`) so tests inject `bytes.Buffer`.
- **EOF handling**: `io.EOF` / `io.ErrUnexpectedEOF` -> clean shutdown (exit 0). All other decode errors -> fatal (exit 1).
- **Signals**: `SIGINT`/`SIGTERM` via `signal.NotifyContext` cancel the server context for graceful shutdown.

## Per-Message Size Enforcement

The `countingReader` wraps stdin and tracks bytes read since the last reset. When the 4MB limit is exceeded, the server sends a `-32700` (parse error) response and exits fatally. The counter resets before each `Decode` call.

| Limit | Value | Scope |
|---|---|---|
| Max message size | 4 MB | Per incoming JSON-RPC message |
| Max result size | 1 MB | Per tool call response body |

## Tool Handler Dispatch

Tool calls are dispatched in a goroutine with:

1. **Panic recovery** -- catches panics, logs stack trace to stderr, returns `-32603` with structured `panicDiag` data (tool name, panic value -- no stack in wire response).
2. **Handler timeout** -- `context.WithTimeout` (default 30s, configurable via `WithHandlerTimeout`).
3. **Safety margin** -- a backup timer (default 1s) that fires if the handler ignores context cancellation entirely.
4. **Context cancellation** -- parent context cancellation propagates to the handler.

All error responses include structured `Error.Data` with timing diagnostics (`elapsedMs`, `timeoutMs`, `toolName`).

## Tool Schema Derivation

Tool input schemas are generated automatically from Go struct field tags via reflection (`tools/schema.go`):

- `json:"fieldName"` -> property name
- `json:"fieldName,omitempty"` -> optional (not in `required`)
- `description:"..."` tag -> property description
- Go types map to JSON Schema: `string` -> `"string"`, `int*` -> `"integer"`, `float*` -> `"number"`, `bool` -> `"boolean"`, `[]T` -> `"array"`, `map[string]T` -> `"object"` with `additionalProperties`, structs -> nested `"object"`

## Method Routing (Ready State)

| Method Pattern | Handler |
|---|---|
| `tools/list` | Returns all registered tools (alphabetically sorted) |
| `tools/call` | Dispatches to named tool handler with timeout/panic recovery |
| `prompts/*` | `-32601` with guidance data (supported capabilities) |
| `resources/*` | `-32601` with guidance data (supported capabilities) |
| `rpc.*` | `-32601` ("reserved method") |
| Other | `-32601` ("unknown method") |

## JSON-RPC 2.0 Specifics

- Newline-delimited JSON objects. No LSP framing. No batch requests (JSON array -> `-32700`).
- `id` is `json.RawMessage` -- preserve original type (string, number, null), echo back exactly.
- A message without `id` is a notification -- never respond to it.
- `params`: when absent or `null`, default to `{}` before unmarshaling.
- Error `message` fields are contextual (e.g., `"unknown tool: foo"`, not generic `"invalid params"`).

## Error Code Catalog

| Code | Meaning | When Used |
|---|---|---|
| `-32700` | Parse error | Malformed JSON, batch arrays, message size exceeded |
| `-32600` | Invalid request | Wrong JSON-RPC version, empty method, non-object params, not initialized, already initialized |
| `-32601` | Method not found | Unknown method, `rpc.*` reserved, unsupported capabilities (prompts/, resources/) |
| `-32602` | Invalid params | Wrong types, missing required fields, unknown tool name, empty tool name |
| `-32603` | Internal error | Handler panics, timeouts, context cancellation, marshal failures |

## Security Measures

- **Input validation**: Path traversal prevention, null byte rejection, length limits (4096 chars per input string)
- **Symlink safety**: `O_NOFOLLOW` on Unix for file opens in search tool; symlink resolution with working directory containment check
- **Binary file detection**: Files containing null bytes in first 512 bytes are skipped
- **No shell execution**: The search tool uses Go's `regexp` and `filepath.WalkDir`, never shells out
- **Supply chain**: Pinned GitHub Actions by hash, Dependabot for automated updates, Cosign signing, SBOM generation
- **Static analysis**: CodeQL weekly, 50+ golangci-lint rules, `depguard` blocks external deps and json/v2

## Trace Mode

Enabled via `MCP_TRACE=1` environment variable. Logs every incoming request and outgoing response to stderr. Zero overhead when disabled (boolean check before marshaling).
