# API Contracts

**Project:** github.com/andygeiss/mcp
**Generated:** 2026-04-05

## Overview

The MCP server exposes a JSON-RPC 2.0 interface over stdin/stdout. All communication is newline-delimited JSON objects. No HTTP, no WebSocket, no LSP framing.

**MCP Protocol Version:** `2024-11-05`
**JSON-RPC Version:** `2.0`

## Protocol Methods

### initialize

Initiates the MCP handshake. Must be the first request (after optional `ping`).

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
    "protocolVersion": "2024-11-05",
    "serverInfo": {
      "name": "mcp",
      "version": "dev"
    }
  }
}
```

**State transition:** uninitialized -> initializing

**Error conditions:**
- Duplicate `initialize` -> `-32600` ("already initialized")
- After `notifications/initialized` -> `-32600` ("already initialized")

---

### notifications/initialized

Client notification confirming initialization is complete. No response sent (notifications never receive responses).

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "notifications/initialized"
}
```

**State transition:** initializing -> ready

**Behavior in other states:** Silently ignored.

---

### ping

Health check. Works in any state (uninitialized, initializing, ready).

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "ping"
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

---

### tools/list

Returns all registered tools in alphabetical order.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/list"
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
        "description": "Search files for a pattern",
        "inputSchema": {
          "type": "object",
          "properties": {
            "caseSensitive": {
              "type": "boolean",
              "description": "Case-sensitive search (default: false)"
            },
            "extensions": {
              "type": "array",
              "items": { "type": "string" },
              "description": "File extensions to include (e.g. [\".go\", \".md\"])"
            },
            "maxResults": {
              "type": "integer",
              "description": "Maximum results to return (default: 100)"
            },
            "path": {
              "type": "string",
              "description": "Directory to search"
            },
            "pattern": {
              "type": "string",
              "description": "Regex pattern to search for"
            }
          },
          "required": ["path", "pattern"]
        }
      }
    ]
  }
}
```

---

### tools/call

Invokes a registered tool by name.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "search",
    "arguments": {
      "path": "/path/to/project",
      "pattern": "func main"
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
        "text": "cmd/mcp/main.go:10: func main() {"
      }
    ]
  }
}
```

**Error Response (unknown tool):**
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "error": {
    "code": -32602,
    "message": "unknown tool: nonexistent"
  }
}
```

**Handler timeout:**
- Default: 30 seconds
- Returns `-32603` with `timingDiag` data including elapsed time and configured timeout

**Handler panic:**
- Returns `-32603` with `panicDiag` data including panic value, tool name, and stack trace logged to stderr

---

## Error Responses

### Parse Error (-32700)

Malformed JSON, oversized message (>4 MB), or batch array.

```json
{
  "jsonrpc": "2.0",
  "id": null,
  "error": {
    "code": -32700,
    "message": "parse error: invalid character ..."
  }
}
```

### Invalid Request (-32600)

Bad structure, wrong JSON-RPC version, non-object params, or state violation.

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32600,
    "message": "server not initialized"
  }
}
```

### Method Not Found (-32601)

Unknown method, `rpc.*` reserved namespace, or unsupported capability namespace.

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32601,
    "message": "unknown method: foo/bar"
  }
}
```

For `prompts/*` and `resources/*` namespaces, the error includes guidance data:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32601,
    "message": "unsupported capability namespace: prompts",
    "data": {
      "namespace": "prompts",
      "hint": "This server does not support the prompts capability"
    }
  }
}
```

### Invalid Params (-32602)

Wrong parameter types, missing required fields, or unknown tool name.

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32602,
    "message": "unknown tool: nonexistent"
  }
}
```

### Internal Error (-32603)

Handler panic or unexpected internal failure. Includes diagnostic data.

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32603,
    "message": "tool panic: search: runtime error: index out of range",
    "data": {
      "tool": "search",
      "panic": "runtime error: index out of range"
    }
  }
}
```

## Protocol Behaviors

| Behavior | Implementation |
|----------|----------------|
| Framing | Newline-delimited JSON objects |
| Batch requests | Rejected with `-32700` |
| Missing `params` | Normalized to `{}` |
| Null `params` | Normalized to `{}` |
| Request `id` | Preserved as `json.RawMessage`, echoed exactly |
| String `id` | Preserved with quotes |
| Number `id` | Preserved as raw JSON number |
| Null `id` | Preserved as literal `null` |
| Notifications (no `id`) | Never responded to |
| Unknown notifications | Silently ignored -- no response, no log |
| EOF | Clean shutdown (exit 0) |
| Truncated JSON + EOF | Clean shutdown (exit 0) |
| Decode errors (non-EOF) | Fatal (exit 1) |
| Message size limit | 4 MB per message |

## Registered Tools

### search

Recursive file content search with regex pattern matching.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `path` | string | yes | -- | Directory to search |
| `pattern` | string | yes | -- | Regex pattern to search for |
| `caseSensitive` | boolean | no | false | Case-sensitive search |
| `extensions` | string[] | no | all | File extensions to include |
| `maxResults` | integer | no | 100 | Maximum results to return |

**Security features:**
- Path must be within current working directory
- Symlink resolution with validation (no directory traversal)
- `O_NOFOLLOW` flag on Unix to prevent symlink attacks
- Binary file detection (null byte check in first 512 bytes)
- 1 MB file size limit
- Context-aware cancellation
