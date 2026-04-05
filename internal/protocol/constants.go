package protocol

// JSON-RPC 2.0 error codes.
const (
	InternalError  = -32603
	InvalidParams  = -32602
	InvalidRequest = -32600
	MethodNotFound = -32601
	ParseError     = -32700
)

// MCP method constants.
const (
	MCPVersion              = "2024-11-05"
	MethodInitialize        = "initialize"
	MethodPing              = "ping"
	MethodToolsCall         = "tools/call"
	MethodToolsList         = "tools/list"
	NotificationInitialized = "notifications/initialized"
)

// Version is the JSON-RPC version string.
const Version = "2.0"
