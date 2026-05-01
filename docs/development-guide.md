# Development Guide

How to build, test, fuzz, and contribute to `github.com/andygeiss/mcp`.

---

## Prerequisites

- **Go 1.26+** — `go.mod` is the source of truth for the exact minimum.
- **golangci-lint** — must pass with **zero issues**. `.golangci.yml` is read-only; do not weaken to silence findings.
- **No external dependencies** — the project is stdlib-only. CI tooling (golangci-lint, goreleaser, GitHub Actions, Codecov) is exempt infrastructure, not a project dependency.
- *(Optional)* **bc** for `make coverage` (used to compare against the 90% threshold).
- *(Optional)* **benchstat** for `make bench`.

## Initial setup

```bash
git clone https://github.com/andygeiss/mcp.git
cd mcp
make setup        # configures local pre-commit hooks (.githooks)
make check        # build + test + lint — your "everything green" smoke test
```

## Make targets

The Makefile is the canonical entry point for development tasks.

| Target | What it does |
|---|---|
| `make setup` | Configure `git config core.hooksPath .githooks` (run once after clone). |
| `make build` | `go build -trimpath -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/`. Reproducible — flags match the release binary. |
| `make test` | `go test -race ./...`. Race detector is **mandatory** — never run plain `go test`. |
| `make lint` | `golangci-lint run ./...`. Must pass with zero issues. |
| `make check` | `build + test + lint`. The full quality gate. Run this before opening a PR. |
| `make coverage` | Runs tests with coverage and **enforces the 90% threshold**. Fails if total drops below. |
| `make fuzz` | Fuzzes `Fuzz_Decoder_With_ArbitraryInput` for 30s. `make fuzz FUZZTIME=5m` to extend. |
| `make bench` | Runs benchmarks (count=6) and compares against `testdata/benchmarks/baseline.txt` via benchstat. |
| `make smoke` | End-to-end smoke test: pipes `initialize` + `notifications/initialized` + `tools/list` through the binary and checks the response. On success, prints a one-liner naming the tool count; on failure, prints diagnostic hints + captured stderr (exit 1). |
| `make spec-coverage` | Regenerate `docs/spec-coverage.txt` from the in-memory `protocol.Clauses` registry. Run after adding a new clause; commit the updated file alongside the test change. |
| `make init MODULE=...` | Scaffold rewriter — rewrites the module path, repoints badges, runs `go mod tidy`, removes `cmd/scaffold/`, resets git history. **Refuses to run on a dirty tree** — commit/stash first or pass `--force`. |

## Direct `go` commands (canonical incantations)

For verifying a change locally, the Makefile is preferred. If you need to invoke `go` directly, **cite verbatim** — paraphrasing breaks the `-ldflags`:

```
go build ./...                                                                          # build all packages
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/  # build with version
go test -race ./...                                                                     # unit tests; race detector mandatory
go test -race ./... -tags=integration                                                   # include integration tests
go test -fuzz Fuzz_Decoder_With_ArbitraryInput ./internal/protocol -fuzztime=30s        # fuzz the decoder
golangci-lint run ./...                                                                 # lint; must pass with zero issues
```

## Test conventions

The project enforces a strict test discipline. Reviewers will reject PRs that violate these.

- **Naming:** `Test_<Unit>_With_<Condition>_Should_<Outcome>`. Fuzz targets: `Fuzz_<Unit>_<Aspect>`.
- **Structure:** `// Arrange` / `// Act` / `// Assert` blocks.
- **`t.Parallel()` is mandatory** on every test — and implies **local fixtures**: no package-level state, no shared servers across subtests, no global registries.
- **Black-box default:** `package foo_test`. White-box (`package foo`) is reserved for testing unexported internals with no observable surface.
- **Assertions:** `assert.That(t, "description", got, expected)` from `internal/assert`. **No third-party assertion library** (testify, gomega, etc.) — same stdlib-only posture as production code.
- **I/O in tests:** inject `bytes.Buffer` via the constructor's `io.Reader`/`io.Writer` parameters. The test *is* the client — no separate harness.
- **No mocks for `internal/protocol`, `internal/tools`, `internal/resources`, `internal/prompts`.** Fakes over mocks; the canonical fake is `bytes.Buffer` + a real codec.
- **Goldens** in `testdata/`, byte-for-byte JSON. A newline difference is a regression. **A golden change is a wire-format change** — treat it as a SemVer signal.
- **Integration tests** are gated by `//go:build integration` at the top of the file.

### Decision rule — which test layer?

