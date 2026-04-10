# Architecture

## Executive Summary

A minimal, zero-dependency Go implementation of the Model Context Protocol (MCP) communicating over stdin/stdout using JSON-RPC 2.0. The project is both a working MCP server and a template for scaffolding custom MCP servers. It prioritizes correctness, simplicity, and security over feature breadth.

## Technology Stack

| Category | Technology | Version | Justification |
|---|---|---|---|
| Language | Go | 1.26 | Green Tea GC, `reflect.Type.Fields` iterators, `signal.NotifyContext` cancel cause, `errors.AsType[T]` |
| Transport | stdin/stdout | — | Simplest possible transport; no HTTP, no WebSocket |
| Protocol | JSON-RPC 2.0 | — | MCP specification requirement |
| Codec | `encoding/json` v1 | — | Stable stdlib; v2 (`GOEXPERIMENT=jsonv2`) explicitly unsupported via depguard |
| Logging | `log/slog` | — | Structured JSON logging to stderr via `slog.JSONHandler`; `snake_case` keys enforced by sloglint |
| Testing | `testing` stdlib | — | Fuzz, benchmarks, race detector, parallel execution, `testing/synctest` for virtual time |
| Linting | golangci-lint v2 | — | 45+ linters enabled, zero suppression policy |
| Release | GoReleaser | — | Cross-platform builds (darwin/linux, amd64/arm64), cosign signing, SBOM generation |
| Fuzzing | OSS-Fuzz | — | Continuous fuzzing with libFuzzer + address sanitizer |

## Architecture Pattern

Flat and simple — no hexagonal layers, no bounded contexts. The architecture follows a direct dependency chain:

```
cmd/mcp/ ──→ internal/server/ ──→ internal/protocol/
                    │
                    └──→ internal/tools/ ──→ internal/protocol/

internal/assert/    (test-only, zero internal deps)
internal/protocol/  (zero internal deps — foundation layer)
```

- `protocol` has zero dependencies on other internal packages
- `tools` may import `protocol` but never `server`
- `assert` is test-only
- These constraints are enforced by `architecture_test.go` which inspects import graphs at test time

## Core Components

### Protocol Layer (`internal/protocol/`)

The wire format layer. Handles JSON-RPC 2.0 encoding, decoding, validation, and error construction. Zero dependencies on other internal packages — this is the foundation layer.

**Files:** `codec.go`, `constants.go`, `message.go`

#### Types

```go
// Request represents a JSON-RPC 2.0 request or notification.
type Request struct {
    ID      json.RawMessage `json:"id,omitempty"`
    JSONRPC string          `json:"jsonrpc"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
    Error   *Error          `json:"error,omitempty"`
    ID      json.RawMessage `json:"id,omitempty"`
    JSONRPC string          `json:"jsonrpc"`
    Result  json.RawMessage `json:"result,omitempty"`
}

// Error is the wire-format error object.
type Error struct {
    Code    int             `json:"code"`
    Data    json.RawMessage `json:"data,omitempty"`
    Message string          `json:"message"`
}

