# Development Guide

## Prerequisites

- **Go 1.26+** (see `go.mod` for exact version)
- **golangci-lint** — [install instructions](https://golangci-lint.run/welcome/install/)
- No external dependencies required — standard library only

## Initial Setup

```bash
git clone https://github.com/andygeiss/mcp.git
cd mcp
make setup    # Configures pre-commit hooks (run once after cloning)
```

The `make setup` command sets `core.hooksPath` to `.githooks/`, enabling the pre-commit hook that runs `make check` before every commit.

## Build Commands

| Command | Description |
|---|---|
| `make build` | Compile all packages (`go build ./...`) |
| `make check` | Full quality pipeline: build + test + lint |
| `make test` | Run all tests with race detector (`go test -race ./...`) |
| `make lint` | Run golangci-lint (must pass with 0 issues) |
| `make fuzz` | Fuzz the protocol decoder (30s default, override with `FUZZTIME=2m`) |
| `make bench` | Run benchmarks (6 iterations) and compare against `testdata/benchmarks/baseline.txt` |
| `make coverage` | Run tests with coverage, enforce 90% threshold |
| `make init` | Initialize template with new module path (`MODULE=github.com/org/repo`) |

### Building with Version

```bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/
```

The `--version` flag prints the embedded version string and exits.

### Running the Server

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}' | ./mcp
```

### Running with Trace Mode

Set `MCP_TRACE=1` to log all incoming requests and outgoing responses to stderr:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}' | MCP_TRACE=1 ./mcp
```

### Using as a Template

Fork/clone, then run:

```bash
go run ./cmd/init github.com/yourorg/yourproject
```

This performs a complete project rebrand:
1. Rewrites `go.mod` with new module path
2. Rewrites all `.go` import paths (handles aliased imports)
3. Rewrites text files (`.md`, `.yml`, `.json`, etc.) for module path and binary name
4. Renames `cmd/mcp/` to `cmd/yourproject/`
5. Runs `go mod tidy`
6. Self-deletes `cmd/init/` directory
7. Removes build artifacts
8. Verifies zero template fingerprint (no remaining `github.com/andygeiss/mcp` references)

The operation is **idempotent** — running twice with the same path produces identical results.

## Testing

### Test Categories

| Category | Command | Build Tag | Count |
|---|---|---|---|
| Unit tests | `go test -race ./...` | none | 170+ |
| Integration tests | `go test -race -tags=integration ./...` | `integration` | 8+ |
| Conformance tests | Part of integration suite | `integration` | 33 scenarios |
| Fuzz tests | `make fuzz` or individual targets | none | 4 targets |
| Benchmarks | `make bench` | none | 11 |
| Architecture tests | Part of unit suite | none | 2 |
| Example tests | Part of unit suite | none | 3 |

### Test Conventions

- **Naming**: `Test_<Unit>_With_<Condition>_Should_<Outcome>`
- **Structure**: `// Arrange` / `// Act` / `// Assert` comments in every test
- **Parallelism**: Every test calls `t.Parallel()`
- **Package**: Black-box (`package foo_test`) by default; white-box (`_internal_test.go`) only for unexported internals
- **Assertions**: `assert.That(t, "description", got, expected)` from `internal/assert` — generic deep-equal comparison
- **I/O testing**: Inject `bytes.Buffer` for stdin/stdout, write JSON-RPC requests + EOF, read responses from output buffer
- **Golden tests**: Byte-for-byte JSON comparison for protocol correctness
- **Fuzz**: `Fuzz_<Unit>_<Aspect>` naming convention
- **Benchmarks**: All use `b.ReportAllocs()` and `b.ResetTimer()` for accurate allocation tracking

### Fuzz Testing

Four fuzz targets cover different layers:

```bash
# Protocol decoder (primary target)
go test -fuzz Fuzz_Decoder_With_ArbitraryInput ./internal/protocol -fuzztime=30s -timeout=0

# Full server pipeline
go test -fuzz Fuzz_Server_Pipeline ./internal/server -fuzztime=30s -timeout=0 -tags=integration

# Path validator
go test -fuzz Fuzz_ValidatePath_With_ArbitraryInput ./internal/tools -fuzztime=30s

# Input validator
go test -fuzz Fuzz_ValidateInput_With_ArbitraryInput ./internal/tools -fuzztime=30s
```

