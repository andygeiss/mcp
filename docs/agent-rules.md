# Agent Rules — Operational Rules for AI Agents

_This file contains critical rules and patterns that AI agents must follow when implementing code in this project. Focus on unobvious details that agents might otherwise miss._

_Authoritative companions: [`CLAUDE.md`](../CLAUDE.md) (engineering philosophy + conventions), [`go.mod`](../go.mod) (Go version), [`internal/protocol/constants.go`](../internal/protocol/constants.go) (`MCPVersion`), [`.golangci.yml`](../.golangci.yml) (lint), [`Makefile`](../Makefile) (commands). On conflict, the codebase wins._

---

## Technology Stack & Versions

- **Language:** Go — version pinned in `go.mod` (currently `1.26`). Treat `go.mod` as the source of truth; never hardcode the version elsewhere.
- **External dependencies:** **None.** Project is stdlib-only. Adding any non-stdlib import to `go.mod` is forbidden — must be raised as an architectural decision first. CI tooling (golangci-lint, GitHub Actions, Codecov) is exempt infrastructure, not a project dependency.
- **MCP protocol version:** `2025-11-25` — defined as `MCPVersion` in `internal/protocol/constants.go`. That constant is the source of truth, not docs.
- **JSON codec:** `encoding/json` v1. **`GOEXPERIMENT=jsonv2` is unsupported** — the protocol codec relies on v1 behavior.
- **Why Go 1.26:** Green Tea GC (10–40% overhead reduction), `reflect.Type.Fields` iterators, `signal.NotifyContext` cancel cause, `errors.AsType[T]`. Prefer the newest stdlib API available at this version.
- **Lint:** `golangci-lint` v2 (config in `.golangci.yml`). Must pass with **zero issues**. The `integration` build tag is enabled for analysis.
- **Test runner:** `go test -race ./...` mandatory. Integration tests guarded by `//go:build integration`.
- **Coverage gate:** **90% minimum** enforced by `make coverage` (CI fails below threshold).
- **Fuzzing:** 5 targets across `internal/protocol`. `make fuzz` runs the decoder fuzzer for `FUZZTIME` (default 30s). OSS-Fuzz integration in `oss-fuzz/`.
- **Release pipeline:** goreleaser + cosign keyless signing + SBOM + SLSA L3 provenance (see `.goreleaser.yml`, `.github/workflows/release.yml`).
- **Release-equivalent local build:** `go build -trimpath -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/` — `make build` does this.

## Critical Implementation Rules

### Language-Specific Rules (Go)

- **JSON tags**: every exported protocol field gets `json:"camelCaseName"` matching the MCP spec. Optional fields use **`omitempty`** — never `omitzero` (project standardizes on v1 codec semantics).
- **Error wrapping**: `fmt.Errorf("operation: %w", err)`. Preserve chains. Map errors to JSON-RPC codes only at the protocol boundary (`internal/server`).
- **I/O injection**: constructors accept `io.Reader` / `io.Writer` — not `*os.File`. Tests inject `bytes.Buffer`.
- **Logging**: `log/slog` with `slog.JSONHandler` exclusively, on `os.Stderr`. Levels: `Info` for lifecycle only, `Warn` recoverable, `Error` unrecoverable. Log keys are `snake_case`. Never `fmt.Println`, never the `log/*` package.
- **Imports**: stdlib first → blank line → internal packages. `goimports` enforces local prefix `github.com/andygeiss/mcp`.
- **Constants**: protocol constants live in `internal/protocol/constants.go`. Use `const`, never `var`. Never inline a literal that has a named constant.
- **Stdlib first**: prefer the newest stdlib API at the Go version in `go.mod`. No external imports.
- **No utility packages**: no `utils`, `helpers`, `common`. No premature interfaces. No dead code.
- **Decoder**: persistent `json.NewDecoder` over a per-message counting reader. **Never** `bufio.Scanner` (line-length footgun).
- **Context propagation**: pass `ctx context.Context` through every cancellable path. SIGINT/SIGTERM cancel the server context via `signal.NotifyContext` (cancel cause leveraged where useful).
- **No behavior in `init()`**: explicit wiring only in `cmd/mcp/main.go`.

### MCP Protocol Rules

- **Init state machine** — three states, strict gating:
  - **Uninitialized**: only `initialize` and `ping` allowed; everything else returns `-32000` ("server not initialized").
  - **Initializing**: only `ping` allowed; everything else returns `-32000`.
  - **Ready**: all methods allowed.
- `initialize` duplicate → `-32000`. `notifications/initialized` outside `initializing` → silently ignore. `ping` always works.
- **Notifications**: a message without `id` is a notification — **never** respond, **never** log unknown ones.
- **`id`** is `json.RawMessage` — preserve original type (string / number / null) and echo back exact bytes.
- **`params`**: when absent or `null`, default to `{}` before unmarshaling.
- **Error code map**:
  - `-32700` parse error / size limit exceeded
  - `-32600` invalid request / wrong jsonrpc version / params not an object
  - `-32601` method not found / `rpc.*` reserved methods
  - `-32602` invalid params / unknown tool name / wrong types
  - `-32603` internal error (should not happen in normal operation)
  - `-32000` server error (state prevents processing)
  - `-32001` server timeout (handler timed out / cancelled)
