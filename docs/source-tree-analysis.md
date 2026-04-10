# Source Tree Analysis

## Directory Structure

```
mcp/
├── cmd/                          # CLI entry points
│   ├── mcp/                      # ★ MCP server binary
│   │   ├── main.go               # Entry point: flags, I/O injection, signal handling, os.Exit
│   │   ├── main_test.go          # Integration test setup (builds binary)
│   │   ├── integration_test.go   # Version flag, EOF, malformed JSON tests
│   │   └── signal_test.go        # SIGINT/SIGTERM clean shutdown tests (unix only)
│   └── init/                     # Template rewriter (not part of normal builds)
│       ├── main.go               # Entry point: validates args, calls rewriteProject
│       ├── rewrite.go            # Module path rewriting, directory rename, self-cleanup
│       ├── rewrite_test.go       # Unit tests for rewrite functions
│       ├── integration_test.go   # Full init pipeline test
│       ├── template_consumer_test.go  # End-to-end template consumption test
│       └── testdata/
│           └── greet.go.fixture  # Fixture for template consumer test
│
├── internal/                     # Private packages (Go visibility boundary)
│   ├── assert/                   # Test assertion helpers
│   │   ├── assert.go             # assert.That[T] — generic deep-equal assertion
│   │   └── assert_test.go        # Black-box tests with fakeTB mock
│   │
│   ├── protocol/                 # JSON-RPC 2.0 codec and types (zero internal deps)
│   │   ├── codec.go              # Decode, Validate, Encode, response constructors
│   │   ├── constants.go          # Error codes, MCP methods, namespace prefixes
│   │   ├── message.go            # Request, Response, Error, CodeError types
│   │   ├── protocol_test.go      # 48 unit tests (decode, encode, validate, round-trip)
│   │   ├── benchmark_test.go     # 6 benchmarks (decode/encode performance)
│   │   ├── fuzz_test.go          # Fuzz_Decoder_With_ArbitraryInput (23 seeds)
│   │   └── testdata/fuzz/        # Fuzz corpus (auto-generated findings)
│   │
│   ├── server/                   # MCP server lifecycle and dispatch
│   │   ├── server.go             # ★ Core: Run loop, state machine, async dispatch, timeouts
│   │   ├── counting_reader.go    # Per-message 4MB size limit enforcement
│   │   ├── server_test.go        # 70+ unit tests (protocol, lifecycle, tools, errors)
│   │   ├── integration_test.go   # Full pipeline, panic recovery, timeout, oversized
│   │   ├── conformance_test.go   # 33 conformance scenarios from testdata/
│   │   ├── synctest_test.go      # Deterministic concurrency tests (testing/synctest)
│   │   ├── io_test.go            # Slow stdin, partial reads, stdout failure
│   │   ├── stdout_test.go        # Protocol purity (stdout = valid JSON-RPC only)
│   │   ├── architecture_test.go  # Import graph enforcement
│   │   ├── claudemd_test.go      # CLAUDE.md claims verification
│   │   ├── benchmark_test.go     # 3 benchmarks (echo, multi-call, ping)
│   │   ├── fuzz_test.go          # Fuzz_Server_Pipeline (full server fuzzing)
│   │   ├── example_test.go       # ExampleNewServer, Example_fullToolLifecycle
│   │   ├── counting_reader_internal_test.go  # White-box: exact/over/under limit
│   │   ├── error_sanitize_internal_test.go   # White-box: error message sanitization
│   │   └── testdata/conformance/ # 33 JSON-RPC conformance test scenarios
│   │       ├── README.md
│   │       ├── *.request.jsonl   # Input sequences
│   │       └── *.response.jsonl  # Expected outputs (optional)
│   │
│   └── tools/                    # Tool registry and schema derivation
│       ├── registry.go           # Registry, Register[T], Tool, Result types
│       ├── schema.go             # Reflection-based JSON Schema derivation
│       ├── validate.go           # ValidatePath, ValidateInput, unmarshalAndValidate
│       ├── echo.go               # Echo tool — reference implementation
│       ├── annotations.go        # Tool annotations (hints for MCP clients)
│       ├── registry_test.go      # Registration, lookup, ordering, required fields
│       ├── schema_test.go        # 28 schema derivation scenarios
│       ├── validate_test.go      # Path traversal, null bytes, length limits
│       ├── echo_test.go          # Echo handler tests
│       ├── annotations_test.go   # Annotation option pattern, JSON serialization
│       ├── example_test.go       # ExampleRegister (tool-author workflow)
│       ├── fuzz_test.go          # Fuzz_ValidatePath, Fuzz_ValidateInput
│       └── benchmark_test.go     # Schema derivation benchmarks
│
├── docs/                         # Generated project documentation
│   ├── index.md                  # ★ Master index — primary AI retrieval source
│   ├── project-overview.md       # Executive summary and quick reference
│   ├── architecture.md           # Technical architecture documentation
│   ├── source-tree-analysis.md   # This file
│   ├── development-guide.md      # Developer setup and workflow
│   ├── deployment-guide.md       # Build, release, CI/CD documentation
│   └── project-scan-report.json  # Scan state (for workflow resumption)
│
├── testdata/
│   └── benchmarks/
│       └── baseline.txt          # Benchmark baseline for regression detection
│
├── oss-fuzz/                     # OSS-Fuzz continuous fuzzing integration
│   ├── build.sh                  # Build script for OSS-Fuzz
│   ├── Dockerfile                # Container for local fuzz testing
│   └── project.yaml              # OSS-Fuzz project metadata
│
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                # CI: build, test, lint, fuzz, bench, integration, vulncheck
│   │   ├── codeql.yml            # CodeQL static security analysis (weekly + PR)
│   │   ├── scorecard.yml         # OpenSSF Scorecard security posture
│   │   ├── release.yml           # GoReleaser + cosign signing + SBOM generation
│   │   └── fuzz.yml              # Nightly fuzzing (4 targets, 5min each)
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.md         # Bug report template
│   │   └── feature_request.md    # Feature request template
│   ├── PULL_REQUEST_TEMPLATE.md  # PR checklist (race, lint, TDD, no deps)
│   └── dependabot.yml            # Weekly updates for actions + gomod
│
├── .githooks/
│   └── pre-commit                # Local quality gate (make check with lint fallback)
│
├── .golangci.yml                 # 45+ linters, strict config, depguard rules
├── .goreleaser.yml               # Cross-platform release: darwin/linux, amd64/arm64
├── .mcp.json                     # Claude Code MCP server config
├── CLAUDE.md                     # AI agent instructions and project conventions
├── CONTRIBUTING.md               # Developer onboarding, PR process, testing conventions
├── go.mod                        # Go 1.26, zero external dependencies
├── LICENSE                       # MIT — Andreas Geiss
├── Makefile                      # build, test, lint, check, fuzz, bench, coverage, init
├── README.md                     # Project overview, quickstart, architecture
└── SECURITY.md                   # Vulnerability reporting and response timeline
```

