package server

import (
	"context"
	"encoding/json"

	"github.com/andygeiss/mcp/internal/protocol"
)

// initializeResult is the result of the initialize method.
type initializeResult struct {
	Capabilities    initializeCapabilities `json:"capabilities"`
	Instructions    string                 `json:"instructions,omitempty"`
	ProtocolVersion string                 `json:"protocolVersion"`
	ServerInfo      serverInfo             `json:"serverInfo"`
}

type initializeCapabilities struct {
	Experimental map[string]any       `json:"experimental,omitempty"`
	Logging      *loggingCapability   `json:"logging,omitempty"`
	Prompts      *promptsCapability   `json:"prompts,omitempty"`
	Resources    *resourcesCapability `json:"resources,omitempty"`
	Tools        *toolsCapability     `json:"tools,omitempty"`
}

type loggingCapability struct{}

type promptsCapability struct{}

type resourcesCapability struct{}

type toolsCapability struct{}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// initializeParams is the subset of the initialize request we inspect for
// version negotiation and clientInfo logging. Unknown fields are ignored.
// The struct is unmarshal-only, so json tags need no `omitempty`.
type initializeParams struct {
	Capabilities    protocol.ClientCapabilities `json:"capabilities"`
	ClientInfo      clientInfo                  `json:"clientInfo"`
	ProtocolVersion string                      `json:"protocolVersion"`
}

// clientInfo mirrors the client-advertised identification block.
type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// handleInitialize processes the initialize request. Per MCP 2025-11-25, if the
// server supports the client's protocolVersion it echoes that version back;
// otherwise it responds with its own supported version and the client decides
// whether to proceed.
func (s *Server) handleInitialize(ctx context.Context, msg protocol.Request) protocol.Response {
	var params initializeParams
	_ = json.Unmarshal(msg.Params, &params)

	loggerFromContext(ctx, s.logger).Info("server_initializing",
		"client_name", params.ClientInfo.Name,
		"client_version", params.ClientInfo.Version,
		"client_protocol_version", params.ProtocolVersion,
	)
	// Snapshot client capabilities for AI9 capability-gate enforcement during
	// outbound (sampling/elicitation/roots). atomic.Pointer keeps reads
	// lock-free from handler goroutines.
	caps := params.Capabilities
	s.clientCaps.Store(&caps)
	s.state = stateInitializing

	srvCaps := initializeCapabilities{
		Experimental: map[string]any{
			"concurrency": map[string]any{
				"maxInFlight": protocol.MaxConcurrentRequests,
			},
		},
	}
	srvCaps.Logging = &loggingCapability{}
	if s.prompts != nil {
		srvCaps.Prompts = &promptsCapability{}
	}
	if s.resources != nil {
		srvCaps.Resources = &resourcesCapability{}
	}
	if s.registry != nil {
		srvCaps.Tools = &toolsCapability{}
	}

	negotiated := protocol.NegotiateVersion(params.ProtocolVersion)

	result := initializeResult{
		Capabilities:    srvCaps,
		Instructions:    s.instructions,
		ProtocolVersion: negotiated,
		ServerInfo:      serverInfo{Name: s.name, Version: s.version},
	}

	resp, err := protocol.NewResultResponse(msg.ID, result)
	if err != nil {
		loggerFromContext(ctx, s.logger).Error("marshal_initialize", "error", err)
		return s.errorResponse(ctx, msg.ID, protocol.ErrInternalError("failed to marshal initialize result"))
	}
	return resp
}
