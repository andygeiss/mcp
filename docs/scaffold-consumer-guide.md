---
title: 'Scaffold Consumer Guide'
audience: 'developer who just ran `make init MODULE=...`'
last_updated: 2026-05-03
status: 'living draft — Q2 sprint deliverable'
---

# Scaffold Consumer Guide

You ran `make init MODULE=github.com/yourorg/yourproject`. The template rewriter substituted your module path, reset git history, and printed a banner. This guide is the orientation that picks up from there.

If you have not run `make init` yet, see the [README](../README.md) — the install path lives there.

## The welcome banner

After `make init` succeeds, the scaffold prints to stderr:

```
Your MCP server is running.

  Edit:   internal/tools/echo.go
  Wire:   cmd/mcp/main.go
  Verify: make smoke

Full guide: README.md
```

The three verbs are not aspirational — they are the entire workflow. This guide expands each one in turn.

## The three-step wiring flow

### 1. Edit — `internal/tools/echo.go`

Open the file. The first comment line is the entry point:

```go
// START HERE — your first tool. Edit, copy, rename. It's yours.
```

The echo tool is the minimal reference: one input struct, one output struct, one handler. The conventions on display:

- **Input/output are typed structs.** `EchoInput` and `EchoOutput` carry `json:"…"` and `description:"…"` struct tags. The reflection engine in `internal/schema` derives the tool's `inputSchema` and `outputSchema` from these tags — there is no separate schema definition to keep in sync.
- **The description drives the agent.** The `description` tag becomes part of the JSON Schema the LLM sees. A wrong description produces a misused tool. Treat the tag like a public contract.
- **The handler is plain Go.** `func Echo(_ context.Context, input EchoInput) (EchoOutput, Result)` — context for cancellation, typed input, typed output, plus a `Result` for optional content blocks. Most handlers ignore `Result{}` and just return the typed struct.
- **Single-surface output.** The dispatch wrapper auto-marshals the non-zero `Out` into `result.structuredContent`. There is no legacy text block by default. Clients read the typed payload using the advertised `outputSchema`.

To author your own tool, copy `echo.go` to a new file, rename `EchoInput`/`EchoOutput`/`Echo`, and replace the body. Keep the `description` tags meaningful — they are the API surface the agent sees.

### 2. Wire — `cmd/mcp/main.go`

Tools are registered in the `run()` function. The current echo registration:

```go
if err := tools.Register[tools.EchoInput, tools.EchoOutput](registry, "echo", "Echoes the input message", tools.Echo); err != nil {
    return fmt.Errorf("register echo: %w", err)
}
```

The generic parameters `[tools.EchoInput, tools.EchoOutput]` give the registry the types it needs to derive the schemas reflectively. Add one such line per tool. If you remove `echo`, also remove the registration; the linker will not catch unreferenced tool types in this layout.

Resources and prompts use parallel registries — see `server.WithResources(...)` and `server.WithPrompts(...)` if you need them. Capability-honesty is automatic: the server advertises the corresponding capability only when its registry is wired AND non-empty. You will not see `resources` or `prompts` capabilities advertised until you register at least one entry. This came from the operator-dignity bundle (commit `e8d01d2`, R2).

### 3. Verify — `make smoke`

Run `make smoke` from the repo root. It pipes three JSON-RPC messages into a fresh `go run ./cmd/mcp/`:

1. `initialize` (with a minimal `clientInfo`),
2. `notifications/initialized`,
3. `tools/list`.

On success you see:

```
Your server works. It exposes N tool(s) with outputSchema advertised.
```

The smoke test asserts that every tool in `tools/list` carries an `outputSchema` field — this is the AC8-of-Story-2.2 guard from the Q1 sprint. If it fails, the most common causes are exactly what the failure output names:

- Forgot to register the tool in `cmd/mcp/main.go`?
- Tool handler does not compile? Run `go build ./...`.

`make smoke` is the fastest signal that your tool is wired correctly. Run it after every edit.

## Operator-dignity outcomes already in your scaffold

The scaffold ships with several operator-facing affordances that landed in the Q1 sprint. They are not optional features; they are the baseline you inherit:

