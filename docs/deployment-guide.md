# Deployment Guide

How `github.com/andygeiss/mcp` is built, signed, released, and verified.

---

## Distribution

Three install paths:

```bash
go install github.com/andygeiss/mcp/cmd/mcp@latest
```

Or download a signed release archive from [GitHub Releases](https://github.com/andygeiss/mcp/releases) (macOS / Linux × amd64 / arm64) and verify it (see below).

Or scaffold your own MCP server from this repo:

```bash
git clone https://github.com/andygeiss/mcp.git yourproject
cd yourproject
make init MODULE=github.com/yourorg/yourproject
```

## Release artifacts

Each tagged release publishes (per OS × arch combination):

| Artifact | Purpose |
|---|---|
| `mcp_<version>_<os>_<arch>.tar.gz` | Binary archive (contains `mcp`, `LICENSE`, `README.md`) |
| `mcp_<version>_<os>_<arch>.tar.gz.sigstore.json` | cosign keyless signature bundle |
| `mcp_<version>_<os>_<arch>.tar.gz.sbom.json` | SBOM (Syft, SPDX format) |
| `checksums.txt` | SHA-256 digests for all archives |
| `multiple.intoto.jsonl` | SLSA L3 build provenance attestation |

**Supported platforms:** `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`. Windows is tested in CI but no release binaries are published.

## Release pipeline

Triggered by tag push matching `v*.*.*` (or `workflow_dispatch`). Pipeline lives in `.github/workflows/release.yml` + `.goreleaser.yml`.

```
tag v1.x.y → release.yml → goreleaser
                          ├── build (trimpath, ldflags=-X main.version)
                          ├── archive (tar.gz: binary + LICENSE + README)
                          ├── checksums.txt (SHA-256)
                          ├── SBOM (Syft → *.sbom.json per archive)
                          └── cosign sign-blob (keyless OIDC, →*.sigstore.json bundle)
                          ↓
                          actions/attest-build-provenance (GitHub-native attestation)
                          ↓
                          slsa-framework/slsa-github-generator → SLSA L3 provenance
                          ↓
                          GitHub Release published with all artifacts
```

**Build flags:** `-trimpath` (reproducible — strips local paths), `-ldflags "-X main.version={{ .Version }}"` (injects the tag into the `version` package var).

**Signing:** cosign **keyless** via OIDC — no long-lived signing keys. The signing identity is `https://github.com/andygeiss/mcp/.github/workflows/release.yml@refs/tags/v<version>`, issued by `https://token.actions.githubusercontent.com`.

## Verifying a release

Each archive ships with a `.sigstore.json` bundle. SHA-256 digests are in `checksums.txt`. SBOMs are attached as `*.sbom.json`.

```bash
# Replace <version>, <os>, <arch> with your target (e.g. 1.3.2, Linux, x86_64)
cosign verify-blob \
  --bundle mcp_<version>_<os>_<arch>.tar.gz.sigstore.json \
  --certificate-identity-regexp "^https://github.com/andygeiss/mcp/" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  mcp_<version>_<os>_<arch>.tar.gz
```

Successful output: `Verified OK`.

## Verifying SLSA provenance

The pipeline produces a SLSA L3 provenance attestation via `slsa-framework/slsa-github-generator`. Verify with `slsa-verifier`:

```bash
slsa-verifier verify-artifact \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/andygeiss/mcp \
  --source-tag v<version> \
  mcp_<version>_<os>_<arch>.tar.gz
```

## Running the binary

The binary is a stdin/stdout MCP server. Point any MCP client at it:

```json
{
  "mcpServers": {
    "mcp": {
      "command": "/absolute/path/to/mcp"
    }
  }
}
```

(Format above is the Claude Desktop / VS Code MCP client convention.)

### Flags

| Flag | Purpose |
|---|---|
| `--version` | Prints the injected `main.version` to stderr and exits 0. |

### Environment variables

| Variable | Effect |
|---|---|
| `MCP_TRACE=1` | Logs every request and response to stderr via `slog`. **Do not enable in production** when handlers may receive credentials or PII — trace output includes full tool arguments. |

### Lifecycle

- **Stdin:** persistent JSON decoder. The binary reads NDJSON requests until EOF.
- **Stdout:** protocol-only — every byte is a JSON-RPC message.
- **Stderr:** `slog.JSONHandler` diagnostics (lifecycle events, errors).
- **EOF on stdin:** clean shutdown, exit 0.
- **Decode error (other than EOF):** fatal, exit 1.
- **`SIGINT` / `SIGTERM`:** server context cancelled with cause; in-flight handler aborted; exit promptly. **No drain.**

### Resource limits enforced by the binary

- Per-message size cap: **4 MB** (counting reader; resets per message).
- JSON depth cap: **64** (`MaxJSONDepth`) — prevents stack-exhaustion.
- Handler timeout: **30 seconds** — slow handlers receive `-32001` (`ServerTimeout`).
- Pending server-to-client request map: **1024** in-flight outbound requests; excess returns `protocol.ErrPendingRequestsFull`.
- Concurrency: **sequential** (`maxInFlight: 1` advertised in capabilities).

## Supply-chain hardening (defense-in-depth)

- **Zero external dependencies.** No `go.mod` requires beyond the stdlib. Reduces transitive supply-chain risk to zero.
- **Reproducible builds.** `-trimpath` + pinned Go version + deterministic ldflags.
- **cosign keyless signing.** Every archive signed by an ephemeral OIDC-issued cert tied to a specific GitHub Actions run.
- **SBOM per archive.** Syft-generated, SPDX-format, attached to the release.
- **SLSA L3 provenance.** Generated by `slsa-framework/slsa-github-generator`; attests the build environment, source commit, and inputs.
- **OSS-Fuzz integration.** `oss-fuzz/project.yaml` describes the build harness; corpus runs continuously upstream.
- **CI security gates:** `codeql.yml` (CodeQL static analysis), `scorecard.yml` (OpenSSF Scorecard).
- **Pinned action versions.** Every `uses:` line in `.github/workflows/release.yml` is pinned by SHA — `goreleaser-action`, `cosign-installer`, `actions/checkout`, `actions/setup-go`, `actions/attest-build-provenance`. Dependabot owns version bumps.

## CI workflows

| Workflow | Trigger | Purpose |
|---|---|---|
| `ci.yml` | push, PR | Build + test (race + integration) + lint matrix on macOS/Linux/Windows |
| `codeql.yml` | push, PR, schedule | GitHub Advanced Security CodeQL analysis |
| `fuzz.yml` | schedule, dispatch | Fuzz the decoder with the latest corpus |
| `release.yml` | tag push `v*.*.*`, dispatch | The release pipeline (described above) |
| `scorecard.yml` | schedule | OpenSSF Scorecard scoring |

## Release process

1. **Run the gating checks** (per the v1.3.x retrospective convention): retrospective action items from the prior release must be closed; pre-release gap & refactor scan must be cleared. PRD work for the next minor is blocked until prior gates clear.
2. **Update CHANGELOG.md** under the appropriate version heading. Use Keep a Changelog format (`### Added` / `### Changed` / `### Fixed` / `### Removed`).
3. **Tag** with `git tag -s v<version>` (signed). Push the tag.
4. **GitHub Actions runs `release.yml`** — ~3 minutes for goreleaser + signing + provenance.
5. **Verify the published release** end-to-end: download, `cosign verify-blob`, `slsa-verifier verify-artifact`, run `mcp --version`.
6. **Backfill CHANGELOG if needed** (per the v1.3.1 + v1.3.2 precedent: PR #61 commit `1df7c21` backfilled both versions). Backfilled entries live on `main` post-tag.

## Versioning policy

See [VERSIONING.md](../VERSIONING.md). SemVer with these specifics:

- A **golden test change** is a wire-format change — treat it as a SemVer signal.
- The **`Peer` interface method set and parameter types** are a v1.x stability commitment per ADR-003.
- The **MCP protocol version** (`2025-11-25`) is a fixed constant in `internal/protocol/constants.go` — **never bump opportunistically**. A bump is a coordinated release that includes spec review.

## Operational guidance for downstream consumers

If you scaffold via `make init MODULE=...`:

- Your fork **inherits** the stdlib-only constraint, the `io.Reader`/`io.Writer` injection contract, the test discipline (`assert.That`, `t.Parallel()`, `//go:build integration`), and the security posture. Re-derive these only with explicit reason.
- **The binary directory stays at `cmd/mcp/`** — every scaffolded project produces a binary named `mcp`. If two MCP servers share `$GOBIN`, disambiguate with `go build -o <name>` or rename `cmd/mcp/`.
- **`make init` requires a clean working tree** — `resetGitHistory` is destructive. Commit/stash first, or pass `--force` to override.
- **Update the badges** in your fork's README — they're auto-pointed at the new repo by the rewriter, but verify the URLs work after publishing.

## Security disclosure

See [SECURITY.md](../SECURITY.md) for the disclosure process and supported-version policy.

## See also

- [Architecture](./architecture.md) — system design
- [Development Guide](./development-guide.md) — local build/test/fuzz
- [README — Verify a release](../README.md#verify-a-release) — short-form verification snippet
- [VERSIONING.md](../VERSIONING.md) — SemVer policy
