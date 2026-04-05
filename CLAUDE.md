<!-- Last reviewed: 2026-04-05 -->

> **Last reviewed:** 2026-04-05

# CLAUDE.md

## Engineering Philosophy

Every decision — code, architecture, review, tooling — is evaluated from the perspective of a world-class Go engineering expert building a minimal, effective, efficient CLI MCP server. Optimize for correctness, clarity, and simplicity. Do not over-engineer, over-specify, or add abstraction ahead of need.

## Project Overview

Go implementation of the Model Context Protocol (MCP). Module path: `github.com/andygeiss/mcp`. The required Go version is defined in `go.mod` — always trust `go.mod` as the source of truth.

Minimal, efficient MCP server communicating over stdin/stdout using JSON-RPC 2.0. Single CLI binary — no HTTP, no WebSocket. MCP protocol version: `2024-11-05`.

Prefer the newest stdlib API available at the Go version declared in `go.mod`. No external dependencies beyond the standard library. Go 1.26 was chosen for Green Tea GC (10-40% overhead reduction), `reflect.Type.Fields` iterators, `signal.NotifyContext` cancel cause, and `errors.AsType[T]`. `GOEXPERIMENT=jsonv2` is **not supported** — the protocol codec relies on `encoding/json` v1 behavior.

## Build & Test

```bash
go build ./...                                                                          # build all packages
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/  # build with version
go test -race ./...                                                                     # unit tests (race detector mandatory)
go test -race ./... -tags=integration                                                   # include integration tests
go test -fuzz Fuzz_Decoder ./internal/protocol -fuzztime=30s                            # fuzz the JSON decoder
golangci-lint run ./...                                                                 # lint (must pass with 0 issues)
```

Every change must pass `go test -race ./...` and `golangci-lint run ./...` with zero issues before it is considered complete. Do not modify `.golangci.yml` to suppress findings — fix the code instead.

## Architecture

Flat and simple — no hexagonal layers, no bounded contexts. Complexity is added only when the code demands it.

### Package Structure

```
cmd/mcp/                  # main.go — wiring only: parse flags, inject os.Stdin/os.Stdout/os.Stderr, call server.Run, os.Exit
cmd/init/                 # template rewriter — for template-repo consumers: rewrites module path, renames binary dir, then self-deletes; not part of normal builds
internal/
  pkg/
    assert/               # lightweight test assertion helpers (assert.That) — stdlib only
  protocol/               # JSON-RPC 2.0 types, message codec, constants
  server/                 # MCP server: lifecycle, capability negotiation, method dispatch, CLAUDE.md self-test
  tools/                  # tool registry, reflection-based schema derivation, individual tool handler implementations
```

### Dependency Direction

`cmd/mcp/ → internal/server/ → internal/protocol/` and `internal/server/ → internal/tools/`. The `protocol` package has zero dependencies on other internal packages. The `tools` package may import `protocol` but never `server`. The `assert` package is test-only.

### Transport Constraints

- **stdin**: Persistent `json.NewDecoder`. No `bufio.Scanner`.
- **stdout**: Protocol-only. Every byte must be a valid JSON-RPC message. No logs, no debug output.
- **stderr**: `log/slog` with `slog.JSONHandler` exclusively.
- **I/O injection**: Constructors accept `io.Reader`/`io.Writer` — not `*os.File` — so tests inject buffers.
- **EOF handling**: `io.EOF` / `io.ErrUnexpectedEOF` → clean shutdown (exit 0). All other decode errors → fatal (exit 1).
- **Signals**: `SIGINT`/`SIGTERM` cancel the server context for graceful shutdown. No drain — exit promptly.
- **Size limits**: Per-message via counting reader. No single `io.LimitReader` (cumulative).

### Initialization State Machine

Three states: **uninitialized** → **initializing** → **ready**.

| State | Allowed | Rejected with |
|---|---|---|
| **Uninitialized** | `initialize`, `ping` | `-32600` ("server not initialized") |
| **Initializing** | `ping` | `-32600` ("server not initialized") |
| **Ready** | All methods | — |

- `initialize` → respond with capabilities, transition to **initializing**. Duplicate → `-32600`.
- `notifications/initialized` in **initializing** → transition to **ready**. Other states → silently ignore.
- `ping` always works. Unknown notifications are silently ignored — never respond, never log.

### JSON-RPC 2.0

- Newline-delimited JSON objects. No LSP framing. No batch requests (JSON array → `-32700`).
- `id` is `json.RawMessage` — preserve original type, echo back exactly.
- A message without `id` is a notification — never respond to it.
- `params`: when absent or `null`, default to `{}` before unmarshaling.
- Error `message` must be contextual (e.g., `"unknown tool: foo"`, not `"invalid params"`).

