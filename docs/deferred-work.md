# Deferred Work

Items carved off from in-flight specs to keep PR scope tight. Each entry names the parent spec, the carved goal, and the rationale for deferral.

---

## M1b — Decode-limit value tuning + ADR-004 + per-tool v2 plan ✅ SHIPPED

**Status:** Shipped 2026-04-29 on branch `feat/m1b-decode-limits-tuning` (stacked on M1a #66).

**Carved from:** `spec-m1-decode-structural-limits.md` (M1a) on 2026-04-29 after party-mode review hit the 1600-token spec ceiling.

**Goals to ship as a separate small PR (queued before PR #4):**

1. **Raise `MaxJSONStringLen` from 1 MiB → 4 MiB.** Mary's evidence: 1 MiB trips legitimate base64 PNG screenshots, kubectl JSON, terraform plans, fat stack traces. 4 MiB is the smallest number that doesn't block the modal vision/infra workflow on day one. (Tolerable as 1 MiB in M1a only because the non-fatal branch lets clients chunk-retry — without that branch, 1 MiB would be a regression.)
2. **Raise `maxMessageSize` envelope from 4 MiB → 16 MiB** in `internal/server/counting_reader.go`. Preserves the 1:4 inner-to-outer ratio (Winston). Update existing test fixtures (5MB-trips-4MB → 17MB-trips-16MB).
3. **Author `docs/adr/004-decode-limits.md`.** Covers M1a's design + M1b's number rationale + the v2 per-tool design (next item). Doc-comments on M1a's constants get updated in this PR to cite ADR-004.
4. **Name "per-tool schema-annotation limits" as the planned v2 in ADR-004.** Per-tool `maxBytes` annotation derived via `internal/schema` reflection from struct tags, with the global constants as absolute backstop. Don't implement — just lock the design direction so the next maintainer doesn't re-litigate.

**Why not in M1a:** combined token count exceeded the 1600-token spec ceiling; the safety floor (typed error + non-fatal branch + panic guards + RED tests) is the load-bearing correctness work and belongs in its own focused PR. M1b is value tuning + documentation — a separate, easily-reviewable concern.

**Open question to resolve before M1b:** John's "measure real payloads first" challenge. Mary triangulated from public telemetry + own session logs; firmer numbers would benefit from real client traffic measurements if available.

---

## M1a test-coverage gaps surfaced during step-04 review ✅ SHIPPED

**Status:** Shipped 2026-05-01. All four tests landed alongside the retro action item 1 cleanup.

**Carved from:** `spec-m1-decode-structural-limits.md` (M1a) on 2026-04-29 after the edge-case-hunter review of the implementation. None of these were bugs — the production behavior was correct in each case. They were coverage gaps where an explicit test hardens the invariant against future drift.

1. **In-flight + structural-limit interaction test.** ✅ `Test_Server_With_OversizedString_DuringInFlight_Should_RejectNonFatallyAndContinue` in `internal/server/integration_test.go`. Drives a slow `tools/call`, pushes an oversized `ping` mid-flight, then a valid ping. Asserts the ordering contract (structural error first → handler response → final ping).
2. **Mid-flight handler-abandonment test.** ✅ `Test_handleDecodeErrorDuringInFlight_With_StructuralLimit_AndHandlerStuck_Should_LogAbandon` in `internal/server/decode_internal_test.go` (white-box). Uses `testing/synctest` for virtual-time control; constructs an `inFlightCh` that nothing ever sends to, lets the abandon timer fire, asserts the `handler_abandoned` log + `request_id` attribute + cleared in-flight state.
3. **Alternating object/array nesting test.** ✅ `Test_checkLimits_With_AlternatingNesting_Should_CountKeysPerScope` in `internal/protocol/codec_internal_test.go`. Pins per-scope key counting (an outer object with 1 key plus an inner object at exactly the limit must pass — proves no aggregation across object scopes) and string-length scanning through alternation.
4. **Tool-timeout vs structural-limit distinguishability test.** ✅ `Test_Server_With_ToolTimeoutAndStructuralLimit_Should_BeDistinguishable` in `internal/server/integration_test.go`. Pins the wire-level contract that the two `-32001` failure modes differ by id (`2` vs `null`), message content, and `error.data` presence — operators and clients can distinguish them.

---

## S3 — Frozen schema bytes (pre-marshaled at Register)

**Carved from:** Q1 ship-list item #6 (S3+Q60). Q60 (golden snapshots) shipped on 2026-04-29; S3's pre-marshal optimization is deferred.

**Goal:** At `tools.Register[T]`, marshal the derived `inputSchema` to bytes once and cache them on the Tool. `tools/list` writes the cached bytes verbatim instead of re-marshaling the typed struct on every request.

**Why deferred:** Q60 delivers the conscious-evolution discipline (every schema change forces a golden update). S3 is a marginal allocs/op win on a non-hot path (`tools/list` is called once per session, not per tool call). Implementing it cleanly requires either a custom `Tool.MarshalJSON` or restructuring `Tool.InputSchema` from `schema.InputSchema` to `json.RawMessage` — the latter touches ~30 test sites that introspect typed schema fields. Net cost > current value.

**Revisit when:** benchmark evidence shows `tools/list` allocations matter, or when introducing a tool registration pattern (sync.OnceValues per the brainstorming) for many-tools servers.

---

## Deferred from: code review of 2-2-q5-structured-content-and-output-schema (2026-05-01)

- **Schema engine top-level non-struct hardening** — `internal/schema/schema.go:106` (`collectFields`). Calling `Register[In, Out]` with `In` or `Out` set to a non-struct (`int`, `[]string`, `map[K]V`, `time.Time`, `json.RawMessage`, `**Struct`) panics inside `t.Fields()`. Pre-existing for the input side (Q5 surfaces it on the output side too). Add an explicit `t.Kind() == reflect.Struct` guard at entry plus a special-case for top-level `time.Time` and `json.RawMessage`.
- **Opaque `-32603` for `Out` marshaling failures** — `internal/tools/registry.go:148-153`. When a handler's `Out` contains an `interface{}` field holding a non-marshalable runtime value (chan, func, cyclic struct), the response carries a generic Internal error with no hint at the offending field. Add structured `error.data` carrying the field name once we have a schema-aware introspection helper.
- **Per-tool max-output-bytes limit** — `internal/tools/registry.go:148-153`. Multi-MB `structuredContent` payloads are marshaled in full before transport-level limits trim them. Wire a max-output-bytes cap through `Register` opts so handler-level overruns surface as a tool error rather than a transport hang.
- **Schema derivation caching** — `internal/schema/schema.go`. AC4 claimed input-schema derivation was cached per `reflect.Type`; it isn't. Reflection runs on every `Register` call. Defer to a benchmark-driven pass that adds a `sync.Map` cache keyed by `reflect.Type` if `tools/list` allocations become measurable.

---

## Cross-package Clause aggregation (Story 2.1 ripple — surfaced by Story 2.2)

**Carved from:** Story 2.2 (Q5 structuredContent + outputSchema) on 2026-05-01.

**Goal:** `docs/spec-coverage.txt` should aggregate clauses registered from `_test.go` files in any package, not just `internal/protocol`. Today, `make spec-coverage` runs a single `go test` invocation in `internal/protocol/` whose binary only loads init() blocks from that package's test files. Story 2.2's Q5 Clauses are registered in `internal/server/integration_test.go` (because the test functions they reference live there) and therefore never appear in the rendered audit, despite being valid `protocol.Clause` entries.

**Possible approaches:** (a) run multiple `go test` invocations and merge JSON-formatted output; (b) introduce a unified test harness that imports all relevant test packages via a build tag; (c) split the renderer into per-package tables that live alongside their tests, then concatenate at render time. (a) is the smallest change but adds shell glue; (c) is the most idiomatic but requires per-package Render conventions.

**Why deferred:** AC7 of Story 2.2 says "Register both as Clause entries" — registration is satisfied (init blocks fire when integration tests run; compile-time enforcement is intact). The audit-rendering aggregation is a separate concern best solved with a deliberate design pass rather than smuggled into a feature story.

---

## Deferred from: code review of 2-1-a2-spec-conformance-discipline (2026-05-01)

- **`findRepoRoot` walks to ancestor `go.mod` in multi-module workspaces** — `internal/protocol/spec_clauses_test.go:202-218`. The helper walks parent directories until the first `go.mod`. In a Go 1.18+ workspace (`go.work`) where this repo becomes an inner module under a parent that also has `go.mod`, the walk could land on the wrong module root and write `docs/spec-coverage.txt` outside the intended tree. Edge Case Hunter and Blind Hunter both flagged this. Deferred: the project is a single Go module today and there is no plan to introduce `go.work`. Revisit if the project is ever vendored as part of a larger workspace.

---

## Deferred from: code review of 2-3-q6-progress-token-passthrough-discipline (2026-05-01)

- **`warnLogger` nil-derefs when both `p.logger` and `p.server` are nil** — `internal/server/progress.go:93-98`. Production constructor at `inflight.go:119` always sets `server`, so this is a defensive concern only. Add a `slog.Default()` fallback in the next progress.go sweep.
- **`newProgressTestServer` constructs a partial `*Server` literal** — `internal/server/progress_internal_test.go` test helper. Fragile if `sendNotification` grows new dependencies (writeDeadline, capability check, etc.). Refactor to use the production `NewServer` constructor with injected buffers in the next test-hygiene pass.
- **`protocol.Clause` registrations duplicate metadata; no `ID` uniqueness enforcement** — Pre-existing Story 2.1 pattern. Out of scope for 2.3. Track in a future "clause-registry hardening" item: dedupe-on-Register or compile-time-unique IDs.
- **`init()` blocks register clauses at package load even for unrelated `go test -run`** — Pre-existing pattern shared by all clause-tracked tests. Not actionable per-story; track with the clause-registry hardening item above.
- **No concurrent-`Report`-from-two-goroutines test while `suspendForOutbound` is active** — `atomic.Int32` protects the counter, so concurrent loads see consistent values. Coverage extension only; add when a broader concurrency-stress sweep happens.
- **`Test_Progress_Report_Without_Token_Should_NotWarn` only asserts a specific warn substring** — Future unrelated stderr writes would not fail the test. Tighten to an exhaustive stderr-empty assertion when the test scaffolding allows it.
- **`outboundDepth atomic.Int32` by value, no `noCopy` lint guard** — `internal/server/progress.go:16-21`. Only matters if a future method is added with a value receiver. Add `_ noCopy` (or a `noCopyChecker` lint) when revisiting the struct layout.
- **`stdoutW` pipe in integration tests is never explicitly closed** — `internal/server/progress_test.go` scaffolding. Reader may block on pipe-buffer drain after server exit, hiding test races. Add `defer stdoutW.Close()` next time the scaffolding is touched.
- **Nil-receiver `Report` path while `suspendForOutbound` was called on a non-nil receiver** — Coverage gap. The nil-receiver test only exercises `suspendForOutbound`, not the combination. Add when extending the white-box matrix.
- **No test for asymmetric / forgotten `suspendForOutbound` release** — Current behavior (Report stays dropped indefinitely if release is forgotten) is correct but unpinned. Add a regression guard test.
- **`loggerFromContext` fallback semantics not in diff** — `internal/server/inflight.go:121` reaches `loggerFromContext(callCtx, s.logger)` but the function body lives outside this story. Verify in the next progress/inflight sweep that `withRequestLogger` always runs before `Progress` construction; otherwise AI10 warns lose `request_id`.

---

## Deferred from: code review of 3-2-bidi-test-hygiene-sweep (2026-05-02)

- **`initRequest` constant omits `protocolVersion`** — `internal/server/server_test.go:59`. Used by the AI9 negative-path synctest tests via `handshake()`. AC5 of Story 3.2 was scoped to bidi-specific `bidirHandshake` literals only because updating the shared constant touches dozens of non-bidi tests. If the server tightens initialize-time validation, this will need a coordinated sweep.
- **`5 * time.Second` ctx timeout in pipe-based bidi tests may be tight under `-race` on shared CI** — `internal/server/server_test.go:3660`, `:3761`; `internal/server/progress_test.go:210`. Bump to 30s if flake observed in CI.
- **`strings.Count(stdout, "\"jsonrpc\":\"2.0\"")` brittle if a future tool result echoes a JSON-RPC body verbatim** — `internal/server/synctest_test.go:235-236, 295-296`. Decode-based counting via `parseResponses` requires moving the helper out of the integration build tag.
- **`protocol.MCPVersion` string-concatenated unescaped into handshake JSON** — `internal/server/server_test.go:3673, 3771`; `internal/server/progress_test.go:220`. Cosmetic; the constant `"2025-11-25"` has no JSON-significant chars. `json.Marshal(map[string]any{...})` would be more robust but adds noise.
- **`assertCleanStderr` ignores DEBUG/INFO levels** — `internal/server/server_test.go:64`. Explicit AC scope per spec; future test coverage might want INFO-level guards on specific paths.
- **Sibling-test cosmetic divergence: sampling test uses multi-line goroutine spawn, elicitation uses single-line** — `internal/server/server_test.go:3663, 3771`. Pre-existing style drift, no functional impact.

---

## Deferred from: code review of 3-1-cross-package-clause-aggregation (2026-05-02)

- **Header constant duplicated across `protocol.Render` and `cmd/spec-coverage/main.go`** — `cmd/spec-coverage/main.go:36`. Defense-in-depth nicety: if `protocol.Render`'s header schema ever changes, fragments and aggregate would silently disagree for one release window. Re-export the constant from `internal/protocol` or have the aggregator preserve the first fragment's header and assert agreement.
- **`findRepoRoot` triplication** — three copies now exist (`internal/protocol/spec_clauses_test.go`, `internal/server/spec_coverage_test.go`, `cmd/spec-coverage/main.go`). Extends the pre-existing workspace risk; hoist into a shared (testable) location or accept the duplication and document it in all three sites.
- **Fragment regen `os.WriteFile` mode `0o600`** — `internal/protocol/spec_clauses_test.go:323` and `internal/server/spec_coverage_test.go:55`. Differs from repo's tracked-file `0o644` convention but matches the pre-existing protocol fragment test. Consider `0o644` in next test-hygiene sweep.
- **`mv $$TMP docs/spec-coverage.txt` cross-mount** — `Makefile:111-117`. Use `mktemp ./docs/.spec-coverage.XXXXXX` for atomic same-filesystem rename. Rare disk-full failure mode leaves a partial aggregate.
- **`mktemp` failure no contextual diagnostic** — `Makefile:109`. Rare edge case (`/tmp` full or read-only).
- **`bytes.TrimRight(data, "\n")` strictness inconsistency** — `cmd/spec-coverage/main.go:103`. Aggregator accepts multiple trailing newlines while per-fragment drift detection rejects them. No production impact.
- **`findRepoRoot` returns bare `os.ErrNotExist`** — three sibling sites. Cosmetic; would benefit from `fmt.Errorf("no go.mod found walking up from %q", wd)`.
- **Makefile fragment-test grep cannot distinguish "test not invoked" from "test panicked before PASS/FAIL"** — `Makefile:91-99`. Rare edge case, build still fails with non-zero exit.
- **Aggregator emits header-only output when all fragments are empty** — `cmd/spec-coverage/main.go:58-66`. Defense-in-depth gap; per-fragment drift detection plus implicit registry non-empty assertions catch the underlying invariant today.
- **Fragment tests render the entire global registry, not a package-scoped subset** — `internal/protocol/spec_clauses_test.go:319` and `internal/server/spec_coverage_test.go:48`. Theoretical isolation risk if a future `_test.go` causes cross-package init-block leakage in a test binary. Currently safe because Go test binaries do not load `_test.go` files of imported packages.

---

## Deferred from: code review of 2-4-q18-elicitation-create-outbound (2026-05-01)

Theme: bidi-test infrastructure hygiene. The new elicitation tests mirror the existing sampling siblings exactly, so all of these apply to BOTH the sampling and elicitation tests in `internal/server/server_test.go` and `internal/server/synctest_test.go`. Address them as a single test-hygiene sweep, not per-test.

- **No timeout/deadline on the bidi happy-path tests** — `srv.Run(context.Background())` runs in a goroutine; `dec.Decode` calls block forever if the server stalls or exits before emitting the expected outbound. The Go test framework's outer timeout is the only safety net. Wrap with `context.WithTimeout(t.Context(), 5*time.Second)` and `defer cancel()`, plus deadline-bounded decode helpers.
- **`t.Fatalf` on decode/write paths leaks the `Run` goroutine** — `done` channel is never read on the failure path; the goroutine + handler stack stay live across parallel tests. Wrap with `t.Cleanup(func(){ _ = stdinW.Close(); <-done })`.
- **`stdinW.Close` error silenced (`_ = stdinW.Close()`)** — multiple-close or already-errored pipe goes unnoticed. Check the error.
- **Bidi tests do not assert total stdout message count** — only specific-substring checks. A regression that emits side effects via a different method name would slip past. Add "exactly N expected messages" assertions.
- **Happy-path bidi tests capture `stderr` but never read it** — a future warn-level regression would not fail the test. Add a clean-stderr assertion.
- **`bidirHandshake` omits `protocolVersion` field** in `params` — pattern shared across all bidi tests. If the server tightens initialize-time validation, every bidi test breaks at once. Add `"protocolVersion":"2025-11-25"` to the handshake constants.
- **`stdoutW` pipe never explicitly closed** in happy-path tests — reader can block on pipe-buffer drain after server exit. Add `defer stdoutW.Close()`.