// CodeError carries a JSON-RPC error code from creation to the protocol boundary.
// Implements the error interface. Unwrapped via errors.AsType[*CodeError] at dispatch.
type CodeError struct {
    Code    int
    Data    json.RawMessage
    Message string
}
```

#### Functions

| Function | Signature | Purpose |
|---|---|---|
| `Decode` | `(dec *json.Decoder) (Request, error)` | Reads one message, detects batch arrays (→ `-32700`), normalizes null/absent params to `{}` |
| `Validate` | `(req Request) *CodeError` | Checks version is `"2.0"`, method non-empty, params is object, ID is string/integer/null (no fractional/exponent) |
| `Encode` | `(enc *json.Encoder, resp Response) error` | Writes one response, sets JSONRPC to `"2.0"` |
| `NewErrorResponse` | `(id json.RawMessage, code int, message string) Response` | Creates error response |
| `NewErrorResponseFromCodeError` | `(id json.RawMessage, ce *CodeError) Response` | Creates error response from CodeError, propagates Data field |
| `NewResultResponse` | `(id json.RawMessage, result any) (Response, error)` | Creates success response by marshaling result to JSON |
| `NullID` | `() json.RawMessage` | Returns fresh `null` bytes (copy prevents mutation) |

#### Error Constructors

| Constructor | Code | Usage |
|---|---|---|
| `ErrParseError(msg)` | `-32700` | Malformed JSON, size limit exceeded, batch array |
| `ErrInvalidRequest(msg)` | `-32600` | Bad structure, wrong jsonrpc version, non-object params |
| `ErrMethodNotFound(msg)` | `-32601` | Unknown method, reserved `rpc.*`, unsupported capabilities |
| `ErrInvalidParams(msg)` | `-32602` | Wrong types, missing required fields, unknown tool |
| `ErrInternalError(msg)` | `-32603` | Should not happen in normal operation |
| `ErrServerError(msg)` | `-32000` | Not initialized, already initialized, server busy |
| `ErrServerTimeout(msg)` | `-32001` | Tool handler timed out or was cancelled |

#### Constants

```go
// MCP protocol version
const MCPVersion = "2025-06-18"

// JSON-RPC methods
const (
    MethodInitialize        = "initialize"
    MethodPing              = "ping"
    MethodToolsCall         = "tools/call"
    MethodToolsList         = "tools/list"
    NotificationCancelled   = "notifications/cancelled"
    NotificationInitialized = "notifications/initialized"
)

// Namespace prefixes for unsupported capability detection
const (
    NamespaceCompletion  = "completion/"
    NamespaceElicitation = "elicitation/"
    NamespacePrompts     = "prompts/"
    NamespaceResources   = "resources/"
    PrefixRPC            = "rpc."
)

// Sequential dispatch constraint
const MaxConcurrentRequests = 1
```

### Server Layer (`internal/server/`)

The core server implementing the MCP lifecycle, message dispatch, and tool execution with resilience features.

**Files:** `server.go` (main server), `counting_reader.go` (size limit enforcement)

#### Server Construction

```go
func NewServer(name, version string, registry *tools.Registry,
    stdin io.Reader, stdout, stderr io.Writer, opts ...Option) *Server

