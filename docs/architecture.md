# Architecture

## Executive Summary

A minimal, zero-dependency Go implementation of the Model Context Protocol (MCP) communicating over stdin/stdout using JSON-RPC 2.0. The project is both a working MCP server and a template for scaffolding custom MCP servers. It prioritizes correctness, simplicity, and security over feature breadth.

## Technology Stack

| Category | Technology | Version | Justification |
|---|---|---|---|
| Language | Go | 1.26 | Green Tea GC, `reflect.Type.Fields` iterators, `signal.NotifyContext` cancel cause, `errors.AsType[T]` |
| Transport | stdin/stdout | — | Simplest possible transport; no HTTP, no WebSocket |
| Protocol | JSON-RPC 2.0 | — | MCP specification requirement |
| Codec | `encoding/json` v1 | — | Stable stdlib; v2 (`GOEXPERIMENT=jsonv2`) explicitly unsupported |
| Logging | `log/slog` | — | Structured JSON logging to stderr via `slog.JSONHandler` |
| Testing | `testing` stdlib | — | Fuzz, benchmarks, race detector, parallel execution |
| Linting | golangci-lint v2 | — | 60+ linters enabled, zero suppression policy |
| Release | GoReleaser | — | Cross-platform builds, cosign signing, SBOM generation |
| Fuzzing | OSS-Fuzz | — | Continuous fuzzing with libFuzzer + address sanitizer |

## Architecture Pattern

Flat and simple — no hexagonal layers, no bounded contexts. The architecture follows a direct dependency chain:

```
cmd/mcp/ → internal/server/ → internal/protocol/
                            → internal/tools/
```

- `protocol` has zero dependencies on other internal packages
- `tools` may import `protocol` but never `server`
- `assert` is test-only

## Core Components

### Protocol Layer (`internal/protocol/`)

The wire format layer. Handles JSON-RPC 2.0 encoding, decoding, validation, and error construction.

**Key types:**
- `Request` — JSON-RPC 2.0 request/notification with `json.RawMessage` fields for `ID` and `Params`
- `Response` — JSON-RPC 2.0 response with optional `Error` and `Result`
- `CodeError` — Typed error carrying a JSON-RPC error code from creation to the protocol boundary
- `Error` — Wire-format error object with code, message, and optional data

**Key functions:**
- `Decode(dec)` — Reads one message, detects batch arrays, normalizes null/absent params to `{}`
- `Validate(req)` — Checks version, method, params structure, ID type
- `Encode(enc, resp)` — Writes one response
- Error constructors: `ErrParseError`, `ErrInvalidRequest`, `ErrInvalidParams`, `ErrMethodNotFound`, `ErrInternalError`

**Error codes:**
| Code | Meaning |
|---|---|
| `-32700` | Parse error — malformed JSON, size limit exceeded |
| `-32600` | Invalid request — bad structure, not initialized, already initialized |
| `-32601` | Method not found — unknown method, reserved `rpc.*` methods |
| `-32602` | Invalid params — wrong types, missing fields, unknown tool |
| `-32603` | Internal error — should not happen in normal operation |

### Server Layer (`internal/server/`)

The core server implementing the MCP lifecycle, message dispatch, and tool execution.

**Initialization State Machine:**

```
uninitialized → initializing → ready
     │                │           │
     │  initialize    │  notif/   │  all methods
     │  ping          │  init'd   │
     └────────────────┘  ping     │
                                  └──────────────
```

| State | Allowed | Rejected |
|---|---|---|
| Uninitialized | `initialize`, `ping` | All others → `-32600` |
| Initializing | `ping` | All others → `-32600` |
| Ready | All methods | Duplicate `initialize` → `-32600` |

**Dispatch Model:**
- Sequential dispatch with `maxInFlight: 1` advertised via experimental capabilities
- Tool calls are dispatched asynchronously so the decode loop can continue reading for cancellation and ping support
- Additional requests while a handler is in flight are rejected with `-32600`
- Handler timeout: 30s default, configurable via `WithHandlerTimeout`
- Safety margin: 1s additional time before force-fail for unresponsive handlers
- Panic recovery with structured diagnostics (no stack traces in responses)

