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
- Template rewriter end-to-end cycle (`cmd/scaffold`)
- Stdout protocol-only enforcement
- MCP conformance suite (37 file-driven scenarios in `internal/server/testdata/conformance/`)
- Bidirectional transport: `Peer.SendRequest`, outbound cancellation (A7), shutdown races (`synctest_test.go`)
- Smoke test (`internal/tools/smoke_integration_test.go`) backing `make smoke`

### Fuzz Testing

```bash
make fuzz                                                                      # default: 30s on Fuzz_Decoder_With_ArbitraryInput
go test -fuzz Fuzz_Decoder_With_ArbitraryInput ./internal/protocol -fuzztime=30s # specific target
```

Five fuzz targets:
1. `Fuzz_Decoder_With_ArbitraryInput` (`internal/protocol`) -- protocol decoder resilience
2. `Fuzz_Server_Pipeline` (`internal/server`) -- end-to-end server fuzz
3. `Fuzz_ValidateInput_With_ArbitraryInput` (`internal/tools`) -- input validation
4. `Fuzz_ValidatePath_With_ArbitraryInput` (`internal/tools`) -- path validation
5. `Fuzz_LookupTemplate_With_ArbitraryInputs` (`internal/resources`) -- URI template matcher

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

golangci-lint v2 configured in `.golangci.yml`. Zero suppression policy -- fix the code, never add `//nolint` or modify `.golangci.yml` to silence findings.

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
    p.Report(0, 100)                      // requires client _meta.progressToken
    p.Log("info", "my_tool", "started")   // notifications/message
    // ... work ...
    p.Report(100, 100)
    return tools.TextResult("done")
}
```

AI10 invariant: do not interleave `Report` calls with an outbound `protocol.SendRequest` that is awaiting a reply.

## Server-to-Client Requests (v1.3.0)

Tool and prompt handlers can initiate JSON-RPC requests back to the client for sampling, elicitation, and roots:

```go
import "github.com/andygeiss/mcp/internal/protocol"

func MyTool(ctx context.Context, input MyInput) tools.Result {
    resp, err := protocol.SendRequest(ctx, "sampling/createMessage", map[string]any{
        "messages": []map[string]any{{"role": "user", "content": "..."}},
    })
    if err != nil {
        // errors.Is(err, protocol.ErrServerShutdown)
        // errors.Is(err, protocol.ErrPendingRequestsFull)
        // errors.Is(err, protocol.ErrNoPeerInContext)
        // errors.AsType[*protocol.CapabilityNotAdvertisedError](err)
        return tools.ErrorResult(err.Error())
    }
    // inspect resp.Result or resp.Error
    return tools.TextResult("ok")
}
```

Gated methods: `sampling/createMessage`, `elicitation/create`, `roots/list`. The server synchronously rejects with `*protocol.CapabilityNotAdvertisedError` if the client did not advertise the required capability during `initialize`.

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
| `build` | Compile binary with `-trimpath` and version ldflags |
| `check` | Full pipeline: build + test + lint |
| `coverage` | Run with race detector, enforce 90% threshold |
| `fuzz` | Fuzz decoder (30s default, configurable via `FUZZTIME=2m`) |
| `init` | Execute template rewriter (`MODULE=github.com/org/repo`) |
| `lint` | Run golangci-lint |
| `setup` | Configure git hooks from `.githooks/` |
| `smoke` | Minimal `initialize → tools/list` round-trip verification |
| `test` | Unit tests with race detector |

---

*Generated: 2026-04-18 | Scan level: deep | Reflects v1.3.0*
