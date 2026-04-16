# Source Tree Analysis

## Directory Tree

```
mcp/
├── .github/
│   └── workflows/
│       ├── ci.yml              # Main CI: build, test, lint, fuzz, bench, integration, vulncheck
│       ├── codeql.yml          # GitHub CodeQL security analysis
│       ├── fuzz.yml            # Nightly fuzz testing (4 targets, 5m each)
│       ├── release.yml         # Release pipeline with cosign signing and SBOM
│       └── scorecard.yml       # OpenSSF Scorecard
├── .githooks/
│   └── pre-commit              # Git hook: runs tests and lint before commit
├── cmd/
│   ├── init/                   # Template rewriter (self-deletes after use)
│   │   ├── main.go             # Entry point: validates module path, calls rewriteProject
│   │   ├── rewrite.go          # Rewrites go.mod, imports, binary dir, runs tidy, cleans up
│   │   ├── integration_test.go # Integration: full template rewrite cycle
│   │   └── template_consumer_test.go # Integration: end-to-end consumer test
│   └── mcp/                    # Main binary entry point
│       ├── main.go             # Wiring: flags, I/O injection, signal context, os.Exit
│       ├── integration_test.go # Integration: full server lifecycle via stdin/stdout
│       ├── main_test.go        # Build tag verification
│       └── signal_test.go      # Integration: SIGINT/SIGTERM graceful shutdown (Unix only)
├── docs/                       # Generated project documentation
├── internal/
│   ├── assert/
│   │   └── assert.go           # assert.That(t, desc, got, want) -- test helper
│   ├── prompts/
│   │   ├── registry.go         # Prompt registry, Register[T], argument derivation
│   │   └── registry_test.go    # Unit tests for prompt registration and lookup
│   ├── protocol/
│   │   ├── codec.go            # DecodeMessage: classify, normalize params, reject batches
│   │   ├── constants.go        # Error codes, method names, MCP version, namespaces
│   │   ├── message.go          # Request, Response, Notification, Error, Decode, Encode, Validate
│   │   ├── codec_test.go       # Codec classification tests
│   │   ├── constants_test.go   # Constant value verification
│   │   ├── fuzz_test.go        # Fuzz_Decoder_With_ArbitraryInput (22 seed corpus entries)
│   │   └── message_test.go     # Encode/decode/validate golden tests
│   ├── resources/
│   │   ├── registry.go         # Resource/template registry, URI matching (RFC 6570 Level 1)
│   │   └── registry_test.go    # Unit tests for resource registration and lookup
│   ├── schema/
│   │   ├── schema.go           # Reflection-based JSON Schema derivation engine
│   │   └── schema_test.go      # Schema derivation tests for all supported types
│   ├── server/
│   │   ├── option.go           # Functional options: timeout, resources, prompts, trace, safety
│   │   ├── progress.go         # Progress notifier: Report(current, total), Log(level, logger, data)
│   │   ├── server.go           # Server: lifecycle, dispatch, tool call async, bidirectional
│   │   ├── benchmark_test.go   # Server benchmarks
│   │   ├── conformance_test.go # MCP conformance test suite (33 scenarios)
│   │   ├── fuzz_test.go        # Fuzz_Server_Pipeline
│   │   ├── integration_test.go # Integration: full server pipelines
│   │   ├── progress_test.go    # Progress/logging notification tests
│   │   ├── server_test.go      # Unit tests for dispatch, state machine, timeouts
│   │   └── stdout_test.go      # Integration: stdout protocol-only enforcement
│   └── tools/
│       ├── annotations.go      # Tool annotations (destructive, idempotent, readOnly, etc.)
│       ├── echo.go             # Built-in echo tool
│       ├── registry.go         # Tool registry, Register[T], Lookup, Names, Tools
│       ├── schema.go           # deriveSchema[T] -- struct-to-InputSchema via reflection
│       ├── validate.go         # ValidatePath, ValidateInput, unmarshalAndValidate
│       ├── annotations_test.go # Annotation option tests
│       ├── benchmark_test.go   # Tool benchmarks (5 targets)
│       ├── echo_test.go        # Echo handler tests
│       ├── fuzz_test.go        # Fuzz_ValidateInput, Fuzz_ValidatePath
│       ├── registry_test.go    # Registry registration and lookup tests
│       ├── schema_test.go      # Schema derivation tests
│       └── validate_test.go    # Input/path validation tests
├── testdata/
│   ├── benchmarks/
│   │   └── baseline.txt        # Benchmark baseline for regression detection
│   └── fuzz/                   # Fuzz corpus storage
├── .golangci.yml               # 54 linter rules, zero suppression policy
├── .goreleaser.yml             # Release: darwin/linux, cosign signing, SBOM
├── CLAUDE.md                   # AI agent instructions and engineering conventions
├── CONTRIBUTING.md             # Prerequisites, dev setup, testing, PR process
├── LICENSE                     # MIT
├── Makefile                    # build, test, lint, fuzz, bench, coverage, init, setup, check
├── README.md                   # Project overview, quickstart, architecture, protocol compliance
├── SECURITY.md                 # Vulnerability reporting and response timeline
└── go.mod                      # github.com/andygeiss/mcp, Go 1.26, zero dependencies
```

