# API Contracts

**Project:** mcp
**Generated:** 2026-04-05

## Transport

- **Protocol:** JSON-RPC 2.0 over stdin/stdout
- **Framing:** Newline-delimited JSON objects (one JSON object per line)
- **MCP Version:** `2025-06-18`

## Methods

### `initialize` (Request)

Initiates the MCP handshake. Must be the first request. Transitions server from **uninitialized** to **initializing**.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "capabilities": {}
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "capabilities": {
      "tools": {}
    },
    "protocolVersion": "2025-06-18",
    "serverInfo": {
      "name": "mcp",
      "version": "<version>"
    }
  }
}
```

**Error conditions:**
- Already initialized: `-32600` ("already initialized")
- Sent during initializing state: `-32600` ("already initialized")

### `notifications/initialized` (Notification)

Client confirms initialization is complete. Transitions server from **initializing** to **ready**. No response is sent (notifications never receive responses).

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "notifications/initialized"
}
```

### `ping` (Request)

Health check. Works in any server state.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "ping",
  "params": {}
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {}
}
```

### `tools/list` (Request)

Returns all registered tools with their input schemas. Tools are always returned in alphabetical order.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/list",
  "params": {}
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "tools": [
      {
        "name": "search",
        "description": "Searches files for a pattern",
        "inputSchema": {
          "type": "object",
          "properties": {
            "caseSensitive": { "type": "boolean", "description": "Whether the search is case-sensitive" },
            "extensions": { "type": "array", "items": { "type": "string" }, "description": "File extensions to include (e.g. .go, .md)" },
            "maxResults": { "type": "integer", "description": "Maximum number of results to return" },
            "path": { "type": "string", "description": "Root directory to search in" },
            "pattern": { "type": "string", "description": "The search pattern (regex supported)" }
          },
          "required": ["path", "pattern"]
        }
      }
    ]
  }
}
```

### `tools/call` (Request)

Invokes a registered tool by name with the given arguments.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "search",
    "arguments": {
      "pattern": "func main",
      "path": ".",
      "extensions": [".go"],
      "maxResults": 10
    }
  }
}
```

**Success Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "cmd/mcp/main.go:16: func main() {"
      }
    ]
  }
}
```

**Tool-level Error Response** (invalid arguments, tool logic errors):
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "invalid pattern: error parsing regexp: ..."
      }
    ],
    "isError": true
  }
}
```

**Protocol-level Error Response** (unknown tool):
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "error": {
    "code": -32602,
    "message": "unknown tool: nonexistent (available: search)"
  }
}
```

**Timeout Error Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "error": {
    "code": -32603,
    "message": "tool \"search\" execution timed out",
    "data": {
      "toolName": "search",
      "elapsedMs": 30001,
      "timeoutMs": 30000
    }
  }
}
```

**Panic Error Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "error": {
    "code": -32603,
    "message": "internal error: tool \"search\" panicked",
    "data": {
      "toolName": "search",
      "panicValue": "<panic message>"
    }
  }
}
```

## Registered Tools

### `search`

Searches files under a root directory for lines matching a pattern (regex supported).

**Input Schema:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `path` | string | Yes | Root directory to search in |
| `pattern` | string | Yes | Search pattern (regex supported) |
| `caseSensitive` | boolean | No | Whether the search is case-sensitive (default: false) |
| `extensions` | string[] | No | File extensions to include (e.g., `.go`, `.md`) |
| `maxResults` | integer | No | Maximum number of results (default: 100) |

**Security constraints:**
- Path must be within the working directory (no escape via symlinks)
- Path traversal (`..`) rejected
- Null bytes in path or pattern rejected
- Input length limit: 4096 characters
- Binary files skipped (null byte detection in first 512 bytes)
- Files over 1MB skipped
- Maximum directory depth: 20 levels
- Symlinks opened with `O_NOFOLLOW` on Unix

**Output format:** One match per line: `<relative-path>:<line-number>: <line-content>`

## Error Codes Reference

| Code | Name | Description |
|---|---|---|
| `-32700` | Parse Error | Malformed JSON, batch arrays, message exceeds 4MB |
| `-32600` | Invalid Request | Wrong JSON-RPC version, empty method, non-object params, server not initialized, already initialized |
| `-32601` | Method Not Found | Unknown method, `rpc.*` reserved methods, unsupported capabilities |
| `-32602` | Invalid Params | Wrong types, missing fields, unknown/empty tool name |
| `-32603` | Internal Error | Handler panics, timeouts, context cancellation, marshal failures |

## Unsupported Capability Response

Methods under `prompts/` and `resources/` namespaces return `-32601` with guidance data:

```json
{
  "error": {
    "code": -32601,
    "message": "method not found: resources/list",
    "data": {
      "hint": "this server supports tools only; use tools/list and tools/call",
      "supportedCapabilities": ["tools"]
    }
  }
}
```

## Protocol Behaviors

| Behavior | Implementation |
|---|---|
| Missing `params` | Normalized to `{}` |
| `null` params | Normalized to `{}` |
| Missing `arguments` in tools/call | Normalized to `{}` |
| `null` arguments in tools/call | Normalized to `{}` |
| Notification (no `id`) | Never responded to |
| Unknown notification | Silently ignored |
| String `id` | Preserved and echoed exactly |
| Number `id` | Preserved and echoed exactly |
| `null` `id` | Preserved and echoed exactly |
| Request validation failure on notification | Silently dropped (no response) |
