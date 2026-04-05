# Contributing

## Prerequisites

- Go 1.26+ (see `go.mod` for exact version)
- `golangci-lint` installed
- No external dependencies required

## Dev Setup

```bash
make check          # build + test + lint (recommended)
make build          # compile all packages
make test           # run tests with race detector
make fuzz           # fuzz the protocol decoder (30s default)
make lint           # lint with zero issues required
make coverage       # generate coverage report
```

## Testing Requirements

- Race detector is mandatory: always use `go test -race`
- TDD workflow: write a failing test first, then implement
- Black-box packages: `package foo_test` by default; white-box only for unexported internals
- Test naming: `Test_<Unit>_With_<Condition>_Should_<Outcome>`
- Structure: `// Arrange` / `// Act` / `// Assert`
- Every test calls `t.Parallel()`
- Use `assert.That(t, "description", got, expected)` from `internal/pkg/assert`

## Pull Request Process

1. Branch from `main`
2. CI must pass: build, test, fuzz, lint
3. One approval required
4. No force-push to `main`

## Commit Conventions

- Imperative mood, concise
- Prefix with area when helpful: `protocol: fix id echo for null`

## Fuzzing

Run fuzz tests locally:

```bash
make fuzz                 # 30s default
make fuzz FUZZTIME=5m     # custom duration
```

The project is integrated with [OSS-Fuzz](https://github.com/google/oss-fuzz) for continuous fuzzing. The `oss-fuzz/` directory contains the build harness. To test locally with Docker:

```bash
docker build -f oss-fuzz/Dockerfile -t mcp-fuzz-test .
```

## What We Won't Accept

- External dependencies (stdlib only)
- HTTP/WebSocket transport
- Non-protocol data on stdout
- `//nolint` directives without fixing the underlying issue
- `.golangci.yml` modifications to suppress findings
