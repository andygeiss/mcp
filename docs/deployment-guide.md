# Deployment Guide

## Build

```bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/
```

Produces a single static binary with no runtime dependencies.

## Release Pipeline

Releases are managed via GoReleaser (`.goreleaser.yml`):

- **Platforms**: darwin (amd64, arm64), linux (amd64, arm64)
- **Archives**: tar.gz with LICENSE and README.md
- **Signing**: cosign keyless signing via GitHub OIDC
- **Provenance**: SLSA L3 via `slsa-framework/slsa-github-generator`
- **SBOM**: Generated per archive (`*.sbom.json`)
- **Trigger**: Git tag push

Users verify a release archive with:

```bash
cosign verify-blob \
  --bundle mcp_<version>_<os>_<arch>.tar.gz.sigstore.json \
  --certificate-identity-regexp "^https://github.com/andygeiss/mcp/" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  mcp_<version>_<os>_<arch>.tar.gz
```

## CI/CD Workflows

### ci.yml -- Main Pipeline

Runs on push to main and pull requests. Jobs:

| Job | Purpose |
|---|---|
| `build` | Verify go.mod tidy, compile binary |
| `test` | Race-detected tests on macOS, Ubuntu, Windows; 90% coverage |
| `lint` | golangci-lint with strict config |
| `fuzz` | Fuzz decoder for 2 minutes |
| `bench` | Compare benchmarks against baseline (20% threshold) |
| `integration` | Tests with `-tags=integration` |
| `vulncheck` | govulncheck for known vulnerabilities |
| `ci-ok` | Summary gate, fails if any job failed |

Concurrency: cancels in-progress runs for the same ref.

### fuzz.yml -- Nightly Fuzz

Runs daily at 03:00 UTC. Four targets, 5 minutes each (configurable via workflow_dispatch):
- `Fuzz_Decoder_With_ArbitraryInput`
- `Fuzz_Server_Pipeline`
- `Fuzz_ValidateInput_With_ArbitraryInput`
- `Fuzz_ValidatePath_With_ArbitraryInput`

### codeql.yml -- Security Analysis

GitHub CodeQL for Go code analysis.

### scorecard.yml -- Supply Chain Security

OpenSSF Scorecard for repository security posture.

## MCP Client Configuration

The server communicates over stdin/stdout. Example client configuration:

```json
{
  "mcpServers": {
    "mcp": {
      "command": "/path/to/mcp"
    }
  }
}
```

### Environment Variables

| Variable | Purpose | Default |
|---|---|---|
| `MCP_TRACE` | Enable protocol trace logging to stderr (`1` to enable) | disabled |

Trace output includes full tool arguments — do not enable in production if handlers may receive credentials or PII.

### Version Flag

```bash
./mcp --version
```

## Platform Support

| Platform | Architecture | CI Tested |
|---|---|---|
| macOS | amd64, arm64 | Yes |
| Linux | amd64, arm64 | Yes |
| Windows | amd64 | Yes (tests only, no release binary) |

## Security Pipelines

- **govulncheck**: Checks for known Go vulnerabilities in CI
- **CodeQL**: Static analysis for security issues
- **OpenSSF Scorecard**: Repository security posture assessment
- **cosign**: Keyless signing of release archives (GitHub OIDC)
- **SLSA L3 provenance**: Attached to every release via `slsa-framework/slsa-github-generator`
- **SBOM**: Software bill of materials per archive
- **OSS-Fuzz**: Continuous fuzzing of the JSON-RPC decoder

---

*Generated: 2026-04-18 | Scan level: deep | Reflects v1.3.0*
