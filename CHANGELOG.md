# Changelog

All notable changes to this project are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html) — see [VERSIONING.md](VERSIONING.md).

## [Unreleased]

### Added

- `internal/server`: MCP `initialize` now negotiates `protocolVersion` — echoes the client's version when it matches the server's supported version, otherwise falls back to the server's version. `clientInfo` is logged at the uninitialized→initializing transition.
- `internal/server`: `ErrPendingRequestsFull` sentinel and `maxPendingRequests` cap (1024) on the server-to-client correlation map; `SendRequest` now returns the sentinel under back-pressure instead of growing the map without bound.
- `internal/protocol`: JSON nesting depth guard (`maxJSONDepth = 64`) scanned before `Unmarshal` to prevent stack-exhaustion on pathological payloads.
- `cmd/scaffold`: refuses to run when the working tree has uncommitted changes. The trailing `resetGitHistory` step is destructive, so the guard prevents silent loss of in-progress edits. Pass `--force` to override.
- `internal/schema`: `time.Time` now derives as `{"type":"string","format":"date-time"}`; `json.RawMessage` derives as an unconstrained schema (any JSON); recursive types fail fast with a clear error instead of exhausting the depth budget.

### Changed

- `internal/tools`: the `Tool` struct now references `schema.InputSchema` / `*schema.OutputSchema` directly instead of aliasing them — `tools.InputSchema`, `tools.OutputSchema`, and `tools.Property` are removed. Callers import `internal/schema` to reach the JSON Schema types, matching `internal/prompts`. This aligns tools and prompts on the same vocabulary without a shadow namespace.
- `cmd/init` → `cmd/scaffold`: the template rewriter binary directory was renamed to remove the collision with `go mod init`. The user-facing surface (`make init MODULE=…`) is unchanged.
- `internal/server`: request context is now threaded through `dispatch → handle{Resources,Prompts}{Method,Read,Get}` so client disconnect and server shutdown cancel resource/prompt handlers promptly (previously rooted at `context.Background()`).
- `internal/protocol`: response classification now rejects messages carrying both `result` and `error` per JSON-RPC 2.0 §5; a `null` value in either field is treated as absent so `{"result":null}` is no longer misclassified as a valid response.
- `internal/prompts`: argument marshal/unmarshal errors are returned as `*protocol.CodeError{InvalidParams}` at the handler boundary (consistent with `internal/tools`).
- `cmd/mcp`: `--version` now writes to stderr so stdout stays protocol-only even when the flag is invoked against the server binary.
- Release and `make build`: enabled `-trimpath` so `go install …@latest` and release artifacts are path-reproducible.
- `README.md`: clarified that bidirectional transport is a generic server-to-client primitive — no built-in handlers for sampling, elicitation, or roots. Added `MCP_TRACE=1` production warning.
- `internal/protocol`: removed unused constants for out-of-scope methods (`MethodCompletionComplete`, `MethodResourcesSubscribe`, `MethodResourcesUnsubscribe`) and notifications (`NotificationResourcesListChanged`, `NotificationToolsListChanged`, `NotificationPromptsListChanged`). Remaining method/notification constants are alphabetized.
- `internal/server`: capability structs (`prompts`, `resources`, `tools`) are now empty objects — previously they advertised `listChanged: false` / `subscribe: false` for features the server does not implement. The capability is still advertised (key presence signals support); the zero-value flags were noise.

### Fixed

- `internal/server`: `inFlightCancelled` converted from `bool` to `atomic.Bool` and reset at handler spawn (not post-processing) so a stale `notifications/cancelled` for a prior request cannot suppress the current handler's response.
- `internal/server`: `resources/read` and `prompts/get` parameter-unmarshal errors now include the underlying parse error instead of the opaque `"invalid ... params"` string.

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
