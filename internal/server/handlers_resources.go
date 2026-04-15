package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/resources"
)

// resourcesListResult is the result of resources/list. Templates are returned
// by the distinct resources/templates/list method per MCP 2025-11-25.
type resourcesListResult struct {
	NextCursor string               `json:"nextCursor,omitempty"`
	Resources  []resources.Resource `json:"resources"`
}

// resourcesTemplatesListResult is the result of resources/templates/list.
type resourcesTemplatesListResult struct {
	NextCursor        string                       `json:"nextCursor,omitempty"`
	ResourceTemplates []resources.ResourceTemplate `json:"resourceTemplates"`
}

// resourcesReadParams is the expected structure of resources/read params.
type resourcesReadParams struct {
	URI string `json:"uri"`
}

// resourcesReadResult is the result of resources/read.
type resourcesReadResult struct {
	Contents []resources.ContentBlock `json:"contents"`
}

// handleResourcesMethod dispatches resources/ methods.
func (s *Server) handleResourcesMethod(ctx context.Context, msg protocol.Request) protocol.Response {
	switch msg.Method {
	case protocol.MethodResourcesList:
		return s.handleResourcesList(msg)
	case protocol.MethodResourcesRead:
		return s.handleResourcesRead(ctx, msg)
	case protocol.MethodResourcesTemplatesList:
		return s.handleResourcesTemplatesList(msg)
	default:
		return s.errorResponse(msg.ID, protocol.ErrMethodNotFound("unknown method: "+msg.Method))
	}
}

// handleResourcesList returns all registered static resources.
func (s *Server) handleResourcesList(msg protocol.Request) protocol.Response {
	result := resourcesListResult{Resources: s.resources.Resources()}
	resp, err := protocol.NewResultResponse(msg.ID, result)
	if err != nil {
		s.logger.Error("marshal_resources_list", "error", err)
		return s.errorResponse(msg.ID, protocol.ErrInternalError("failed to marshal resources list"))
	}
	return resp
}

// handleResourcesTemplatesList returns all registered resource templates.
func (s *Server) handleResourcesTemplatesList(msg protocol.Request) protocol.Response {
	result := resourcesTemplatesListResult{ResourceTemplates: s.resources.Templates()}
	resp, err := protocol.NewResultResponse(msg.ID, result)
	if err != nil {
		s.logger.Error("marshal_resources_templates_list", "error", err)
		return s.errorResponse(msg.ID, protocol.ErrInternalError("failed to marshal resource templates list"))
	}
	return resp
}

// handleResourcesRead reads a specific resource by URI.
func (s *Server) handleResourcesRead(ctx context.Context, msg protocol.Request) protocol.Response {
	var params resourcesReadParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.errorResponse(msg.ID, protocol.ErrInvalidParams(fmt.Sprintf("invalid resources/read params: %v", err)))
	}
	if params.URI == "" {
		return s.errorResponse(msg.ID, protocol.ErrInvalidParams("uri is required"))
	}

	ctx, cancel := context.WithTimeout(ctx, s.handlerTimeout)
	defer cancel()

	var result resources.Result
	var err error

	if res, ok := s.resources.Lookup(params.URI); ok {
		result, err = res.Handler(ctx, params.URI)
	} else if tmpl, ok := s.resources.LookupTemplate(params.URI); ok {
		result, err = tmpl.Handler(ctx, params.URI)
	} else {
		return s.errorResponse(msg.ID, protocol.ErrResourceNotFound("resource not found: "+params.URI))
	}
	if err != nil {
		return s.errorResponse(msg.ID, err)
	}

	resp, rErr := protocol.NewResultResponse(msg.ID, resourcesReadResult{Contents: result.Contents})
	if rErr != nil {
		s.logger.Error("marshal_resource_read", "error", rErr)
		return s.errorResponse(msg.ID, protocol.ErrInternalError("failed to marshal resource read result"))
	}
	return resp
}
