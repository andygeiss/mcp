# Development Guide

**Project:** github.com/andygeiss/mcp
**Generated:** 2026-04-05

## Prerequisites

- **Go 1.26+** (see `go.mod` for exact version)
- **golangci-lint** installed ([installation guide](https://golangci-lint.run/welcome/install/))
- No external dependencies required -- standard library only

## Quick Start

```bash
# Clone
git clone https://github.com/andygeiss/mcp.git
cd mcp

# Full quality check (build + test + lint)
make check

# Build with version
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/

# Run
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}' | ./mcp
```

## Build Commands

| Command | Description |
|---------|-------------|
| `make check` | Full pipeline: build + test + lint |
| `make build` | `go build ./...` |
| `make test` | `go test -race ./...` |
| `make lint` | `golangci-lint run ./...` |
| `make coverage` | Tests with coverage report (`coverage.out`) |
| `make fuzz` | Fuzz decoder for 30s (override: `make fuzz FUZZTIME=5m`) |
| `make init MODULE=github.com/org/name` | Initialize as new project from template |

## Testing

### Running Tests

```bash
go test -race ./...                                            # unit tests (race detector mandatory)
go test -race ./... -tags=integration                          # include integration tests
go test -fuzz Fuzz_Decoder ./internal/protocol -fuzztime=30s   # fuzz the decoder
golangci-lint run ./...                                        # lint (must pass with 0 issues)
```

### Test Conventions

- **Naming:** `Test_<Unit>_With_<Condition>_Should_<Outcome>`
- **Structure:** `// Arrange` / `// Act` / `// Assert`
- **Parallel:** Every test calls `t.Parallel()`
- **Package:** Black-box (`package foo_test`) by default. White-box only for unexported internals.
- **Assertions:** `assert.That(t, "description", got, expected)` from `internal/pkg/assert`
- **I/O:** Inject `bytes.Buffer`. Write JSON-RPC requests + EOF, run server, read responses.
- **Golden tests:** Byte-for-byte JSON comparison for protocol correctness.
- **Fuzz:** `Fuzz_<Unit>_<Aspect>` targets for decoder/parser.

### Test Categories

| Category | Location | Build Tag | Description |
|----------|----------|-----------|-------------|
| Unit tests | `*_test.go` | none | Per-function, parallel, black-box |
| Integration | `integration_test.go` | `integration` | Full pipeline through server |
| Fuzz | `fuzz_test.go` | none | Native Go fuzz for decoder |
| Benchmarks | `benchmark_test.go` | none | Codec and schema performance |
| I/O robustness | `io_test.go` | none | Slow/partial/closed streams |
| Concurrency | `synctest_test.go` | none | Virtual time via `testing/synctest` |
| Doc validation | `claudemd_test.go` | none | CLAUDE.md claims match code |

## Adding a New Tool

1. Define an input struct in `internal/tools/`:

```go
// internal/tools/greet.go
package tools

import "context"

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

The input schema is auto-derived from struct tags via reflection. No manual JSON Schema needed.

### Schema Derivation Rules

| Go Type | JSON Schema Type |
|---------|-----------------|
| `string` | `"string"` |
| `int`, `int64`, etc. | `"integer"` |
| `float32`, `float64` | `"number"` |
| `bool` | `"boolean"` |
| `[]T` | `"array"` with items |
| `map[string]T` | `"object"` with additionalProperties |
| nested struct | `"object"` with properties |
| `*T` | unwrapped to underlying type |

- `json:"name"` -> property name
- `json:"-"` -> field excluded
- `description:"..."` -> property description
- No `omitempty` -> field is required

## Coding Conventions

- **Constants:** Protocol constants in `protocol/constants.go`. Use `const`, never `var`.
- **Ordering:** Declarations alphabetically. Constructor first after type, then methods alphabetically.
- **JSON tags:** Every exported protocol field gets `json:"fieldName"` matching MCP spec camelCase. `omitempty` for optional.
- **Error handling:** `fmt.Errorf("operation: %w", err)`. Map to JSON-RPC error codes at boundary.
- **Imports:** stdlib first, blank line, then internal packages.
- **Logging:** `slog.LevelInfo` default. `snake_case` log keys. stderr only.
- **No** `utils`/`helpers`/`common` packages. No premature interfaces. No dead code.

## Agentic Workflow (TDD)

1. **Perceive:** Read code, understand state. Do NOT edit.
2. **Act:** Test first (RED), then production code (GREEN).
3. **Verify:** `go test -race ./...` + `golangci-lint run ./...`. Exit code is authoritative.
4. **Iterate:** Fix root cause, loop to step 2. Do NOT proceed while red.
5. **Refactor:** Only after green. Re-verify after every structural change.

## Pull Request Process

1. Branch from `main`
2. CI must pass: build, test, fuzz, lint
3. One approval required
4. No force-push to `main`

### Commit Conventions

- Imperative mood, concise
- Prefix with area when helpful: `protocol: fix id echo for null`

## What Won't Be Accepted

- External dependencies (stdlib only)
- HTTP/WebSocket transport
- Non-protocol data on stdout
- `//nolint` directives without fixing the underlying issue
- `.golangci.yml` modifications to suppress findings

## Using as a Template

Fork or clone, then run:

```bash
go run ./cmd/init -module github.com/yourorg/yourproject -name yourproject
```

This rewrites all imports, renames `cmd/mcp/` to `cmd/yourproject/`, runs `go mod tidy`, and self-deletes `cmd/init/`.