- **Error messages must be contextual** (`"unknown tool: foo"`, not `"invalid params"`).
- **No JSON-RPC batches**: a top-level JSON array → `-32700`.
- **Newline-delimited JSON only** — no LSP framing.
- **stdout is protocol-only** — every byte must be a valid JSON-RPC message. No logs, no banners, no debug. **stderr** is for `slog` exclusively.
- **EOF**: `io.EOF` / `io.ErrUnexpectedEOF` → exit 0 (clean shutdown). All other decode errors → exit 1.
- **Per-message size cap**: 4 MB via counting reader. Do **not** use a cumulative `io.LimitReader`.
- **Sequential dispatch**: server advertises `experimental.concurrency.maxInFlight: 1`. Don't introduce concurrent dispatch.
- **Handler timeout**: 30s → `-32001`.
- **Capabilities**: server auto-advertises `resources` / `prompts` only when their registries are wired via `server.WithResources(...)` / `server.WithPrompts(...)`.
- **Bidi (`protocol.SendRequest`)**:
  - **AI9 capability gate is the FIRST statement** on outbound paths — `sampling/*`, `elicitation/*`, `roots/*` return `*protocol.CapabilityNotAdvertisedError` with **zero side effects** when the client did not advertise.
  - Plumb the server as `protocol.Peer` via `protocol.ContextWithPeer` at dispatch.
  - Handler packages must **not** import `internal/server` (Invariant **I1**).
  - **AI7** cancel chain, **AI8** typed errors, **AI10** no progress notifications during outbound requests.
- **Not implemented (return `-32601`)**: `resources/subscribe`, `resources/unsubscribe`, `completion/complete`, `roots/list`, server-hosted `sampling/*`, `elicitation/*`. No `*/list_changed` notifications today (planned v1.4.0).
- **`tools/list` ordering must be deterministic** (alphabetical by tool name) — golden tests pin this.

### Testing Rules

- **Naming**: `Test_<Unit>_With_<Condition>_Should_<Outcome>`. Fuzz: `Fuzz_<Unit>_<Aspect>`.
- **Structure**: `// Arrange` / `// Act` / `// Assert`. Every test calls `t.Parallel()`.
- **Package**: black-box (`package foo_test`) by default; white-box only for unexported internals (document why).
- **Assertions**: `assert.That(t, "description", got, expected)` from `internal/assert`. No `reflect.DeepEqual` ad hoc, no `testify`.
- **I/O fixtures**: inject `bytes.Buffer` for stdin/stdout/stderr. Write JSON-RPC requests + EOF, run server, read responses from output buffer.
- **Golden tests**: byte-for-byte JSON comparison for protocol correctness. Update only via deliberate review — never auto-regen as a fix.
- **Fuzz**: keep all 5 targets in `internal/protocol` green. `make fuzz` invokes `Fuzz_Decoder_With_ArbitraryInput` for `FUZZTIME` (default 30s).
- **Race detector**: `go test -race ./...` is mandatory; never skip.
- **Integration tests**: `//go:build integration`. Run via `go test -race ./... -tags=integration`. Drive through the full server.
- **Coverage**: 90% threshold via `make coverage`. CI fails below.
- **Bug fix discipline**: reproduce with a failing test FIRST (RED) → fix minimally (GREEN) → refactor only after green.
- **Never weaken or delete a test assertion** to make a build pass.
- **Deterministic ordering** required in tests that compare list outputs (e.g., `tools/list`).

### Code Quality & Style Rules

- **Lint must be 0 issues** (`golangci-lint run ./...`). **Never** modify `.golangci.yml` to suppress findings — fix the code.
- **`//nolint` is forbidden** without first fixing the root cause; if truly unavoidable, scope it tightly with a code reference comment.
- **Formatting**: `gofumpt` + `goimports` (local-prefix `github.com/andygeiss/mcp`). Don't fight goimports.
- **Declaration ordering**:
  - Files: alphabetical where practical; logical grouping for state machines / dispatch.
  - Types: `NewTypeName` constructor first after its type, then methods alphabetical.
  - `case` clauses: alphabetical in `switch`.
  - YAML/Make: schema-defined top-level keys keep tool's canonical order (e.g., GitHub Actions: `name → on → permissions → jobs`); user-defined keys within those (job names, permission names, linter lists, env vars, Makefile targets after the default) are alphabetical. Steps remain sequential. Value lists (`goos`, `goarch`, linter enable lists) are alphabetical.