- **Structured `error.data` on `tools/call` failures** (commit `1473184`, M2/M3/Q41 bundle). When a tool handler returns an error, the JSON-RPC error response carries an `error.data` object naming the offending field where applicable, plus a `request_id` echoing the request id. Operators do not have to grep dispatch code to find the failure site.
- **`request_id` auto-injection in error responses.** Same bundle. Every error response carries a request correlation id so log lines and protocol bytes line up at a glance.
- **Stderr startup banner.** Same bundle. The server logs a structured `info`-level startup line on stderr (`slog.JSONHandler`), naming the binary version and tool count. You see it the moment your server attaches to a client.
- **Capability honesty** (commit `e8d01d2`, R2). The server advertises `resources` and `prompts` capabilities only when their registries are non-empty. No phantom capabilities appear in `initialize` responses for an echo-only server.
- **Sequential dispatch frame** (commit `5689f0e`). The server advertises `experimental.concurrency.maxInFlight: 1` to signal sequential request handling. This is documented as a load-bearing constraint in [`docs/architecture.md`](./architecture.md#load-bearing-constraints) — see the architecture doc before lifting it.

Read the [Architecture overview](./architecture.md) for the full picture; the operator-dignity items are the user-visible end of the architectural decisions documented there.

## What's next (placeholders for Q2 user-facing tools)

The following sections are deliberate stubs — each will be filled in by the corresponding Q2 story as the tooling lands. The placeholders exist so this guide stays the single landing page for scaffold consumers, even mid-sprint.

### Inspect a server's surface

Run `mcp --inspect-only` to dump every registered tool, resource, and prompt — with derived JSON schemas — as a single JSON document on stdout. The process never reads stdin and never enters the dispatch loop, so the inspection is a one-shot CLI call you can pipe into `jq` or any other JSON tool:

```bash
$ go run ./cmd/mcp/ --inspect-only | jq '.tools[].name'
"echo"
```

Or for a full snapshot:

```bash
$ go run ./cmd/mcp/ --inspect-only
{
  "server": {"name": "mcp", "version": "dev"},
  "protocolVersion": "2025-11-25",
  "tools": [
    {
      "name": "echo",
      "description": "Echoes the input message",
      "inputSchema": {"type": "object", "properties": {...}, "required": [...]},
      "outputSchema": {"type": "object", "properties": {...}, "required": [...]}
    }
  ],
  "resources": [],
  "resourceTemplates": [],
  "prompts": []
}
```

The output is deterministic — lists are sorted by their identity key (tool name, resource URI, prompt name), and empty registries appear as empty arrays so consumers can rely on the field set being stable.

Use cases:

- **Procurement / integrators**: audit a server's surface without a JSON-RPC client.
- **CI**: `make inspect-smoke` runs `--inspect-only` and validates the JSON contract.
- **Tooling chain**: the same inspector primitive (`internal/inspect`) backs `mcp doctor` (Story 3.3) and `make catalog` (Story 3.4).

### Scaffold a new tool

Run `make new-tool TOOL=<Name>` to copy `internal/tools/_TOOL_TEMPLATE.go` into a new file with your identifiers substituted in. `TOOL` must be a CamelCase Go identifier (`Greeter`, `WeatherFetch`); the new file is named with the lowercase form (`greeter.go`, `weatherfetch.go`).

```bash
$ make new-tool TOOL=Greeter
Created internal/tools/greeter.go

Add this to cmd/mcp/main.go inside run():
  tools.Register[tools.GreeterInput, tools.GreeterOutput](registry, "greeter", "...description...", tools.Greeter)
```

If `$EDITOR` is set, your editor opens the new file automatically. If unset, the command exits cleanly — you find and edit the file by name.

What the target does:

- Copies the template, dropping the `//go:build ignore` line so the new file enters the build.
- Substitutes `YourTool → <Name>` and `your-tool → <name>` consistently across struct names, the handler function, and the registration-line example.
- Prints the exact `tools.Register[...]` line for you to paste into `cmd/mcp/main.go`.
- Refuses (non-zero exit) when `TOOL=` is missing, not CamelCase, or would collide with an existing file in `internal/tools/`.

The scaffolder is **stdlib + POSIX tools only** — no LLM, no `curl`, no external code-gen library. It is safe to run offline and produces byte-identical output for a given input.

After scaffolding:

1. Edit your input/output struct fields and handler body.
2. Paste the printed registration line into `cmd/mcp/main.go`.
3. Run `make smoke` — the smoke test exercises the full server pipeline and reports `outputSchema advertised`.

### Drive your server interactively

Run `make repl` to spawn the local `mcp` server in a subprocess and drop into a line-based interactive client. The REPL handles the `initialize` handshake automatically and translates short commands into JSON-RPC requests:

```text
$ make repl
initialize OK
connected to mcp (protocol 2025-11-25); type 'quit' to exit
mcp> tools list
{
  "id": 1,
  "jsonrpc": "2.0",
  "result": {
    "tools": [
      { "name": "echo", "description": "Echoes the input message", ... }
    ]
  }
}
mcp> tools call echo {"message":"hello"}
{
  "id": 2,
  "jsonrpc": "2.0",
  "result": {
    "content": [],
    "structuredContent": { "echoed": "hello" }
  }
}
mcp> quit
```

The full command vocabulary:

- `tools list` → `tools/list`
- `tools call <name> <json-args>` → `tools/call`
- `resources list` → `resources/list`
- `resources read <uri>` → `resources/read`
- `prompts list` → `prompts/list`
- `prompts get <name> <json-args>` → `prompts/get`
- `quit` (or Ctrl-D / SIGINT) — exit cleanly; the child server is signalled and reaped, no orphan process

JSON arguments must parse as a JSON object. Default to `{}` when omitted. Unknown verbs and subcommands are rejected with a one-line message and the prompt stays active.

The REPL is **stdlib + sequential dispatch**: it sends one request, waits for the response, prints it, then accepts the next command. Same single-threaded discipline the server advertises (`experimental.concurrency.maxInFlight: 1`).

To point the REPL at a different binary (e.g., a release build you want to validate), set `MCP_REPL_SERVER=/path/to/mcp` before running.

### Validate client configurations

Run `mcp doctor` to scan for known MCP client configurations on the host (Claude Desktop, Cursor, VS Code), validate that every referenced binary path exists and is executable, and surface drift between the configured binary and the running invoker.

```bash
$ mcp doctor
[OK] Claude Desktop — local-mcp (/Users/andy/Library/Application Support/Claude/claude_desktop_config.json): binary exists and is executable
[WARN] Claude Desktop — local-mcp (...): inspector reports tools: echo

1 row(s); 0 FAIL
```

The check is strict by failure modes:

- `[FAIL]` — binary missing or not executable. Exits non-zero so CI / pre-commit hooks can gate on it.
- `[WARN]` — version mismatch between configured and running binary, or informational tool-set rendering when the configured binary supports `--inspect-only`. Does not affect exit code.
- `[OK]` — passed every required check.

Severity matrix and why doctor is read-only (no auto-repair) are documented in [ADR-004](./adr/ADR-004-doctor-config-detection.md). Run the command before reporting issues — most install problems show up as a `[FAIL]` row with the exact path that needs editing.

### Generate a TOOLS.md catalog

Run `make catalog` to render [`docs/TOOLS.md`](./TOOLS.md) from the production tool registry. The catalog is deterministic (alphabetical) and idempotent — the Makefile target fails on drift, the same `cmp -s` pattern used for `make spec-coverage`.

```bash
$ make catalog
OK: docs/TOOLS.md matches the registry
```

The catalog reuses the inspector primitive from `mcp --inspect-only`, so the documented surface always matches what the running server advertises in `tools/list`. To force a regeneration (after editing a tool's description, for example):

```bash
$ make catalog        # fails non-zero, regenerates docs/TOOLS.md
$ git diff docs/TOOLS.md
$ git commit docs/TOOLS.md -m "docs: refresh TOOLS catalog"
```

The output goes through `make doc-lint` like every other checked-in doc, so tool descriptions referencing gitignored paths (`_bmad-output/`, `_bmad/`, `.claude/`, `docs/.archive/`) will fail the gate. Keep descriptions self-contained.

## Perf measurement

Benchmarks live next to the code they measure (`internal/protocol/benchmark_test.go`, `internal/schema/schema_bench_test.go`, `internal/tools/registry_bench_test.go`, `internal/server/benchmark_test.go`). Run `make bench` to execute the suite and compare against the committed baseline:

```bash
$ make bench
go test -bench=. -count=6 -benchmem ./internal/... > current.txt
benchstat testdata/benchmarks/baseline.txt current.txt
```

The output shows per-metric deltas (allocations, ns/op) between the committed `testdata/benchmarks/baseline.txt` and your local `current.txt`. Negative numbers are improvements; positive numbers are regressions.

Discipline:

- **`benchstat` is required.** Install with `go install golang.org/x/perf/cmd/benchstat@latest`. It is exempt CI infrastructure, not a `go.mod` dependency.
- **Baseline updates are deliberate.** When an intentional perf change ships, regenerate `testdata/benchmarks/baseline.txt` in the same PR with a commit-message justification. Do NOT regenerate the baseline on every PR — that defeats the comparison.
- **`make bench` is separate from `make check`.** Running benchmarks under `-race` is wasteful (10–20× slower). Keep them out of the quality gate.
- **Cross-architecture noise is real.** Apple silicon vs. x86 Linux can show 2× deltas on the same code. The committed baseline is host-specific to its capture environment; treat cross-machine comparisons as informational, not strict.

Headline metrics worth pinning:

- `Benchmark_ToolsList_AllocsPerOp_With_OneTool` — the per-call allocation cost of the registry accessor on the production wiring.
- `Benchmark_DeriveInputSchema_With_*` — schema derivation cost across small / large / deeply-nested struct shapes; feeds future schema-cache evaluation.
- `Benchmark_Decode_*` and `Benchmark_Encode_*` in `internal/protocol` — codec hot path.

## What to read next

Order from "I just ran `make smoke` and it worked" to "I am ready to ship":

1. [`CLAUDE.md`](../CLAUDE.md) — engineering philosophy and conventions. The "ALWAYS" / "NEVER" sections apply to every change, not just AI-driven ones.
2. [`docs/architecture.md`](./architecture.md) — system design, lifecycle state machine, transport, bidi, schema derivation, error taxonomy. The "Load-bearing constraints" section is mandatory reading before any architectural change.
3. [`docs/development-guide.md`](./development-guide.md) — Make targets, test conventions, adding tools/resources/prompts, the bidi handler pattern, commit conventions, PR process.
4. [`docs/agent-rules.md`](./agent-rules.md) — operational rules sheet (Tech Stack, Language Rules, MCP Protocol Rules, Testing Rules, Code Quality, Development Workflow, Don't-Miss Rules).

For the conformance evidence model — what `protocol.Clause` is, how `make spec-coverage` produces the audit fragment, and how the registry binds tests to MUST-bearing claims — see the [Spec Conformance Narrative (draft)](./conformance/index.md).

## Source-of-truth pointers

| Topic | File |
|---|---|
| Welcome banner text | [`cmd/scaffold/main.go`](../cmd/scaffold/main.go) (`welcomeBanner` const) |
| Echo tool reference | [`internal/tools/echo.go`](../internal/tools/echo.go) |
| Tool wiring example | [`cmd/mcp/main.go`](../cmd/mcp/main.go) (`tools.Register[...]` line) |
| Smoke test | `Makefile` (`smoke` target) |
| Quality pipeline | `Makefile` (`check` = build + test + lint + doc-lint) |
| Engineering philosophy | [`CLAUDE.md`](../CLAUDE.md) |
| Architecture | [`docs/architecture.md`](./architecture.md) |
| Conformance | [`docs/conformance/index.md`](./conformance/index.md) |

If a checked-in source disagrees with this guide, the source wins. File an issue (or fix the guide) when you spot drift.
