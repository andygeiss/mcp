# Deployment Guide

**Project:** github.com/andygeiss/mcp
**Generated:** 2026-04-05

## Build

### Local Build

```bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/
```

Produces a single static binary `mcp` in the current directory.

### Cross-Platform Builds

The project uses [GoReleaser](https://goreleaser.com/) for automated cross-platform builds.

**Supported targets:**

| OS | Architecture |
|----|-------------|
| darwin (macOS) | amd64, arm64 |
| linux | amd64, arm64 |

## Release Pipeline

Releases are triggered by pushing a semantic version tag:

```bash
git tag v1.2.3
git push origin v1.2.3
```

### Release Workflow (.github/workflows/release.yml)

1. **Checkout** with full git history (for changelog)
2. **Go setup** from `go.mod` version
3. **Cosign install** for binary signing (sigstore/cosign-installer@v3)
4. **GoReleaser** builds, packages, and publishes:
   - Cross-compiled binaries for all targets
   - `tar.gz` archives with LICENSE and README.md
   - SHA256 checksums (`checksums.txt`)
   - Cosign signatures on checksums
   - SBOM (Software Bill of Materials) via anchore/sbom-action
   - Changelog (sorted ascending)

### Artifact Integrity

| Artifact | Purpose |
|----------|---------|
| `checksums.txt` | SHA256 checksums for all archives |
| `checksums.txt.sig` | Cosign signature on checksums |
| SBOM | Software Bill of Materials for supply chain security |

## CI/CD Pipeline

### Continuous Integration (.github/workflows/ci.yml)

Triggered on: pull requests and pushes to `main`.

Four parallel jobs:

| Job | Command | Purpose |
|-----|---------|---------|
| build | `make build` | Compilation check |
| fuzz | `make fuzz` | 30s fuzz test run |
| lint | `golangci-lint run ./...` | 48-linter static analysis |
| test | `make coverage` | Tests with race detector + coverage |

### Nightly Fuzzing (.github/workflows/fuzz.yml)

- Schedule: daily at 03:00 UTC
- Duration: 5 minutes per fuzz target (configurable)
- Auto-discovers all `Fuzz_*` functions across the codebase
- Manual trigger via workflow_dispatch with custom duration

### OpenSSF Scorecard (.github/workflows/scorecard.yml)

- Schedule: weekly (Monday 06:00 UTC) + on push to main
- Publishes SARIF results to GitHub Security tab
- Badge: [![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/andygeiss/mcp/badge)](https://scorecard.dev/viewer/?uri=github.com/andygeiss/mcp)

### Dependency Updates (.github/dependabot.yml)

- Weekly updates for GitHub Actions versions and Go modules
- Maximum 5 concurrent dependency PRs

## OSS-Fuzz Integration

The project is integrated with [Google OSS-Fuzz](https://github.com/google/oss-fuzz) for continuous fuzzing.

**Configuration (oss-fuzz/):**
- Engine: libfuzzer
- Sanitizer: AddressSanitizer
- Target: `Fuzz_Decoder_With_ArbitraryInput` in `internal/protocol`

**Local testing:**
```bash
docker build -f oss-fuzz/Dockerfile -t mcp-fuzz-test .
```

## Running the Server

The MCP server communicates via stdin/stdout. It is designed to be launched by an MCP client (e.g., Claude Desktop, Claude Code).

### Direct Execution

```bash
./mcp
```

### Via go run (development)

```bash
go run ./cmd/mcp/
```

### MCP Client Configuration (.mcp.json)

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

### Environment

- No environment variables required
- No configuration files needed
- Version is embedded at build time via `-ldflags`
- Logging goes to stderr via `slog.JSONHandler`

## Security

- **Vulnerability reporting:** [GitHub Security Advisories](https://github.com/andygeiss/mcp/security/advisories/new)
- **Response timeline:** Acknowledgment within 72 hours, assessment within 7 days, fix within 30 days
- **Scope:** `cmd/mcp` binary is in scope; `cmd/init` is not security-critical
- **Supply chain:** Cosign-signed releases, SBOM generation, OSS-Fuzz, OpenSSF Scorecard