// Options
func WithHandlerTimeout(d time.Duration) Option  // default: 30s
func WithTrace(enabled bool) Option               // logs all messages to stderr
func WithSafetyMargin(d time.Duration) Option     // default: 1s after timeout
```

The server accepts `io.Reader`/`io.Writer` (not `*os.File`) so tests inject buffers.

#### Initialization State Machine

```
uninitialized ──[initialize]──→ initializing ──[notifications/initialized]──→ ready
```

| State | Allowed | Rejected with |
|---|---|---|
| **Uninitialized** | `initialize`, `ping` | `-32000` ("server not initialized") |
| **Initializing** | `ping` | `-32000` ("awaiting notifications/initialized") |
| **Ready** | All methods | Duplicate `initialize` → `-32000` ("already initialized") |

- `notifications/initialized` in wrong state → silently ignored
- Unknown notifications → silently ignored (never respond, never log)

#### Dispatch Model

The `Run(ctx context.Context) error` method implements the main dispatch loop:

1. **Decode** — Read one JSON message from stdin via persistent `json.NewDecoder`
2. **Validate** — Check structural validity (version, method, params, ID)
3. **State check** — Verify the request is allowed in current lifecycle state
4. **Route** — Dispatch to appropriate handler based on method

**Async tool dispatch:**
- `tools/call` requests are spawned as background goroutines
- The decode loop continues reading for `notifications/cancelled` and `ping` during execution
- Only one tool runs at a time (`maxInFlight: 1` advertised via `experimental.concurrency`)
- Additional requests while a handler is in flight → `-32000` ("server busy: request in flight")

**Timeout enforcement:**
- Tool handlers wrapped with `context.WithTimeout(handlerTimeout + safetyMargin)`
- If handler exceeds deadline: `-32001` error with timing diagnostics (`elapsedMs`, `timeoutMs`, `toolName`)
- If handler ignores cancellation beyond safety margin: goroutine abandoned

**Panic recovery:**
- Panic in tool handler → recovered, logged to stderr with stack info
- Response: `-32603` with diagnostic data (`panicValue`, `toolName`)
- Panic details never sent to client in error message

**Error sanitization:**
- `CodeError` types → returned as-is with code and message
- Non-`CodeError` errors → sanitized to `-32603` ("internal error"), details logged to stderr only
- This prevents leaking internal implementation details to clients

#### Counting Reader (`counting_reader.go`)

Per-message size enforcement to prevent memory exhaustion:

```go
type countingReader struct {
    count  int64
    limit  int64     // 4 MB (maxMessageSize)
    reader io.Reader
}
```

- **Reset** before each `json.Decoder.Decode()` call — not cumulative
- **Exceeded()** reports whether the limit was breached
- Returns `errMessageTooLarge` when count exceeds limit
- The decoder's internal buffer can read ahead, so effective enforcement is 4 MB + buffer size

#### Size Limits

| Limit | Value | Enforcement |
|---|---|---|
| Per-message | 4 MB | `countingReader` wrapping stdin |
| Per-result | 1 MB | Result marshaled, length checked, truncated if exceeded |

Truncated results include the message: `"[result truncated: exceeded maximum size of X bytes]"`

#### Transport Rules

| Stream | Usage | Constraints |
|---|---|---|
| **stdin** | Persistent `json.NewDecoder` | No `bufio.Scanner`; handles slow/partial reads |
| **stdout** | Protocol-only | Every byte must be valid JSON-RPC; HTML escaping disabled on encoder |
| **stderr** | `slog.JSONHandler` exclusively | `snake_case` keys, structured fields |

#### Shutdown Behavior

| Trigger | Exit Code | Behavior |
|---|---|---|
| `io.EOF` / `io.ErrUnexpectedEOF` | 0 | Clean shutdown, logs reason |
| `SIGINT` / `SIGTERM` | 0 | Context cancelled, graceful shutdown |
| Decode error (non-EOF) | 1 | Fatal, logs error |
| `errMessageTooLarge` | 1 | Fatal, message size exceeded |

Startup/shutdown logs include structured fields: `name`, `protocol_version`, `tools`, `version`, `uptime_ms`, `errors`, `requests`, `reason`.

#### Cancellation Support

- `notifications/cancelled` cancels the in-flight handler if the request ID matches
- Cancelled requests suppress the response entirely (per MCP spec)
- Non-matching or stale cancellations → silently ignored
- Cancellation timing diagnostics: `elapsedMs`, `toolName`

#### Unsupported Capabilities

Methods in these namespaces return `-32601` with structured guidance data:

| Namespace | Guidance |
|---|---|
| `completion/` | Points to `tools/list` and `tools/call` |
| `elicitation/` | Points to `tools/list` and `tools/call` |
| `prompts/` | Points to `tools/list` and `tools/call` |
| `resources/` | Points to `tools/list` and `tools/call` |

The guidance includes `hint` (human-readable) and `supportedCapabilities` (list of available method prefixes).

### Tools Layer (`internal/tools/`)

The tool registry and automatic schema derivation system.

**Files:** `registry.go`, `schema.go`, `validate.go`, `echo.go`, `annotations.go`

#### Registry

```go
type Registry struct { /* index map + sorted tools slice */ }

func NewRegistry() *Registry
func (r *Registry) Lookup(name string) (Tool, bool)  // O(1) via index map
func (r *Registry) Names() []string                   // alphabetical order
func (r *Registry) Tools() []Tool                     // copy, alphabetical order

func Register[T any](r *Registry, name, description string,
    handler func(ctx context.Context, input T) Result, opts ...Option) error
```

- Generic registration with type-safe handler wrapping
- Schema derived automatically from `T` via reflection
- Handler wrapper: unmarshals `json.RawMessage` params → `T`, validates required fields, calls typed handler
- Duplicate names return an error at registration time (fail fast)
- Tools maintained in alphabetical order for deterministic `tools/list` responses

#### Types

```go
type Tool struct {
    Annotations *Annotations `json:"annotations,omitempty"`
    Description string       `json:"description"`
    Handler     toolHandler  // unexported
    InputSchema InputSchema  `json:"inputSchema"`
    Name        string       `json:"name"`
}

