// Package inspect renders the registered tools/resources/prompts of an MCP
// server as a single deterministic JSON document. The Inspect function is the
// shared primitive used by the `mcp --inspect-only` flag (FR7), `mcp doctor`
// (FR10), and `make catalog` (FR11), so the inspection mode never diverges
// from what the running server would advertise on the wire.
package inspect

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/andygeiss/mcp/internal/prompts"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/resources"
	"github.com/andygeiss/mcp/internal/tools"
)

// Output is the top-level shape emitted by Inspect. The shape is committed as
// a Clause-pinned wire contract so downstream consumers (mcp doctor, make
// catalog, tooling integrations) can rely on the field set and ordering.
type Output struct {
	Server            ServerInfo                   `json:"server"`
	ProtocolVersion   string                       `json:"protocolVersion"`
	Tools             []tools.Tool                 `json:"tools"`
	Resources         []resources.Resource         `json:"resources"`
	ResourceTemplates []resources.ResourceTemplate `json:"resourceTemplates"`
	Prompts           []prompts.Prompt             `json:"prompts"`
}

// ServerInfo names the binary producing the inspection output. Mirrors the
// `serverInfo` block of the JSON-RPC `initialize` response so consumers see
// the same identity in both surfaces.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Inspect writes a deterministic JSON inspection document for the supplied
// registries to w. Empty registries are emitted as empty arrays (never
// omitted) so consumers can rely on the field set being stable across servers
// with different capability profiles.
//
// Determinism is enforced defensively: each list is sorted by its identity
// key (tool name, resource URI, prompt name) before encoding so the output
// is byte-identical across runs regardless of registration order.
//
// Nil registries are treated as empty — callers can pass nil for registries
// that are not wired (e.g., a server with only tools passes nil for resources
// and prompts and still gets a well-formed inspection document).
func Inspect(serverName, serverVersion string, t *tools.Registry, r *resources.Registry, p *prompts.Registry, w io.Writer) error {
	out := Output{
		Server:            ServerInfo{Name: serverName, Version: serverVersion},
		ProtocolVersion:   protocol.MCPVersion,
		Tools:             collectTools(t),
		Resources:         collectResources(r),
		ResourceTemplates: collectTemplates(r),
		Prompts:           collectPrompts(p),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode inspect output: %w", err)
	}
	return nil
}

func collectTools(r *tools.Registry) []tools.Tool {
	if r == nil {
		return []tools.Tool{}
	}
	out := append([]tools.Tool{}, r.Tools()...)
	slices.SortFunc(out, func(a, b tools.Tool) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func collectResources(r *resources.Registry) []resources.Resource {
	if r == nil {
		return []resources.Resource{}
	}
	out := append([]resources.Resource{}, r.Resources()...)
	slices.SortFunc(out, func(a, b resources.Resource) int { return strings.Compare(a.URI, b.URI) })
	return out
}

func collectTemplates(r *resources.Registry) []resources.ResourceTemplate {
	if r == nil {
		return []resources.ResourceTemplate{}
	}
	out := append([]resources.ResourceTemplate{}, r.Templates()...)
	slices.SortFunc(out, func(a, b resources.ResourceTemplate) int { return strings.Compare(a.URITemplate, b.URITemplate) })
	return out
}

func collectPrompts(r *prompts.Registry) []prompts.Prompt {
	if r == nil {
		return []prompts.Prompt{}
	}
	out := append([]prompts.Prompt{}, r.Prompts()...)
	slices.SortFunc(out, func(a, b prompts.Prompt) int { return strings.Compare(a.Name, b.Name) })
	return out
}
