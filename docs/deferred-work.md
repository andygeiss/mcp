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
