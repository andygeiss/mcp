# Source Tree Analysis

## Directory Tree

```
mcp/
├── .github/
│   └── workflows/
│       ├── ci.yml              # Main CI: build, test, lint, fuzz, bench, integration, vulncheck
│       ├── codeql.yml          # GitHub CodeQL security analysis
│       ├── fuzz.yml            # Nightly fuzz testing
│       ├── release.yml         # Release pipeline with cosign signing and SBOM
│       └── scorecard.yml       # OpenSSF Scorecard
├── .githooks/
│   └── pre-commit              # Git hook: runs tests and lint before commit
├── cmd/
│   ├── mcp/                    # Main binary entry point
│   │   ├── main.go             # Wiring: flags, I/O injection, signal context, os.Exit
│   │   ├── integration_test.go # Integration: full server lifecycle via stdin/stdout
│   │   ├── main_test.go        # Entry point smoke tests
│   │   ├── run_test.go         # `run()` function unit tests
│   │   └── signal_test.go      # Integration: SIGINT/SIGTERM graceful shutdown (Unix only)
│   └── scaffold/               # Template rewriter (self-deletes after use)
│       ├── main.go             # Entry point: validates module path, calls rewriteProject
│       ├── rewrite.go          # Rewrites go.mod, imports, binary dir, runs tidy, cleans up
│       ├── integration_test.go # Integration: full template rewrite cycle
│       ├── main_test.go        # Scaffold entry-point tests
│       ├── rewrite_test.go     # Unit tests for rewrite logic
│       └── template_consumer_test.go # Integration: end-to-end consumer test
├── docs/                       # Generated project documentation
│   └── adr/                    # Architecture Decision Records
│       ├── ADR-001-stdio-ndjson-transport.md
│       ├── ADR-002-internal-package-layout.md
│       └── ADR-003-bidi-reader-split.md
├── internal/
│   ├── assert/
│   │   ├── assert.go           # assert.That(t, desc, got, want) -- test helper
│   │   └── assert_test.go
│   ├── prompts/
│   │   ├── registry.go         # Prompt registry, Register[T], argument derivation
│   │   └── registry_test.go    # Unit tests for prompt registration and lookup
│   ├── protocol/
│   │   ├── capabilities.go     # Capability enum, ClientCapabilities struct, Has(name)
│   │   ├── codec.go            # DecodeMessage: classify request/notification/response, normalize params, reject batches
│   │   ├── constants.go        # Error codes, method names, MCP version, namespaces, MaxJSONDepth
│   │   ├── errors.go           # CodeError, ErrServerShutdown, ErrPendingRequestsFull, CapabilityNotAdvertisedError, ClientRejectedError
│   │   ├── message.go          # Request, Response, Notification, Error, NewResultResponse, NewErrorResponse
│   │   ├── peer.go             # Peer interface, ContextWithPeer, SendRequest wrapper, MethodCapability
│   │   ├── benchmark_test.go   # Encode/Decode microbenchmarks
│   │   ├── capabilities_test.go
│   │   ├── fuzz_test.go        # Fuzz_Decoder_With_ArbitraryInput
│   │   ├── peer_test.go
│   │   ├── protocol_test.go    # Golden tests for encode/decode/validate
│   │   └── testdata/fuzz/      # Seed corpus
│   ├── resources/
│   │   ├── registry.go         # Resource/template registry, URI matching (RFC 6570 Level 1)
│   │   ├── fuzz_test.go        # Fuzz_LookupTemplate_With_ArbitraryInputs
│   │   └── registry_test.go
│   ├── schema/
│   │   ├── schema.go           # Reflection-based JSON Schema derivation engine
│   │   ├── has_option_internal_test.go
│   │   └── schema_test.go
│   ├── server/
│   │   ├── server.go           # Lifecycle, Run loop, Peer.SendRequest, pending map, A7 outbound cancel
│   │   ├── dispatch.go         # Routing, handleNotification, errorResponse, sendNotification, encodeResponse
│   │   ├── decode.go           # runInFlight, runIdle, decodeResult plumbing, handleDecodeError
│   │   ├── inflight.go         # Async tool dispatch, executeToolCall, dispatchToolCall, panic recovery
│   │   ├── handlers.go         # handleMethod dispatch, toolsList, capabilityGuidance
│   │   ├── handlers_initialize.go # initialize: version negotiation, clientCaps snapshot
│   │   ├── handlers_logging.go # logging/setLevel (RFC 5424 → slog mapping)
│   │   ├── handlers_prompts.go # prompts/list, prompts/get with arg validation
│   │   ├── handlers_resources.go # resources/list, resources/read, resources/templates/list
│   │   ├── progress.go         # Progress notifier: Report(current, total), Log(level, logger, data)
│   │   ├── counting_reader.go  # Per-message 4 MB size enforcement
│   │   ├── architecture_test.go
│   │   ├── benchmark_test.go
│   │   ├── claudemd_test.go    # CLAUDE.md invariant guard tests
│   │   ├── conformance_test.go # MCP conformance runner (37 .request.jsonl scenarios)
│   │   ├── counting_reader_internal_test.go
│   │   ├── error_sanitize_internal_test.go
│   │   ├── example_test.go
│   │   ├── execute_internal_test.go
│   │   ├── fuzz_test.go        # Fuzz_Server_Pipeline
│   │   ├── integration_test.go # Full server pipelines
│   │   ├── io_test.go
│   │   ├── server_test.go      # Unit tests for dispatch, state machine, timeouts, bidi
│   │   ├── stdout_test.go      # Integration: stdout protocol-only enforcement
│   │   ├── synctest_test.go    # Deterministic bidi scenarios under synctest.Bubble
│   │   └── testdata/conformance/ # 37 MCP conformance scenario pairs
│   └── tools/
│       ├── _TOOL_TEMPLATE.go   # Copy-target for adding new tools (no working logic)
│       ├── annotations.go      # Tool annotations (destructive, idempotent, readOnly, etc.)
│       ├── echo.go             # Built-in echo tool -- "START HERE" anchor for scaffold users
│       ├── registry.go         # Tool registry, Register[T], Lookup, Names, Tools
│       ├── validate.go         # ValidatePath, ValidateInput, unmarshalAndValidate
│       ├── annotations_test.go
│       ├── benchmark_test.go   # Schema-derivation microbenchmarks
│       ├── echo_test.go
│       ├── example_test.go
│       ├── fuzz_test.go        # Fuzz_ValidateInput, Fuzz_ValidatePath
│       ├── registry_test.go
│       ├── schema_test.go      # Schema derivation tests for all supported types
│       ├── smoke_integration_test.go # `make smoke` backing test
│       └── validate_test.go
├── testdata/
│   └── benchmarks/
│       └── baseline.txt        # Benchmark baseline for regression detection
├── .golangci.yml               # Linter rules, zero suppression policy
├── .goreleaser.yml             # Release: darwin/linux, cosign signing, SBOM
├── CLAUDE.md                   # AI agent instructions and engineering conventions
├── CONTRIBUTING.md             # Prerequisites, dev setup, testing, PR process
├── LICENSE                     # MIT
├── Makefile                    # build, test, lint, fuzz, bench, coverage, init, setup, check, smoke
├── README.md                   # Project overview, quickstart, architecture, protocol compliance
├── SECURITY.md                 # Vulnerability reporting and response timeline
├── VERSIONING.md               # SemVer + Peer stability policy
└── go.mod                      # github.com/andygeiss/mcp, Go 1.26, zero dependencies
```

