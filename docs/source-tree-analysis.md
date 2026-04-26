# Source Tree Analysis

**Repository:** `github.com/andygeiss/mcp`
**Type:** monolith (single Go module)
**LOC:** ~3,770 production / ~12,930 test (16,703 total across `cmd/` + `internal/`)
**Files:** 29 production `.go` + 42 test `.go`

---

## Top-level layout

```
mcp/
├── cmd/                    # Binary entry points
│   ├── mcp/                # Server binary (the thing you ship)
│   └── scaffold/           # Template rewriter (deleted by `make init`)
├── internal/               # All implementation (no exported API surface)
│   ├── assert/             # Test assertion helpers (test-only)
│   ├── prompts/            # Prompt registry, argument derivation
│   ├── protocol/           # JSON-RPC 2.0 codec, types, constants, Peer
│   ├── resources/          # Resource registry, URI templates
│   ├── schema/             # Reflection-based JSON Schema derivation
│   ├── server/             # Lifecycle, dispatch, bidi, notifications
│   └── tools/              # Tool registry, handlers
├── docs/                   # This documentation
├── oss-fuzz/               # OSS-Fuzz build harness
├── testdata/
│   └── benchmarks/         # benchstat baseline.txt
├── _bmad/                  # BMad agent workflow scaffolding
├── _bmad-output/           # Gitignored BMad artifacts (planning, implementation)
├── .github/
│   └── workflows/          # CI: ci.yml, codeql.yml, fuzz.yml, release.yml, scorecard.yml
├── .githooks/              # Git pre-commit hook scripts (enabled via `make setup`)
├── .claude/                # Claude Code skills, hooks, settings
├── CHANGELOG.md            # Keep a Changelog format
├── CLAUDE.md               # AI agent engineering philosophy
├── CONTRIBUTING.md         # Contributor guide
├── LICENSE                 # MIT
├── Makefile                # bench, build, check, coverage, fuzz, init, lint, setup, smoke, test
├── README.md               # User-facing introduction
├── SECURITY.md             # Security policy
├── VERSIONING.md           # SemVer policy
├── go.mod                  # Source of truth for Go version (1.26+)
├── .golangci.yml           # Lint config — read-only (no //nolint to silence)
├── .goreleaser.yml         # Release pipeline config
└── .gitignore              # Excludes _bmad-output/, docs/.archive/, build artifacts
```

## `cmd/` — entry points

### `cmd/mcp/main.go`

The binary. Wiring only — flags, signal handling, registry construction, server lifecycle. No business logic.

```
- Parse `--version` (prints version to stderr, exit 0)
- signal.NotifyContext on SIGINT/SIGTERM (Go 1.26 cancel cause)
- tools.NewRegistry() + tools.Register("echo", ...)
- server.NewServer(name, version, registry, os.Stdin, os.Stdout, os.Stderr,
    server.WithTrace(os.Getenv("MCP_TRACE") == "1"))
- srv.Run(ctx)
- os.Exit(1) on error
```

**Constraint:** when scaffolded via `make init`, the binary directory stays at `cmd/mcp/` so every fork produces a binary named `mcp`. Disambiguate via `go build -o <name>` if multiple servers share `$GOBIN`.

### `cmd/scaffold/`

Template rewriter. Not part of normal builds.

- `main.go` — CLI entry; refuses to run if working tree is dirty (`--force` to override).
- `rewrite.go` — rewrites all imports, repoints badge URLs (shields.io, codecov, Actions) at the new repo, runs `go mod tidy`, removes `cmd/scaffold/`, resets git history with an initial commit on `main`.

After `make init MODULE=github.com/yourorg/yourproject` succeeds, the welcome banner names the three steps: **Edit** (`internal/tools/echo.go`) → **Wire** (`cmd/mcp/main.go`) → **Verify** (`make smoke`).

## `internal/protocol/` — foundation (zero internal deps)

