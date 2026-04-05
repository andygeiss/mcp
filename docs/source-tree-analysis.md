# Source Tree Analysis

**Project:** mcp
**Generated:** 2026-04-05

## Directory Tree

```
mcp/
├── .github/
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.md           # Bug report issue template
│   │   └── feature_request.md      # Feature request issue template
│   ├── PULL_REQUEST_TEMPLATE.md    # PR template
│   ├── dependabot.yml              # Automated dependency updates (Actions + gomod)
│   ���── workflows/
│       ├── ci.yml                  # CI: build, test, lint, fuzz on PR/push
│       ├── codeql.yml              # Weekly CodeQL static analysis
│       ├── fuzz.yml                # Nightly extended fuzz testing (5min/target)
│       ├── release.yml             # GoReleaser + Cosign on version tags
│       └── scorecard.yml           # OpenSSF Scorecard on push to main
├── cmd/
│   ├── init/                       # Template rewriter (self-deleting after use)
│   │   ├── main.go                 # CLI entry: parse args, call rewriteProject
│   │   ├── rewrite.go              # Module path rewriting, binary dir rename, cleanup
│   │   ├── rewrite_test.go         # Unit tests for rewrite logic
│   │   └── integration_test.go     # Integration tests for full rewrite pipeline
│   └── mcp/                        # ** Main binary entry point **
│       └── main.go                 # Wiring: signal handling, registry setup, server.Run
├── internal/
│   ├── pkg/
│   │   └── assert/                 # Test-only assertion helpers
│   │       ├── assert.go           # assert.That[T] generic deep-equal helper
│   │       └── assert_test.go
│   ├── protocol/                   # JSON-RPC 2.0 types and codec (zero internal deps)
│   │   ├── codec.go                # Decode, Validate, Encode, response constructors
│   │   ├── constants.go            # Error codes, MCP version, method constants
│   │   ├── message.go              # Request, Response, Error, CodeError types + constructors
│   │   ├── benchmark_test.go       # Performance benchmarks
│   │   ├── fuzz_test.go            # Fuzz_Decoder_With_ArbitraryInput (OSS-Fuzz target)
│   │   ├── protocol_test.go        # Comprehensive codec tests, golden tests, round-trips
│   │   └── testdata/fuzz/          # Fuzz corpus (300+ entries)
│   ├── server/                     # MCP server lifecycle and dispatch
│   │   ├── server.go               # Server struct, Run loop, dispatch, state machine, tool call
│   │   ├── counting_reader.go      # Per-message size limit enforcement
│   │   ├── architecture_test.go    # Import graph verification (dependency direction)
│   │   ├── claudemd_test.go        # CLAUDE.md claims have matching tests
│   │   ├── conformance_test.go     # Data-driven conformance tests from testdata/
│   │   ├── counting_reader_internal_test.go  # White-box countingReader tests
│   │   ├── example_test.go         # Testable examples
│   │   ├── fuzz_test.go            # Fuzz_Server_Pipeline (full server fuzzing)
│   │   ├── integration_test.go     # Full pipeline: init -> tools/call, panic, timeout
│   │   ├── io_test.go              # I/O edge cases: slow stdin, partial reads, stdout close
│   │   ├── server_test.go          # Unit tests: state machine, methods, errors, timeouts
│   │   ├── stdout_test.go          # Stdout purity: every byte is valid JSON-RPC
│   │   ├── synctest_test.go        # Deterministic concurrency tests via testing/synctest
│   │   └── testdata/conformance/   # Request/response JSONL files for conformance suite
│   └── tools/                      # Tool registry and handlers
│       ├── registry.go             # Registry, Register[T], Tool type, Result helpers
│       ├── schema.go               # Reflection-based InputSchema derivation
│       ├── search.go               # Search tool: file pattern matching with security
│       ├── validate.go             # Input validation: path traversal, null bytes, length
│       ├── open_unix.go            # O_NOFOLLOW for Unix
│       ├── open_other.go           # No-op flag for non-Unix
│       ├── benchmark_test.go       # Performance benchmarks
│       ├── example_test.go         # Testable examples
│       ├── registry_test.go        # Registry unit tests
│       ├── schema_test.go          # Schema derivation: all Go types, edge cases
│       ├── search_test.go          # Search tool: matches, limits, security, symlinks
│       ├── search_internal_test.go # White-box search internals
│       ├── search_unix_internal_test.go  # Unix-specific search tests
│       └── validate_test.go        # Validation: traversal, null bytes, length
├── oss-fuzz/                       # Google OSS-Fuzz integration
│   ├── Dockerfile                  # Pinned base image (hash)
│   ├── build.sh                    # Compile native Go fuzzer
│   └── project.yaml                # OSS-Fuzz project config (libfuzzer + ASAN)
├── .golangci.yml                   # Linter config: 50+ linters, strict rules
├── .goreleaser.yml                 # Release: darwin/linux, amd64/arm64, cosign, SBOM
├── .mcp.json                       # MCP server config for local development
├── CLAUDE.md                       # AI agent engineering instructions
├── CONTRIBUTING.md                 # Contributor guide: setup, testing, PR process
├── LICENSE                         # MIT License
├── Makefile                        # build, test, lint, fuzz, coverage, init targets
├── README.md                       # Project overview, quickstart, architecture summary
├── SECURITY.md                     # Security policy, vulnerability reporting
└── go.mod                          # Module: github.com/andygeiss/mcp, Go 1.26
```

## Critical Directories

| Directory | Purpose | Key Files |
|---|---|---|
| `cmd/mcp/` | Binary entry point | `main.go` -- all wiring, no business logic |
| `internal/protocol/` | Wire format | `codec.go`, `message.go`, `constants.go` |
| `internal/server/` | Server core | `server.go` (490 LOC), `counting_reader.go` |
| `internal/tools/` | Tool system | `registry.go`, `schema.go`, `search.go`, `validate.go` |
| `.github/workflows/` | CI/CD | 5 workflows: CI, fuzz, release, CodeQL, Scorecard |
| `oss-fuzz/` | Continuous fuzzing | Google OSS-Fuzz harness |

## Entry Points

| Entry Point | Purpose |
|---|---|
| `cmd/mcp/main.go` | MCP server binary (production) |
| `cmd/init/main.go` | Template rewriter (one-time use, self-deleting) |

## File Counts

| Category | Count |
|---|---|
| Production `.go` files | 13 |
| Test `.go` files | 20 |
| Fuzz corpus entries | 300+ |
| CI/CD workflow files | 5 |
| Documentation files | 5 (README, CLAUDE, CONTRIBUTING, SECURITY, LICENSE) |