Fuzz corpus is committed to `internal/protocol/testdata/fuzz/`. New crash findings are added automatically by `go test`.

**OSS-Fuzz integration:**
- The `oss-fuzz/` directory contains `Dockerfile`, `build.sh`, and `project.yaml` for Google OSS-Fuzz
- Local Docker testing: `docker build -f oss-fuzz/Dockerfile -t mcp-fuzz-test .`
- Uses libFuzzer engine with address sanitizer

### Conformance Tests

33 data-driven protocol compliance scenarios in `internal/server/testdata/conformance/`:

Each scenario has a `.request.jsonl` file (input sequence) and optionally a `.response.jsonl` file (expected output). The conformance runner discovers and executes all scenarios automatically.

**Categories covered:**
- ID handling (string, number, null, boolean, array, object — valid and invalid)
- State machine transitions (initialize, duplicate init, uninitialized access, initializing state)
- Request validation (version, method, params, extra fields)
- Parameter handling (absent, null, array positional)
- Notifications (invalid, followed by request)
- Tool operations (list, call success, unknown tool, empty name)
- Batch rejection and unsupported capabilities

### Benchmarks

```bash
# Run benchmarks and compare against baseline
make bench

# Update baseline after confirmed improvements
go test -bench=. -count=6 -benchmem ./internal/... > testdata/benchmarks/baseline.txt
```

CI detects regressions exceeding 20% using `benchstat` comparison.

**Benchmark targets:**
- Protocol codec: decode (single, initialize, large params, notification), encode (success, error)
- Server: echo tool call, 10 sequential tool calls, ping only
- Tools: schema derivation (simple struct, complex struct)

### Deterministic Concurrency Testing

`internal/server/synctest_test.go` uses Go 1.23+ `testing/synctest` with virtual time:

- Handler timeout behavior — virtual time advances to exact timeout
- Context cancellation during in-flight handlers
- Concurrent request rejection (`maxInFlight: 1`)
- No flaky timing-dependent assertions

### Architecture Tests

`internal/server/architecture_test.go` enforces dependency direction at test time by inspecting Go import graphs:

- `protocol` imports zero internal packages
- `tools` never imports `server`
- `server` never imports `cmd`
- `assert` imports zero internal packages

### Self-Tests

`internal/server/claudemd_test.go` verifies that behavioral claims in `CLAUDE.md` have matching tests:

- Maps key claims (e.g., "uninitialized returns -32000") to test function names
- Verifies all error code constants have test coverage
- Validates import graph constraints
- Confirms `go.mod` has zero external dependencies

### Coverage

Coverage threshold is enforced at 90%. Run:

```bash
make coverage
```

The command runs `go test -race -coverprofile=coverage.out ./internal/...`, generates a function-level report, and fails if total coverage is below 90%.

