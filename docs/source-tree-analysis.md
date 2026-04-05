# Source Tree Analysis

**Project:** github.com/andygeiss/mcp
**Generated:** 2026-04-05

## Annotated Directory Tree

```
mcp/
├── cmd/
│   ├── init/                          # Template rewriter (self-deleting after use)
│   │   ├── integration_test.go        #   End-to-end init test (//go:build integration)
│   │   ├── main.go                    #   CLI entry point: parse args, call rewriteProject
│   │   ├── rewrite.go                 #   Core rewrite logic: go.mod, imports, text files, rename
│   │   └── rewrite_test.go            #   Unit tests for rewrite functions
│   └── mcp/                           # MCP server binary [ENTRY POINT]
│       └── main.go                    #   Wiring only: signals, registry, server.Run, os.Exit
│
├── internal/
│   ├── pkg/
│   │   └── assert/                    # Lightweight test assertion helpers
│   │       ├── assert.go              #   assert.That[T] -- generic deep equality
│   │       └── assert_test.go         #   Assertion helper verification
│   │
│   ├── protocol/                      # JSON-RPC 2.0 types, codec, constants [ZERO DEPS]
│   │   ├── benchmark_test.go          #   Codec performance: decode/encode benchmarks
│   │   ├── codec.go                   #   Decode, Validate, Encode, response constructors
│   │   ├── constants.go               #   Error codes, MCP methods, protocol version
│   │   ├── fuzz_test.go               #   Fuzz_Decoder with 19-entry seed corpus
│   │   ├── message.go                 #   Request, Response, Error, CodeError types
│   │   ├── protocol_test.go           #   33 unit tests: decode/encode/validate/round-trip
│   │   └── testdata/
│   │       └── fuzz/                  #   Fuzz corpus (auto-generated crash inputs)
│   │           └── Fuzz_Decoder_With_ArbitraryInput/
│   │               └── ...            #   ~300+ corpus entries
│   │
│   ├── server/                        # MCP server: lifecycle, dispatch, negotiation
│   │   ├── claudemd_test.go           #   Documentation validation: claims match tests
│   │   ├── counting_reader.go         #   Per-message 4MB size limiter
│   │   ├── counting_reader_internal_test.go  # White-box counting reader tests
│   │   ├── example_test.go            #   ExampleNewServer runnable example
│   │   ├── integration_test.go        #   Full pipeline integration tests
│   │   ├── io_test.go                 #   I/O robustness: slow/partial/closed streams
│   │   ├── server.go                  #   Core server: state machine, dispatch, timeouts
│   │   ├── server_test.go             #   35+ unit tests: all states, errors, timeouts
│   │   └── synctest_test.go           #   Deterministic concurrency tests (virtual time)
│   │
│   └── tools/                         # Tool registry, schema derivation, handlers
│       ├── benchmark_test.go          #   Schema derivation performance benchmarks
│       ├── example_test.go            #   ExampleRegister runnable example
│       ├── open_other.go              #   Non-Unix: openNoFollowFlag = 0
│       ├── open_unix.go               #   Unix: openNoFollowFlag = syscall.O_NOFOLLOW
│       ├── registry.go                #   Registry, Register[T], Tool, Result types
│       ├── registry_test.go           #   Registration, lookup, ordering tests
│       ├── schema.go                  #   Reflection-based JSON schema derivation
│       ├── schema_test.go             #   Schema derivation for all Go types
│       ├── search.go                  #   Search tool: regex file search with security
│       ├── search_internal_test.go    #   White-box matchFile tests
│       ├── search_test.go             #   Search functionality + security tests
│       └── search_unix_internal_test.go  # Unix-specific symlink/permission tests
│
├── oss-fuzz/                          # OSS-Fuzz integration
│   ├── build.sh                       #   Fuzz build script for oss-fuzz infrastructure
│   ├── Dockerfile                     #   OSS-Fuzz Docker build image
│   └── project.yaml                   #   OSS-Fuzz project config (libfuzzer, address sanitizer)
│
├── .github/
│   ├── dependabot.yml                 #   Weekly dependency updates (actions + gomod)
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.md              #   Bug report template with MCP request field
│   │   └── feature_request.md         #   Feature request template
│   ├── PULL_REQUEST_TEMPLATE.md       #   PR checklist: race, lint, TDD, no deps
│   └── workflows/
│       ├── ci.yml                     #   Build + test + fuzz + lint (on PR/push)
│       ├── fuzz.yml                   #   Nightly extended fuzz (5m per target)
│       ├── release.yml                #   GoReleaser + cosign + SBOM (on tag)
│       └── scorecard.yml              #   OpenSSF Scorecard (weekly + on push)
│
├── CLAUDE.md                          #   Engineering guide for AI agents
├── CONTRIBUTING.md                    #   Dev setup, testing, PR process
├── go.mod                             #   Module: github.com/andygeiss/mcp, Go 1.26
├── .golangci.yml                      #   48 linters, strict config
├── .goreleaser.yml                    #   Cross-platform release (darwin/linux, amd64/arm64)
├── LICENSE                            #   MIT license
├── Makefile                           #   check, build, test, fuzz, lint, coverage, init
├── .mcp.json                          #   Claude MCP server config (go run ./cmd/mcp/)
├── README.md                          #   Project overview, quickstart, architecture
└── SECURITY.md                        #   Security policy, vulnerability reporting
```

## Critical Folders

| Folder | Purpose | Key Files |
|--------|---------|-----------|
| `cmd/mcp/` | Server binary entry point | `main.go` (31 lines -- wiring only) |
| `internal/protocol/` | JSON-RPC 2.0 codec layer | `codec.go`, `message.go`, `constants.go` |
| `internal/server/` | MCP server core | `server.go` (460 lines), `counting_reader.go` |
| `internal/tools/` | Tool system | `registry.go`, `schema.go`, `search.go` |
| `internal/pkg/assert/` | Test utilities | `assert.go` (16 lines) |
| `cmd/init/` | Template rewriter | `rewrite.go` (284 lines -- self-deleting) |
| `oss-fuzz/` | Continuous fuzzing | `build.sh`, `Dockerfile`, `project.yaml` |

## Entry Points

- **Server binary:** `cmd/mcp/main.go` -> `server.NewServer()` -> `server.Run(ctx)`
- **Template init:** `cmd/init/main.go` -> `rewriteProject(dir, modulePath)`
- **Fuzz target:** `internal/protocol/fuzz_test.go` -> `Fuzz_Decoder_With_ArbitraryInput`

## File Statistics

| Category | Files | Lines |
|----------|-------|-------|
| Production source (.go) | 12 | ~1,400 |
| Test files (_test.go) | 22 | ~3,800 |
| CI/CD workflows (.yml) | 4 | ~200 |
| Config files | 5 | ~250 |
| Documentation (.md) | 7 | ~350 |
| **Total** | **~50** | **~6,000** |