| File | Purpose |
|---|---|
| `codec.go` | `Decode` validates `jsonrpc=="2.0"`, balanced JSON depth ≤ 64, top-level type, batch rejection. `Encode` writes newline-delimited responses. |
| `message.go` | `Request`, `Response`, `Error` types. `id` is `json.RawMessage` for verbatim echo. |
| `constants.go` | `MCPVersion = "2025-11-25"`, `MaxConcurrentRequests = 1`, `MaxJSONDepth = 64`, error codes (`-32700`/`-32600`/`-32601`/`-32602`/`-32603`/`-32000`/`-32001`/`-32002`), method/notification/namespace name constants. |
| `capabilities.go` | `ClientCapabilities` (with `*SamplingCapability`, `*ElicitationCapability`, `*RootsCapability`), `Capability` enum (`CapSampling`, `CapElicitation`, `CapRoots`), nil-safe `Has(Capability)`. |
| `errors.go` | Sentinels: `ErrPendingRequestsFull`, `ErrServerShutdown`, `ErrNoPeerInContext`. Struct errors: `*CapabilityNotAdvertisedError` (Capability + Method), `*ClientRejectedError` (Code + Message + Data). |
| `peer.go` | `Peer` interface (`SendRequest(ctx, method, params) (*Response, error)`); `ContextWithPeer`/`PeerFromContext`; package-level `SendRequest` returning `ErrNoPeerInContext` if no peer attached; `MethodCapability(method) (Capability, bool)` for AI9. **v1.x stability surface per ADR-003.** |

## `internal/schema/` — reflection engine (zero internal deps)

`schema.go` derives JSON Schema from Go struct types using reflection. Computed once per `reflect.Type` and cached. Used by both `tools` and `prompts` to auto-derive input schemas from struct tags (`json`, `description`).

## `internal/tools/` — tool registry

| File | Purpose |
|---|---|
| `registry.go` | `Registry`, `NewRegistry`, generic `Register[T]`, `Lookup`, deterministic `List` ordering. |
| `validate.go` | Validates tool input against the derived schema before invoking the handler. |
| `annotations.go` | Tool annotation metadata. |
| `echo.go` | Reference handler: `// START HERE — your first tool` anchor, ~5 lines, copy-target shape. |
| `_TOOL_TEMPLATE.go` | `//go:build ignore` — annotated copy-target with required + optional field patterns; the leading `_` and build tag belt-and-suspenders the exclusion. |

Adding a new tool: define an input struct with `json` and `description` tags → write `func(ctx, T) Result` → call `tools.Register(registry, "name", "desc", handler)` in `cmd/mcp/main.go`.

## `internal/resources/` — resource registry

`registry.go` — `Register(uri, name, description, handler)` for static resources, `RegisterTemplate(template, ...)` for URI-template-backed resources. Pass to the server via `server.WithResources(registry)`. The `resources` capability is auto-advertised when configured.

## `internal/prompts/` — prompt registry

`registry.go` — `Register[T]` with reflection-derived argument schemas. Pass via `server.WithPrompts(registry)`. The `prompts` capability is auto-advertised when configured.

## `internal/server/` — lifecycle, dispatch, bidi (11 files)

| File | Purpose |
|---|---|
| `server.go` | `Server` type, three-state lifecycle, compile-time `Peer` assertion, `pendingEntry`, `outboundIDCounter atomic.Int64`, `clientCaps atomic.Pointer[ClientCapabilities]`, `maxPendingRequests = 1024`, `defaultHandlerTimeout = 30s`. |
| `dispatch.go` | Method routing, state-machine gating (`-32000` pre-init), `protocol.ContextWithPeer(callCtx, s)` injection at handler entry, A7 server→client cancel emission. |
| `decode.go` | Persistent `json.Decoder` on stdin behind a counting reader. |
| `counting_reader.go` | Per-message byte counting; **resets per message** (NOT cumulative `io.LimitReader`). |
| `inflight.go` | In-flight tracking, handler timeout, cancellation. |
| `progress.go` | `*Progress` accessed via context (`ProgressFromContext`); `Report`/`Log` are nil-safe; AI10 invariant — handlers must not invoke `Report` while parked in `protocol.SendRequest`. |
| `handlers.go` | Common handler infrastructure. |
| `handlers_initialize.go` | `initialize` handshake; snapshots client capabilities into `clientCaps`. |
| `handlers_logging.go` | `logging/setLevel` adjusts the `slog.LevelVar` for stderr. |
| `handlers_prompts.go` | `prompts/list`, `prompts/get`. |
| `handlers_resources.go` | `resources/list`, `resources/read`, `resources/templates/list`. |