Coverage reports are uploaded to [Codecov](https://codecov.io/gh/andygeiss/mcp) via CI on every push (except Windows).

### Goroutine Leak Detection

Integration tests verify no goroutine leaks after server shutdown by comparing goroutine counts before and after, with a tolerance period for settling.

## Environment Variables

| Variable | Description | Default |
|---|---|---|
| `MCP_TRACE` | Set to `"1"` to enable protocol trace logging | Disabled |
| `FUZZTIME` | Override fuzz test duration in Makefile | `30s` |

## Pre-commit Hook

The `.githooks/pre-commit` hook:
1. Checks if `golangci-lint` is installed
2. If yes: runs `make check` (build + test + lint)
3. If no: runs `make build test` with a warning to install golangci-lint

Target: completes within 60 seconds on developer workstations.

## Adding a New Tool

### Step 1: Define the input struct and handler

Create `internal/tools/yourtool.go`:

```go
package tools

import "context"

// YourInput defines the parameters for the your-tool tool.
type YourInput struct {
    Name    string `json:"name"    description:"Name to process"`
    Count   int    `json:"count"   description:"Number of repetitions"`
    Verbose bool   `json:"verbose,omitempty" description:"Enable verbose output"`
}

// YourTool processes the input and returns a result.
func YourTool(_ context.Context, input YourInput) Result {
    return TextResult("processed: " + input.Name)
}
```

**Schema derivation rules:**
- `json:"name"` tag → property name (camelCase)
- `description:"text"` tag → schema description
- Fields without `omitempty` → added to `required` array
- Fields with `omitempty` → optional
- Supported types: `string`, `int*`, `uint*`, `float*`, `bool`, `[]T`, `map[string]V`, nested structs, pointers
- Unsupported types (channels, funcs) → error at registration

### Step 2: Register in main

Add to `cmd/mcp/main.go`:

```go
if err := tools.Register(registry, "your-tool", "Description of what it does", tools.YourTool); err != nil {
    return fmt.Errorf("register your-tool: %w", err)
}
```

Optional annotations:

```go
if err := tools.Register(registry, "your-tool", "Description", tools.YourTool,
    tools.WithAnnotations(tools.Annotations{
        ReadOnlyHint: true,
        Title:        "Your Tool",
    }),
); err != nil {
    return fmt.Errorf("register your-tool: %w", err)
}
```

### Step 3: Write tests

Create `internal/tools/yourtool_test.go`:

```go
package tools_test

import (
    "context"
    "testing"

    "github.com/andygeiss/mcp/internal/assert"
    "github.com/andygeiss/mcp/internal/tools"
)

func Test_YourTool_With_ValidInput_Should_ReturnResult(t *testing.T) {
    t.Parallel()
    // Arrange
    input := tools.YourInput{Name: "test", Count: 1}
    // Act
    result := tools.YourTool(context.Background(), input)
    // Assert
    assert.That(t, "text", result.Content[0].Text, "processed: test")
    assert.That(t, "isError", result.IsError, false)
}
```

### Step 4: Verify

```bash
make check   # build + test + lint — must pass with zero issues
```

### Input Validation

For tools that accept file paths or user-provided strings, use the validation helpers:

```go
func YourTool(_ context.Context, input YourInput) Result {
    if err := ValidatePath(input.Path); err != nil {
        return ErrorResult(err.Error())
    }
    if err := ValidateInput(input.Query); err != nil {
        return ErrorResult(err.Error())
    }
    // ... process input
}
```

- `ValidatePath` — prevents directory traversal (`..`), null bytes, length > 4096
- `ValidateInput` — prevents null bytes, length > 4096

## Code Conventions

- **Constants**: Protocol constants in `protocol/constants.go`. Use `const`, never `var`. Never inline magic values.
- **Ordering**: Declarations alphabetically where practical; logical grouping for state machines. `NewTypeName` constructor first after its type, then methods alphabetically. `case` clauses alphabetically in switches.
- **JSON tags**: Every exported protocol field gets `json:"fieldName"` matching MCP spec camelCase. `omitempty` for optional fields. Never use `omitzero`.
- **Error handling**: `fmt.Errorf("operation: %w", err)`. Map to JSON-RPC error codes at the protocol boundary using `protocol.Err*` constructors.
- **Imports**: stdlib first, blank line, then internal packages. Enforced by goimports with `github.com/andygeiss/mcp` as local prefix.
- **Logging**: `slog.LevelInfo` default. `Error` for unrecoverable. `Warn` for recoverable. `Info` for lifecycle only. `snake_case` log keys. `slog.JSONHandler` to stderr exclusively.
- **No** `utils`/`helpers`/`common` packages. No premature interfaces. No dead code. No external dependencies.
- **stdout**: Protocol-only. Never write logs, debug output, or non-JSON-RPC data. Enforced by `forbidigo` linter (denies `fmt.Print*`).

## PR Process

1. Branch from `main`
2. Write failing test first (TDD)
3. Implement the fix/feature
4. Run `make check` (build + test + lint) — must pass with zero issues
5. CI runs: build, test (3 OS matrix), lint, vulncheck, fuzz (2m), integration, bench
6. One approval required
7. No force-push to `main`

### What Won't Be Accepted

- External dependencies (stdlib only)
- HTTP/WebSocket transport
- Non-protocol data on stdout
- `//nolint` directives without fixing the underlying issue
- `.golangci.yml` modifications to suppress findings
- Skipping commit hooks (`--no-verify`)
