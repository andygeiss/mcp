# Changelog

All notable changes to this project are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html) — see [VERSIONING.md](VERSIONING.md).

## [Unreleased]

### Added

- `internal/tools.Register[In, Out]`: tool handlers now declare a typed structured-output `Out` alongside the input `In`. Handler signature changed from `func(ctx, In) Result` to `func(ctx, In) (Out, Result)`. The dispatch wrapper auto-marshals a non-zero `Out` into `Result.StructuredContent` via `encoding/json` v1; zero-value `Out` is skipped via `reflect.Value.IsZero` so `omitempty` stays honest. Marshal failures surface as `-32603` Internal error.
- `internal/tools.Tool.OutputSchema *schema.OutputSchema` populated at `Register` from the `Out` type via the shared reflection engine. Every registered tool now advertises an `outputSchema` in `tools/list` (the empty `struct{}` shape produces `{"type":"object"}` — the spec-conformant "any object" default).
- `internal/schema.DeriveOutputSchema[T]()`: companion to `DeriveInputSchema`, sharing the engine via a new private `deriveStructSchema[T]` helper. Same rules: struct-only, `json` + `description` tags, pointer-or-omitempty for optional fields.
- `internal/tools.EchoOutput`: typed structured output the reference Echo handler now produces (mirrors the input message). Demonstrates the new pattern for downstream scaffold consumers.
- `internal/tools/testdata/golden_schemas/`: each tool now has `<name>.input.json` and `<name>.output.json` goldens. Three new tools added to the golden test table to exercise the new outputSchema surface across nested structs, pointer-typed optional fields, and array-of-struct shapes (`nested-output`, `optional-output`, `array-output`).
- `internal/server/integration_test.go`: two MUST tests pinning the Q5 wire shape — `Test_Server_With_StructuredContent_Should_RoundTrip` (handler's typed `Out` round-trips byte-for-byte via the codec) and `Test_ToolsList_With_TypedOutputs_Should_AdvertiseOutputSchema` (every registered tool advertises an `outputSchema` in `tools/list`). Both registered as `protocol.Clause` entries so the audit story (Story 2.1) carries them.

### Changed

- **Breaking:** `tools.Register` API: `Register[T]` → `Register[In, Out]`. Every existing call site updated. Backward compatibility with the single-parameter form is intentionally not preserved (v1.4.0 protocol-feature add, all callers move together).
- `internal/tools/_TOOL_TEMPLATE.go`: updated to demonstrate the `(Out, Result)` signature and dual-type registration.
- `cmd/scaffold/testdata/greet.go.fixture`: scaffold consumer fixture updated to the new pattern (`GreetOutput` type + `(GreetOutput, Result)` return).
- `internal/server/testdata/conformance/tools-list.response.jsonl`: golden response updated to include the `outputSchema` field that every tool now advertises.
- `docs/development-guide.md`: "Adding a new tool" section rewritten for the typed-output pattern.

## [1.3.2] — 2026-04-25

### Changed

- `docs/adr/ADR-003-bidi-reader-split.md`: made self-contained. Removed citations to two `_bmad-output/`-prefixed paths (`g4-inbound-cancel`, `g8-server-test-breakage`) that are gitignored and dead from any clone. Inlined the load-bearing facts: G4 dispatch.go routing for inbound `notifications/cancelled` (the three-shape classifier rationale), and G8 pointer-return cascade scope (26 single + ~30 slice decls, one `runServer` anchor; zero typed-error or depguard migrations). Corrected two shipped-name drifts in the Invariants table: `protocol.ErrPendingFull` → `protocol.ErrPendingRequestsFull` and `protocol.ErrCapabilityNotSupported` → `*protocol.CapabilityNotAdvertisedError`. No code or behavior change.

## [1.3.1] — 2026-04-18

### Changed

- `docs/`: comprehensive refresh for v1.3.0 bidi trio. `architecture.md` updated for the bidi reader classifier, Peer stability surface, typed errors (AI8), AI10 progress invariant, and refreshed error-code table (+ -32002). `source-tree-analysis.md` updated for the `cmd/scaffold` rename, the 11-file server split with per-file LOC, the new `protocol/{peer,capabilities,errors}.go` files, and accurate metrics (29 src / 42 test / 463 tests / 5 fuzz / 11 bench / 37 conformance / 10 integration). `project-overview.md` updated for the v1.0.0..v1.3.0 timeline, SLSA L3, and OSS-Fuzz. `development-guide.md` updated for the 5 fuzz targets, `Peer.SendRequest` handler pattern, and `make smoke` target. `deployment-guide.md` updated with a `cosign verify-blob` example and the SLSA L3 provenance surface. `index.md` ADR section + refreshed quick-reference numbers. `.gitignore` excludes `docs/.archive/` (workflow state).

## [1.3.0] — 2026-04-18

### Added

- `docs/adr/ADR-003-bidi-reader-split.md`: documents the single-reader + mutex-protected pending-map design for server-initiated requests, the `Peer` stability surface (v1.x commitment), four ratified invariants (AI7 cancel chain, AI8 typed errors, AI9 capability gate, AI10 no-progress-during-outbound), and five failure modes (malformed mid-session, EOF with pending, SIGTERM during outbound, late response after cancel, writer-mutex contention).
- `internal/protocol/peer.go`: `Peer` interface (`SendRequest(ctx, method, params) (*Response, error)`), `ContextWithPeer` / `PeerFromContext` helpers, package-level `SendRequest` wrapper returning `ErrNoPeerInContext` when no `Peer` is attached, and `MethodCapability(method) (Capability, bool)` for the AI9 gate. Method set and parameter types are a v1.x stability commitment per ADR-003.
- `internal/protocol/capabilities.go`: `ClientCapabilities` struct with pointer-to-empty-struct sub-capabilities (`SamplingCapability`, `ElicitationCapability`, `RootsCapability` with `ListChanged`), `Capability` named-type enum (`CapSampling`, `CapElicitation`, `CapRoots`), and nil-safe `(*ClientCapabilities).Has(Capability) bool`.
- `internal/protocol/errors.go`: typed errors relocated from `internal/server` to `internal/protocol` so handler packages can match without import cycles. Sentinels: `ErrPendingRequestsFull`, `ErrServerShutdown`, `ErrNoPeerInContext`. Struct errors: `CapabilityNotAdvertisedError` (carries `Capability` + `Method`), `ClientRejectedError` (carries `Code` + `Message` + `Data` mirroring JSON-RPC error shape).
- `internal/protocol/constants.go`: `ErrorCode` named type with mirrors of the well-known JSON-RPC codes (`ErrCodeInternalError`, `ErrCodeInvalidParams`, etc.) for compile-time-typed `ClientRejectedError.Code`.
- `internal/server`: `pendingEntry` struct (createdAt / method / `chan *protocol.Response`); `(*Server).registerPending` is the SOLE pending-map insert site (Invariant I3); `outboundIDCounter atomic.Int64` replaces `srv-N` string IDs with plain integers via `strconv.AppendInt`; AI9 capability gate is the first statement in `(*Server).SendRequest` — synchronous reject with zero side effects on absence; A7 server→client cancel emission via `emitOutboundCancel` BEFORE pending-entry cleanup; priority select ensures shutdown wins over ctx-cancel.
- `internal/server`: `(*Server).clientCaps atomic.Pointer[protocol.ClientCapabilities]` snapshotted at `initialize`; `protocol.ContextWithPeer(callCtx, s)` injected at handler dispatch (`inflight.go`) so handlers reach the bidi path via `protocol.SendRequest` without importing `internal/server` (Invariant I1).
- `cmd/scaffold`: post-success welcome banner emitted to stderr after `rewriteProject` returns nil. Banner names the three imperative steps (Edit / Wire / Verify) and points at the README.
- `internal/tools/echo.go`: `// START HERE — your first tool` anchor + annotated input/handler comments + trailing wiring hint. Grep-style anchor test in `echo_test.go` enforces the anchor across refactors.
- `internal/tools/_TOOL_TEMPLATE.go`: annotated copy-target with `//go:build ignore` tag and leading-underscore filename (belt-and-suspenders exclusion). Demonstrates required (non-pointer) and optional (pointer) field patterns.
- `Makefile`: `make smoke` POSIX-sh pipeline pipes `initialize` + `notifications/initialized` + `tools/list` through `go run ./cmd/mcp/`, greps stdout for `"result":{"tools":` (tightened pattern), prints success banner on exit 0 or two-cause diagnostic + captured stderr on failure. Build-tag-gated integration test at `internal/tools/smoke_integration_test.go` invokes the target end-to-end. Verified under `/bin/sh`, bash, and zsh.
- `README.md`: "Your first tool" walkthrough section with Edit / Wire / Verify sub-steps matching the welcome banner verbatim. README breadcrumb after the `make init` block points at `internal/tools/echo.go`.
- `internal/server/testdata/conformance/`: 7 new fixture pairs (`bidi-{sampling,elicitation,roots}-orphan-response` + `cancel-notification-no-inflight`) capture the wire shape of the bidi trio's response/notification side per NFR-M5.
- `internal/protocol/testdata/fuzz/Fuzz_Decoder_With_ArbitraryInput/`: 5 G1 response-shape seeds (`g1-response-{valid,error,malformed-both,malformed-neither,orphan}`).
- `.golangci.yml`: `depguard.no-server-in-handlers` rule denies `internal/server` import from `internal/tools/**` and `internal/prompts/**` (Invariant I1, CI-enforced); `forbidigo` rules ban `os.Stdout` writes and `fmt.Fprint(os.Stdout, …)` patterns in the same packages (Invariant I4, CI-enforced).
- `internal/server/synctest_test.go`: AI9 capability-gate scenario (`Test_Server_With_CapabilityGate_Should_RejectSamplingWithoutAdvertisement`) verifies zero outbound bytes hit stdout when the client did not advertise the required capability.

### Changed

- `(*Server).SendRequest` now returns `*protocol.Response` (was `protocol.Response`). The `Peer` interface signature mirrors. Internal `routeResponse` and `pending` map types updated to pointer-shape.
- `internal/server/progress.go`: `(*Progress).Report` godoc gains explicit AI10 callout — handlers MUST NOT invoke `Report` while parked in `protocol.SendRequest` / outbound-awaiting; `protocol.ServerTimeout` (`-32001`) is the sole slow-client recovery mechanism.
- `internal/server`: local `errors.New("server shutting down")` and the unexported `ErrPendingRequestsFull` sentinel are now `protocol.ErrServerShutdown` / `protocol.ErrPendingRequestsFull`. Compile-time assertion `var _ protocol.Peer = (*Server)(nil)` guards against signature drift.
### Removed

- `internal/server`: `SendRequestFromContext` (and its `cmd/scaffold` consumer pattern) — handlers now use `protocol.SendRequest(ctx, ...)` with a Peer attached via `protocol.ContextWithPeer`. Two test call sites migrated.
- `internal/server`: local `ErrPendingRequestsFull` sentinel (relocated to `protocol`).

## [1.2.0] — 2026-04-16

### Added

- `docs/adr/ADR-002-internal-package-layout.md`: records the 10-agent audit verdict that the seven-package `internal/` layout is already optimal. Future refactor proposals that touch package boundaries must rebut this ADR with a concrete problem.
- `internal/resources`: `Fuzz_LookupTemplate_With_ArbitraryInputs` covers the RFC 6570 matching state machine; a unit test adds the `k==0` branch (`advancePastVariable`: 81.8% → 90.9%).
- `cmd/scaffold`: `checkCleanWorkingTree` dirty-tree and broken-`.git` tests (26.7% → 80.0%) — prevents the scaffold from silently wiping a damaged repo.
- `internal/server`: `SendRequest` pending-map cleanup tests under ctx cancel and under `s.done` shutdown (race-tested).
- `internal/server`: `resources/read` and `prompts/get` timeout tests pin the new `ServerTimeout (-32001)` mapping.

### Changed

- `internal/tools`: the `Tool` struct now references `schema.InputSchema` / `*schema.OutputSchema` directly instead of aliasing them — `tools.InputSchema`, `tools.OutputSchema`, and `tools.Property` are removed. Callers import `internal/schema` to reach the JSON Schema types, matching `internal/prompts`. This aligns tools and prompts on the same vocabulary without a shadow namespace.
- `cmd/init` → `cmd/scaffold`: the template rewriter binary directory was renamed to remove the collision with `go mod init`. The user-facing surface (`make init MODULE=…`) is unchanged.

### Fixed

- `internal/server`: `handlers_resources.go` and `handlers_prompts.go` now map `ctx.Err() != nil` after handler return to `ServerTimeout (-32001)`, matching `tools/call` and MCP §Error Codes. Previously returned `InternalError (-32603)`. `validatePromptArgs` was extracted to keep cognitive complexity under the linter's 15.

## [1.1.3] — 2026-04-15

### Changed

- `internal/server`: the 1 415-line `server.go` was split along semantic seams into 8 cohesive files (`decode`, `dispatch`, `inflight`, `handlers`, `handlers_{initialize,resources,prompts,logging}`) so new readers can page in one concern at a time. Pure extraction — no logic, behavior, test, or public-API changes. Largest file after the split is 312 lines.

## [1.1.2] — 2026-04-14

### Added

- `docs/adr/ADR-001-stdio-ndjson-transport.md`: records the stdio + NDJSON transport choice with the alternatives considered (LSP framing, HTTP+SSE, WebSocket) and the consequences for future revisit.
- `docs/SPEC_UPGRADE.md`: the 5-step MCP spec upgrade playbook (diff the spec, update constants, update conformance fixtures, decide the bump per VERSIONING.md, tag/release/verify).
- `README.md`: one-sentence pointer from the Protocol compliance section to ADR-001.
- `cmd/init` integration test asserts that all four template-only paths (`CLAUDE.md`, `_bmad/`, `_bmad-output/`, `.claude/`) are absent post-init and that the consumer's full quality gate (build + `test -race` + lint) still passes, including after adding a second tool.

### Changed

- `cmd/init`: now deletes `CLAUDE.md`, `_bmad/`, `_bmad-output/`, and `.claude/` before the initial commit so consumers start with a clean scaffold free of the template's personal workflow artifacts. `rewriteProject` was refactored to a slice-of-steps pattern to keep cyclomatic complexity within the linter budget with the new step.
- `internal/server/claudemd_test.go`: the claim-verification test now skips when `CLAUDE.md` is absent. Consumers inherit the test file; without the skip, their CI would fail after `make init` strips `CLAUDE.md`. The three remaining ClaudeMD meta-tests (error-code coverage, dependency rules, zero external deps) stay active on consumer scaffolds.

## [1.1.1] — 2026-04-12

### Added

- `internal/server`: `resources/templates/list` as a first-class method per MCP 2025-11-25; `resources/list` no longer includes `resourceTemplates`.
- `internal/server`: `InitializeResult.instructions` field and a `WithInstructions` server option.
- `internal/server`: `logging/setLevel` now enforces RFC 5424 levels and actually drives a `*slog.LevelVar` (revives the previously dead `s.logLevel` field). Trace logs emit at `Debug`; `WithTrace` auto-elevates the logger level.
- `cmd/mcp/main`: startup errors are written to stderr before `os.Exit(1)`.
- `internal/server/testdata/conformance`: 13 new byte-exact `.response.jsonl` goldens (was 1/33); a `SendRequest` correlation test that asserts the full round-trip payload; new coverage for pointer-as-optional, `any`-as-open-schema, `rpc.*` pre-ready, duplicate-response drop, and the `hasOption` matrix.

### Changed

- `internal/server`: `rpc.*` is now rejected in any state with `-32601` (was `-32000` pre-ready).
- `internal/server`: unknown `resources/read` URIs now return `-32002 ResourceNotFound` (was `-32602 InvalidParams`).
- `internal/protocol`: decode-error wire messages are sanitized — typed sentinels for batch and depth violations, generic `"parse error"` for stdlib JSON failures; the raw error stays in stderr only. `MaxJSONDepth` was promoted to `protocol/constants.go`.
- `internal/server`: `routeResponse` now does a non-blocking send plus delete-under-lock, closing a duplicate-response deadlock vector.
- `internal/server`: `SendRequest` now has a new `s.done` channel that wakes waiters on server shutdown.
- `internal/schema`: pointer fields (`*T`) are now correctly optional (not required); `interface{}` / `any` maps to an open `{}` schema (was `"unsupported type"`); `omitempty` lookup uses comma-split instead of `strings.Contains` to eliminate substring false positives.
- `internal/server`: out-of-state `notifications/initialized` and malformed `notifications/cancelled` are now silently ignored per the notification contract.
- `internal/server`: duplicate-`initialize` error message is now contextual while in the initializing state.
- `internal/tools`, `internal/prompts`, `internal/resources`: `strings.Compare` → `cmp.Compare` across all three registries.
- `internal/server`: `errors.As` → `errors.AsType[T]` unified in tool dispatch.

## [1.1.0] — 2026-04-12

### Added

- `internal/server`: MCP `initialize` now negotiates `protocolVersion` — echoes the client's version when it matches the server's supported version, otherwise falls back to the server's version. `clientInfo` is logged at the uninitialized → initializing transition.
- `internal/server`: `ErrPendingRequestsFull` sentinel and `maxPendingRequests` cap (1024) on the server-to-client correlation map; `SendRequest` now returns the sentinel under back-pressure instead of growing the map without bound.
- `internal/protocol`: JSON nesting depth guard (`maxJSONDepth = 64`) scanned before `Unmarshal` to prevent stack-exhaustion on pathological payloads.
- New unit tests for depth guard, response classification, null `result` / `error`, `protocolVersion` negotiation, and pending-map back-pressure.

### Changed

- `internal/tools`: `InputSchema`, `OutputSchema`, and `Property` are now type aliases to the `internal/schema` types so tools and prompts share the same JSON Schema vocabulary. The conversion shim and duplicate `SchemaType*` constants were removed. Consumer source compiles unchanged. (Superseded in v1.2.0, where the aliases were removed and callers import `internal/schema` directly.)
- `internal/server`: request context is now threaded through `dispatch → handle{Resources,Prompts}{Method,Read,Get}` so client disconnect and server shutdown cancel resource/prompt handlers promptly (previously rooted at `context.Background()`).
- `internal/protocol`: response classification now rejects messages carrying both `result` and `error` per JSON-RPC 2.0 §5; a `null` value in either field is treated as absent so `{"result":null}` is no longer misclassified as a valid response.
- `internal/prompts`: argument marshal/unmarshal errors are returned as `*protocol.CodeError{InvalidParams}` at the handler boundary (consistent with `internal/tools`).
- `cmd/mcp`: `--version` now writes to stderr so stdout stays protocol-only even when the flag is invoked against the server binary.
- Release and `make build`: `-trimpath` is now enabled so `go install …@latest` and release artifacts are path-reproducible.
- `README.md`: clarified that bidirectional transport is a generic server-to-client primitive — no built-in handlers for sampling, elicitation, or roots. Added `MCP_TRACE=1` production warning (the trace output logs tool arguments).

### Fixed

- `internal/server`: `inFlightCancelled` was converted from `bool` to `atomic.Bool` and now resets at handler spawn (not post-processing) so a stale `notifications/cancelled` for a prior request cannot suppress the current handler's response.
- `internal/server`: `resources/read` and `prompts/get` parameter-unmarshal errors now include the underlying parse error instead of the opaque `"invalid ... params"` string.

## [1.0.4] — 2026-04-12

### Added

- `cmd/init`: refuses to run when the working tree has uncommitted changes. The trailing `resetGitHistory` step is destructive, so the guard prevents silent loss of in-progress edits. Pass `--force` to override.
- `internal/schema`: `time.Time` now derives as `{"type":"string","format":"date-time"}`; `json.RawMessage` derives as an unconstrained schema (any JSON); recursive types fail fast with a clear error instead of exhausting the depth budget.

### Changed

- `internal/protocol`: removed unused constants for out-of-scope methods (`MethodCompletionComplete`, `MethodResourcesSubscribe`, `MethodResourcesUnsubscribe`) and notifications (`NotificationResourcesListChanged`, `NotificationToolsListChanged`, `NotificationPromptsListChanged`). Remaining method/notification constants are alphabetized.
- `internal/server`: capability structs (`prompts`, `resources`, `tools`) are now empty objects — previously they advertised `listChanged: false` / `subscribe: false` for features the server does not implement. The capability is still advertised (key presence signals support); the zero-value flags were noise.

## [1.0.3] — 2026-04-12

### Added

- `cmd/init`: resets git history with a single clean `feat: initial version` commit on branch `main` after the template rewrite. Consumers start under their own git identity instead of inheriting the template's PRs, tags, and authorship.

## [1.0.2] — 2026-04-12

### Fixed

- `cmd/init`: validates module paths up front and rejects two-segment paths (e.g. `atruvia.de/sia-mcp`) that caused `rewriteTextFile` to skip the bare-slug pass, leaving `andygeiss/mcp` in README badge URLs and failing `verifyZeroFingerprint`. A clear error replaces the partial rewrite.

## [1.0.1] — 2026-04-12

### Changed

- `cmd/init`: no longer renames `cmd/mcp` to the module suffix — every scaffolded project ships a binary named `mcp`, regardless of module path. Makes MCP client config copy-pasteable and `go install …/cmd/mcp@latest` a universal command; the tradeoff is `$GOBIN` collision for consumers who install multiple scaffolded servers (documented in the README).
- `cmd/init`: rewrites the bare `andygeiss/mcp` slug that appears in README badge URLs (shields.io, codecov, GitHub Actions), so a scaffolded fork's badges point at its own repo out of the box. `verifyZeroFingerprint` now checks both full and short forms.
- Regression tests lock in that `.goreleaser.yml` and `.mcp.json` survive `init` byte-identical — their `cmd/mcp` paths must not drift.

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

[Unreleased]: https://github.com/andygeiss/mcp/compare/v1.3.2...HEAD
[1.3.2]: https://github.com/andygeiss/mcp/compare/v1.3.1...v1.3.2
[1.3.1]: https://github.com/andygeiss/mcp/compare/v1.3.0...v1.3.1
[1.3.0]: https://github.com/andygeiss/mcp/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/andygeiss/mcp/compare/v1.1.3...v1.2.0
[1.1.3]: https://github.com/andygeiss/mcp/compare/v1.1.2...v1.1.3
[1.1.2]: https://github.com/andygeiss/mcp/compare/v1.1.1...v1.1.2
[1.1.1]: https://github.com/andygeiss/mcp/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/andygeiss/mcp/compare/v1.0.4...v1.1.0
[1.0.4]: https://github.com/andygeiss/mcp/compare/v1.0.3...v1.0.4
[1.0.3]: https://github.com/andygeiss/mcp/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/andygeiss/mcp/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/andygeiss/mcp/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/andygeiss/mcp/releases/tag/v1.0.0
[0.6.10]: https://github.com/andygeiss/mcp/releases/tag/v0.6.10
[0.6.9]: https://github.com/andygeiss/mcp/releases/tag/v0.6.9
[0.6.8]: https://github.com/andygeiss/mcp/releases/tag/v0.6.8
[0.6.7]: https://github.com/andygeiss/mcp/releases/tag/v0.6.7
[0.6.6]: https://github.com/andygeiss/mcp/releases/tag/v0.6.6
[0.6.5]: https://github.com/andygeiss/mcp/releases/tag/v0.6.5
