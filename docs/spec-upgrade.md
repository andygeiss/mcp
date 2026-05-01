# MCP Spec Upgrade Playbook

When the MCP specification publishes a new version (e.g., `2025-11-25` → a future date), follow this checklist in order. Every step has a verifiable outcome; none can be skipped.

## 1. Diff the spec

Open the [MCP specification](https://github.com/modelcontextprotocol/specification) at the new tag. Compare it to the prior version across the surface this project implements; the [README Scope section](../README.md#scope) is the source of truth. Out-of-scope methods (`resources/subscribe`, `completion/complete`, `roots/list`, `sampling/*`, `elicitation/*`, `*/list_changed`) stay rejected with `-32601` unless the narrow-scope stance is explicitly revisited.

Focus areas:

- **Lifecycle** — `initialize`, capability negotiation, `notifications/initialized`
- **Tools** — `tools/list`, `tools/call`, `_meta.progressToken`, input-schema shape
- **Resources** — `resources/list`, `resources/read`, URI templates
- **Prompts** — `prompts/list`, `prompts/get`, argument derivation
- **Logging** — `logging/setLevel`, `notifications/message`
- **Progress and cancellation** — `notifications/progress`, `notifications/cancelled`
- **Error semantics** — new codes, changed meanings

Record every breaking change, additive change, renamed field, and capability shift in the CHANGELOG draft before touching code.

## 2. Update the version constants

- `internal/protocol/constants.go` — bump `MCPVersion` to the new date. This is the only source of truth.
- Sweep all checked-in docs for the old version string and update every occurrence:

  ```bash
  grep -rln "$OLD_VERSION" docs/ README.md *.md
  ```

  Update each hit to the new version. Don't enumerate doc paths in this playbook — the grep is authoritative and won't go stale.

## 3. Update conformance fixtures

```
go test -race ./internal/server -run Conformance
```

Fixtures at `internal/server/testdata/conformance/*.request.jsonl` and `*.response.jsonl` capture the wire protocol byte-for-byte. When the spec bump changes observable output:

- **Renamed methods** — add new fixture pair; decide whether the old name still returns `-32601` or is removed.
- **Changed response shapes** — update the `.response.jsonl` fixture.
- **New capability fields** — update the `initialize-handshake` fixtures.

Re-run the suite. It must pass before moving on.

## 4. Decide the version bump per VERSIONING.md

Consult [VERSIONING.md](../VERSIONING.md). Rules for an MCP-spec bump:

- **Purely additive** spec changes (new optional fields, new methods we don't yet implement) — **MINOR** (`v1.0.0` → `v1.1.0`)
- **Breaking** spec changes we propagate (renamed methods, removed fields, new MUST-reject errors, new mandatory capability) — **MAJOR** (`v1.0.0` → `v2.0.0`)
- Supporting the old spec version in parallel is itself additive — **MINOR** — and shows up in capability negotiation.

Record the upgrade under a dedicated "Spec upgrade" section in `CHANGELOG.md` naming both the old and new MCP versions.

## 5. Tag, release, verify

- `git tag v<version>` and push.
- goreleaser runs via `.github/workflows/release.yml` — cosign-signed binaries, SBOMs, SLSA L3 provenance (see README "Verify a release").
- After the release publishes, run the new binary against a real MCP client (Claude Desktop, VS Code extension) and confirm `initialize` returns the new `MCPVersion` and that at least one tool call round-trips.

## Backward-compatibility posture

The `initialize` handler currently pins `MCPVersion` to a single value. If a future upgrade needs a transition window, `initialize` can be extended to accept a list of supported spec versions and respond with the highest mutually-supported one. That extension is additive and falls under MINOR per VERSIONING.md.

The default posture is: one spec version at a time. Clients pinned to the old spec version upgrade when the server upgrades. For a template-first repo with a small active consumer set, this minimizes maintained surface until concrete demand justifies the parallel-version complexity.
