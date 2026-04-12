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
	ResourceNotFound = -32002 // resources/read target URI does not match any registered resource or template
	ServerError      = -32000 // server state prevents processing a structurally valid request
	ServerTimeout    = -32001 // tool handler timed out or was cancelled
)

// MaxConcurrentRequests is a protocol-level constraint advertising sequential
// dispatch to clients. Not configurable — the server processes one request at a time.
const MaxConcurrentRequests = 1

// MaxJSONDepth caps nesting depth of any decoded JSON-RPC message. The stdlib
// json decoder has no native depth limit, so the codec scans the raw bytes for
// balanced `{` / `[` before handing them to Unmarshal. 64 comfortably covers
// legal MCP payloads while preventing stack-exhaustion attacks.
const MaxJSONDepth = 64

// MCPVersion is the MCP protocol version advertised during initialize.
const MCPVersion = "2025-11-25"

// MCP method constants.
const (
	MethodInitialize             = "initialize"
	MethodLoggingSetLevel        = "logging/setLevel"
	MethodPing                   = "ping"
	MethodPromptsGet             = "prompts/get"
	MethodPromptsList            = "prompts/list"
	MethodResourcesList          = "resources/list"
	MethodResourcesRead          = "resources/read"
	MethodResourcesTemplatesList = "resources/templates/list"
	MethodToolsCall              = "tools/call"
	MethodToolsList              = "tools/list"
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
