# mcp

[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/andygeiss/mcp/badge)](https://scorecard.dev/viewer/?uri=github.com/andygeiss/mcp)

A minimal, zero-dependency Go implementation of the [Model Context Protocol](https://modelcontextprotocol.io) (MCP).

Single binary. Stdin/stdout transport. JSON-RPC 2.0. Nothing else.

## Why

MCP servers don't need HTTP frameworks, routers, or dependency trees. This project proves it: a fully compliant MCP server in pure Go, with automatic tool schema derivation and a three-state initialization handshake -- all backed by the standard library alone.

Use it directly, or use it as a **template** to scaffold your own MCP server in seconds.

## Features

- **JSON-RPC 2.0** over stdin/stdout -- newline-delimited, no LSP framing
- **Automatic input schema** derived from Go struct tags via reflection -- no manual JSON Schema
- **Three-state lifecycle** (uninitialized / initializing / ready) per the MCP spec
- **Graceful shutdown** on SIGINT, SIGTERM, or EOF
- **Per-message size limits** and handler timeouts with panic recovery
- **Structured logging** to stderr via `slog.JSONHandler`
- **Zero external dependencies** -- standard library only
- **Fuzz-tested** JSON decoder

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
go run ./cmd/init -module github.com/yourorg/yourproject -name yourproject
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
tools.Register(registry, "greet", "Greets someone by name", tools.Greet)
```

The input schema (`{"type":"object","properties":{"name":{"type":"string","description":"Name to greet"}},"required":["name"]}`) is derived automatically from struct tags. No manual schema definition needed.

## Architecture

```
cmd/mcp/           main.go -- wiring only: flags, I/O injection, os.Exit
cmd/init/          template rewriter -- not part of normal builds
internal/
  protocol/        JSON-RPC 2.0 codec, types, constants
  server/          lifecycle, dispatch, capability negotiation
  tools/           registry, schema derivation, tool handlers
  pkg/assert/      test assertion helpers
```

**Dependency direction:** `cmd/mcp/ -> server/ -> protocol/`, `server/ -> tools/`. Protocol has zero internal dependencies. Tools may import protocol but never server.

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
```

The test suite uses table-driven subtests, parallel execution, black-box packages, byte-exact golden comparisons, and fuzz targets for the protocol decoder.

## Protocol compliance

MCP version `2024-11-05`. JSON-RPC 2.0 with these specifics:

| Behavior | Implementation |
|---|---|
| Framing | Newline-delimited JSON objects |
| Batch requests | Rejected with `-32700` |
| Missing `params` | Normalized to `{}` |
| Request `id` | Preserved as `json.RawMessage`, echoed exactly |
| Notifications | Never responded to |
| Unknown notifications | Silently ignored |
| Error messages | Contextual (e.g. `"unknown tool: foo"`) |

## License

[MIT](LICENSE) -- Andreas Geiss
