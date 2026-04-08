# Deployment Guide

## Overview

The MCP server is a single static binary with no runtime dependencies. It communicates over stdin/stdout and requires no network configuration, database, or service discovery.

## Release Process

Releases are automated via [GoReleaser](https://goreleaser.com/) triggered by pushing a semantic version tag:

```bash
git tag v1.0.0
git push origin v1.0.0
```

### Release Artifacts

The release workflow (`.github/workflows/release.yml`) produces:

| Artifact | Description |
|---|---|
| `mcp_<version>_darwin_amd64.tar.gz` | macOS Intel binary |
| `mcp_<version>_darwin_arm64.tar.gz` | macOS Apple Silicon binary |
| `mcp_<version>_linux_amd64.tar.gz` | Linux x86_64 binary |
| `mcp_<version>_linux_arm64.tar.gz` | Linux ARM64 binary |
| `checksums.txt` | SHA-256 checksums for all archives |
| SBOM | Software Bill of Materials (via Syft) |
| Cosign signature | Keyless signing of checksums via Sigstore |

### Build Flags

The binary embeds the version via ldflags:

```bash
go build -ldflags "-X main.version={{ .Version }}" ./cmd/mcp/
```

The `--version` flag prints the embedded version string.

## CI/CD Pipelines

### CI Pipeline (`ci.yml`)

Triggered on: push to `main`, pull requests.

| Job | Description | Dependencies |
|---|---|---|
| `build` | Compile all packages | none |
| `test` | Run tests with race detector + coverage | none |
| `lint` | golangci-lint with 60+ linters | none |
| `fuzz` | 30s protocol decoder fuzzing | none |
| `integration` | Integration tests (`-tags=integration`) | build |
| `bench` | Benchmark regression detection (20% threshold) | build |

### Security Pipelines

| Pipeline | Schedule | Description |
|---|---|---|
| CodeQL (`codeql.yml`) | Weekly + PR/push | Go static analysis for security vulnerabilities |
| OpenSSF Scorecard (`scorecard.yml`) | Weekly + push to main | Supply chain security assessment |
| Nightly Fuzz (`fuzz.yml`) | Daily 03:00 UTC | Extended fuzz testing (5m per target) |

### OSS-Fuzz Integration

The project is integrated with [Google OSS-Fuzz](https://github.com/google/oss-fuzz) for continuous fuzzing. The `oss-fuzz/` directory contains:

- `Dockerfile` — Build container based on `base-builder-go`
- `build.sh` — Compiles the `Fuzz_Decoder_With_ArbitraryInput` target
- `project.yaml` — Project metadata (libFuzzer engine, address sanitizer)

## MCP Client Configuration

For local development, the `.mcp.json` file configures Claude Code to use the server:

```json
{
  "mcpServers": {
    "mcp": {
      "command": "go",
      "args": ["run", "./cmd/mcp/"]
    }
  }
}
```

## Dependency Management

- **Runtime dependencies**: None (stdlib only)
- **Dependabot**: Configured in `.github/dependabot.yml` for GitHub Actions version updates
- **Action pinning**: All GitHub Actions are pinned to full commit SHAs for supply chain security