## Critical Directories

| Directory | Role | Key Files |
|---|---|---|
| `cmd/mcp/` | Binary entry point | `main.go` (44 lines, wiring only) |
| `cmd/scaffold/` | Template scaffold | `rewrite.go` (module rewriting, self-cleanup) |
| `internal/protocol/` | JSON-RPC 2.0 foundation | `message.go`, `codec.go`, `constants.go` |
| `internal/server/` | Core engine | `server.go` (~8K lines, lifecycle + dispatch) |
| `internal/tools/` | Tool system | `registry.go`, `echo.go`, `validate.go` |
| `internal/resources/` | Resource system | `registry.go` (URI templates, RFC 6570) |
| `internal/prompts/` | Prompt system | `registry.go` (argument derivation) |
| `internal/schema/` | Schema engine | `schema.go` (shared reflection-based derivation) |

## Per-Package Exports

### cmd/mcp
- `main()`, `run() error`
- `version` string (set via ldflags)

### cmd/scaffold
- `main()`, `run() error`
- `rewriteProject(dir, modulePath) error`

### internal/protocol
- Types: `Error`, `CodeError`, `Notification`, `Request`, `Response`, `IncomingMessage`
- Functions: `Decode`, `DecodeMessage`, `Validate`, `Encode`, `NullID`
- Error constructors: `ErrInternalError`, `ErrInvalidParams`, `ErrInvalidRequest`, `ErrMethodNotFound`, `ErrParseError`, `ErrServerError`, `ErrServerTimeout`
- `NewErrorResponse`, `NewErrorResponseFromCodeError`, `NewResultResponse`
- Constants: 7 error codes, 14 methods, 7 notifications, 4 namespace prefixes, `MaxConcurrentRequests`, `MCPVersion`

### internal/server
- Types: `Server`, `Progress`, `Option`
- Functions: `NewServer`, `Run`, `ProgressFromContext`, `SendRequestFromContext`
- Options: `WithHandlerTimeout`, `WithResources`, `WithPrompts`, `WithTrace`, `WithSafetyMargin`

### internal/tools
- Types: `Tool`, `Registry`, `Result`, `ContentBlock`, `InputSchema`, `OutputSchema`, `Property`, `Annotations`
- Functions: `NewRegistry`, `Register[T]`, `Lookup`, `Names`, `Tools`
- Options: `WithAnnotations`, `WithOutputSchema`, `WithTitle`
- Helpers: `TextResult`, `ErrorResult`, `StructuredResult`
- Validation: `ValidatePath`, `ValidateInput`

### internal/resources
- Types: `Resource`, `ResourceTemplate`, `Registry`, `Result`, `ContentBlock`
- Functions: `NewRegistry`, `Register`, `RegisterTemplate`, `Lookup`, `LookupTemplate`, `Resources`, `Templates`
- Helpers: `TextResult`, `BlobResult`, `WithMimeType`

### internal/prompts
- Types: `Prompt`, `Argument`, `Message`, `ContentBlock`, `Registry`, `Result`
- Functions: `NewRegistry`, `Register[T]`, `Lookup`, `Prompts`
- Helpers: `UserMessage`, `AssistantMessage`

### internal/schema
- Types: `InputSchema`, `OutputSchema`, `Property`
- Functions: `DeriveInputSchema[T]`
- Constants: `TypeString`, `TypeInteger`, `TypeNumber`, `TypeBoolean`, `TypeArray`, `TypeObject`

### internal/assert
- Functions: `That(t, description, got, expected)`

## Test Inventory

| Package | Unit Tests | Integration Tests | Fuzz Targets | Benchmarks |
|---|---|---|---|---|
| cmd/scaffold | -- | 2 | -- | -- |
| cmd/mcp | 1 | 2 | -- | -- |
| internal/protocol | 3 files | -- | 1 | 3 |
| internal/server | 2 files | 4 | 1 | 2 |
| internal/tools | 5 files | -- | 2 | 5 |
| internal/resources | 1 file | -- | -- | -- |
| internal/prompts | 1 file | -- | -- | -- |
| internal/schema | 1 file | -- | -- | -- |
| **Total** | **14 files** | **8 files** | **4** | **10** |

## Conformance Test Scenarios (33 total)

The `internal/server/conformance_test.go` covers the full MCP protocol surface including initialization handshake, state machine enforcement, tool dispatch, resource/prompt operations, progress notifications, logging, error codes, timeout handling, cancellation, and bidirectional transport.

---

*Generated: 2026-04-11 | Scan level: exhaustive*