type Result struct {
    Content []ContentBlock `json:"content"`
    IsError bool           `json:"isError,omitempty"`
}

type ContentBlock struct {
    Text string `json:"text"`
    Type string `json:"type"`
}

type InputSchema struct {
    Properties map[string]Property `json:"properties"`
    Required   []string            `json:"required"`
    Type       string              `json:"type"`
}

type Property struct {
    AdditionalProperties *Property           `json:"additionalProperties,omitempty"`
    Description          string              `json:"description,omitempty"`
    Items                *Property           `json:"items,omitempty"`
    Properties           map[string]Property `json:"properties,omitempty"`
    Required             []string            `json:"required,omitempty"`
    Type                 string              `json:"type"`
}
```

#### Schema Derivation (`schema.go`)

Reflection-based JSON Schema generation from Go struct types:

| Go Type | JSON Schema Type | Notes |
|---|---|---|
| `string` | `"string"` | |
| `int`, `int8`...`int64`, `uint`...`uint64` | `"integer"` | |
| `float32`, `float64` | `"number"` | |
| `bool` | `"boolean"` | |
| `[]T` | `"array"` with `items` | Nested arrays supported |
| `map[string]V` | `"object"` with `additionalProperties` | Non-string keys → error |
| `struct` | `"object"` with `properties` | Recursive, depth limit: 10 |
| `*T` | Unwrapped to `T` | |
| `chan`, `func` | Error | Named in error message |

**Struct field handling:**
- `json:"name"` tag → property name (camelCase per MCP spec)
- `json:"-"` → excluded from schema
- `json:",omitempty"` → not added to `required` array
- `description:"text"` tag → property description
- Required fields: those without `omitempty` in json tag
- Anonymous embedded structs without json tags → fields promoted to parent (like Go embedding)
- Tagged embedded structs → nested as object property
- Shadowed fields: parent field wins over embedded field
- Unexported embedded structs → not promoted

#### Tool Annotations

```go
type Annotations struct {
    DestructiveHint bool   `json:"destructiveHint,omitempty"`
    IdempotentHint  bool   `json:"idempotentHint,omitempty"`
    OpenWorldHint   bool   `json:"openWorldHint,omitempty"`
    ReadOnlyHint    bool   `json:"readOnlyHint,omitempty"`
    Title           string `json:"title,omitempty"`
}

type Option func(*Tool)

func WithAnnotations(a Annotations) Option
```

- Annotations provide behavioral hints to MCP clients for pre-invocation decisions
- `nil` Annotations pointer → omitted from JSON (no empty object)
- Applied via functional option pattern during `Register[T]`

#### Input Validation (`validate.go`)

```go
const MaxInputLength = 4096

func ValidatePath(path string) error   // traversal, null bytes, length
func ValidateInput(input string) error // null bytes, length
```

**Path validation:**
1. Check for null bytes (`\x00`) → reject
2. Check length > `MaxInputLength` → reject
3. `filepath.Clean(path)` to normalize
4. Check if cleaned path starts with `..` → reject (traversal)
5. Literal `..` in filenames (e.g., `foo..bar`) → allowed

**Internal validation (`unmarshalAndValidate`):**
- `json.Decoder` with `DisallowUnknownFields()` for strict parsing
- Lightweight key-presence scan via `json.Unmarshal` to `map[string]json.RawMessage` for required field validation
- Error wrapping: `"invalid arguments: <decode error>"`

#### Result Constructors

```go
func TextResult(text string) Result   // Content: [{Type: "text", Text: text}], IsError: false
func ErrorResult(text string) Result  // Content: [{Type: "text", Text: text}], IsError: true
```

#### Echo Tool (`echo.go`) — Reference Implementation

```go
type EchoInput struct {
    Message string `json:"message" description:"The message to echo back"`
}

