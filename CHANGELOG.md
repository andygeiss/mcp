# Changelog

All notable changes to this project are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html) — see [VERSIONING.md](VERSIONING.md).

## [Unreleased]

## [1.0.0] — 2026-04-12

Initial stable release. See [VERSIONING.md](VERSIONING.md) for the public-API boundary, compatibility guarantees, and support window. No code changes since [0.6.10]; this tag marks the point at which the documented surface becomes covered by semver.

## [0.6.10] — 2026-04-12

### Added

- `VERSIONING.md` defining the v1.0.0 public-API boundary, compatibility guarantees, and support window.
- `CHANGELOG.md` (this file).
- Branch protection on `main`: required `ci-ok` status check, required signed commits, linear history, admin enforcement, no force-pushes, no deletions.

## [0.6.9] — 2026-04-12

### Changed

- Rescoped user-facing documentation to reflect the methods actually implemented; removed "spec-complete" and "completion" overclaims. Added a `Scope` subsection to the README enumerating implemented methods and the `-32601` rejection list.
- Softened the bidirectional-transport wording in `docs/project-overview.md` and `docs/architecture.md` to clarify that `SendRequestFromContext` is a primitive — no built-in sampling, elicitation, or roots handlers.

### Fixed

- `internal/server/testdata/conformance/README.md`: stale MCP spec version `2024-11-05` → `2025-11-25`.

## [0.6.8] — 2026-04-12

### Changed

- Bumped `codecov/codecov-action` to the Scorecard-pinned SHA.
- Annotated the SLSA generator's tag-pin as `not-supported` for OpenSSF Scorecard (the generator cannot be SHA-pinned).

## [0.6.7] — 2026-04-12

### Fixed

- Release signing now uses the Scorecard-recognized `.sigstore.json` bundle extension.
- SLSA L3 provenance published via `slsa-framework/slsa-github-generator`.

## [0.6.6] — 2026-04-11

### Fixed

- `prompts/get` now rejects unknown argument names with JSON-RPC `-32602` ("invalid params") instead of silently ignoring them.

## [0.6.5] — 2026-04-11

### Added

- Release archives, SBOMs, and checksums are keyless-signed with cosign.
- Per-archive SBOMs (`*.sbom.json`) attested alongside each release artifact.

## Pre-0.6.5 — 2026-04-10 / 2026-04-11

Pre-release development leading up to the first signed release. Highlights:

- MCP 2025-11-25 protocol foundation: tools, resources (list/read), prompts, logging, progress, and a bidirectional server-to-client request primitive.
- Three-state server lifecycle (uninitialized → initializing → ready) with `-32000` rejection outside the state window.
- Per-message size cap (4 MB), handler timeout (30 s) with panic recovery, 4 096-char tool input cap, 10-level schema recursion.
- Auto-derived JSON schemas from Go struct tags for tools and prompts via a shared reflection engine.
- Resource URI templates with alphabetical ordering.
- OSS-Fuzz integration; 4 fuzz targets (decoder, pipeline, input/path validation).
- 90 % coverage threshold enforced in CI.
- OpenSSF Scorecard, CodeQL, and govulncheck running in CI; GitHub Actions pinned to SHAs; Dependabot weekly for `gomod` and `github-actions`.

[Unreleased]: https://github.com/andygeiss/mcp/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/andygeiss/mcp/releases/tag/v1.0.0
[0.6.10]: https://github.com/andygeiss/mcp/releases/tag/v0.6.10
[0.6.9]: https://github.com/andygeiss/mcp/releases/tag/v0.6.9
[0.6.8]: https://github.com/andygeiss/mcp/releases/tag/v0.6.8
[0.6.7]: https://github.com/andygeiss/mcp/releases/tag/v0.6.7
[0.6.6]: https://github.com/andygeiss/mcp/releases/tag/v0.6.6
[0.6.5]: https://github.com/andygeiss/mcp/releases/tag/v0.6.5
