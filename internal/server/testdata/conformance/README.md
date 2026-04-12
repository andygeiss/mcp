# MCP Conformance Fixtures

Static JSONL fixtures for black-box conformance testing of any MCP 2025-11-25 server over stdin/stdout.

## Format

Each `.request.jsonl` file contains one JSON-RPC 2.0 object per line (compact, no trailing whitespace). Lines are fed to the server's stdin in order, followed by EOF. Notifications (messages without `id`) are included in the stream where the protocol requires them — the server must never respond to them.

Handshake convention: most fixtures that test ready-state behavior begin with the standard two-line handshake:

```
{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{},"clientInfo":{"name":"test"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
```

Fixtures that intentionally test pre-ready behavior omit part or all of the handshake.

## Naming Convention

```
<descriptive-name>.request.jsonl    # input stream
<descriptive-name>.response.jsonl   # golden output (byte-exact, optional)
```

Golden `.response.jsonl` files contain the expected server stdout, one JSON-RPC response object per line. Where a golden file is absent, the fixture is used for smoke-testing only (assert non-empty output, or assert the server exits cleanly).

## Usage

Pipe a fixture directly to any conforming MCP server binary:

```sh
< fixtures/ping.request.jsonl ./mcp 2>/dev/null
```

Compare against a golden file:

```sh
< fixtures/ping.request.jsonl ./mcp 2>/dev/null | diff - fixtures/ping.response.jsonl
```

Run all fixtures in a loop:

```sh
for req in fixtures/*.request.jsonl; do
  name="${req%.request.jsonl}"
  actual=$(< "$req" ./mcp 2>/dev/null)
  if [ -f "${name}.response.jsonl" ]; then
    diff <(echo "$actual") "${name}.response.jsonl" || echo "FAIL: $name"
  fi
done
```

The server under test must write only valid JSON-RPC objects to stdout. Any non-JSON byte on stdout is a protocol violation.

## Coverage Matrix

### Tier 1 — Error code completeness

| Fixture | Tests | Expected Outcome |
|---|---|---|
| `batch-array-rejection` | JSON array as top-level message | `-32700` parse error, `id: null` |
| `params-array-positional` | `params` is a JSON array instead of object | `-32600` invalid request |
| `jsonrpc-version-missing` | `jsonrpc` field absent | `-32600` invalid request |
| `jsonrpc-version-wrong` | `jsonrpc: "1.0"` | `-32600` invalid request |
| `method-empty` | `method: ""` | `-32601` method not found |
| `method-rpc-reserved` | `method: "rpc.discover"` (reserved namespace) | `-32601` method not found |
| `method-unknown` | `method: "foo/bar"` | `-32601` method not found |
| `tools-call-unknown-tool` | `tools/call` with non-existent tool name | `-32602` invalid params |
| `tools-call-empty-name` | `tools/call` with `name: ""` | `-32602` invalid params |

### Tier 2 — Id type edge cases

| Fixture | Tests | Expected Outcome |
|---|---|---|
| `id-string` | String id `"abc-123"` | Response echoes id as string |
| `id-zero` | Numeric id `0` | Response echoes id as `0` |
| `id-negative` | Numeric id `-1` | Response echoes id as `-1` |
| `id-large-number` | Numeric id beyond IEEE 754 safe integer | Response echoes id exactly as received |
| `id-empty-string` | String id `""` | Response echoes id as `""` |
| `id-null` | `id: null` (treated as notification per spec) | No response emitted |
| `id-boolean-invalid` | `id: true` (non-spec type) | `-32600` invalid request |
| `id-array-invalid` | `id: [1]` (non-spec type) | `-32600` invalid request |
| `id-object-invalid` | `id: {"a":1}` (non-spec type) | `-32600` invalid request |

### Tier 3 — State machine and capabilities

| Fixture | Tests | Expected Outcome |
|---|---|---|
| `initialize-handshake` | Full two-step handshake (`initialize` + `notifications/initialized`) | `initialize` response with capabilities; no response to notification |
| `state-uninitialized-method` | `tools/list` sent before `initialize` | `-32600` server not initialized |
| `state-ping-all-states` | `ping` in uninitialized, initializing, and ready states | Three successful `ping` responses |
| `state-duplicate-initialize` | Second `initialize` after handshake complete | `-32600` already initialized |
| `state-request-during-initializing` | `tools/list` sent after `initialize` but before `notifications/initialized` | `-32600` server not initialized |
| `notification-then-request` | Unknown notification followed by a valid `ping` | No response to notification; `ping` succeeds |
| `notification-invalid-silently-dropped` | Notification with array `params` | Silently ignored; no response emitted |
| `unsupported-capability-prompts` | `prompts/list` on a tools-only server | `-32601` method not found |
| `unsupported-capability-resources` | `resources/list` on a tools-only server | `-32601` method not found |

### Tier 4 — Protocol edge cases

| Fixture | Tests | Expected Outcome |
|---|---|---|
| `ping` | Minimal `ping` after handshake | Empty-result success response |
| `params-absent` | Request with no `params` field | Server defaults to `{}`; success response |
| `params-null` | `params: null` | Server defaults to `{}`; success response |
| `extra-fields` | Message with unknown top-level fields (`extra`, `result`) | Extra fields ignored; success response |
| `tools-list` | `tools/list` after handshake | Success response with tools array |
| `tools-call-success` | `tools/call` with valid tool name and arguments | Success response with tool result |