## Critical Directories

| Directory | Role | Key Files |
|---|---|---|
| `cmd/mcp/` | Binary entry point | `main.go` (48 lines, wiring only) |
| `cmd/scaffold/` | Template scaffold | `rewrite.go` (module rewriting, self-cleanup) |
| `internal/protocol/` | JSON-RPC 2.0 + Peer surface | `message.go`, `codec.go`, `constants.go`, `peer.go`, `capabilities.go`, `errors.go` |
| `internal/server/` | Core engine (11 source files) | `server.go`, `dispatch.go`, `decode.go`, `inflight.go`, `handlers_*.go`, `progress.go`, `counting_reader.go` |
| `internal/tools/` | Tool system | `registry.go`, `echo.go`, `validate.go`, `annotations.go`, `_TOOL_TEMPLATE.go` |
| `internal/resources/` | Resource system | `registry.go` (URI templates, RFC 6570) |
| `internal/prompts/` | Prompt system | `registry.go` (argument derivation) |
| `internal/schema/` | Schema engine | `schema.go` (shared reflection-based derivation) |

## Server Package File Split (ADR-002)

`internal/server/` uses file-level partitioning to keep each concern ≤300 LOC:

| File | LOC | Responsibility |
|---|---|---|
| `server.go` | 396 | `Server` struct, lifecycle, `Run`, `Peer.SendRequest`, pending map, A7 cancel |
| `inflight.go` | 290 | Async tool dispatch, `dispatchToolCall`, timeout + panic recovery |
| `decode.go` | 256 | Read loop (`runInFlight`, `runIdle`), decode-error handling |
| `dispatch.go` | 181 | Request routing, `handleNotification`, `errorResponse`, stdout serialization |
| `handlers_prompts.go` | 122 | `prompts/list`, `prompts/get` |
| `handlers_resources.go` | 112 | `resources/list`, `resources/read`, `resources/templates/list` |
| `handlers_initialize.go` | 109 | `initialize` with version negotiation + clientCaps snapshot |
| `handlers.go` | 93 | `handleMethod` switchboard, `toolsList`, capability guidance |
| `progress.go` | 73 | `Progress` notifier — `Report`, `Log` |
| `counting_reader.go` | 60 | Per-message 4 MB enforcement |
| `handlers_logging.go` | 49 | `logging/setLevel` with RFC 5424 → slog mapping |

## Per-Package Exports

### cmd/mcp
- `main()`, `run() error`
- `version` string (set via ldflags)

### cmd/scaffold
- `main()`, `run() error`
- `rewriteProject(dir, modulePath) error`

