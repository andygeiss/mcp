package server

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/tools"
)

// capabilityGuidance is the Error.Data for unsupported capability namespaces.
type capabilityGuidance struct {
	Hint                  string   `json:"hint"`
	SupportedCapabilities []string `json:"supportedCapabilities"`
}

// toolsListResult is the result of tools/list.
type toolsListResult struct {
	NextCursor string       `json:"nextCursor,omitempty"`
	Tools      []tools.Tool `json:"tools"`
}

// handleMethod dispatches ready-state methods. rpc.* reservation is enforced
// earlier in dispatch() so it applies in any state.
func (s *Server) handleMethod(ctx context.Context, msg protocol.Request) protocol.Response {
	switch {
	case strings.HasPrefix(msg.Method, protocol.NamespaceCompletion):
		return s.handleUnsupportedCapability(msg)
	case strings.HasPrefix(msg.Method, protocol.NamespaceElicitation):
		return s.handleUnsupportedCapability(msg)
	case msg.Method == protocol.MethodLoggingSetLevel:
		return s.handleLoggingSetLevel(msg)
	case strings.HasPrefix(msg.Method, protocol.NamespacePrompts):
		if s.prompts == nil {
			return s.handleUnsupportedCapability(msg)
		}
		return s.handlePromptsMethod(ctx, msg)
	case strings.HasPrefix(msg.Method, protocol.NamespaceResources):
		if s.resources == nil {
			return s.handleUnsupportedCapability(msg)
		}
		return s.handleResourcesMethod(ctx, msg)
	case msg.Method == protocol.MethodToolsCall:
		// tools/call in ready state is intercepted by Run for async dispatch.
		// If we reach here, something unexpected happened.
		return s.errorResponse(msg.ID, protocol.ErrInternalError("unexpected tools/call in handleMethod"))
	case msg.Method == protocol.MethodToolsList:
		return s.handleToolsList(msg)
	default:
		return s.errorResponse(msg.ID, protocol.ErrMethodNotFound("unknown method: "+msg.Method))
	}
}

// handleUnsupportedCapability returns a -32601 response with guidance data
// for methods in unsupported MCP namespaces.
func (s *Server) handleUnsupportedCapability(msg protocol.Request) protocol.Response {
	supported := s.supportedCapabilities()
	data, _ := json.Marshal(&capabilityGuidance{
		Hint:                  "this capability is not enabled; supported: " + strings.Join(supported, ", "),
		SupportedCapabilities: supported,
	})
	ce := protocol.ErrMethodNotFound("method not found: " + msg.Method)
	ce.Data = data
	return s.errorResponse(msg.ID, ce)
}

// supportedCapabilities returns the list of capability names that this server
// advertises, derived from which registries are configured.
func (s *Server) supportedCapabilities() []string {
	var caps []string
	if s.prompts != nil {
		caps = append(caps, "prompts")
	}
	if s.resources != nil {
		caps = append(caps, "resources")
	}
	if s.registry != nil {
		caps = append(caps, "tools")
	}
	return caps
}

// handleToolsList returns all registered tools.
func (s *Server) handleToolsList(msg protocol.Request) protocol.Response {
	result := toolsListResult{Tools: s.registry.Tools()}
	resp, err := protocol.NewResultResponse(msg.ID, result)
	if err != nil {
		s.logger.Error("marshal_tools_list", "error", err)
		return s.errorResponse(msg.ID, protocol.ErrInternalError("failed to marshal tools list"))
	}
	return resp
}