**Size Limits:**
- Per-message: 4 MB via `countingReader` (reset before each decode)
- Per-result: 1 MB — results exceeding this are truncated with a descriptive message

**Transport Rules:**
- `stdin`: Persistent `json.NewDecoder` — no `bufio.Scanner`
- `stdout`: Protocol-only — every byte is a valid JSON-RPC message
- `stderr`: `slog.JSONHandler` exclusively
- EOF/UnexpectedEOF → clean shutdown (exit 0); all other decode errors → fatal (exit 1)
- `SIGINT`/`SIGTERM` → cancel server context for graceful shutdown

**Cancellation Support:**
- `notifications/cancelled` cancels the in-flight handler if the request ID matches
- Cancelled requests do not receive a response (per MCP spec)
- Non-matching or stale cancellations are silently ignored

**Unsupported Capabilities:**
- Methods in `completion/`, `elicitation/`, `prompts/`, `resources/` namespaces return `-32601` with guidance data pointing to `tools/list` and `tools/call`

### Tools Layer (`internal/tools/`)

The tool registry and automatic schema derivation system.

**Registry:**
- `Register[T](registry, name, description, handler, ...opts)` — Generic registration with type-safe handler
- Tools are sorted alphabetically for deterministic `tools/list` responses
- Duplicate names panic at registration time (fail fast)

**Schema Derivation (`schema.go`):**
- Reflects over the generic type `T` to build JSON Schema
- Handles: primitives (string, int, float, bool), slices, maps with string keys, nested structs, pointers
- Fields without `omitempty` in their `json` tag are marked as required
- `description` struct tag populates the schema description
- Max depth: 10 levels
- Anonymous embedded structs are promoted (like Go embedding)
- Unsupported types (channels, funcs) panic with a descriptive message

**Tool Annotations:**
- `Annotations` struct: `DestructiveHint`, `IdempotentHint`, `OpenWorldHint`, `ReadOnlyHint`, `Title`
- Applied via `WithAnnotations(a)` option during registration

**Input Validation (`validate.go`):**
- `ValidatePath(path)` — checks for traversal (`..`), null bytes, length limit (4096)
- `ValidateInput(input)` — checks for null bytes, length limit (4096)

**Result Types:**
- `TextResult(text)` — Success with text content
- `ErrorResult(text)` — Tool execution failure with `isError: true`

### Template Rewriter (`cmd/init/`)

One-time utility for template consumers. Performs:

1. Rewrite `go.mod` module path
2. Rewrite all `.go` file import paths (using `bytes.ReplaceAll`)
3. Rewrite text files (`.md`, `.yml`, `.json`, etc.) for module path and binary name references
4. Rename `cmd/mcp/` to `cmd/<projectName>/`
5. Run `go mod tidy`
6. Self-delete `cmd/init/` directory
7. Remove build artifacts
8. Verify zero template fingerprint remains

## Testing Strategy

| Level | Description | Command |
|---|---|---|
| Unit | Black-box tests for all packages | `go test -race ./...` |
| Integration | Full pipeline through compiled server | `go test -race -tags=integration ./...` |
| Fuzz | Decoder and schema derivation | `go test -fuzz Fuzz_Decoder ./internal/protocol` |
| Conformance | Protocol compliance verification | Part of unit test suite |
| Architecture | Dependency direction enforcement | `architecture_test.go` |
| Self-test | CLAUDE.md claims verified against code | `claudemd_test.go` |
| Benchmark | Performance regression detection (20% threshold) | `make bench` |
| Goroutine leak detection | Verifies no leaked goroutines after shutdown | Integration tests |

## Security Measures

- **Zero external dependencies** — eliminates supply chain risk
- **GitHub Actions pinned to full SHAs** — prevents tag-based supply chain attacks
- **Cosign keyless signing** of release checksums via Sigstore
- **SBOM generation** via Syft for every release
- **OpenSSF Scorecard** for continuous supply chain assessment
- **CodeQL** for Go static analysis
- **Per-message size limits** prevent memory exhaustion attacks
- **Input validation** for tool parameters (path traversal, null bytes, length)
- **Panic recovery** in tool handlers — no stack traces exposed in responses
- **OSS-Fuzz** with address sanitizer for continuous fuzzing
