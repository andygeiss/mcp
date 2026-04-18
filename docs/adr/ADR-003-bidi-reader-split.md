# ADR-003: Bidi Reader-Split

## Status

Accepted — 2026-04-18

## Context

v1.3.0 brings the bidirectional trio (sampling, elicitation, roots) — three primitives that require the server to *initiate* JSON-RPC requests to the client and correlate the eventual responses. Until now, the stdin reader has been monodirectional: every framed message either was a request the server answered or a notification the server processed. With v1.3.0, the same stream now carries three shapes:

1. **Inbound request** — `id` + `method` set; server dispatches and replies (existing path).
2. **Inbound response** — `id` set, `method` absent; correlates a server-initiated request and unblocks the awaiting handler goroutine (new in v1.3.0).
3. **Inbound notification** — no `id`; routed to `handleNotification`, which already handles `notifications/cancelled` and `notifications/initialized` per CLAUDE.md.

The G4 pre-authorship inspection (Story 1.1) confirmed that inbound `notifications/cancelled` is **already wired** in the existing dispatch loop (`internal/server/dispatch.go:117-130`), so the classifier topology must be **three-shape**, not two — see [g4-inbound-cancel-finding-2026-04-18.md](../../_bmad-output/planning-artifacts/g4-inbound-cancel-finding-2026-04-18.md) for the full citation trail.

This ADR settles how the new response-correlation surface is wired into the reader without breaking the architectural invariants:

- **AI7 (cancellation chain).** A client `notifications/cancelled` for an in-flight inbound request still cancels the handler context and suppresses its response.
- **AI8 (typed errors).** Errors crossing the reader-pending-map boundary are sentinels in `internal/protocol`, not anonymous `errors.New` strings.
- **AI9 (capability gate).** The server may not initiate sampling/elicitation requests unless the client advertised support during `initialize`.
- **AI10 (no progress during outbound).** A handler awaiting an outbound response may not interleave its own `notifications/progress` with the awaited reply.

`maxInFlight: 1` (advertised under `experimental.concurrency`) is preserved for **inbound** dispatch. The outbound axis is independent: a single in-flight handler may issue multiple sequential outbound requests, each correlated by ID.

## Decision

A **single reader goroutine** consumes stdin and demuxes the three shapes. Outbound correlation lives in a **mutex-protected pending-request map** owned by the server.

- **Reader.** One goroutine. Reads `*json.Decoder` (existing), classifies each message:
  - `id` + `method` → existing dispatch path (handler goroutine spawned).
  - `id` only (no `method`) → look up `id` in pending map; deliver to the entry's `respChan`.
  - no `id` → `handleNotification` (existing).