| Bug type | Test layer |
|---|---|
| Handler logic, schema derivation, registry semantics | Unit (`package foo_test`) |
| Dispatch path, state machine, EOF/cancel/bidi correlation | Integration (`//go:build integration`) |
| A client would notice as a wire break | Golden (byte-for-byte JSON) |
| Adversarial input on a parser surface | Fuzz (`Fuzz_*` targets) |

If you can't name which box catches a regression, you've duplicated coverage in two and have a gap in another.

### Required test categories for new code

- **New tool, resource, or prompt:** unit test (handler in isolation) + integration test (through the full server).
- **New server-to-client request type (bidi):** the **triad** — correlation (concurrent in-flight, no ID collision, no deadlock), cancellation (handler abort propagates), capability-gate (returns `*protocol.CapabilityNotAdvertisedError` with **zero side effects**: pending-map untouched, no bytes written).
- **New parser surface:** add a fuzz target (`Fuzz_<Unit>_With_ArbitraryInput`) and run `-fuzztime=30s` minimum locally before opening the PR.
- **New error code or state transition:** golden test for the wire shape.

## Adding a new tool

1. **Define the input and output structs** in `internal/tools/yourtool.go`:
   ```go
   type GreetInput struct {
       Name string `json:"name" description:"Name to greet"`
   }
   type GreetOutput struct {
       Greeting string `json:"greeting" description:"The greeting message"`
   }
   ```
2. **Write the handler.** Return both the typed `Out` (auto-marshaled into `structuredContent` when non-zero) and the legacy `Result` (carries Content/IsError):
   ```go
   func Greet(_ context.Context, input GreetInput) (GreetOutput, Result) {
       msg := "Hello, " + input.Name + "!"
       return GreetOutput{Greeting: msg}, TextResult(msg)
   }
   ```
