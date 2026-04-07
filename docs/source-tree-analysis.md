# Source Tree Analysis

**Project:** mcp
**Generated:** 2026-04-07

## Directory Tree

```
mcp/
├── .github/
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.md            # Bug report template
│   │   └── feature_request.md       # Feature request template
│   ├── workflows/
│   │   ├── ci.yml                   # Build + test + fuzz + lint on PR/push
│   │   ├── codeql.yml               # CodeQL static analysis (weekly + PR)
│   │   ├── fuzz.yml                 # Nightly fuzz (all targets, 5m each)
│   │   ├── release.yml              # GoReleaser on version tags
│   │   └── scorecard.yml            # OpenSSF Scorecard (weekly)
│   ├── PULL_REQUEST_TEMPLATE.md     # PR checklist
│   └── dependabot.yml               # Weekly updates for actions + gomod
├── cmd/
│   ├── init/                        # Template rewriter (self-deleting)
│   │   ├── main.go                  # ← Entry point: parse args, call rewriteProject
│   │   ├── rewrite.go               # Rewrite logic: go.mod, imports, text files, rename
│   │   ├── rewrite_test.go          # Unit tests for rewrite functions
│   │   ├── integration_test.go      # Integration: full rewrite in temp dir
│   │   └── template_consumer_test.go # Verifies template consumer workflow
│   └── mcp/                         # MCP server binary
│       ├── main.go                  # ← Entry point: signal setup, registry wiring, server.Run
│       ├── integration_test.go      # Full server integration tests
│       └── signal_test.go           # Signal handling tests
├── internal/
│   ├── pkg/
│   │   └── assert/
│   │       ├── assert.go            # assert.That[T] — generic deep-equal helper
│   │       └── assert_test.go       # Self-tests for the assertion helper
│   ├── protocol/
│   │   ├── codec.go                 # Decode, Validate, Encode, response builders
│   │   ├── constants.go             # Error codes, method names, MCP version
│   │   ├── message.go               # Request, Response, Error, CodeError types
│   │   ├── benchmark_test.go        # Codec benchmarks
│   │   ├── fuzz_test.go             # Fuzz targets for decoder
│   │   └── protocol_test.go         # Codec unit tests
│   ├── server/
│   │   ├── server.go                # Server struct, Run loop, dispatch, lifecycle
│   │   ├── counting_reader.go       # Per-message size limit enforcement
│   │   ├── architecture_test.go     # Dependency direction tests
│   │   ├── claudemd_test.go         # CLAUDE.md self-consistency tests
│   │   ├── conformance_test.go      # MCP protocol conformance suite
│   │   ├── counting_reader_internal_test.go # White-box counting reader tests
│   │   ├── example_test.go          # Example tests for documentation
│   │   ├── fuzz_test.go             # Server-level fuzz tests
│   │   ├── integration_test.go      # Server integration tests
│   │   ├── io_test.go               # I/O behavior tests
│   │   ├── server_test.go           # Core server unit tests
│   │   ├── stdout_test.go           # Stdout protocol-only enforcement
│   │   └── synctest_test.go         # Synchronization tests
│   └── tools/
│       ├── annotations.go           # Annotations type + WithAnnotations option
│       ├── echo.go                  # Echo tool handler
│       ├── open_other.go            # O_NOFOLLOW stub for non-Unix
│       ├── open_unix.go             # O_NOFOLLOW for Unix (symlink protection)
│       ├── registry.go              # Registry, Register[T], Tool types
│       ├── schema.go                # Reflection-based JSON Schema derivation
│       ├── search.go                # Search tool handler (file grep)
│       ├── validate.go              # Input validation (path, string)
│       ├── annotations_test.go      # Annotations tests
│       ├── benchmark_test.go        # Registry benchmarks
│       ├── echo_test.go             # Echo tool tests
│       ├── example_test.go          # Example tests
│       ├── registry_test.go         # Registry unit tests
│       ├── schema_test.go           # Schema derivation tests
│       ├── search_internal_test.go  # White-box search tests
│       ├── search_test.go           # Search tool tests
│       ├── search_unix_internal_test.go # Unix-specific search tests
│       └── validate_test.go         # Validation tests
├── oss-fuzz/
│   ├── build.sh                     # OSS-Fuzz build script
│   ├── Dockerfile                   # OSS-Fuzz container
│   └── project.yaml                 # OSS-Fuzz project config
├── .gitignore
├── .golangci.yml                    # Lint config: 50+ linters, strict rules
├── .goreleaser.yml                  # Release: darwin/linux, amd64/arm64, cosign
├── .mcp.json                        # MCP server config for Claude Code
├── CLAUDE.md                        # AI-facing project instructions
├── CONTRIBUTING.md                  # Contribution guidelines
├── LICENSE                          # MIT
├── Makefile                         # build, check, coverage, fuzz, init, lint, test
├── README.md                        # Project documentation
├── SECURITY.md                      # Vulnerability reporting policy
└── go.mod                           # Module: github.com/andygeiss/mcp, Go 1.26
```

## Critical Directories

| Directory | Purpose | Key Files |
|---|---|---|
| `cmd/mcp/` | Server binary entry point | `main.go` — all wiring, no logic |
| `cmd/init/` | Template rewriter (for consumers) | `rewrite.go` — module path + binary rename |
| `internal/protocol/` | JSON-RPC 2.0 foundation | `codec.go`, `message.go`, `constants.go` |
| `internal/server/` | MCP server core | `server.go` — lifecycle, dispatch, resilience |
| `internal/tools/` | Tool registry + handlers | `registry.go`, `schema.go`, `echo.go`, `search.go` |
| `internal/pkg/assert/` | Test assertions | `assert.go` — single generic helper |
| `.github/workflows/` | CI/CD pipelines | 5 workflows: ci, codeql, fuzz, release, scorecard |

## Entry Points

| Entry Point | Binary | Purpose |
|---|---|---|
| `cmd/mcp/main.go` | `mcp` | MCP server — production binary |
| `cmd/init/main.go` | (go run) | Template rewriter — one-time use, self-deleting |

## File Metrics

| Category | Count |
|---|---|
| Production `.go` files | 14 |
| Test `.go` files | 24 |
| CI/CD workflow files | 5 |
| Config files | 7 |
| Documentation files | 5 |
