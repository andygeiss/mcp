package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/andygeiss/mcp/internal/prompts"
	"github.com/andygeiss/mcp/internal/protocol"
)

// promptsListResult is the result of prompts/list.
type promptsListResult struct {
	NextCursor string           `json:"nextCursor,omitempty"`
	Prompts    []prompts.Prompt `json:"prompts"`
}

// promptsGetParams is the expected structure of prompts/get params.
type promptsGetParams struct {
	Arguments map[string]string `json:"arguments,omitempty"`
	Name      string            `json:"name"`
}

// promptsGetResult is the result of prompts/get.
type promptsGetResult struct {
	Description string            `json:"description,omitempty"`
	Messages    []prompts.Message `json:"messages"`
}

// handlePromptsMethod dispatches prompts/ methods.
func (s *Server) handlePromptsMethod(ctx context.Context, msg protocol.Request) protocol.Response {
	switch msg.Method {
	case protocol.MethodPromptsList:
		return s.handlePromptsList(ctx, msg)
	case protocol.MethodPromptsGet:
		return s.handlePromptsGet(ctx, msg)
	default:
		return s.errorResponse(ctx, msg.ID, protocol.ErrMethodNotFound("unknown method: "+msg.Method))
	}
}

// handlePromptsList returns all registered prompts.
func (s *Server) handlePromptsList(ctx context.Context, msg protocol.Request) protocol.Response {
	result := promptsListResult{Prompts: s.prompts.Prompts()}
	resp, err := protocol.NewResultResponse(msg.ID, result)
	if err != nil {
		loggerFromContext(ctx, s.logger).Error("marshal_prompts_list", "error", err)
		return s.errorResponse(ctx, msg.ID, protocol.ErrInternalError("failed to marshal prompts list"))
	}
	return resp
}

// validatePromptArgs checks that required arguments are present and no
// unknown arguments were supplied. Returns a *protocol.CodeError on mismatch.
func validatePromptArgs(prompt prompts.Prompt, args map[string]string) error {
	known := make(map[string]bool, len(prompt.Arguments))
	for _, arg := range prompt.Arguments {
		known[arg.Name] = true
		if arg.Required {
			if _, ok := args[arg.Name]; !ok {
				return protocol.ErrInvalidParams("missing required argument: " + arg.Name)
			}
		}
	}
	for name := range args {
		if !known[name] {
			return protocol.ErrInvalidParams("unknown argument: " + name)
		}
	}
	return nil
}

// handlePromptsGet resolves a prompt by name with arguments.
func (s *Server) handlePromptsGet(ctx context.Context, msg protocol.Request) protocol.Response {
	var params promptsGetParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.errorResponse(ctx, msg.ID, protocol.ErrInvalidParams(fmt.Sprintf("invalid prompts/get params: %v", err)))
	}
	if params.Name == "" {
		return s.errorResponse(ctx, msg.ID, protocol.ErrInvalidParams("name is required"))
	}

	prompt, ok := s.prompts.Lookup(params.Name)
	if !ok {
		return s.errorResponse(ctx, msg.ID, protocol.ErrInvalidParams("unknown prompt: "+params.Name))
	}

	args := params.Arguments
	if args == nil {
		args = make(map[string]string)
	}

	if err := validatePromptArgs(prompt, args); err != nil {
		return s.errorResponse(ctx, msg.ID, err)
	}

	ctx, cancel := context.WithTimeout(ctx, s.handlerTimeout)
	defer cancel()

	result, err := prompt.Handler(ctx, args)
	// Timeout/cancellation maps to ServerTimeout regardless of the handler's
	// return value — consistent with tools/call and MCP §Error Codes.
	if ctx.Err() != nil {
		return s.errorResponse(ctx, msg.ID, &protocol.CodeError{
			Code:    protocol.ServerTimeout,
			Message: fmt.Sprintf("prompt %q execution timed out", params.Name),
		})
	}
	if err != nil {
		return s.errorResponse(ctx, msg.ID, err)
	}

	resp, rErr := protocol.NewResultResponse(msg.ID, promptsGetResult{
		Description: result.Description,
		Messages:    result.Messages,
	})
	if rErr != nil {
		loggerFromContext(ctx, s.logger).Error("marshal_prompt_get", "error", rErr)
		return s.errorResponse(ctx, msg.ID, protocol.ErrInternalError("failed to marshal prompt get result"))
	}
	return resp
}
