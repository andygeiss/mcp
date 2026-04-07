# Development Guide

**Project:** mcp
**Generated:** 2026-04-07

## Prerequisites

- **Go 1.26+** (see `go.mod` for exact version)
- **golangci-lint** installed ([install guide](https://golangci-lint.run/welcome/install/))
- No external dependencies required

## Getting Started

```bash
git clone https://github.com/andygeiss/mcp.git
cd mcp
make check    # build + test + lint (full quality pipeline)
```

## Build Commands

| Command | Purpose |
|---|---|
| `make build` | Compile all packages |
| `make test` | Run all tests with race detector |
| `make lint` | Run golangci-lint (zero issues required) |
| `make check` | Build + test + lint (recommended) |
| `make coverage` | Generate test coverage report |
| `make fuzz` | Fuzz the protocol decoder (30s default) |
| `make fuzz FUZZTIME=5m` | Fuzz with custom duration |
| `make init MODULE=github.com/org/repo` | Initialize as new project from template |

## Build with Version

```bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/
```

## Run Locally

```bash
# Direct execution
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}' | ./mcp

# Via go run
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}' | go run ./cmd/mcp/

# With protocol tracing
echo '...' | MCP_TRACE=1 go run ./cmd/mcp/
```

## Use as MCP Server in Claude Code

The `.mcp.json` file configures this server for Claude Code:
```json
{
  "mcpServers": {
    "mcp": {
      "command": "go",
      "args": ["run", "./cmd/mcp/"]
    }
  }
}
```

## Testing

### Philosophy
- **TDD**: Write a failing test first, then implement
- **Race detector mandatory**: Always `go test -race`
- **Black-box by default**: `package foo_test`; white-box (`_internal_test.go`) only for unexported internals

### Naming Convention
```
Test_<Unit>_With_<Condition>_Should_<Outcome>
```

### Test Structure
```go
func Test_Something_With_Condition_Should_Outcome(t *testing.T) {
    t.Parallel()
    // Arrange
    ...
    // Act
    ...
    // Assert
    assert.That(t, "description", got, expected)
}
```

### Assertion Helper
Use `assert.That(t, "description", got, expected)` from `internal/pkg/assert`. Generic deep-equal comparison using `reflect.DeepEqual`.

### I/O Testing Pattern
Inject `bytes.Buffer` for stdin/stdout/stderr. Write JSON-RPC requests + trigger EOF, run server, read responses from output buffer. Byte-exact golden comparisons for protocol correctness.

### Test Commands
```bash
go test -race ./...                                            # Unit tests
go test -race ./... -tags=integration                          # Include integration tests
go test -fuzz Fuzz_Decoder ./internal/protocol -fuzztime=30s   # Fuzz the decoder
```

### Test Types by Package

| Package | Test Types |
|---|---|
| `protocol/` | Codec unit tests, benchmarks, fuzz targets |
| `server/` | Unit, integration, conformance, architecture, fuzz, I/O, stdout, sync |
| `tools/` | Unit, benchmarks, schema derivation, search (including Unix-specific) |
| `cmd/mcp/` | Integration tests, signal handling |
| `cmd/init/` | Unit, integration, template consumer verification |

## Linting

Strict golangci-lint configuration (`.golangci.yml`) with 50+ linters:
- **No external dependencies**: `depguard` denies `encoding/json/v2`, `log` (must use `log/slog`)
- **No stdout writes**: `forbidigo` blocks `fmt.Print*` (stdout is protocol-only)
- **Complexity limits**: `cyclop` max 15, `gocognit` min 20, `funlen` max 80 lines / 50 statements
- **JSON tags**: `tagliatelle` enforces camelCase
- **Structured logging**: `sloglint` enforces `snake_case` keys, static messages
- **Parallel tests**: `paralleltest` + `tparallel` enforce `t.Parallel()`
- **Test packages**: `testpackage` enforces black-box (`_test`) packages

Exclusions:
- `cmd/` exempt from `forbidigo` (entry points may write to stdout for `--version`)
- `_test.go` exempt from `funlen`, `gocognit`, `cyclop`
- `_internal_test.go` exempt from `testpackage`

## Adding a New Tool

1. Create `internal/tools/<toolname>.go` with input struct and handler:
```go
type GreetInput struct {
    Name string `json:"name" description:"Name to greet"`
}

func Greet(_ context.Context, input GreetInput) Result {
    return TextResult("Hello, " + input.Name + "!")
}
```

2. Register in `cmd/mcp/main.go`:
```go
tools.Register(registry, "greet", "Greets someone by name", tools.Greet)
```

3. Input schema is derived automatically from struct tags via reflection. Fields without `omitempty` are marked required.

4. Write unit test for the handler in isolation. Write integration test through the full server (`//go:build integration`).

## Coding Conventions

- **Constants**: Protocol constants in `protocol/constants.go`. Use `const`, never `var`.
- **Ordering**: Declarations alphabetically. `NewTypeName` constructor first after its type, then methods alphabetically. `case` clauses alphabetically.
- **JSON tags**: Every exported protocol field gets `json:"fieldName"` matching MCP spec camelCase. `omitempty` for optional fields.
- **Error handling**: `fmt.Errorf("operation: %w", err)`. Map to JSON-RPC error codes at the protocol boundary.
- **Imports**: stdlib first, blank line, then internal packages.
- **Logging**: `slog.LevelInfo` default. `snake_case` keys. Error for unrecoverable, Warn for recoverable, Info for lifecycle only.

## Environment Variables

| Variable | Purpose | Default |
|---|---|---|
| `MCP_TRACE` | Enable protocol trace logging (`1` to enable) | disabled |

## Pull Request Process

1. Branch from `main`
2. CI must pass: build, test, fuzz, lint
3. One approval required
4. No force-push to `main`
5. Use PR template checklist

## Detailed Testing Guide

### Test Coverage by Package

#### `internal/protocol/` -- Codec Tests
- **Decode tests**: Valid requests, string/number/null IDs, absent params, null params, batch arrays, notifications
- **encoding/json v1 behavior tests**: Case-insensitive field matching, duplicate keys (last wins), invalid UTF-8 passthrough
- **Validate tests**: Wrong version, empty method, array params, valid IDs (string, number, null, negative, zero, empty string), invalid IDs (boolean, array, object)
- **Encode tests**: Success responses, error responses, null ID error responses
- **Golden tests**: Byte-exact JSON comparison for response format correctness
- **Round-trip tests**: Decode request, build response, encode, verify valid JSON
- **CodeError tests**: `errors.AsType[*protocol.CodeError]` unwrapping, constructor functions, Error.Data round-trip
- **Benchmarks**: Decode/encode throughput measurement

#### `internal/server/` -- Server Tests
- **Handshake tests**: Initialize response contains capabilities, protocol version, server info, concurrency config
- **State machine tests**: Methods rejected in uninitialized state, duplicate initialize rejected, initializing state behavior
- **Dispatch tests**: Ping in all states, tools/list after ready, tools/call with echo/search, unknown methods, reserved `rpc.*` methods
- **Unsupported capability tests**: `completion/`, `elicitation/`, `prompts/`, `resources/` methods return -32601 with guidance data
- **Error handling tests**: Invalid JSON, batch arrays, invalid params, missing tool name, unknown tool
- **Notification tests**: `notifications/initialized` transitions state, `notifications/cancelled` cancels in-flight, unknown notifications silently ignored
- **Cancellation tests**: Cancel matching request, cancel non-matching request (no-op), cancel with no in-flight
- **Size limit tests**: Message > 4MB rejected with -32700
- **Result truncation tests**: Result > 1MB truncated with warning
- **Timeout tests**: Handler exceeding timeout returns -32603 with timing diagnostics
- **Panic recovery tests**: Handler panic caught and returned as -32603 with panicDiag (no stack trace on wire)
- **In-flight rejection tests**: Second request while handler running returns -32600
- **EOF handling tests**: Clean shutdown on EOF, unexpected EOF
- **Trace mode tests**: MCP_TRACE=1 logs requests and responses to stderr
- **Architecture tests**: Import graph verified against documented dependency rules using `go/parser`
- **CLAUDE.md self-tests**: Verify CLAUDE.md constants match code
- **Conformance suite**: File-driven tests with `.request.jsonl`/`.response.jsonl` golden pairs
- **Fuzz tests**: Server-level fuzzing
- **Synchronization tests**: `testing/synctest` for time-dependent behavior

#### `internal/tools/` -- Tool Tests
- **Echo tests**: Simple message passthrough
- **Search tests**: Pattern matching, case sensitivity, extension filtering, max results, depth limit
- **Search security tests**: Path traversal rejection, null byte rejection, symlink protection (O_NOFOLLOW on Unix), working directory containment
- **Schema derivation tests**: Single field, multiple fields, `omitempty` handling, slices (array type), maps (object with additionalProperties), nested structs, pointers (unwrap), nested slices (array of arrays), `json:"-"` exclusion, bare comma tags, unsupported types (channels panic), non-string map keys (panic)
- **Registry tests**: Registration, lookup, alphabetical ordering, duplicate name detection (panic), deterministic tools/list
- **Annotations tests**: WithAnnotations option attaches annotations to tool
- **Benchmarks**: Registry operations, schema derivation

#### `cmd/mcp/` -- Integration Tests
- **Full lifecycle tests**: Initialize -> tools/list -> tools/call -> EOF
- **Signal handling tests**: SIGINT/SIGTERM trigger graceful shutdown

#### `cmd/init/` -- Template Rewriter Tests
- **Rewrite tests**: go.mod rewrite, import path rewrite, text file rewrite, binary dir rename
- **Integration tests**: Full rewrite in temp dir, `go build` after rewrite
- **Template consumer tests**: Verify end-to-end consumer workflow
- **Validation tests**: Invalid module paths, path extension attacks, zero fingerprint verification

### Conformance Test Suite

File-driven protocol conformance tests in `internal/server/testdata/conformance/`:

```
testdata/conformance/
├── 01-initialize.request.jsonl     # Request sequence
├── 01-initialize.response.jsonl    # Expected responses (byte-exact golden)
├── 02-ping.request.jsonl
├── 02-ping.response.jsonl
└── ...
```

The test runner (`conformance_test.go`) discovers all `.request.jsonl` files, feeds them to the server, and compares output against `.response.jsonl` golden files using compacted JSON byte comparison.

### Fuzz Testing Strategy

Three layers of fuzz testing:

| Layer | Target | Location | Runner |
|---|---|---|---|
| **Protocol decoder** | `Fuzz_Decoder_With_ArbitraryInput` | `internal/protocol/fuzz_test.go` | CI (30s), Nightly (5m), OSS-Fuzz (continuous) |
| **Server** | Server-level fuzz targets | `internal/server/fuzz_test.go` | CI + Nightly |
| **OSS-Fuzz** | `Fuzz_Decoder_With_ArbitraryInput` | `oss-fuzz/build.sh` | Google infrastructure (continuous) |

The protocol decoder fuzz target has a rich seed corpus covering:
- Valid requests, notifications, batch arrays
- Edge cases: empty input, null, truncated JSON, numbers, booleans, strings
- Unicode methods, extra fields, large/negative/float IDs
- Whitespace variations, deeply nested params
- Empty method, very long method names

## Schema Derivation Deep Dive

The reflection-based schema system (`internal/tools/schema.go`) automatically builds JSON Schema from Go struct types:

### Type Mapping

| Go Type | JSON Schema Type | Notes |
|---|---|---|
| `string` | `string` | — |
| `bool` | `boolean` | — |
| `int`, `int8`..`int64`, `uint`..`uint64` | `integer` | All integer variants |
| `float32`, `float64` | `number` | — |
| `[]T` | `array` with `items` | Recursive; `[][]string` produces nested arrays |
| `map[string]T` | `object` with `additionalProperties` | Non-string keys panic |
| `*T` | (unwrapped to T) | Pointer is transparent |
| `struct` | `object` with `properties` | Recursive nested objects |
| `chan`, `func`, etc. | **PANIC** | Unsupported types cause panic with field name |

### Tag Processing
- `json:"name"` -- field name in schema
- `json:"name,omitempty"` -- field is optional (not in `required` array)
- `json:"-"` -- field excluded from schema
- `json:",omitempty"` (bare comma) -- excluded (empty name)
- `description:"text"` -- custom struct tag mapped to schema `description`

### Required Fields
Fields are marked required unless their `json` tag contains `omitempty`. The `required` array is sorted alphabetically for deterministic output.

## CI/CD Pipeline Details

### CI Workflow (`.github/workflows/ci.yml`)

```
Trigger: push to main, pull requests
├── build job
│   └── make build
├── test job
│   └── make coverage (race detector + coverage profile)
├── fuzz job
│   └── make fuzz (30s per target)
└── lint job
    └── golangci-lint run ./... (via golangci-lint-action)

All jobs run in parallel on ubuntu-latest.
Go version: read from go.mod (go-version-file).
Actions: pinned by SHA hash.
```

### Nightly Fuzz (`.github/workflows/fuzz.yml`)

```
Trigger: daily at 03:00 UTC, manual with custom duration
Steps:
  1. grep -r 'func Fuzz_' to discover all fuzz targets
  2. For each target: go test -fuzz $func $dir -fuzztime=5m
Default duration: 5m per target (override via workflow_dispatch input)
```

### Release Workflow (`.github/workflows/release.yml`)

```
Trigger: push tag v*.*.*
Steps:
  1. Checkout with full history (fetch-depth: 0)
  2. Install cosign (keyless signing via Sigstore)
  3. Install syft (SBOM generation)
  4. GoReleaser: build, archive, checksum, SBOM, sign, publish

Artifacts per release:
  - mcp_linux_amd64.tar.gz     (binary + LICENSE + README)
  - mcp_linux_arm64.tar.gz
  - mcp_darwin_amd64.tar.gz
  - mcp_darwin_arm64.tar.gz
  - checksums.txt               (SHA-256)
  - checksums.txt.sig            (cosign signature)
  - *.sbom.json                  (Syft SBOM for each archive)
```

### Security Workflows

| Workflow | Schedule | Purpose |
|---|---|---|
| **CodeQL** | Weekly Mon 06:00 UTC + every PR | Static analysis for Go security issues |
| **Scorecard** | Weekly Mon 06:00 UTC + push to main | OpenSSF supply chain security assessment |
| **Dependabot** | Weekly | GitHub Actions + Go module update PRs |

### Supply Chain Security Measures

1. **Pinned actions**: All GitHub Actions referenced by full SHA hash (not tags)
2. **Minimal permissions**: `permissions: read-all` default, write only where needed
3. **Cosign signing**: Keyless signing of release checksums via Sigstore OIDC
4. **SBOM generation**: Every release archive gets a Syft-generated SBOM
5. **OpenSSF Scorecard**: Continuous monitoring with badge on README
6. **CodeQL**: Weekly + PR static analysis
7. **Zero external dependencies**: No third-party Go modules to compromise