### internal/protocol
- **Types:** `Error`, `CodeError`, `Notification`, `Request`, `Response`, `IncomingMessage`, `Peer`, `Capability`, `ClientCapabilities`, `ElicitationCapability`, `RootsCapability`, `SamplingCapability`, `CapabilityNotAdvertisedError`, `ClientRejectedError`, `ErrorCode`
- **Functions:** `Decode`, `DecodeMessage`, `Validate`, `Encode`, `NullID`, `NewResultResponse`, `NewErrorResponse`, `NewErrorResponseFromCodeError`, `ContextWithPeer`, `PeerFromContext`, `SendRequest`, `MethodCapability`
- **Error constructors:** `ErrInternalError`, `ErrInvalidParams`, `ErrInvalidRequest`, `ErrMethodNotFound`, `ErrParseError`, `ErrResourceNotFound`, `ErrServerError`, `ErrServerTimeout`
- **Sentinels:** `ErrBatchNotSupported`, `ErrJSONDepthExceeded`, `ErrNoPeerInContext`, `ErrPendingRequestsFull`, `ErrServerShutdown`
- **Constants:** 8 error codes, 10 methods, 4 notifications, 6 namespace prefixes, `MaxConcurrentRequests`, `MaxJSONDepth`, `MCPVersion`, `Version`, 3 `Capability` values

### internal/server
- **Types:** `Server`, `Progress`, `Option`
- **Functions:** `NewServer`, `Run`, `SendRequest` (satisfies `protocol.Peer`), `ProgressFromContext`
- **Options:** `WithHandlerTimeout`, `WithInstructions`, `WithResources`, `WithPrompts`, `WithSafetyMargin`, `WithTrace`

### internal/tools
- **Types:** `Tool`, `Registry`, `Result`, `ContentBlock`, `Annotations`
- **Functions:** `NewRegistry`, `Register[T]`, `Lookup`, `Names`, `Tools`
- **Options:** `WithAnnotations`, `WithOutputSchema`, `WithTitle`
- **Helpers:** `TextResult`, `ErrorResult`, `StructuredResult`
- **Validation:** `ValidatePath`, `ValidateInput`

### internal/resources
- **Types:** `Resource`, `ResourceTemplate`, `Registry`, `Result`, `ContentBlock`
- **Functions:** `NewRegistry`, `Register`, `RegisterTemplate`, `Lookup`, `LookupTemplate`, `Resources`, `Templates`
- **Helpers:** `TextResult`, `BlobResult`, `WithMimeType`

### internal/prompts
- **Types:** `Prompt`, `Argument`, `Message`, `ContentBlock`, `Registry`, `Result`
- **Functions:** `NewRegistry`, `Register[T]`, `Lookup`, `Prompts`
- **Helpers:** `UserMessage`, `AssistantMessage`

### internal/schema
- **Types:** `InputSchema`, `OutputSchema`, `Property`
- **Functions:** `DeriveInputSchema[T]`
- **Constants:** `TypeString`, `TypeInteger`, `TypeNumber`, `TypeBoolean`, `TypeArray`, `TypeObject`

### internal/assert
- **Functions:** `That(t, description, got, expected)`

## Test Inventory

| Package | Test Files | Fuzz Targets | Benchmarks |
|---|---|---|---|
| cmd/mcp | 4 | — | — |
| cmd/scaffold | 4 | — | — |
| internal/assert | 1 | — | — |
| internal/prompts | 1 | — | — |
| internal/protocol | 5 | 1 | 6 |
| internal/resources | 2 | 1 | — |
| internal/schema | 2 | — | — |
| internal/server | 15 | 1 | 3 |
| internal/tools | 8 | 2 | 2 |
| **Total** | **42** | **5** | **11** |

- **Test functions:** 463 (`go test ./...`)
- **Integration-tagged files:** 10 (require `-tags=integration`)
- **Fuzz seed corpus:** `internal/protocol/testdata/fuzz/`

## Conformance Test Suite

`internal/server/conformance_test.go` runs a file-driven harness (`Test_Conformance_Runner`) over 37 request/response pairs in `internal/server/testdata/conformance/`. Coverage includes:

- Initialization handshake and state machine
- ID shapes (string, number, zero, large, null, negative, empty, invalid arrays/objects/booleans)
- Method validation (unknown, rpc.* reserved, empty)
- Notifications (cancelled-without-in-flight, initialized, unknown-silent-drop)
- Params shapes (absent, null, array positional, extra fields)
- Batch array rejection
- JSON-RPC version missing/wrong
- Bidi orphan-response scenarios for sampling, elicitation, roots (v1.3.0)
- Ping across states

## Project Metrics

| Metric | Value |
|---|---|
| Source files (non-test) | 29 |
| Test files | 42 |
| Total Go files | 71 |
| Non-test LOC | ~3,784 |
| Test LOC | ~13,029 |
| Packages | 9 |
| Go version | 1.26 |
| External dependencies | 0 |

---

*Generated: 2026-04-18 | Scan level: deep | Reflects v1.3.0*
