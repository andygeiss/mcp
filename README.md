# mcp

[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/andygeiss/mcp/badge)](https://scorecard.dev/viewer/?uri=github.com/andygeiss/mcp)
[![Coverage](https://codecov.io/gh/andygeiss/mcp/branch/main/graph/badge.svg)](https://codecov.io/gh/andygeiss/mcp)

A minimal, zero-dependency Go implementation of the [Model Context Protocol](https://modelcontextprotocol.io) (MCP).

Single binary. Stdin/stdout transport. JSON-RPC 2.0. Nothing else.

## Why

MCP servers don't need HTTP frameworks, routers, or dependency trees. This project proves it: a fully compliant MCP server in pure Go, with automatic tool schema derivation and a three-state initialization handshake -- all backed by the standard library alone.

Use it directly, or use it as a **template** to scaffold your own MCP server in seconds.

## Features

- **MCP 2025-11-25** spec-complete protocol foundation
- **JSON-RPC 2.0** over stdin/stdout -- newline-delimited, no LSP framing
- **Tools, Resources, Prompts** -- all capabilities with auto-derived schemas via reflection
- **Progress & Logging** -- context-injected notifications during tool execution
- **Bidirectional transport** -- server-to-client requests (sampling, elicitation, roots)
- **Three-state lifecycle** (uninitialized / initializing / ready) per the MCP spec
- **Graceful shutdown** on SIGINT, SIGTERM, or EOF
- **Per-message size limits** (4 MB) and handler timeouts (30s) with panic recovery
- **Structured logging** to stderr via `slog.JSONHandler`
- **Zero external dependencies** -- standard library only
- **Fuzz-tested** JSON decoder with 22-entry seed corpus
- **54 linter rules** via golangci-lint v2 -- zero suppression policy

## Requirements

- Go 1.26+

## Quickstart

### Build

```bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/
```

### Run

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}' | ./mcp
```

### Use as a template

Fork or clone this repo, then run the init tool to rewrite the module path and binary name:

```bash
go run ./cmd/init github.com/yourorg/yourproject
```

This rewrites all imports, renames `cmd/mcp/` to `cmd/yourproject/`, runs `go mod tidy`, and self-deletes.

## Adding a tool

Define an input struct, write a handler, register it.

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

```go
// cmd/mcp/main.go
if err := tools.Register(registry, "greet", "Greets someone by name", tools.Greet); err != nil {
    return fmt.Errorf("register greet: %w", err)
}
```

The input schema (`{"type":"object","properties":{"name":{"type":"string","description":"Name to greet"}},"required":["name"]}`) is derived automatically from struct tags. No manual schema definition needed.

## Architecture

```
cmd/mcp/           main.go -- wiring only: flags, I/O injection, os.Exit
cmd/init/          template rewriter -- not part of normal builds
internal/
  assert/          test assertion helpers
  prompts/         prompt registry, argument derivation
  protocol/        JSON-RPC 2.0 codec, types, constants
  resources/       resource registry, static resources, URI templates
  schema/          shared JSON Schema derivation via reflection
  server/          lifecycle, dispatch, notifications, bidirectional transport
  tools/           tool registry, schema derivation, tool handlers
```

**Dependency direction:** `cmd/mcp/ -> server/ -> protocol/`, `server/ -> tools/`, `server/ -> resources/`, `server/ -> prompts/`. Protocol and schema have zero internal dependencies.

**Transport rules:**
- **stdout** is protocol-only. Every byte is a valid JSON-RPC message.
- **stderr** is diagnostics-only via `slog.JSONHandler`.
- Constructors accept `io.Reader`/`io.Writer` so tests inject buffers.

## Testing

```bash
go test -race ./...                                            # unit tests
go test -race ./... -tags=integration                          # include integration tests
go test -fuzz Fuzz_Decoder ./internal/protocol -fuzztime=30s   # fuzz the decoder
golangci-lint run ./...                                        # lint
make check                                                     # build + test + lint
```

419 test functions, 33 conformance scenarios, 4 fuzz targets, 11 benchmarks, 90% coverage threshold enforced in CI.

## Protocol compliance

MCP version `2025-11-25`. JSON-RPC 2.0 with these specifics:

| Behavior | Implementation |
|---|---|
| Framing | Newline-delimited JSON objects |
| Batch requests | Rejected with `-32700` |
| Missing `params` | Normalized to `{}` |
| Request `id` | Preserved as `json.RawMessage`, echoed exactly |
| Notifications | Never responded to |
| Unknown notifications | Silently ignored |
| Error messages | Contextual (e.g. `"unknown tool: foo"`) |

## Documentation

Full project documentation lives in [`docs/`](docs/index.md):

- [Project Overview](docs/project-overview.md) -- Executive summary, features, protocol compliance
- [Architecture](docs/architecture.md) -- Package structure, state machine, dispatch model, schema derivation
- [Source Tree Analysis](docs/source-tree-analysis.md) -- Annotated directory tree, per-package exports, test inventory
- [Development Guide](docs/development-guide.md) -- Setup, testing, tool authoring, code conventions
- [Deployment Guide](docs/deployment-guide.md) -- Build, release pipeline, CI/CD, platform support

## License

[MIT](LICENSE) -- Andreas Geiß
