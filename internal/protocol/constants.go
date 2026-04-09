package protocol

// JSON-RPC 2.0 error codes.
const (
	InternalError  = -32603
	InvalidParams  = -32602
	InvalidRequest = -32600
	MethodNotFound = -32601
	ParseError     = -32700
)

// Implementation-defined server error codes (JSON-RPC 2.0 reserves -32000 to -32099).
const (
	ServerError   = -32000 // server state prevents processing a structurally valid request
	ServerTimeout = -32001 // tool handler timed out or was cancelled
)

// MaxConcurrentRequests advertises the server's sequential dispatch limit.
const MaxConcurrentRequests = 1

// MCP method constants.
const (
	MCPVersion              = "2025-06-18"
	MethodInitialize        = "initialize"
	MethodPing              = "ping"
	MethodToolsCall         = "tools/call"
	MethodToolsList         = "tools/list"
	NotificationCancelled   = "notifications/cancelled"
	NotificationInitialized = "notifications/initialized"
)

// Namespace prefix constants for method dispatch.
const (
	NamespaceCompletion  = "completion/"
	NamespaceElicitation = "elicitation/"
	NamespacePrompts     = "prompts/"
	NamespaceResources   = "resources/"
	PrefixRPC            = "rpc."
)

// Version is the JSON-RPC version string.
const Version = "2.0"