- **Pending map.** `map[string]chan *protocol.Response`, key = string-form of the outbound ID. Guarded by `sync.Mutex`. Insert on send, delete on receive or context-cancel.
- **`respChan` shape.** `chan *protocol.Response` with **buffer 1**. The reader writes once and never blocks; the awaiting goroutine reads once. Buffer-1 means the reader is not coupled to the handler's selection latency.
- **Outbound ID namespace.** `atomic.Int64`, monotonically incremented; serialized as the JSON-RPC `id`. Distinct from inbound IDs (which echo the client's `id`).
- **Writer.** A single `*json.Encoder` over `os.Stdout` guarded by `sync.Mutex` (existing `s.stdoutMu`). All writes — responses, notifications, **outbound requests** — pass through this mutex. No reorder, no interleave.
- **Shutdown.** A shared `chan struct{}` closed by the reader on EOF / fatal decode error. All pending entries select on `<-respChan` *and* `<-ctx.Done()` *and* `<-shutdownCh`. The cleanup path drains the pending map by closing each entry from the awaiting side via context cancel — the reader itself never closes any `respChan`.

The `Peer` interface — see Peer Stability Surface below — is the only type a tool/prompt handler ever sees for outbound work. It is extracted into `internal/protocol` so that handler packages do not import `internal/server`.

## Consequences

- **Synctest-bubble safe.** A single reader plus a single mutex makes the entire bidi machine deterministic under `synctest.Bubble`. Story 2.5 will land scenarios for: in-flight outbound + reader EOF, double-cancel race, capability-gated outbound rejection, late response after cancel, writer-mutex contention.
- **Story 2.3 cascade.** Cluster A's pointer-return migration of `(*Server).SendRequest` flips response shape from `protocol.Response` to `*protocol.Response`. The G8 audit ([g8-server-test-breakage-audit-2026-04-18.md](../../_bmad-output/planning-artifacts/g8-server-test-breakage-audit-2026-04-18.md)) found 26 single-response value decls + ~30 slice decls in `internal/server/server_test.go` that the `runServer` helper signature change handles in one anchor edit. Items (b) typed-error refs and (d) depguard violations are already at zero — no migration churn.
- **Code clarity.** The reader gains a 3-arm switch but loses no existing branches. The pending map is one struct, one mutex, two methods (`add` / `deliver`). Total new server LOC is on the order of 150 lines.
- **Existing dispatch loop is preserved.** Inbound request dispatch, the in-flight cancellation flag, and the response-suppression logic (`internal/server/inflight.go`) are untouched. The G4 invariant — `notifications/cancelled` cancels the in-flight handler context and suppresses the response — is enforced by the existing tests at `internal/server/server_test.go:1278, 1315, 1662, 2803`, which MUST continue to pass unmodified.
- **No drain on shutdown.** SIGTERM cancels the server context; pending outbound requests receive their context-done signal and return `protocol.ErrServerShutdown` to the awaiting handler. No "wait for stragglers" — exit promptly per ADR-001's stdio contract.

## Peer Stability Surface

The `protocol.Peer` interface is the **public API contract** between handler code (in `internal/tools`, `internal/prompts`) and the server's outbound machinery. Once shipped in v1.3.0, its method set and parameter/return types are a commitment for the lifetime of v1.x.

**Stability rules:**

1. Adding a method to `Peer` is a MAJOR-version change (per [VERSIONING.md](../../VERSIONING.md)) because consumer mocks/fakes must be updated.
2. Removing a method from `Peer` is a MAJOR-version change.
3. Changing the parameter or return type of any existing method is a MAJOR-version change.
4. Method signatures MUST use only stdlib types and `protocol`-native types. Specifically forbidden:
   - No `*server.Server` return types.
   - No `internal/tools`-typed or `internal/prompts`-typed parameters or returns (would create import cycles and break the layered dep direction set in ADR-002).
   - No third-party-package types in the signature.

Allowed in signatures: `context.Context`, `error`, `string`, primitive types, `json.RawMessage`, anything declared in `internal/protocol`.

The minimum v1.3.0 method set is:

```go
type Peer interface {
    SendRequest(ctx context.Context, method string, params any) (*Response, error)
}
```

Future bidi primitives (e.g., `ListRoots`, `CreateMessage`) MAY add typed convenience methods; each addition is a MAJOR bump and a separate ADR amendment.

## Invariants

The reader-split MUST preserve the following architectural invariants from the PRD:

| ID | Invariant | Enforcement point |
|---|---|---|
| **AI7** | Inbound `notifications/cancelled` cancels handler ctx and suppresses response. | `dispatch.go:117-130` (G4 finding); existing tests at `:1278, :1315`. |
| **AI8** | Errors crossing the reader↔pending-map boundary are typed sentinels in `protocol`. | `protocol.ErrServerShutdown`, `protocol.ErrPendingFull`, `protocol.ErrCapabilityNotSupported` (Story 2.1). |
| **AI9** | Outbound sampling/elicitation rejected if client did not advertise the capability during `initialize`. | Capability gate at `Peer.SendRequest` (Story 2.4). |
| **AI10** | Handler awaiting an outbound response may NOT interleave `notifications/progress` with the awaited reply. | Documented invariant on `Progress.Report`; enforced by single writer mutex. |

## Failure Modes

The minimum scenario set the reader-split MUST handle correctly. Each is exercised by Story 2.5 synctest scenarios.

1. **Malformed JSON mid-session with in-flight outbound.**
   *Trigger:* client sends `{"jsonrpc":"2.0","id":...` then EOF (no closing brace) while a handler is awaiting a response.
   *Outcome:* fatal decode → reader closes `shutdownCh` → all pending entries observe shutdown → handlers receive `protocol.ErrServerShutdown`. Server exits 1 (per CLAUDE.md transport rules).
   *Invariant preserved:* AI8 (typed sentinel, not `errors.New`).

2. **Client disconnect (EOF) with pending entries.**
   *Trigger:* client closes stdin while handler awaits outbound response.
   *Outcome:* `io.EOF` on decode → reader closes `shutdownCh` → pending entries get `protocol.ErrServerShutdown`. Server exits 0 (clean shutdown per CLAUDE.md).
   *Invariant preserved:* AI8 + clean-shutdown contract.

3. **SIGTERM during handler outbound-awaiting.**
   *Trigger:* server context cancelled via signal while a tool handler awaits sampling response.
   *Outcome:* handler's `select` observes `<-ctx.Done()` → returns `ctx.Err()` (`context.Canceled`) → cleanup deletes entry from pending map. Server exits 0 promptly. No drain.
   *Invariant preserved:* graceful-shutdown contract from ADR-001.

4. **Late response for cancelled outbound.**
   *Trigger:* handler cancels its outbound (ctx-done fires; entry deleted from pending map). The client's response arrives 50ms later.
   *Outcome:* reader looks up the ID, finds no entry, logs at `warn` level (`late_outbound_response`), drops the message. No double-deliver, no panic.
   *Invariant preserved:* idempotent cleanup; no goroutine leak.

5. **Writer-mutex contention during reader-initiated error-response write.**
   *Trigger:* handler is mid-write of an outbound request via `enc.Encode` (holds `stdoutMu`) when the reader detects a malformed inbound and needs to write an error response.
   *Outcome:* reader's write blocks until handler releases `stdoutMu`; messages serialize, no interleave on stdout.
   *Invariant preserved:* stdout protocol-only contract from CLAUDE.md (no torn JSON frames).

## Alternatives Considered

- **Dedicated writer goroutine with a write channel.** Rejected. Adds an extra goroutine and a buffered channel for negligible benefit at this scale (outbound volume is bounded by sequential `maxInFlight: 1`). The mutex-guarded encoder is simpler and removes shutdown-coordination edge cases (channel close vs goroutine exit).

- **Separate correlator goroutine for response routing.** Rejected. Splits atomicity of *insert-into-pending-map* and *await-on-respChan* across two goroutines, requiring an additional sync primitive. The current design lets the sender perform both halves under one critical section.

- **Close `respChan` on cleanup as the cancel signal.** Rejected. Races the late-response path: if the reader is mid-write to a `respChan` that the handler is concurrently closing, behavior is undefined. The convention is **NEVER close `respChan`**; cancellation is signalled via `ctx.Done()`, and the entry is removed from the pending map by the goroutine that owns it.

- **Two-shape classifier (assume cancel deferred to v1.4.0).** Rejected based on G4 finding: inbound cancel is already implemented and must remain functional. Two-shape would require either deleting working code or pretending it doesn't exist — both worse than the three-shape design.

- **Use `sync.Map` for pending requests instead of `map + sync.Mutex`.** Rejected. `sync.Map` is optimized for read-heavy or disjoint-key workloads; here every operation is paired insert+delete with high contention on the same key range. A plain map with a mutex is the right fit and matches existing patterns in `internal/server/inflight.go`.

---

Supersedes no prior ADR. See [ADR-001](ADR-001-stdio-ndjson-transport.md) for the transport contract and [ADR-002](ADR-002-internal-package-layout.md) for the package-layout invariants this design respects.
