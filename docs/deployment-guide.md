# Deployment Guide

**Project:** mcp
**Generated:** 2026-04-05

## Build

### Local Build

```bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/
```

Produces a single static binary `mcp` in the current directory.

### Cross-Platform Build (GoReleaser)

The project uses GoReleaser for release builds:

| OS | Architecture |
|---|---|
| darwin (macOS) | amd64, arm64 |
| linux | amd64, arm64 |

### Release Artifacts

Each release produces:
- `mcp_<version>_<os>_<arch>.tar.gz` (binary + LICENSE + README.md)
- `checksums.txt` (SHA-256)
- Cosign signature on checksums (keyless Sigstore)
- SBOM (Software Bill of Materials via Syft)

## CI/CD Pipeline

### GitHub Actions Workflows

| Workflow | Trigger | Purpose |
|---|---|---|
| **CI** (`ci.yml`) | Push to main, PRs | Build, test (with coverage), lint, fuzz (30s) |
| **Nightly Fuzz** (`fuzz.yml`) | Daily 03:00 UTC, manual | Extended fuzz testing (5min/target, configurable) |
| **Release** (`release.yml`) | Push `v*.*.*` tag, manual | GoReleaser + Cosign signing + SBOM |
| **CodeQL** (`codeql.yml`) | Push to main, PRs, weekly Mon 06:00 UTC | Static analysis for Go |
| **Scorecard** (`scorecard.yml`) | Push to main, weekly Mon 06:00 UTC, manual | OpenSSF security posture scoring |

### CI Steps (per PR)

1. **bench** -- `make bench` (benchmarks with benchstat comparison)
2. **build** -- `make build` (compile all packages)
3. **fuzz** -- `make fuzz` (30s fuzz run)
4. **lint** -- golangci-lint via official action
5. **test** -- `make cover` (tests with race detector + coverage report + 60% threshold gate)

All five jobs run in parallel on `ubuntu-latest`.

### Continuous Fuzzing

Two layers of fuzzing:

1. **CI fuzz** -- 30s per PR, auto-discovers all `Fuzz_*` targets
2. **OSS-Fuzz** -- Google's continuous fuzzing infrastructure, runs the protocol decoder fuzzer with libFuzzer + AddressSanitizer. Config in `oss-fuzz/`:
   - `Dockerfile` -- pinned base image by SHA256 hash
   - `build.sh` -- compiles native Go fuzzer
   - `project.yaml` -- libfuzzer engine, address sanitizer

### Dependency Management

- **Dependabot** updates GitHub Actions and Go module dependencies weekly
- All GitHub Actions are pinned by full commit SHA (not version tags) for supply chain security
- Zero external Go dependencies (stdlib only)

## Release Process

1. Ensure all CI checks pass on `main`
2. Create and push a version tag: `git tag v1.0.0 && git push origin v1.0.0`
3. The Release workflow triggers automatically:
   - GoReleaser builds cross-platform binaries
   - Cosign signs the checksums (keyless via GitHub OIDC)
   - Syft generates SBOM
   - GitHub Release is created with all artifacts

## Running in Production

The MCP server is designed to be run by an MCP client (e.g., Claude Desktop, IDE extensions). The client launches the binary and communicates via stdin/stdout.

```bash
# Direct execution
./mcp

# With version info
./mcp  # version is baked in at build time via ldflags

# With trace logging
MCP_TRACE=1 ./mcp 2>trace.log
```

### Signal Handling

- `SIGINT` / `SIGTERM` -- graceful shutdown via context cancellation
- stdin EOF -- clean shutdown (exit 0)

### Exit Codes

| Code | Meaning |
|---|---|
| 0 | Clean shutdown (EOF, signal, context cancel) |
| 1 | Fatal error (decode error, encode error) |

## Container

> **Note:** Untested reference — validate in your target environment before production use.

```dockerfile
FROM golang:1.26 AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags "-X main.version=$(git describe --tags --always --dirty)" \
    -o /mcp ./cmd/mcp/

FROM scratch
COPY --from=builder /mcp /mcp
ENTRYPOINT ["/mcp"]
```

The MCP client launches the container with stdin attached, e.g. `docker run -i mcp`.

## Systemd

> **Note:** Untested reference — validate in your target environment before production use.

```ini
[Unit]
Description=MCP Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mcp
Restart=on-failure
StandardInput=socket
StandardOutput=socket

[Install]
WantedBy=multi-user.target
```

The MCP server reads from stdin and writes to stdout. In a systemd context, the MCP client must manage the I/O connection (e.g. via socket activation).

## Security Considerations

- The binary reads from stdin only -- no network listeners
- Stdout is protocol-only (no data leakage)
- Structured logs go to stderr only
- Tool handlers are sandboxed with timeouts and panic recovery
- File operations in the search tool are restricted to the working directory