func Echo(_ context.Context, input EchoInput) Result {
    return TextResult(input.Message)
}
```

Registered in `cmd/mcp/main.go` as the default tool. Demonstrates the minimal pattern for tool implementation.

### Entry Point (`cmd/mcp/main.go`)

Wiring only — no business logic:

1. Parse `--version` flag → print version and exit
2. Set up signal handling: `signal.NotifyContext` for `SIGINT`/`SIGTERM`
3. Create tool registry, register echo tool
4. Read `MCP_TRACE` environment variable for trace mode
5. Create server with `os.Stdin`, `os.Stdout`, `os.Stderr`
6. Call `server.Run(ctx)` → exit 0 on nil, exit 1 on error

Version embedded via ldflags: `-X main.version=$(git describe --tags --always --dirty)`

### Template Rewriter (`cmd/init/`)

One-time utility for template consumers. Performs:

1. Validate new module path (must contain `/`, must not extend template path)
2. Derive project name from last path segment
3. Rewrite `go.mod` module line
4. Walk all `.go` files and rewrite import paths (`bytes.ReplaceAll`, handles aliased imports)
5. Walk text files (`.md`, `.yml`, `.json`, etc.) and replace module path + binary name references
6. Rename `cmd/mcp/` to `cmd/<projectName>/`
7. Run `go mod tidy`
8. Remove build artifacts
9. Verify zero template fingerprint (no remaining `github.com/andygeiss/mcp` references)
10. Self-delete `cmd/init/` directory

**Idempotent:** Running twice with the same path produces identical results (verified by test).

## Testing Strategy

### Test Levels

| Level | Count | Description | Command |
|---|---|---|---|
| Unit | 170+ | Black-box tests for all packages | `go test -race ./...` |
| Integration | 8+ | Full pipeline through compiled server | `go test -race -tags=integration ./...` |
| Conformance | 33 | Data-driven protocol compliance | Part of integration suite |
| Fuzz | 4 targets | Decoder, server pipeline, path/input validation | `make fuzz` |
| Benchmark | 11 | Performance regression detection | `make bench` |
| Architecture | 2 | Import graph + CLAUDE.md claims | Part of unit suite |
| Examples | 3 | Testable documentation | Part of unit suite |

### Conformance Test Scenarios

33 data-driven scenarios in `internal/server/testdata/conformance/`:

| Category | Scenarios |
|---|---|
| **ID handling** | `id-string`, `id-zero`, `id-negative`, `id-large-number`, `id-null`, `id-empty-string`, `id-boolean-invalid`, `id-array-invalid`, `id-object-invalid` |
| **State machine** | `initialize-handshake`, `state-duplicate-initialize`, `state-uninitialized-method`, `state-request-during-initializing`, `state-ping-all-states` |
| **Validation** | `jsonrpc-version-missing`, `jsonrpc-version-wrong`, `method-empty`, `method-unknown`, `method-rpc-reserved`, `extra-fields` |
| **Params** | `params-absent`, `params-null`, `params-array-positional` |
| **Notifications** | `notification-invalid-silently-dropped`, `notification-then-request` |
| **Tools** | `tools-list`, `tools-call-success`, `tools-call-unknown-tool`, `tools-call-empty-name` |
| **Batching** | `batch-array-rejection` |
| **Capabilities** | `unsupported-capability-prompts`, `unsupported-capability-resources` |
| **Ping** | `ping` |

Each scenario has a `.request.jsonl` file (input sequence) and optionally a `.response.jsonl` file (expected output for byte-exact comparison).

### Fuzz Targets

| Target | Package | Seeds | What It Tests |
|---|---|---|---|
| `Fuzz_Decoder_With_ArbitraryInput` | `protocol` | 23 | Decoder robustness against arbitrary input |
| `Fuzz_Server_Pipeline` | `server` | 10 | Full server pipeline — no panics, valid JSON-RPC output |
| `Fuzz_ValidatePath_With_ArbitraryInput` | `tools` | 8 | Path validator robustness |
| `Fuzz_ValidateInput_With_ArbitraryInput` | `tools` | 7 | Input validator robustness |

### Benchmarks

| Benchmark | Package | What It Measures |
|---|---|---|
| `Benchmark_Decode_SingleRequest` | `protocol` | Single tools/call decode |
| `Benchmark_Decode_InitializeRequest` | `protocol` | Initialize request decode |
| `Benchmark_Decode_LargeParams` | `protocol` | 1024-byte payload decode |
| `Benchmark_Decode_Notification` | `protocol` | Notification (no ID) decode |
| `Benchmark_Encode_SuccessResponse` | `protocol` | Success response encode |
| `Benchmark_Encode_ErrorResponse` | `protocol` | Error response encode |
| `Benchmark_RequestResponse_EchoTool` | `server` | Single tool call end-to-end |
| `Benchmark_RequestResponse_MultipleToolCalls` | `server` | 10 sequential tool calls |
| `Benchmark_RequestResponse_PingOnly` | `server` | Single ping request |
| `Benchmark_SchemaDerivation_With_SimpleStruct` | `tools` | 1-field schema derivation |
| `Benchmark_SchemaDerivation_With_ComplexStruct` | `tools` | 6-field mixed-type schema derivation |

### Deterministic Concurrency Testing

`synctest_test.go` uses Go 1.23+ `testing/synctest` with virtual time for deterministic testing of:
- Handler timeout behavior (virtual time advances to exact timeout)
- Context cancellation during in-flight handlers
- Concurrent request rejection (`maxInFlight: 1`)

### Self-Testing

`claudemd_test.go` verifies that behavioral claims in `CLAUDE.md` have matching test coverage:
- Each key claim (e.g., "uninitialized returns -32000") is mapped to a test function name
- Error code constants are verified to have test coverage
- Import graph constraints are validated against actual Go imports
- `go.mod` is verified to have zero external dependencies

## Security Measures

- **Zero external dependencies** — eliminates supply chain risk
- **GitHub Actions pinned to full commit SHAs** — prevents tag-based supply chain attacks
- **Cosign keyless signing** of release checksums via Sigstore
- **SBOM generation** via Syft for every release
- **OpenSSF Scorecard** for continuous supply chain assessment
- **CodeQL** for Go static analysis (weekly + on PR/push)
- **govulncheck** for known vulnerability detection in CI
- **Per-message size limits** (4 MB) prevent memory exhaustion attacks
- **Per-result size limits** (1 MB) prevent oversized responses
- **Input validation** for tool parameters — path traversal, null bytes, length limits
- **Panic recovery** in tool handlers — diagnostics logged, never sent to client
- **Error sanitization** — non-CodeError messages replaced with generic "internal error"
- **OSS-Fuzz** with libFuzzer and address sanitizer for continuous fuzzing
- **depguard rules** — `encoding/json/v2` and `log` (old) explicitly denied
- **forbidigo rules** — `fmt.Print*` and `print`/`println` forbidden (stdout is protocol-only)

## Linter Configuration

45+ linters enabled in `.golangci.yml` v2 with notable rules:

| Linter | Configuration |
|---|---|
| `cyclop` | Max complexity: 12 |
| `depguard` | Deny `encoding/json/v2`, `encoding/json/jsontext`, `log` |
| `forbidigo` | Deny `fmt.Print*`, `print`, `println` (stdout is protocol-only) |
| `funlen` | Max 80 lines, 50 statements |
| `gocognit` | Min complexity: 15 |
| `goconst` | Min length: 3, min occurrences: 3 |
| `nestif` | Min complexity: 4 |
| `nolintlint` | Require explanation and specific linter name |
| `sloglint` | `snake_case` keys, static messages |
| `tagliatelle` | JSON tags must be camelCase |
| `testpackage` | Tests must use `_test` suffix (black-box) |

**Exclusions:**
- `cmd/` exempt from `forbidigo` (allowed to use fmt.Print for version output)
- `_test.go` exempt from `funlen`, `gocognit`, `cyclop`
- `_internal_test.go` exempt from `testpackage` (white-box tests allowed)