### Error Codes

| Code | Meaning |
|---|---|
| `-32700` | Parse error — malformed JSON, size limit exceeded |
| `-32600` | Invalid request — bad structure, not initialized, already initialized, `params` not an object |
| `-32601` | Method not found — unknown method, `rpc.*` reserved methods |
| `-32602` | Invalid params — wrong types, missing required fields, unknown tool name |
| `-32603` | Internal error — should not happen in normal operation |

### JSON Package

Use `encoding/json` with `omitempty` for optional fields — never `omitzero`. While Go 1.24+ supports `omitzero` in the stdlib, this project standardizes on `omitempty` for consistency across all protocol types.

### Adding a New Tool

Follow the simplest existing tool in `internal/tools/` as the template. Define an input struct with `json` and `description` tags — the input schema is auto-derived from struct fields via reflection (`tools/schema.go`), so no manual schema definition is needed. Implement a typed handler `func(ctx context.Context, input T) Result`, register via `tools.Register[T]` in `cmd/mcp/main.go`. Unit test the handler in isolation. Integration test through the full server (`//go:build integration`).

## Coding Conventions

- **Constants**: Protocol constants in `protocol/constants.go`. Use `const`, never `var`. Never inline.
- **Ordering**: Declarations alphabetically. `NewTypeName` constructor first after its type, then methods alphabetically. `case` clauses alphabetically in switches. In YAML/Make config files: schema-defined top-level keys stay in the tool's canonical order (e.g., `name` → `on` → `permissions` → `jobs` for GitHub Actions); user-defined keys within those schemas are alphabetical (job names, permission names, linter lists, env vars, Makefile targets after the default). Value lists (`goos`, `goarch`, linter enable lists) are alphabetical. Steps remain in sequential order.
- **JSON tags**: Every exported protocol field gets `json:"fieldName"` matching MCP spec camelCase. `omitempty` for optional fields.
- **Error handling**: `fmt.Errorf("operation: %w", err)`. Map to JSON-RPC error codes at the protocol boundary.
- **Imports**: stdlib first, blank line, then internal packages.
- **No** `utils`/`helpers`/`common` packages. No premature interfaces. No dead code.
- **Logging**: `slog.LevelInfo` default. `Error` for unrecoverable failures. `Warn` for recoverable anomalies. `Info` for lifecycle events only. `snake_case` log keys.

## Testing Conventions

- **Naming**: `Test_<Unit>_With_<Condition>_Should_<Outcome>`
- **Structure**: `// Arrange` / `// Act` / `// Assert`. Every test calls `t.Parallel()`.
- **Package**: Black-box (`package foo_test`) by default. White-box only for unexported internals.
- **Assertions**: `assert.That(t, "description", got, expected)` from `internal/pkg/assert`.
- **I/O**: Inject `bytes.Buffer`. Write JSON-RPC requests + EOF, run server, read responses from output buffer.
- **Golden tests**: Byte-for-byte JSON comparison for protocol correctness.
- **Fuzz**: `Fuzz_<Unit>_<Aspect>` targets for the decoder/parser.

## Agentic Workflow

**perceive → act → verify → iterate**. Bounded and externally verified.

1. **Perceive**: Read code, understand state. Do NOT edit.
2. **Act**: Test first (RED), then production code (GREEN).
3. **Verify**: `go test -race ./...` + `golangci-lint run ./...`. Exit code is authoritative.
4. **Iterate**: Fix root cause, loop to step 2. Do NOT proceed while red.
5. **Refactor**: Only after green. Re-verify after every structural change.

- 3 consecutive failures without progress → stop and explain to user.
- 2 failed corrections on same error → re-plan before retrying.
- Bug fix: reproduce with failing test first, then fix minimally, then verify.

## Guardrails

**NEVER**:
- Modify `.golangci.yml` to suppress findings
- Delete/weaken test assertions
- Add `//nolint` without fixing the issue
- Skip commit hooks
- Push directly to `main`
- Force-push shared branches
- Add external dependencies
- Commit credentials
- Continue features while a quality gate is failing
- Write non-protocol data to stdout

**ALWAYS**:
- Run tests + lint before declaring done
- Write failing test first
- Wrap errors with context
- Use `io.Reader`/`io.Writer` injection
- Echo request `id` exactly
- Return deterministic ordering from `tools/list`
- Enforce the initialization state machine

## BMad Workflow System

[BMad](https://github.com/bmadcode/BMAD-METHOD) agent workflow in `.claude/skills/` and `_bmad/` — use `/bmad-help` for next steps.
