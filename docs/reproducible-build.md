# Reproducible build verification

Released binaries are built deterministically from source. Anyone with the same Go toolchain version and the tagged commit can rebuild the binary and confirm the SHA-256 matches `checksums.txt` from the release.

This is independent of the cosign signature path: cosign proves the artifact was produced by this repository's CI; this recipe lets you prove that what CI produced matches what your local source tree produces.

## What you need

- Go toolchain matching `go.mod` (currently Go 1.26+)
- `git`, `sha256sum` (or `shasum -a 256` on macOS)

## Recipe

```bash
# 1. Pick a release version
VERSION=0.1.0
GOOS=linux       # darwin | linux
GOARCH=amd64     # amd64 | arm64

# 2. Check out the tagged commit
git clone https://github.com/andygeiss/mcp
cd mcp
git checkout "v${VERSION}"

# 3. Rebuild with the exact flags goreleaser uses (see .goreleaser.yml)
GOOS=$GOOS GOARCH=$GOARCH go build \
  -trimpath \
  -ldflags "-X main.version=${VERSION}" \
  -o mcp \
  ./cmd/mcp/

# 4. Compute the SHA-256 of your local build
sha256sum mcp        # Linux
# shasum -a 256 mcp  # macOS

# 5. Download the published checksum and compare
curl -sSL "https://github.com/andygeiss/mcp/releases/download/v${VERSION}/checksums.txt" \
  | grep "mcp_${VERSION}_${GOOS}_${GOARCH}"
```

The SHA from step 4 should match the SHA in the line printed by step 5 (after extracting the binary from the release archive — see below).

## Verifying the archived binary

The published `checksums.txt` covers the `.tar.gz` archive, not the bare binary. To compare against the binary inside the archive:

```bash
curl -sSLO "https://github.com/andygeiss/mcp/releases/download/v${VERSION}/mcp_${VERSION}_${GOOS}_${GOARCH}.tar.gz"
tar -xzf "mcp_${VERSION}_${GOOS}_${GOARCH}.tar.gz" mcp
sha256sum mcp        # this should match the SHA from step 4
```

## Inspecting the embedded build metadata

Every Go binary built from a Go module embeds the source path, dependency tree (always empty here — stdlib only), and build settings. To confirm the binary was built with the expected flags:

```bash
go version -m mcp
```

You should see:

- `path` — `github.com/andygeiss/mcp/cmd/mcp`
- `mod` — `github.com/andygeiss/mcp` and the version
- `build  -trimpath  true`
- `build  -ldflags  "-X main.version=<version>"`
- `dep` lines — none (stdlib-only)

## Why this matters

`go build -trimpath` strips local filesystem paths from the binary, removing the only common source of nondeterminism in Go builds. Combined with stdlib-only dependencies, the only inputs to the build are the tagged source commit and the Go toolchain version. Two independent rebuilds will produce byte-identical binaries.

This is the supply-chain story the project's [Engineering Philosophy](../CLAUDE.md#engineering-philosophy) commits to: *correctness, clarity, simplicity, security* — verifiable by anyone, not just trusted by signature.