**Invariants enforced by `.golangci.yml`:**
- **I1** — `internal/tools/**` and `internal/prompts/**` cannot import `internal/server` (`depguard.no-server-in-handlers`).
- **I4** — `os.Stdout` writes and `fmt.Fprint(os.Stdout, …)` patterns are banned in tool/prompt packages (`forbidigo`).

## `internal/assert/` — test-only

`assert.go` — single primitive: `assert.That(t, "description", got, expected)`. Stdlib only. **The project's only assertion library** — no testify, no gomega.

## Fuzz targets

5 fuzz targets across the codebase:

| Target | Package | Surface |
|---|---|---|
| `Fuzz_Decoder_With_ArbitraryInput` | `internal/protocol/` | JSON-RPC decoder; primary OSS-Fuzz target |
| `Fuzz_Server_Pipeline` | `internal/server/` | Full server pipeline |
| `Fuzz_LookupTemplate_With_ArbitraryInputs` | `internal/resources/` | URI template matching |
| `Fuzz_ValidateInput_With_ArbitraryInput` | `internal/tools/` | Tool input validation |
| `Fuzz_ValidatePath_With_ArbitraryInput` | `internal/tools/` | Path validation |

`make fuzz` runs `Fuzz_Decoder_With_ArbitraryInput` for 30s (`FUZZTIME=5m` to extend). OSS-Fuzz runs the corpus continuously upstream. Failing inputs become permanent regression seeds in `internal/protocol/testdata/fuzz/Fuzz_Decoder_With_ArbitraryInput/`.

## CI workflows (`.github/workflows/`)

| Workflow | Purpose |
|---|---|
| `ci.yml` | Build, test (race + integration), lint matrix across macOS/Linux/Windows |
| `codeql.yml` | GitHub Advanced Security CodeQL analysis |
| `fuzz.yml` | OSS-Fuzz corpus integration |
| `release.yml` | goreleaser pipeline — cosign signing, SBOM, SLSA L3 provenance |
| `scorecard.yml` | OpenSSF Scorecard scoring |

## Dependency direction (enforced)

```
cmd/mcp/  ──────►  server/  ──┬──►  protocol/      (foundation, zero deps)
                              ├──►  tools/         (handler packages)
                              ├──►  resources/     (handler packages)
                              ├──►  prompts/       (handler packages)
                              └──►  schema/        (used by tools + prompts)

tools/, resources/, prompts/  ──►  protocol/, schema/  (NEVER server)
```

Handler packages reach the bidi path via `protocol.SendRequest(ctx, ...)` and `protocol.ContextWithPeer` — **without** importing `internal/server` (Invariant I1).

## Critical files for an AI agent to read first

1. **`CLAUDE.md`** — engineering philosophy, build/test commands, conventions, guardrails.
2. **`_bmad-output/project-context.md`** — load-bearing rules sheet (gitignored; supplements CLAUDE.md).
3. **`go.mod`** — source of truth for Go version.
4. **`internal/protocol/constants.go`** — protocol version, error codes, method names.
5. **`cmd/mcp/main.go`** — wiring template; how a server is constructed.
6. **`internal/tools/echo.go`** — reference tool (`// START HERE`).
7. **`Makefile`** — canonical build/test/fuzz/smoke commands.
8. **`docs/architecture.md`** — system design (this folder).
