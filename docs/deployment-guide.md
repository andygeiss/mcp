# Deployment Guide

**Project:** mcp
**Generated:** 2026-04-07

## Release Process

Releases are automated via [GoReleaser](https://goreleaser.com/) triggered by Git tags.

### Creating a Release

```bash
git tag v1.0.0
git push origin v1.0.0
```

This triggers `.github/workflows/release.yml` which:
1. Builds binaries for darwin/linux on amd64/arm64
2. Generates tar.gz archives with LICENSE and README.md
3. Creates SHA-256 checksums
4. Generates SBOMs via Syft
5. Signs checksums with [cosign](https://github.com/sigstore/cosign) (keyless, OIDC)
6. Publishes GitHub Release

### Build Matrix

| OS | Architecture |
|---|---|
| darwin (macOS) | amd64, arm64 |
| linux | amd64, arm64 |

### Version Injection

```bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/mcp/
```

The `--version` flag prints the injected version string and exits.

## CI/CD Pipelines

### CI (`.github/workflows/ci.yml`)
Runs on every push to `main` and every pull request.

| Job | Steps |
|---|---|
| `build` | `make build` |
| `test` | `make coverage` (race detector + coverage) |
| `fuzz` | `make fuzz` (30s) |
| `lint` | `golangci-lint run ./...` |

### Nightly Fuzz (`.github/workflows/fuzz.yml`)
Runs daily at 03:00 UTC. Discovers all `Fuzz_*` targets automatically and runs each for 5 minutes. Can be triggered manually with custom duration.

### CodeQL (`.github/workflows/codeql.yml`)
Static analysis. Runs weekly (Monday 06:00 UTC) and on every PR/push.

### OpenSSF Scorecard (`.github/workflows/scorecard.yml`)
Supply chain security assessment. Runs weekly and on push to `main`. Results uploaded as SARIF.

### Dependabot (`.github/dependabot.yml`)
Weekly checks for:
- GitHub Actions updates
- Go module updates (though there are no external dependencies)

## OSS-Fuzz Integration

The project is integrated with [Google OSS-Fuzz](https://github.com/google/oss-fuzz) for continuous fuzzing.

| File | Purpose |
|---|---|
| `oss-fuzz/project.yaml` | Project config: Go, libfuzzer engine, address sanitizer |
| `oss-fuzz/build.sh` | Compiles `Fuzz_Decoder_With_ArbitraryInput` target |
| `oss-fuzz/Dockerfile` | Build container based on `base-builder-go` |

### Local Testing

```bash
docker build -f oss-fuzz/Dockerfile -t mcp-fuzz-test .
```

## Security

### Supply Chain
- Pinned GitHub Actions with SHA hashes (not tags)
- Cosign keyless signing for releases
- SBOM generation with Syft
- OpenSSF Scorecard monitoring
- CodeQL static analysis

### Vulnerability Reporting
Report via [GitHub Security Advisories](https://github.com/andygeiss/mcp/security/advisories/new). Response within 72 hours, assessment within 7 days, fix target within 30 days.

### Scope
- `cmd/mcp` binary: in scope
- `cmd/init` template rewriter: not security-critical

## Deployment as MCP Server

This is a CLI binary — no containers, no services. Deploy by:

1. Download the release binary for your platform
2. Place it in your `PATH`
3. Configure your MCP client to invoke it

### Claude Code Configuration

Create `.mcp.json` in your project root:
```json
{
  "mcpServers": {
    "mcp": {
      "command": "/path/to/mcp"
    }
  }
}
```

### Using as Template

```bash
git clone https://github.com/andygeiss/mcp.git my-server
cd my-server
make init MODULE=github.com/myorg/my-server
```
