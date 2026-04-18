package protocol

// Capability is the named-type enum for client capabilities the server may
// require before initiating outbound requests (sampling, elicitation, roots).
type Capability string

// Well-known capability names per MCP spec 2025-11-25.
const (
	CapElicitation Capability = "elicitation"
	CapRoots       Capability = "roots"
	CapSampling    Capability = "sampling"
)

// ElicitationCapability mirrors the client's `elicitation` capability object.
// Empty today; reserved for future MCP-spec extensions.
type ElicitationCapability struct{}

// SamplingCapability mirrors the client's `sampling` capability object.
// Empty today; reserved for future MCP-spec extensions.
type SamplingCapability struct{}

// RootsCapability mirrors the client's `roots` capability object. The
// `listChanged` flag is part of the MCP spec even though v1.3.0 does not
// consume it; included for wire-format conformance.
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ClientCapabilities is the snapshot of capabilities the client advertised
// during `initialize`. Sub-capability fields use pointer-to-empty-struct so
// that absence (nil) is distinguishable from present-with-no-options
// (`&SamplingCapability{}`) on the JSON wire.
type ClientCapabilities struct {
	Elicitation  *ElicitationCapability `json:"elicitation,omitempty"`
	Experimental map[string]any         `json:"experimental,omitempty"`
	Roots        *RootsCapability       `json:"roots,omitempty"`
	Sampling     *SamplingCapability    `json:"sampling,omitempty"`
}

// Has reports whether the client advertised the given capability. Safe to call
// on a nil receiver — returns false. Unknown capabilities also return false.
func (c *ClientCapabilities) Has(name Capability) bool {
	if c == nil {
		return false
	}
	switch name {
	case CapElicitation:
		return c.Elicitation != nil
	case CapRoots:
		return c.Roots != nil
	case CapSampling:
		return c.Sampling != nil
	default:
		return false
	}
}