3. **Register** in `cmd/mcp/main.go` (provide both type parameters explicitly when the inferrer can't see them):
   ```go
   if err := tools.Register[tools.GreetInput, tools.GreetOutput](registry, "greet", "Greets someone by name", tools.Greet); err != nil {
       return fmt.Errorf("register greet: %w", err)
   }
   ```
4. **Write tests** — unit test for the handler, integration test through the server.
5. **Verify:** `make smoke` prints `"Your server works. It exposes N tool(s)."` (count includes your new tool).

Both `inputSchema` and `outputSchema` (`{"type":"object","properties":{...},"required":[...]}`) are **auto-derived** by `internal/schema/schema.go` from struct tags — never hand-roll JSON Schema. The dispatch layer skips the structured-content marshal when the handler returns the zero value of `Out`, so `omitempty` stays honest.

**Constraints on `In` and `Out` for `tools.Register[In, Out]`:** both must be struct types (or pointer-to-struct). `int`, `map`, slice-of-primitive at the top level error out. For tools without meaningful structured output, pick a small typed wrapper (`type FooOutput struct{}` is allowed) — concrete types document the contract and let the schema engine produce a stable shape.

## Adding a spec clause

The spec-conformance registry (`internal/protocol/spec_clauses.go`) maps every MUST/SHOULD/MAY-bearing requirement of the MCP specification to the test functions that prove the server complies. **Tests are stored as function pointers, never as strings** — so renaming or removing a covered test is a compile-time error, not a silent runtime drift. This is the load-bearing design choice; do not "simplify" by switching to test-name strings.

To register a new clause, add a `func init()` block to the `_test.go` file that holds the test you are pinning. The colocation pattern keeps drift visible at the diff level:

```go
// internal/protocol/protocol_test.go
package protocol_test

import (
    "testing"

    "github.com/andygeiss/mcp/internal/protocol"
)

func init() {
    protocol.Register(protocol.Clause{
        ID:      "MCP-2025-11-25/jsonrpc/MUST-echo-id",
        Level:   "MUST",
        Section: "JSON-RPC 2.0 §5 Response object",
        Summary: "Decoder preserves request id exactly for echo back to the client.",
        Tests: []func(*testing.T){
            Test_Decode_With_StringID_Should_PreserveExactly,
            Test_Decode_With_NumberID_Should_PreserveExactly,
        },
    })
}
```

After adding a clause, run `make spec-coverage` to regenerate `docs/spec-coverage.txt` and commit the file alongside the test change. Reviewers should see the audit grow on every PR that lands a new MUST-bearing test. The renderer at `cmd/spec-coverage/main.go` is the compile-checked entry point; the committed audit artifact is produced by the `Test_RenderSpecCoverage_Should_WriteCommittedReport` test (bootstrap `init()` blocks live in `_test.go` files, so plain `go run ./cmd/spec-coverage` sees an empty registry — that's expected).

## Adding a new resource or prompt

- **Resource:** `resources.Register(registry, uri, name, description, handler)` for static, or `resources.RegisterTemplate(...)` for URI-template-backed. Pass via `server.WithResources(registry)`. The `resources` capability is auto-advertised.
- **Prompt:** `prompts.Register[T](registry, "name", "description", handler)`. Pass via `server.WithPrompts(registry)`. The `prompts` capability is auto-advertised. Same struct-only constraint as tools.

## Reaching the client from a tool handler (bidi)

Handlers can issue requests to the client (sampling, elicitation, roots, or any future method) without importing `internal/server`:

```go
import "github.com/andygeiss/mcp/internal/protocol"

func MyHandler(ctx context.Context, input MyInput) Result {
    resp, err := protocol.SendRequest(ctx, "sampling/createMessage", params)
    if errors.Is(err, protocol.ErrNoPeerInContext) {
        // Running outside the server (e.g., direct unit test). Handle accordingly.
    }
    var capErr *protocol.CapabilityNotAdvertisedError
    if errors.As(err, &capErr) {
        // Client did not advertise this capability. Zero side effects occurred.
    }
    // ... use resp
}
```

The server attaches itself as a `protocol.Peer` to the handler's context (`protocol.ContextWithPeer`) at dispatch. The **AI9 capability gate** is the first statement on the outbound path — outbound `sampling/`, `elicitation/`, `roots/` calls return `*CapabilityNotAdvertisedError` with **zero side effects** when the client did not advertise the corresponding capability during `initialize`.

## Progress reporting from tool handlers

```go
import "github.com/andygeiss/mcp/internal/server"

func LongRunning(ctx context.Context, input MyInput) Result {
    p := server.ProgressFromContext(ctx)  // nil-safe
    for i := 0; i < total; i++ {
        // ... do work
        p.Report(i+1, total)              // no-op without _meta.progressToken
    }
    p.Log(slog.LevelInfo, "done", nil)    // server→client logging notification
    return TextResult("done")
}
```

`Report` is nil-safe AND token-safe — it no-ops without a client-supplied `_meta.progressToken` on the originating request. `Log` is nil-safe but does NOT depend on the progress token (`notifications/message` is governed by the server's `logging` capability and `logging/setLevel`, not per-request progress opt-in).

**AI10 invariant (enforced):** `Report` is automatically dropped while a handler is parked in `protocol.SendRequest`. `(*Server).SendRequest` brackets its outbound await with `suspendForOutbound`, so calling `Report` from a goroutine that fires during the await is safe — the call returns a no-op. Handlers do not need to track outbound state themselves. The slow-client recovery path remains `-32001` (`ServerTimeout`).

**AI10 telemetry:** every dropped `Report` emits a `progress_dropped_during_outbound` warn line on stderr (request-scoped logger, carries `request_id`, `reason=ai10_invariant`, plus the `progress` and `total` of the dropped call). This is operator visibility into a contract violation — the gate silently corrects the wire shape, but the warn lets you see when handler code is interleaving `Report` with `SendRequest` so the handler can be fixed. The no-token no-op path stays silent (no opt-in is normal, not anomalous).

**Token type preservation:** the original JSON type of `progressToken` (string, number) is preserved byte-for-byte on the emitted notification — `json.RawMessage` is used end-to-end, never re-marshaled. A request with `_meta.progressToken: "task-42"` produces `progressToken: "task-42"` on the wire (string), and `_meta.progressToken: 42` produces `progressToken: 42` (number). This matches the request `id` precedent in JSON-RPC 2.0.

## Fuzzing

```bash
make fuzz                 # 30s default
make fuzz FUZZTIME=5m     # custom duration
```

The project ships 5 fuzz targets:
- `Fuzz_Decoder_With_ArbitraryInput` — `internal/protocol/` (primary, OSS-Fuzz target)
- `Fuzz_Server_Pipeline` — `internal/server/`
- `Fuzz_LookupTemplate_With_ArbitraryInputs` — `internal/resources/`
- `Fuzz_ValidateInput_With_ArbitraryInput` — `internal/tools/`
- `Fuzz_ValidatePath_With_ArbitraryInput` — `internal/tools/`

OSS-Fuzz runs the corpus continuously upstream — local `-fuzztime=30s` is a smoke check, not a gate.

**Fuzz failures become permanent regression seeds** under `internal/protocol/testdata/fuzz/<TargetName>/`. Version-controlled, never `.gitignore`d. Skipping the seed-commit step makes OSS-Fuzz start cold and pushes cost upstream.

OSS-Fuzz harness lives in `oss-fuzz/`. To test the harness locally with Docker:

```bash
docker build -f oss-fuzz/Dockerfile -t mcp-fuzz-test .
```

## Coverage

`make coverage` enforces a **90% threshold**. The Makefile target reads the `total:` line from `go tool cover -func=` and fails if total drops below. CI also tracks coverage via Codecov; **do not regress without a stated reason**.

```bash
make coverage    # run tests, generate coverage.out, enforce ≥90%
go tool cover -html=coverage.out  # open in browser to see annotated source
```

## Lint

```bash
make lint        # golangci-lint run ./...
```

Must pass with **zero issues**. `.golangci.yml` is treated as **read-only**:
- **No `//nolint`** to silence findings — fix the code instead.
- **`depguard.no-server-in-handlers`** denies `internal/server` imports from `internal/tools/**` and `internal/prompts/**` (Invariant I1).
- **`forbidigo`** bans `os.Stdout` writes and `fmt.Fprint(os.Stdout, ...)` in handler packages (Invariant I4).

## Commit conventions

Commit messages follow Conventional Commits: `type(scope): summary`.

- **Live `type` vocabulary:** `feat`, `fix`, `chore`, `docs`, `refactor`.
- **Scope grammar is semantic, not the package path.** Scope is one of:
  - a version (`feat(v1.3.0)`)
  - a subsystem (`feat(init)`, `docs(adr-003)`)
  - an artifact name (`docs(CLAUDE.md)`, `chore(deps)`)
- **Do not write** `feat(internal/server)` — wrong by this repo's convention.
- **Body explains why**, not what. The diff shows what.
- **One concern per commit.** Bug-fix commits don't carry refactors; refactor commits don't carry behavior changes.
- **Tests and the code they cover live and die together** in one commit.

For AI-co-authored commits, use a `Co-authored-by:` trailer naming the model that did the work — for example:
```
Co-authored-by: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```
GitHub renders this as a co-author on the commit. Pass commit messages via HEREDOC — inline `-m` mangles newlines and breaks the trailer.

## Pull requests

1. **Branch from `main`.**
2. **Run the local quality gate** (`make check`) before opening the PR.
3. **`gh pr create`** is the canonical PR path. PR title matches the commit subject (`type(scope): summary`).
4. **Body describes what + why + test plan** (test plan is mandatory for non-trivial changes).
5. **All five CI workflows must pass** before merge: `ci`, `codeql`, `fuzz`, `release`, `scorecard`.
6. **Local-CI parity:** CI is authoritative. A local-pass / CI-fail divergence is a *finding* worth investigating, not a "just rerun it" situation.
7. **Never push directly to `main`.** Every change lands via PR — including the maintainer's own (10-for-10 in the git log).
8. **Never force-push shared branches.**
9. **Never skip hooks** (`--no-verify`, `--no-gpg-sign`).

## Dependabot

Dependabot PRs are **not human-style PRs** — auto-merge on CI green. No architecture critique, no design review. Treat them as supply-chain hygiene, not feature work.

## Scaffold-survival check

This repo is **two products**: an MCP server and a template via `make init MODULE=...`. When changing files in `cmd/mcp/`, `internal/`, or anything templated, **verify the change survives `make init`**:

```bash
make init MODULE=github.com/test/scaffold-check  # in a scratch clone
make check                                        # in the rewritten tree
```

Breaking the template is silent until a downstream user files an issue.

## Environment variables

`MCP_TRACE=1` is the only variable; it logs every request and response to stderr for local debugging. Production caveat lives with the operator-facing reference: see [environment variables](./deployment-guide.md#environment-variables).

## What we won't accept

- External `go.mod` dependencies (stdlib only)
- HTTP/WebSocket transport
- Non-protocol data on stdout
- `//nolint` directives without fixing the underlying issue
- `.golangci.yml` modifications to suppress findings
- Tests that hide failures (deleted/relaxed assertions)
- `TODO`/`FIXME`/`XXX`/`HACK` in committed code (file an issue or fix it now)
- Multi-paragraph docstrings; comments answering WHAT instead of WHY

## See also

- [Architecture](./architecture.md) — system design
- [Source Tree Analysis](./source-tree-analysis.md) — package map
- [Deployment Guide](./deployment-guide.md) — releases, signing
- [CONTRIBUTING.md](../CONTRIBUTING.md) — short-form contributor onboarding
- [CLAUDE.md](../CLAUDE.md) — engineering philosophy for AI agents
- [Agent Rules](./agent-rules.md) — the load-bearing operational rule sheet for AI agents
