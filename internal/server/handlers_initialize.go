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
	Logging   *loggingCapability   `json:"logging,omitempty"`
	Prompts   *promptsCapability   `json:"prompts,omitempty"`
	Resources *resourcesCapability `json:"resources,omitempty"`
	Tools     *toolsCapability     `json:"tools,omitempty"`
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

	// Capability honesty (R2): advertise a registry-backed capability ONLY
	// when its registry has at least one entry. An empty registry means a
	// client `tools/list` would return `{tools: []}` — wasted round trip
	// since the client already learned the surface in initialize. Empty →
	// don't advertise. Logging is always advertised (logging/setLevel
	// works regardless of state).
	srvCaps := initializeCapabilities{
		Logging: &loggingCapability{},
	}
	srvCaps.Logging = &loggingCapability{}
	if s.prompts != nil && len(s.prompts.Prompts()) > 0 {
		srvCaps.Prompts = &promptsCapability{}
	}
	if s.resources != nil && (len(s.resources.Resources()) > 0 || len(s.resources.Templates()) > 0) {
		srvCaps.Resources = &resourcesCapability{}
	}
	if s.registry != nil && len(s.registry.Names()) > 0 {
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
