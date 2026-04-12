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

// MaxConcurrentRequests is a protocol-level constraint advertising sequential
// dispatch to clients. Not configurable — the server processes one request at a time.
const MaxConcurrentRequests = 1

// MCPVersion is the MCP protocol version advertised during initialize.
const MCPVersion = "2025-11-25"

// MCP method constants.
const (
	MethodInitialize      = "initialize"
	MethodLoggingSetLevel = "logging/setLevel"
	MethodPing            = "ping"
	MethodPromptsGet      = "prompts/get"
	MethodPromptsList     = "prompts/list"
	MethodResourcesList   = "resources/list"
	MethodResourcesRead   = "resources/read"
	MethodToolsCall       = "tools/call"
	MethodToolsList       = "tools/list"
)

// MCP notification constants.
const (
	NotificationCancelled   = "notifications/cancelled"
	NotificationInitialized = "notifications/initialized"
	NotificationMessage     = "notifications/message"
	NotificationProgress    = "notifications/progress"
)

// Namespace prefix constants for method dispatch.
const (
	NamespaceCompletion  = "completion/"
	NamespaceElicitation = "elicitation/"
	NamespaceLogging     = "logging/"
	NamespacePrompts     = "prompts/"
	NamespaceResources   = "resources/"
	PrefixRPC            = "rpc."
)

// Version is the JSON-RPC version string.
const Version = "2.0"
