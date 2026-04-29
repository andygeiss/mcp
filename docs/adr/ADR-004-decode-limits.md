# ADR-004: Decode-time Structural Limits

## Status

Accepted — 2026-04-29

## Context

The decoder accepts JSON-RPC 2.0 messages from an untrusted peer over stdio. Two cheap classes of attack exist against any decoder that does not gate raw input *before* allocation:

1. **Stack-exhaustion via depth.** Pathological nesting (`{"a":{"a":{"a":...}}}` × 10 000) blows the goroutine stack before the protocol layer ever sees the message.
2. **Memory amplification via cardinality.** A single legal-shape JSON object with 10⁶ keys, or a single string field carrying a multi-MiB blob, drives `json.Unmarshal` into millions of small allocations or a single very large one — both before any tool handler runs.

The transport already enforces a 4 MiB per-message envelope (counting reader) and a depth cap of 64. That left two structural gaps: per-object key count and per-string byte length. This ADR documents how those gaps are closed, the values chosen for each cap, the wire-error semantics, and the design direction for the v2 follow-up.

This work consolidates the M1 ship-list item from the Q1 2026 plan (`_bmad-output/planning-artifacts/q1-plan-2026-04-29.md`), shipped in two PRs:

- **M1a** (PR #66) — typed error, single-pass scanner with bounds-checked depth tracking, non-fatal mapping for the new structural-limit class. Conservative initial values (1 MiB string, 4 MiB envelope) chosen to ship the safety floor without coupling to value-tuning debate.
- **M1b** (this ADR's PR) — value tuning + envelope raise + this ADR.

## Decision

### Constants — compile-time only

| Cap | Constant | Value | Location |
|---|---|---|---|
| Nesting depth | `MaxJSONDepth` | 64 | `internal/protocol/constants.go` |
| Keys per object | `MaxJSONKeysPerObject` | 10 000 | `internal/protocol/constants.go` |
| String byte length | `MaxJSONStringLen` | 4 MiB (`4 << 20`) | `internal/protocol/constants.go` |
| Per-message envelope | `maxMessageSize` | 16 MiB | `internal/server/counting_reader.go` |

All four are `const`. **No flags, no env, no runtime tuning.** Server-builders who need different limits override at compile time by editing the constant, recompiling, and shipping. The rationale is in the [No flags] section below.

Limits are enforced **before** `json.Unmarshal` allocates structured values. The depth/keys/string caps run in a single byte-pass over the raw `json.RawMessage` (`internal/protocol/codec.go::checkLimits`); the envelope cap runs at the read layer (`internal/server/counting_reader.go`).

### Wire-error semantics

| Cap | Code | Disposition | Wire message |
|---|---|---|---|
| Depth (`MaxJSONDepth`) | `-32700` | Fatal — connection closes | `"json nesting exceeds max depth 64"` |
| Envelope (`maxMessageSize`) | `-32700` | Fatal — connection closes | `"message exceeds 16MB size limit"` |
| Keys (`MaxJSONKeysPerObject`) | `-32001` | **Non-fatal** — connection survives | `"payload exceeds maxKeysPerObject: <actual> > <max>"` |
| String length (`MaxJSONStringLen`) | `-32001` | **Non-fatal** — connection survives | `"payload exceeds maxStringLength: <actual> > <max>"` |

The asymmetry is deliberate. Depth and envelope breaches are corruption-adjacent — the message is structurally malformed in a way that makes resuming the byte stream risky. Key-count and string-length breaches are *policy rejections* of a structurally well-formed message: the codec chose to reject because of size, not because the bytes were malformed. Keeping the connection alive lets an LLM client adapt and retry with a smaller payload instead of dropping the session and surfacing an opaque "tool failed" to the human.

The new failures use the typed error `*protocol.StructuralLimitError{Limit, Actual, Max}` (defined in `internal/protocol/codec.go`). The fields are forward-compatible with the structured `error.data` field that PR #4 (M2 + M3 + Q41) will add holistically across all decode errors.

### No flags

The "no flags, no env" rule comes from CLAUDE.md and is load-bearing here for two reasons specific to MCP's threat model:

1. **The launcher is often untrusted.** MCP servers are subprocesses spawned by an orchestrator (Claude Desktop, VS Code, an arbitrary CLI). A flag or env var means the limit is set by whoever calls `exec()` — exactly the boundary the cap is supposed to defend. An attacker who can choose `--max-string-len=999999999` has bypassed the floor.
2. **Constants are auditable.** A compile-time `const` is greppable, fuzzable, citable in a CVE writeup, and reviewable in a single PR. A flag is a footgun that ships with every release.

Server-builders forking via `make init MODULE=...` retain full control: edit the constant, recompile, ship. That is the supported tuning path.

## Number rationale (v1)

The values are engineering judgment grounded in observed payload sizes for typical MCP traffic:

- **Depth = 64** — far above any legal MCP message. Any payload that nests deeper is either an attack or generated code that the server has no business decoding.
- **Keys = 10 000** — covers any plausible config-blob or batch-style argument. No legitimate MCP request has been observed approaching this.
- **String = 4 MiB** — covers the modal large argument:
  - Linux kernel-style source files (~230 KB),
  - kubectl/terraform JSON for small clusters (1–4 MiB),
  - base64-encoded 1080p screenshots (~1.4–1.8 MiB raw → ~2.4 MiB encoded).
  4K screenshots (~6–8 MiB encoded), large monorepo `npm ls --json` (5–50 MiB), and busy-cluster `kubectl get events -A` (3–10 MiB) **will exceed 4 MiB**. Those are exactly the cases v2 (per-tool annotations) is designed for — see [v2 plan](#v2-plan-per-tool-schema-annotation-limits).
- **Envelope = 16 MiB** — preserves a 1:4 string-to-envelope ratio so a single max-size string plus envelope, metadata, and a second medium-sized field fit under the cap. Sits comfortably below the 32 MiB threshold where Go's allocator (Green Tea GC, Go 1.26) starts showing measurable pressure on a single decode.

Honest hedge: these numbers are triangulated from public tool telemetry and small private samples, not a controlled study against real client traffic. The **shape** of the defense is well-precedented (gRPC's 4 MiB cap, Linux kernel JSON parsers in `nft`/`nftables`); the **specific magnitudes** are revisable. The non-fatal disposition of the new caps is the safety net that makes revising them later cheap — clients survive a too-tight limit and degrade rather than crash.

## v2 plan: per-tool schema-annotation limits

A single global cap forces the worst-case tool to set the ceiling for every attack surface. `review_code` legitimately wants several MiB of input; `analyze_screenshot` may want 16 MiB; `ping` wants 4 KB. The right shape of the v2 fix is **per-tool declared limits, derived from each tool's input schema**, with the global constants as the absolute backstop.

Sketch (not implemented in this PR):

```go
type Tool[T any] struct {
    // ...existing fields...
}

// MaxBytes annotation on a struct field becomes part of the derived JSON
// schema and is enforced at decode dispatch.
type ApplyPatchInput struct {
    Patch string `json:"patch" description:"unified diff" mcp:"maxBytes=16777216"`
}
```

The schema engine (`internal/schema/schema.go`) already does reflection-based derivation. Adding a `mcp:"maxBytes=…"` tag annotation extends that pipeline naturally. Per-tool limits are evaluated *after* the global structural-limit caps, so the global cap remains the security floor regardless of any per-tool annotation.

This work is **not in scope** for this ADR's PR. It is named here so the next maintainer does not re-litigate the design direction. v2 will get its own ADR when it ships.

## Consequences

- **Predictable security floor.** Three structural caps + one envelope cap, all compile-time, all enforced before allocation.
- **Better LLM ergonomics.** Non-fatal `-32001` for the new caps lets clients adapt; the connection no longer dies on every too-large tool argument.
- **Test discipline.** All decrement paths in the byte-scan are bounds-checked; black-box tests plus a fuzz target (`Fuzz_Decoder_With_ArbitraryInput` with 11 new structural-limit seeds) prove no panic on malformed input.
- **Forward compatibility.** `*StructuralLimitError` carries `Limit`/`Actual`/`Max` fields ready for PR #4 to serialize into `error.data` without re-typing.
- **Doc-comment drift risk.** Every constant carries a doc-comment citing this ADR. Future tuning must update both.

## Alternatives considered

- **Flag/env tuning.** Rejected: defeats the threat model (see [No flags](#no-flags)).
- **Single 16 MiB string-cap (= envelope) and skip the per-string check.** Rejected: collapses two limits into one and loses defense-in-depth. A single string filling the whole envelope is exactly the shape of attack the per-string cap is meant to flag early.
- **Two-pass scan (one for depth, one for keys/string).** Rejected: shared `inString`/`escaped` state machine means one pass is strictly cheaper, and bounds checks are local to push/pop helpers — fusion does not increase the bug surface (verified by adversarial review on M1a).
- **Cumulative `io.LimitReader` for envelope.** Already rejected pre-M1; the limit must reset per message. Documented in the counting reader.

## References

- M1a PR: [#66](https://github.com/andygeiss/mcp/pull/66) — typed error, single-pass scanner, non-fatal mapping.
- Threat model classes closed: depth-bomb (stack exhaustion), key-cardinality amplification, single-string memory amplification.
- Related ADRs: [ADR-001](./ADR-001-stdio-ndjson-transport.md) (envelope sits at the stdio boundary), [ADR-003](./ADR-003-bidi-reader-split.md) (bidi reader lives downstream of these caps).
