# ADR-002: Internal Package Layout

## Status

Accepted — 2026-04-16

## Context

The `internal/` tree holds seven packages: `assert`, `prompts`, `protocol`, `resources`, `schema`, `server`, `tools`. Each periodic review raises the same question: "should this be flatter, deeper, or reorganized?" Without a durable decision record, the question gets re-litigated on every refactor pass, and minor renames creep in that churn import paths without changing behavior.

A structured audit (ten parallel investigations covering inventory, external Go-project conventions, cognitive-load hotspots, per-package evaluation of `schema` and `assert`, registry-consolidation analysis, protocol/server boundary check, naming audit, a devil's-advocate defense of the status quo, and a comparison against peer MCP SDK implementations) was run on 2026-04-16 to settle the layout.

Findings:

- **Dependency graph is clean.** `protocol`, `schema`, `resources`, and `assert` are leaves with zero internal dependencies; `tools` and `prompts` import only `protocol` + `schema`; `server` depends on all registries; `cmd/mcp` wires it. No cycles, no cross-cutting helpers.
- **External Go projects (Hugo, conc, dagger, helm, terraform, etcd) converge on domain-noun packages with minimal count.** Our seven packages name MCP concepts (tools/prompts/resources) and the mechanics they require (protocol/schema/server); none is a layer or utility bucket.
- **The three registries are load-bearing.** `tools`, `prompts`, and `resources` each map 1:1 to an MCP noun (`tools/list`, `prompts/list`, `resources/list`). Their ~90 LOC of superficial duplication hides meaningful divergence — tools validate JSON input and support `OutputSchema`; prompts accept `map[string]string` handlers; resources split into static-lookup and RFC 6570 template matching. A shared generic would push complexity into parameterization without removing it.
- **The `protocol`/`server` boundary is clean.** Wire types and codec live in `protocol`; lifecycle, dispatch, transport wiring, and registry composition live in `server`. Leakage check found no types in the wrong package.
- **Peer MCP SDKs vary.** The Python SDK splits `mcpserver/{tools,prompts,resources}` like we do; `mark3labs/mcp-go` and the official `modelcontextprotocol/go-sdk` are flatter. Our layout is defensible for a minimal scaffold; our `internal/schema/` is sharper than any peer.

## Decision

Keep the seven-package layout. Specifically:

- **`internal/protocol`** — JSON-RPC 2.0 wire types, codec, and constants. Zero internal dependencies.
- **`internal/schema`** — reflection-based JSON Schema derivation. Imported by `tools` and `prompts`. Stays separate; merging into `protocol` would pull reflection into a package that otherwise only deals with the wire.
- **`internal/tools`, `internal/prompts`, `internal/resources`** — three registries, one per MCP noun. The ~90 LOC cross-registry duplication is accepted; it is not duplication of behavior (each registry is its own domain).
- **`internal/server`** — MCP lifecycle, dispatch, and transport.
- **`internal/assert`** — focused test-assertion primitive (`assert.That`). 17 LOC, 1065 call sites. Kept because it is a single typed function, not a utils grab-bag; the "no helpers" guardrail in CLAUDE.md targets the latter.

Two narrow cleanups were applied in the same change that produced this ADR:

1. **`tools` no longer aliases `schema.*` types.** `tools.InputSchema`, `tools.OutputSchema`, and `tools.Property` are removed; the `Tool` struct references `schema.InputSchema` / `*schema.OutputSchema` directly. `prompts` already imported `schema` directly — the two packages now share one vocabulary without a shadow namespace.
2. **`cmd/init` renamed to `cmd/scaffold`.** The old name read as a sibling of `go mod init` but was a one-shot template rewriter. The `make init MODULE=…` user-facing target is unchanged.

## Consequences

- Future refactor proposals that touch package boundaries must rebut this ADR. The bar is a concrete problem that a reorganization solves — not an aesthetic or novelty claim.
- The `ContentBlock` type is deliberately not unified across `tools`, `prompts`, and `resources`. The MCP spec defines different field sets per context (tools allow `data`/`mimeType`/`uri`, prompts only `text`/`type`, resources use `blob`/`mimeType`/`uri`). Unification would either produce a confusing superset struct or an interface that adds more surface than it removes.
- The handler-wrapping boilerplate in each registry's `Register[T]` is accepted. Each wrapper's unmarshal-and-validate logic is registry-specific (typed struct, map-of-strings, URI dispatch).
- `internal/schema` remains its own package even though it has only two importers. Folding it would break `protocol`'s zero-internal-deps invariant or bury a reusable reflection engine inside one of its two consumers.

## Alternatives considered

- **Unify the three registries behind `internal/registry/Registry[T]`** — rejected: moves ~90 LOC of shallow duplication into type-parameter gymnastics that obscure per-noun differences.
- **Fold `internal/schema` into `internal/protocol`** — rejected: `protocol`'s zero-internal-deps property is worth more than the package-count reduction; `schema` is reflection, not wire concerns.
- **Split `internal/server` into `transport/` + `dispatch/` + `lifecycle/`** — rejected: the current file-per-concern split within one package (`dispatch.go`, `decode.go`, `inflight.go`, `progress.go`, `handlers_*.go`) already gives that structure without cross-package friction.
- **Consolidate `ContentBlock` types** — rejected: mirrors MCP spec divergence; consolidation obscures intent.
- **Remove `internal/assert` in favor of stdlib `t.Errorf`** — rejected: 1065 call sites, 17 LOC of helper code, single focused primitive. Churn cost greatly exceeds clarity gain.
- **Rename plural packages (`tools`/`prompts`/`resources`) to singular** — rejected: Go stdlib itself is mixed (`errors`, `strings` are plural); the plural correctly signals a collection of many items in each.

---

Supersedes no prior ADR. See [ADR-001](ADR-001-stdio-ndjson-transport.md) for the transport decision that frames this layout.