- **Naming**: Go idioms (CamelCase exported, lowerCamel unexported). JSON tag names match MCP-spec camelCase.
- **No `utils` / `helpers` / `common` packages.** Place code in the package that owns the abstraction.
- **No premature interfaces**: introduce only at the smallest seam that needs them. Interfaces live in the consumer package.
- **No dead code, no half-finished implementations.** Don't add features ahead of need.
- **Comments default to none.** Only when the WHY is non-obvious (constraint, invariant, surprising edge case). Never describe WHAT the code does. Never reference current task / fix / callers — that belongs in PR descriptions.

### Development Workflow Rules

- **Quality gate before "done"**: `make check` (build + test + lint) green. That is `go build ./... && go test -race ./... && golangci-lint run ./...`.
- **Coverage gate**: `make coverage` ≥ 90%.
- **Smoke test**: `make smoke` (FR5a) after wiring a new tool — verifies `initialize` + `tools/list`.
- **Fuzz before significant decoder/protocol changes**: `make fuzz` (or `FUZZTIME=2m make fuzz` for confidence).
- **Hooks**: `make setup` configures `core.hooksPath = .githooks`. **Never** `--no-verify`. Never bypass signing.
- **Branching**: `main` is protected. **Never push directly to `main`.** **Never force-push** shared branches.
- **PRs**: every change goes through a PR. CI must be green: build, race-tests, lint, coverage threshold.
- **Releases**: tagged via goreleaser; binaries cosign-signed, SBOM + SLSA L3 attestations attached. Don't release manually.
- **Agentic loop**: perceive → act (RED then GREEN) → verify (`make check`) → iterate. **3 consecutive failures without progress → stop and explain.** **2 failed corrections on the same error → re-plan before retrying.**
- **Never modify** `.golangci.yml`, test assertions, or `go.mod` deps to make CI green. Fix root cause.
- **Adding a tool / resource / prompt**: follow the wiring pattern in `cmd/mcp/main.go`. Schemas are reflection-derived via `internal/schema` from struct tags — do not hand-author JSON Schema. Unit-test handlers in isolation; integration-test through the full server.

### Critical Don't-Miss Rules

**ALWAYS**

- Echo request `id` exactly as `json.RawMessage` (preserve string / number / null type).
- `go.mod` is the only source of truth for the Go version.
- `internal/protocol/constants.go` is the only source of truth for `MCPVersion`.
- Default `params` to `{}` when absent or `null` before unmarshaling.
- Return deterministic ordering from `tools/list` (alphabetical by name).
- Wrap errors with `%w` and a contextual prefix.
- Use `io.Reader` / `io.Writer` injection in constructors, not `*os.File`.
- Run `make check` before declaring any task complete.
- Write a failing test first for bug fixes.

**NEVER**

- Write **anything** non-protocol to **stdout** (banners, logs, prompts, debug). stdout is protocol-only.
- Use `bufio.Scanner` for protocol input — use `json.NewDecoder`.
- Use `omitzero` JSON tags — project standardizes on `omitempty`.
- Set `GOEXPERIMENT=jsonv2`; the codec relies on v1 behavior.
- Add an external dependency to `go.mod`.
- Modify `.golangci.yml` to suppress a finding.
- Add `//nolint` without fixing the underlying issue.
- Skip commit hooks (`--no-verify`) or bypass signing.
- Push directly to `main` or force-push a shared branch.
- Delete or weaken a test assertion to make a build pass.
- Continue feature work while a quality gate is failing.
- Respond to JSON-RPC notifications (messages without `id`) or log unknown ones.
- Inline protocol constants (always reference `internal/protocol/constants.go`).
- Accept JSON-RPC batch arrays (return `-32700`).
- Emit progress notifications during outbound bidi requests (Invariant **AI10**).
- Cause side effects on outbound bidi paths before the **AI9** capability check passes.
- Import `internal/server` from `internal/tools`, `internal/prompts`, `internal/resources`, or `internal/schema` (Invariant **I1** — use `protocol.Peer` instead).
- Cite gitignored paths (`_bmad-output/`, `_bmad/`, `.claude/`) from checked-in docs or ADRs.

**Edge cases the codec must handle correctly (regression watchlist)**

- Numeric vs string `id` round-trip — echo back exact bytes.
- `params` absent / `null` / `{}` produce identical handler input.
- Per-message size cap (4 MB) trips `-32700` without poisoning the decoder.
- EOF mid-message → `io.ErrUnexpectedEOF` → exit 0.
- Duplicate `initialize` → `-32000`, no state change.
- `notifications/initialized` outside `initializing` → silently dropped, no error response.
- Unknown notification → silently dropped.

---

## Usage Guidelines

**For AI Agents:**

- Read this file before implementing any code. Treat it as load-bearing.
- Follow ALL rules exactly as documented.
- When in doubt, prefer the more restrictive option.
- The codebase is the ultimate authority — if this file conflicts with current code, fix the file.

**For Humans:**

- Keep this file lean and focused on agent needs.
- Update when the technology stack, protocol version, or invariants change.
- Review periodically; remove rules that have become obvious.
- A doc-lint gate (`make doc-lint`) ensures checked-in docs do not cite gitignored paths — keep that gate green.
