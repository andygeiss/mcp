# Development Guide

**Project:** mcp
**Generated:** 2026-04-05

## Prerequisites

- **Go 1.26+** (see `go.mod` for exact version)
- **golangci-lint** (v2 config format)
- No external dependencies required

## Quick Start

```bash
# Clone
git clone https://github.com/andygeiss/mcp.git
cd mcp

# Set up pre-commit hooks
make setup

# Full quality pipeline (build + test + lint)
make check

# Build the binary with version info
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/

# Run interactively
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}' | ./mcp
```

## Build Commands

| Command | Description |
|---|---|
| `make check` | Full pipeline: build + test + lint |
| `make build` | Build all packages |
| `make test` | Run tests with race detector |
| `make lint` | Run golangci-lint (must pass with 0 issues) |
| `make fuzz` | Fuzz the protocol decoder (30s default) |
| `make fuzz FUZZTIME=5m` | Fuzz with custom duration |
| `make cover` | Run tests with coverage report |
| `make setup` | Configure local development environment (git hooks) |
| `make init MODULE=github.com/org/repo` | Initialize as new project from template |

## Pre-Commit Hooks

Run `make setup` after cloning to configure git hooks. This sets `core.hooksPath` to `.githooks/`, which contains a pre-commit hook that runs `make check` (build + test + lint) before every commit.

If golangci-lint is not installed, the hook warns and runs `make build test` instead (graceful degradation).

To bypass the hook in emergencies: `git commit --no-verify`. Use sparingly.

## Testing

### Running Tests

```bash
go test -race ./...                                            # Unit tests (race detector mandatory)
go test -race ./... -tags=integration                          # Include integration tests
go test -fuzz Fuzz_Decoder ./internal/protocol -fuzztime=30s   # Fuzz the decoder
golangci-lint run ./...                                        # Lint
```

### Test Conventions

- **Naming:** `Test_<Unit>_With_<Condition>_Should_<Outcome>`
- **Structure:** `// Arrange` / `// Act` / `// Assert`
- **Every test calls `t.Parallel()`**
- **Black-box packages** (`package foo_test`) by default; white-box only for unexported internals
- **Assertions:** `assert.That(t, "description", got, expected)` from `internal/pkg/assert`
- **I/O testing:** Inject `bytes.Buffer` for stdin/stdout/stderr. Write JSON-RPC requests + EOF, run server, read responses from output buffer.

### Test Categories

| Category | Location | Build Tag | Description |
|---|---|---|---|
| Unit tests | `*_test.go` | (none) | Fast, isolated, parallel |
| Integration tests | `*_test.go` | `integration` | Full server pipeline, end-to-end |
| Fuzz tests | `fuzz_test.go` | (none) | Protocol decoder and full server pipeline |
| Conformance tests | `conformance_test.go` | `integration` | Data-driven from `testdata/conformance/` JSONL files |
| Architecture tests | `architecture_test.go` | (none) | Import graph verification |
| Self-documenting tests | `claudemd_test.go` | (none) | CLAUDE.md claims have matching test coverage |
| Synctest tests | `synctest_test.go` | (none) | Deterministic concurrency via `testing/synctest` |
| Benchmark tests | `benchmark_test.go` | (none) | Performance regression detection |

### Fuzz Testing

The project has two fuzz targets:

1. **`Fuzz_Decoder_With_ArbitraryInput`** (`internal/protocol/fuzz_test.go`) -- fuzzes the JSON-RPC decoder with arbitrary input bytes. This is also the target compiled for Google OSS-Fuzz.

2. **`Fuzz_Server_Pipeline`** (`internal/server/fuzz_test.go`) -- fuzzes the full server pipeline (decode -> dispatch -> handle -> encode) and asserts no panics occur and all stdout output is valid JSON-RPC.

Fuzz corpus is committed to `internal/protocol/testdata/fuzz/` (300+ entries).

### Adding a Conformance Test

Create `testdata/conformance/<name>.request.jsonl` with one JSON-RPC message per line. Optionally create `<name>.response.jsonl` for byte-exact golden comparison. The conformance runner discovers these automatically.

## Adding a New Tool

1. Define an input struct with `json` and `description` tags:

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

The input schema is derived automatically from struct tags -- no manual JSON Schema definition needed. Fields without `omitempty` in their json tag are marked as `required`.

3. Write tests for the handler in isolation, plus an integration test through the full server.

## Using as a Template

Fork or clone the repo, then run:

```bash
go run ./cmd/init github.com/yourorg/yourproject
```

This rewrites all imports, renames `cmd/mcp/` to `cmd/yourproject/`, runs `go mod tidy`, verifies zero template fingerprints remain, and self-deletes the `cmd/init/` directory.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `MCP_TRACE` | `""` | Set to `"1"` to enable protocol trace logging to stderr |

## MCP Server Configuration

The `.mcp.json` file in the project root configures the MCP server for local development:

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

## Code Conventions

- **Constants:** Protocol constants in `protocol/constants.go`. Use `const`, never `var`. Never inline magic numbers.
- **Ordering:** Declarations alphabetically. `NewTypeName` constructor first, then methods alphabetically.
- **JSON tags:** Every exported protocol field gets `json:"fieldName"` matching MCP spec camelCase. `omitempty` for optional fields. Never `omitzero`.
- **Error handling:** `fmt.Errorf("operation: %w", err)`. Map to JSON-RPC error codes at the protocol boundary using `CodeError`.
- **Imports:** stdlib first, blank line, then internal packages.
- **Logging:** `slog.LevelInfo` default. `Error` for unrecoverable. `Warn` for recoverable. `Info` for lifecycle events only. `snake_case` keys.
- **No** `utils`/`helpers`/`common` packages. No premature interfaces. No dead code.

## Linting

The project uses golangci-lint v2 with 50+ linters. Key restrictions enforced:

- `depguard`: Blocks `encoding/json/v2`, `encoding/json/jsontext`, and `log` (must use `log/slog`)
- `forbidigo`: Blocks `fmt.Print*`, `print`, `println` (stdout is protocol-only)
- `testpackage`: Enforces black-box test packages
- `tagliatelle`: Enforces camelCase JSON tags
- `sloglint`: Enforces `snake_case` log keys and static messages

Never modify `.golangci.yml` to suppress findings -- fix the code instead.
