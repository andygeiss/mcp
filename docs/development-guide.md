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
| `make build` | Compile all packages |
| `make check` | Full quality pipeline: build + test + lint |
| `make test` | Run all tests with race detector (`go test -race ./...`) |
| `make lint` | Run golangci-lint (must pass with 0 issues) |
| `make fuzz` | Fuzz the protocol decoder (30s default, override with `FUZZTIME=2m`) |
| `make bench` | Run benchmarks and compare against baseline |
| `make coverage` | Run tests with coverage, enforce 75% threshold |

### Building with Version

```bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/
```

### Running the Server

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}' | ./mcp
```

### Using as a Template

Fork/clone, then run:

```bash
go run ./cmd/init github.com/yourorg/yourproject
```

This rewrites all imports, renames `cmd/mcp/` to `cmd/yourproject/`, runs `go mod tidy`, and self-deletes the `cmd/init/` directory.

## Testing

### Test Categories

| Category | Command | Build Tag |
|---|---|---|
| Unit tests | `go test -race ./...` | none |
| Integration tests | `go test -race -tags=integration ./...` | `integration` |
| Fuzz tests | `go test -fuzz Fuzz_Decoder ./internal/protocol -fuzztime=30s` | none |
| Benchmarks | `make bench` | none |

### Test Conventions

- **Naming**: `Test_<Unit>_With_<Condition>_Should_<Outcome>`
- **Structure**: `// Arrange` / `// Act` / `// Assert`
- **Parallelism**: Every test calls `t.Parallel()`
- **Package**: Black-box (`package foo_test`) by default; white-box only for unexported internals
- **Assertions**: `assert.That(t, "description", got, expected)` from `internal/pkg/assert`
- **I/O testing**: Inject `bytes.Buffer` for stdin/stdout, write JSON-RPC requests + EOF, read responses from output buffer

### Coverage

Coverage threshold is enforced at 75%. Run:

```bash
make coverage
```

Coverage reports are uploaded to [Codecov](https://codecov.io/gh/andygeiss/mcp) via CI.

## Environment Variables

| Variable | Description |
|---|---|
| `MCP_TRACE` | Set to `"1"` to enable protocol trace logging (logs every request/response to stderr) |

## Pre-commit Hook

The `.githooks/pre-commit` hook runs `make check` (build + test + lint). If `golangci-lint` is not installed, it falls back to build + test only with a warning.

## Adding a New Tool

1. Create `internal/tools/yourtools.go` with an input struct and handler:

```go
type YourInput struct {
    Field string `json:"field" description:"Field description"`
}

func YourTool(_ context.Context, input YourInput) Result {
    return TextResult("result: " + input.Field)
}
```

2. Register in `cmd/mcp/main.go`:

```go
tools.Register(registry, "your-tool", "Description", tools.YourTool)
```

3. The input schema is auto-derived from struct tags via reflection. No manual JSON Schema needed.

## Code Conventions

- **Constants**: Protocol constants in `protocol/constants.go`. Use `const`, never `var`.
- **JSON tags**: Every exported protocol field gets `json:"fieldName"` matching MCP spec camelCase. `omitempty` for optional fields.
- **Error handling**: `fmt.Errorf("operation: %w", err)`. Map to JSON-RPC error codes at the protocol boundary.
- **Imports**: stdlib first, blank line, then internal packages.
- **Logging**: `slog.LevelInfo` default, `snake_case` log keys, `slog.JSONHandler` to stderr only.
- **No** `utils`/`helpers`/`common` packages. No premature interfaces. No dead code.