## Critical Directories

| Directory | Purpose | Key Files |
|---|---|---|
| `cmd/mcp/` | Server binary entry point | `main.go` — wiring only: flags, signal setup, I/O injection |
| `cmd/init/` | Template rewriter (self-deleting) | `rewrite.go` — module path substitution, dir rename |
| `internal/protocol/` | JSON-RPC 2.0 wire format | `codec.go`, `message.go`, `constants.go` |
| `internal/server/` | Server lifecycle and dispatch | `server.go` — core of the project (Run loop, state machine) |
| `internal/tools/` | Tool registry and schema | `registry.go`, `schema.go` — reflection-based schema derivation |
| `internal/assert/` | Test assertion helpers | `assert.go` — single generic function |
| `oss-fuzz/` | Continuous fuzzing harness | `build.sh` — OSS-Fuzz build integration |
| `.github/workflows/` | CI/CD pipelines | 5 workflows: ci, codeql, scorecard, release, fuzz |

## Entry Points

| Entry Point | File | Description |
|---|---|---|
| MCP Server | `cmd/mcp/main.go` | Primary binary — parses `--version`, sets up signal handling, creates registry, runs server |
| Template Init | `cmd/init/main.go` | One-time tool — rewrites module path in all files, renames dirs, self-deletes |

## Dependency Graph

```
cmd/mcp/ ──→ internal/server/ ──→ internal/protocol/
                    │
                    └──→ internal/tools/ ──→ internal/protocol/

internal/assert/    (test-only, zero internal deps)
internal/protocol/  (zero internal deps — foundation layer)
```

## Source File Details

### `cmd/mcp/`

| File | Exports | Purpose |
|---|---|---|
| `main.go` | `run() error`, `version` var | Entry point: `--version` flag, `signal.NotifyContext` for SIGINT/SIGTERM, registry creation, echo tool registration, `server.Run` |

### `cmd/init/`

| File | Key Functions | Purpose |
|---|---|---|
| `main.go` | `run() error` | Validates module path argument, calls `rewriteProject` |
| `rewrite.go` | `rewriteProject`, `rewriteGoMod`, `rewriteGoFiles`, `rewriteImportsInFile`, `rewriteTextFiles`, `renameBinaryDir`, `runGoModTidy`, `selfCleanup`, `verifyZeroFingerprint` | Complete project rebranding pipeline |

