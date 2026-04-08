# Source Tree Analysis

## Directory Structure

```
mcp/
├── cmd/
│   ├── init/                         # Template rewriter — rewrites module path, renames binary dir, self-deletes
│   │   ├── integration_test.go       # Integration tests for the init rewrite workflow
│   │   ├── main.go                   # Entry point: parse args, call rewriteProject
│   │   ├── rewrite.go                # All rewrite operations: go.mod, imports, text files, rename, cleanup
│   │   ├── rewrite_test.go           # Unit tests for rewrite functions
│   │   └── template_consumer_test.go # Tests verifying the template consumer workflow
│   └── mcp/                          # MCP server binary — wiring only
│       ├── integration_test.go       # Integration tests for the compiled binary
│       ├── main.go                   # Entry point: flags, signal handling, I/O injection, os.Exit
│       ├── main_test.go              # Unit tests for main/run
│       └── signal_test.go            # Tests for SIGINT/SIGTERM graceful shutdown
├── internal/
│   ├── pkg/
│   │   └── assert/                   # Lightweight test assertion helpers (stdlib only)
│   │       ├── assert.go             # assert.That[T] — generic deep-equal assertion
│   │       └── assert_test.go        # Tests for the assert helper
│   ├── protocol/                     # JSON-RPC 2.0 types, codec, constants — zero internal deps
│   │   ├── benchmark_test.go         # Benchmarks for codec operations
│   │   ├── codec.go                  # Decode, Validate, Encode, response constructors
│   │   ├── constants.go              # Error codes, MCP version, method names, namespace prefixes
│   │   ├── fuzz_test.go              # Fuzz targets for the decoder
│   │   ├── message.go                # Request, Response, Error, CodeError types
│   │   └── protocol_test.go          # Unit tests for codec and message types
│   ├── server/                       # MCP server: lifecycle, dispatch, resilience
│   │   ├── architecture_test.go      # Tests verifying architectural constraints
│   │   ├── benchmark_test.go         # Benchmarks for server operations
│   │   ├── claudemd_test.go          # Self-test: verifies CLAUDE.md claims match code
│   │   ├── conformance_test.go       # Protocol conformance test suite
│   │   ├── counting_reader.go        # Per-message size limiter (4 MB)
│   │   ├── counting_reader_internal_test.go  # White-box tests for counting reader
│   │   ├── example_test.go           # Testable examples for documentation
│   │   ├── fuzz_test.go              # Server-level fuzz targets
│   │   ├── integration_test.go       # Full pipeline integration tests (build tag)
│   │   ├── io_test.go                # I/O edge case tests (EOF, partial reads)
│   │   ├── server.go                 # Server struct, Run loop, dispatch, tool execution
│   │   ├── server_test.go            # Unit tests for server lifecycle and dispatch
│   │   ├── stdout_test.go            # Tests verifying stdout protocol purity
│   │   └── synctest_test.go          # Synchronization tests for concurrent dispatch
│   └── tools/                        # Tool registry, schema derivation, handlers
│       ├── annotations.go            # Tool annotations (destructive, idempotent, etc.)
│       ├── annotations_test.go       # Tests for annotation options
│       ├── benchmark_test.go         # Benchmarks for schema derivation
│       ├── echo.go                   # Echo tool — minimal reference implementation
│       ├── echo_test.go              # Tests for the echo tool
│       ├── example_test.go           # Testable examples
│       ├── fuzz_test.go              # Fuzz targets for schema derivation
│       ├── registry.go               # Registry, Register[T], Tool/Result types
│       ├── registry_test.go          # Tests for registry operations
│       ├── schema.go                 # Reflection-based JSON Schema derivation
│       ├── schema_test.go            # Tests for schema derivation
│       ├── validate.go               # Input validation helpers (path, string)
│       └── validate_test.go          # Tests for validation functions
├── oss-fuzz/                         # OSS-Fuzz integration harness
│   ├── build.sh                      # Build script for OSS-Fuzz
│   ├── Dockerfile                    # Container for OSS-Fuzz builds
│   └── project.yaml                  # OSS-Fuzz project metadata
├── testdata/                         # Test fixtures and baselines
│   └── benchmarks/
│       └── baseline.txt              # Benchmark baseline for regression detection
├── .github/
│   ├── ISSUE_TEMPLATE/               # Bug report and feature request templates
│   ├── PULL_REQUEST_TEMPLATE.md      # PR template
│   ├── dependabot.yml                # Dependency update configuration
│   └── workflows/
│       ├── ci.yml                    # CI pipeline: build, test, lint, fuzz, bench, integration
│       ├── codeql.yml                # CodeQL static analysis (weekly + on PR/push)
│       ├── fuzz.yml                  # Nightly fuzz testing (5m per target)
│       ├── release.yml               # GoReleaser + cosign + SBOM
│       └── scorecard.yml             # OpenSSF Scorecard
├── .githooks/
│   └── pre-commit                    # Pre-commit hook: runs make check
├── .gitignore
├── .golangci.yml                     # golangci-lint v2 config (60+ linters enabled)
├── .goreleaser.yml                   # Release config: darwin/linux, amd64/arm64
├── .mcp.json                         # MCP client config for local development
├── CLAUDE.md                         # AI agent instructions and project conventions
├── CONTRIBUTING.md                   # Contributor guide
├── LICENSE                           # MIT license
├── Makefile                          # Build automation: check, test, lint, fuzz, bench, coverage
├── README.md                         # Project overview and quickstart
├── SECURITY.md                       # Security policy and vulnerability reporting
├── coverage.out                      # Test coverage report (generated)
└── go.mod                            # Go 1.26, zero external dependencies
```

## Critical Directories

| Directory | Purpose | Key Files |
|---|---|---|
| `cmd/mcp/` | Server binary entry point | `main.go` — wiring only: flags, signal setup, I/O injection |
| `cmd/init/` | Template rewriter (self-deleting) | `rewrite.go` — module path substitution, dir rename |
| `internal/protocol/` | JSON-RPC 2.0 wire format | `codec.go`, `message.go`, `constants.go` |
| `internal/server/` | Server lifecycle and dispatch | `server.go` — 863 lines, the core of the project |
| `internal/tools/` | Tool registry and schema | `registry.go`, `schema.go` — reflection-based schema derivation |
| `internal/pkg/assert/` | Test assertion helpers | `assert.go` — single generic function |
| `oss-fuzz/` | Continuous fuzzing harness | `build.sh` — OSS-Fuzz build integration |
| `.github/workflows/` | CI/CD pipelines | 5 workflows covering build, test, fuzz, release, security |

## Entry Points

| Entry Point | File | Description |
|---|---|---|
| MCP Server | `cmd/mcp/main.go` | Primary binary — parses `--version`, sets up signal handling, creates registry, runs server |
| Template Init | `cmd/init/main.go` | One-time tool — rewrites module path in all files, renames dirs, self-deletes |
