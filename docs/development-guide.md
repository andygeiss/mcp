# Development Guide

## Prerequisites

- Go 1.26+
- golangci-lint v2
- git

## Setup

```bash
git clone https://github.com/andygeiss/mcp.git
cd mcp
make setup   # configures git hooks from .githooks/
```

## Build

```bash
make build
# or manually:
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/
```

## Testing

### Unit Tests

```bash
make test
# or:
go test -race ./...
```

Race detector is mandatory for all test runs.

### Integration Tests

```bash
go test -race ./... -tags=integration
```

Integration tests cover:
- Full server lifecycle via stdin/stdout buffers
- SIGINT/SIGTERM graceful shutdown (Unix only)
- Template rewriter end-to-end cycle
- Stdout protocol-only enforcement
- MCP conformance suite (33 scenarios)

### Fuzz Testing

```bash
make fuzz                                                                      # default: 30s on Fuzz_Decoder_With_ArbitraryInput
go test -fuzz Fuzz_Decoder_With_ArbitraryInput ./internal/protocol -fuzztime=30s # specific target
```

Four fuzz targets:
1. `Fuzz_Decoder_With_ArbitraryInput` -- protocol decoder resilience (22 seed corpus entries)
2. `Fuzz_Server_Pipeline` -- end-to-end server fuzz
3. `Fuzz_ValidateInput_With_ArbitraryInput` -- input validation
4. `Fuzz_ValidatePath_With_ArbitraryInput` -- path validation

Nightly CI runs all targets for 5 minutes each.

### Benchmarks

```bash
make bench
```

Compares against `testdata/benchmarks/baseline.txt` using benchstat. 20% regression threshold.

### Coverage

```bash
make coverage
```

Enforces 90% minimum coverage. CI fails if coverage drops below threshold.

### Lint

```bash
golangci-lint run ./...
```

54 linter rules. Zero suppression policy -- fix the code, never add `//nolint` or modify `.golangci.yml`.

### Full Check

```bash
make check   # build + test + lint
```

## Adding a Tool

1. Define an input struct with `json` and `description` tags in `internal/tools/`
2. Implement `func(ctx context.Context, input T) Result`
3. Register via `tools.Register[T](registry, name, description, handler)` in `cmd/mcp/main.go`
4. Write unit tests for the handler
5. Add integration test through the full server (`//go:build integration`)

Schema is auto-derived from struct tags. No manual schema definition.

```go
type GreetInput struct {
    Name string `json:"name" description:"Name to greet"`
}

func Greet(_ context.Context, input GreetInput) Result {
    return TextResult("Hello, " + input.Name + "!")
}
```

## Adding a Resource

1. Use `resources.Register(registry, uri, name, description, handler)` for static resources
2. Use `resources.RegisterTemplate(registry, uriTemplate, name, description, handler)` for URI templates
3. Pass registry to server via `server.WithResources(registry)`

## Adding a Prompt

1. Define argument struct with `json` and `description` tags
2. Implement `func(ctx context.Context, input T) Result`
3. Register via `prompts.Register[T](registry, name, description, handler)`
4. Pass registry to server via `server.WithPrompts(registry)`

## Progress and Logging from Handlers

```go
func MyTool(ctx context.Context, input MyInput) tools.Result {
    p := server.ProgressFromContext(ctx)
    p.Report(0, 100)   // progress notification (requires client progressToken)
    p.Log(slog.LevelInfo, "my_tool", map[string]any{"status": "started"})
    // ... work ...
    p.Report(100, 100)
    return tools.TextResult("done")
}
```

## Test Conventions

- **Naming**: `Test_<Unit>_With_<Condition>_Should_<Outcome>`
- **Structure**: `// Arrange` / `// Act` / `// Assert`
- **Parallel**: Every test calls `t.Parallel()`
- **Package**: Black-box (`package foo_test`) by default
- **Assertions**: `assert.That(t, "description", got, expected)`
- **I/O**: Inject `bytes.Buffer`. Write JSON-RPC requests + EOF, run server, read responses

## Code Conventions

- Constants in `protocol/constants.go`. Use `const`, never `var`
- Declarations alphabetically where practical
- `NewTypeName` constructor first after type, then methods alphabetically
- JSON tags: `json:"fieldName"` matching MCP spec camelCase, `omitempty` for optional
- Error handling: `fmt.Errorf("operation: %w", err)`
- Imports: stdlib first, blank line, then internal packages
- Logging: `slog.LevelInfo` default, `snake_case` keys

## Makefile Targets

| Target | Description |
|---|---|
| `bench` | Run benchmarks, compare with baseline (20% threshold) |
| `build` | Compile binary with version ldflags |
| `check` | Full pipeline: build + test + lint |
| `coverage` | Run with race detector, enforce 90% threshold |
| `fuzz` | Fuzz decoder (30s default, configurable via FUZZTIME) |
| `init` | Execute template rewriter (requires MODULE env var) |
| `lint` | Run golangci-lint |
| `setup` | Configure git hooks from .githooks/ |
| `test` | Unit tests with race detector |

---

*Generated: 2026-04-11 | Scan level: exhaustive*