### `internal/assert/`

| File | Exports | Purpose |
|---|---|---|
| `assert.go` | `That[T any](tb testing.TB, desc string, got, want T)` | Generic deep-equal assertion using `reflect.DeepEqual` |

### `internal/protocol/`

| File | Exports | Purpose |
|---|---|---|
| `codec.go` | `Decode`, `Validate`, `Encode`, `NewErrorResponse`, `NewErrorResponseFromCodeError`, `NewResultResponse`, `NullID` | JSON-RPC 2.0 message codec with batch detection and params normalization |
| `constants.go` | `ParseError`, `InvalidRequest`, `InvalidParams`, `MethodNotFound`, `InternalError`, `ServerError`, `ServerTimeout`, `MCPVersion`, `MethodInitialize`, `MethodPing`, `MethodToolsCall`, `MethodToolsList`, `NotificationCancelled`, `NotificationInitialized`, `NamespaceCompletion`, `NamespaceElicitation`, `NamespacePrompts`, `NamespaceResources`, `PrefixRPC`, `MaxConcurrentRequests`, `Version` | All protocol-level constants |
| `message.go` | `Request`, `Response`, `Error`, `CodeError`, `ErrParseError`, `ErrInvalidRequest`, `ErrInvalidParams`, `ErrMethodNotFound`, `ErrInternalError`, `ErrServerError`, `ErrServerTimeout` | Wire types and typed error constructors |

### `internal/server/`

| File | Exports | Purpose |
|---|---|---|
| `server.go` | `Server`, `NewServer`, `Run`, `Option`, `WithHandlerTimeout`, `WithTrace`, `WithSafetyMargin` | Core server: 3-state lifecycle, async dispatch loop, timeout enforcement, panic recovery, error sanitization, cancellation support, counting reader integration |
| `counting_reader.go` | `newCountingReader`, `Read`, `Reset`, `Exceeded` (all unexported) | Per-message 4MB size limit via byte counting with reset between messages |

### `internal/tools/`

| File | Exports | Purpose |
|---|---|---|
| `registry.go` | `Registry`, `NewRegistry`, `Register[T]`, `Lookup`, `Names`, `Tools`, `Tool`, `Result`, `ContentBlock`, `InputSchema`, `Property`, `TextResult`, `ErrorResult`, `ContentTypeText`, `SchemaType*` constants | Tool registration, lookup (O(1) via index map), alphabetical ordering, type-safe handler wrapping |
| `schema.go` | (all unexported: `deriveSchema`, `collectFields`, `deriveProperty`, etc.) | Reflection-based JSON Schema derivation: primitives, slices, maps, nested structs, embedded struct promotion, pointer unwrapping, depth limit 10 |
| `validate.go` | `ValidatePath`, `ValidateInput`, `MaxInputLength` | Security validation: path traversal prevention, null byte detection, length limits (4096), JSON unmarshaling with `DisallowUnknownFields` |
| `echo.go` | `EchoInput`, `Echo` | Reference tool implementation — echoes input message back |
| `annotations.go` | `Annotations`, `Option`, `WithAnnotations` | Tool behavioral hints (destructive, idempotent, openWorld, readOnly, title) via functional option pattern |

## Test Infrastructure

| Category | Count | Location |
|---|---|---|
| Unit tests | 170+ | All `*_test.go` files |
| Integration tests | 8+ | `//go:build integration` tagged files |
| Conformance scenarios | 33 | `internal/server/testdata/conformance/` |
| Fuzz targets | 4 | protocol, server, tools (path + input) |
| Benchmarks | 11 | protocol (6), server (3), tools (2) |
| Examples | 3 | server (2), tools (1) |
| Architecture tests | 2 | Import graph + CLAUDE.md claims verification |

### Conformance Test Scenarios

33 data-driven scenarios in `internal/server/testdata/conformance/`:

