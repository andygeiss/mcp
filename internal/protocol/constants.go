package protocol

// ErrorCode is a strongly typed JSON-RPC 2.0 error code. Used in
// ErrClientRejected to carry the structured code returned by a client for a
// server-initiated request.
type ErrorCode int

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

// ErrorCode-typed mirrors of the well-known codes for use with ErrClientRejected
// and any future API that wants compile-time type safety on the code field.
const (
	ErrCodeInternalError    ErrorCode = -32603
	ErrCodeInvalidParams    ErrorCode = -32602
	ErrCodeInvalidRequest   ErrorCode = -32600
	ErrCodeMethodNotFound   ErrorCode = -32601
	ErrCodeParseError       ErrorCode = -32700
	ErrCodeResourceNotFound ErrorCode = -32002
	ErrCodeServerError      ErrorCode = -32000
	ErrCodeServerTimeout    ErrorCode = -32001
)

// MaxConcurrentRequests is a protocol-level constraint advertising sequential
// dispatch to clients. Not configurable — the server processes one request at a time.
const MaxConcurrentRequests = 1

// MaxJSONDepth caps nesting depth of any decoded JSON-RPC message. The stdlib
// json decoder has no native depth limit, so the codec scans the raw bytes for
// balanced `{` / `[` before handing them to Unmarshal. 64 comfortably covers
// legal MCP payloads while preventing stack-exhaustion attacks.
const MaxJSONDepth = 64

// MaxJSONKeysPerObject caps the number of keys allowed in any single JSON
// object within a decoded message. Counted per-object scope (each `{` resets
// the counter for that nesting level). Prevents amplification via legal-shape
// payloads with millions of keys.
//
// TODO(M1b): value tuning + ADR-004 will revisit this limit alongside the
// envelope cap raise.
const MaxJSONKeysPerObject = 10_000

// MaxJSONStringLen caps the raw byte length of any single JSON string literal
// (including object keys and string values). Counted in raw bytes between the
// surrounding quotes — a conservative over-estimate of decoded length, since
// `\uXXXX` is 6 raw bytes that decode to at most 4 UTF-8 bytes.
//
// TODO(M1b): 1 MiB is conservative; M1b raises to 4 MiB once ADR-004 documents
// the value-tuning rationale.
const MaxJSONStringLen = 1 << 20

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