| Scenario | Category | What It Tests |
|---|---|---|
| `batch-array-rejection` | Batching | JSON array rejected with `-32700` |
| `extra-fields` | Validation | Unknown JSON fields handled gracefully |
| `id-array-invalid` | ID handling | Array ID rejected |
| `id-boolean-invalid` | ID handling | Boolean ID rejected |
| `id-empty-string` | ID handling | Empty string ID accepted |
| `id-large-number` | ID handling | Large numeric ID accepted |
| `id-negative` | ID handling | Negative numeric ID accepted |
| `id-null` | ID handling | Null ID accepted (notification semantics) |
| `id-object-invalid` | ID handling | Object ID rejected |
| `id-string` | ID handling | String ID accepted |
| `id-zero` | ID handling | Numeric ID 0 accepted |
| `initialize-handshake` | State machine | Standard initialization sequence |
| `jsonrpc-version-missing` | Validation | Missing "jsonrpc" field rejected |
| `jsonrpc-version-wrong` | Validation | Wrong JSONRPC version rejected |
| `method-empty` | Validation | Empty method string rejected |
| `method-rpc-reserved` | Validation | Reserved "rpc." prefix rejected with `-32601` |
| `method-unknown` | Validation | Unknown method rejected with `-32601` |
| `notification-invalid-silently-dropped` | Notifications | Invalid notifications silently ignored |
| `notification-then-request` | Notifications | Notification followed by request handled correctly |
| `params-absent` | Params | Missing params field defaults to `{}` |
| `params-array-positional` | Params | Array params (positional) rejected |
| `params-null` | Params | Null params field treated as `{}` |
| `ping` | Ping | Ping request handled in various states |
| `state-duplicate-initialize` | State machine | Second initialize rejected with `-32000` |
| `state-ping-all-states` | State machine | Ping works in all lifecycle states |
| `state-request-during-initializing` | State machine | Requests rejected during initializing state |
| `state-uninitialized-method` | State machine | Non-ping requests before initialize rejected |
| `tools-call-empty-name` | Tools | Empty tool name rejected with `-32602` |
| `tools-call-success` | Tools | Successful tool call execution |
| `tools-call-unknown-tool` | Tools | Unknown tool rejected with `-32602` |
| `tools-list` | Tools | Returns all registered tools |
| `unsupported-capability-prompts` | Capabilities | prompts/* methods return `-32601` with guidance |
| `unsupported-capability-resources` | Capabilities | resources/* methods return `-32601` with guidance |

### Fuzz Targets

| Target | Package | Seeds | What It Tests |
|---|---|---|---|
| `Fuzz_Decoder_With_ArbitraryInput` | `protocol` | 23 | Decoder robustness — valid requests, batch arrays, incomplete JSON, unicode, large IDs, whitespace, deep nesting, scientific notation |
| `Fuzz_Server_Pipeline` | `server` | 10 | Full server pipeline — valid sequences, malformed JSON, edge cases, size limits, binary data, rapid-fire pings |
| `Fuzz_ValidatePath_With_ArbitraryInput` | `tools` | 8 | Path validator — valid paths, traversal variants, null bytes, excessive length, UTF-8 |
| `Fuzz_ValidateInput_With_ArbitraryInput` | `tools` | 7 | Input validator — simple strings, null bytes, excessive length, UTF-8, whitespace |

### Benchmarks

| Benchmark | Package | What It Measures |
|---|---|---|
| `Benchmark_Decode_SingleRequest` | `protocol` | Single tools/call decode |
| `Benchmark_Decode_InitializeRequest` | `protocol` | Initialize request decode |
| `Benchmark_Decode_LargeParams` | `protocol` | 1024-byte payload decode |
| `Benchmark_Decode_Notification` | `protocol` | Notification (no ID) decode |
| `Benchmark_Encode_SuccessResponse` | `protocol` | Success response encode |
| `Benchmark_Encode_ErrorResponse` | `protocol` | Error response encode |
| `Benchmark_RequestResponse_EchoTool` | `server` | Single tool call end-to-end |
| `Benchmark_RequestResponse_MultipleToolCalls` | `server` | 10 sequential tool calls |
| `Benchmark_RequestResponse_PingOnly` | `server` | Single ping request |
| `Benchmark_SchemaDerivation_With_SimpleStruct` | `tools` | 1-field schema derivation |
| `Benchmark_SchemaDerivation_With_ComplexStruct` | `tools` | 6-field mixed-type schema derivation |

## Configuration Files

| File | Purpose | Key Details |
|---|---|---|
| `go.mod` | Go module definition | Go 1.26, zero `require` directives |
| `.golangci.yml` | Linter config | v2 format, 45+ linters, depguard denies `encoding/json/v2` and `log`, forbidigo denies `fmt.Print*`, tagliatelle enforces camelCase JSON tags, testpackage enforces black-box tests |
| `.goreleaser.yml` | Release config | darwin/linux, amd64/arm64 cross-compilation |
| `.mcp.json` | MCP client config | `go run ./cmd/mcp/` — for Claude Code integration |
| `Makefile` | Build automation | Targets: bench, build, check, coverage, fuzz, init, lint, setup, test |
| `.githooks/pre-commit` | Pre-commit hook | Runs `make check`; falls back to `make build test` if lint unavailable |
| `.github/dependabot.yml` | Dependency updates | Weekly for github-actions and gomod, max 5 open PRs each |

---

*Generated: 2026-04-10 | Scan level: exhaustive*
