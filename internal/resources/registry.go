// Package resources provides the resource registry for the MCP server.
package resources

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// ContentBlock represents a single content item in a resource read result.
type ContentBlock struct {
	Blob     string `json:"blob,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	URI      string `json:"uri"`
}

// Option configures a Resource during registration.
type Option func(*Resource)

// Registry holds registered resources sorted alphabetically by URI.
// Not safe for concurrent use — register all resources before starting the server.
type Registry struct {
	index     map[string]int // URI → position in resources slice
	resources []Resource
	templates []ResourceTemplate
}

// Resource represents a registered MCP resource.
type Resource struct {
	Description string          `json:"description,omitempty"`
	Handler     resourceHandler `json:"-"`
	MimeType    string          `json:"mimeType,omitempty"`
	Name        string          `json:"name"`
	URI         string          `json:"uri"`
}

// ResourceTemplate represents a URI template for dynamic resources.
type ResourceTemplate struct {
	Description string          `json:"description,omitempty"`
	Handler     templateHandler `json:"-"`
	MimeType    string          `json:"mimeType,omitempty"`
	Name        string          `json:"name"`
	URITemplate string          `json:"uriTemplate"`
}

// Result is the outcome of reading a resource.
type Result struct {
	Contents []ContentBlock `json:"contents"`
}

// resourceHandler is the function signature for static resource handlers.
type resourceHandler func(ctx context.Context, uri string) (Result, error)

// templateHandler is the function signature for template resource handlers.
type templateHandler func(ctx context.Context, uri string) (Result, error)

// NewRegistry creates an empty resource registry.
func NewRegistry() *Registry {
	return &Registry{
		index:     make(map[string]int),
		resources: []Resource{},
		templates: []ResourceTemplate{},
	}
}

// Lookup finds a resource by URI in O(1) via the index map.
func (r *Registry) Lookup(uri string) (Resource, bool) {
	i, ok := r.index[uri]
	if !ok {
		return Resource{}, false
	}
	return r.resources[i], true
}

// Register adds a static resource to the registry.
func Register(r *Registry, uri, name, description string, handler func(ctx context.Context, uri string) (Result, error), opts ...Option) error {
	if _, exists := r.index[uri]; exists {
		return fmt.Errorf("duplicate resource URI: %s", uri)
	}

	res := Resource{
		Description: description,
		Handler:     handler,
		Name:        name,
		URI:         uri,
	}
	for _, opt := range opts {
		opt(&res)
	}

	r.resources = append(r.resources, res)
	slices.SortFunc(r.resources, func(a, b Resource) int {
		return strings.Compare(a.URI, b.URI)
	})
	for i, res := range r.resources {
		r.index[res.URI] = i
	}
	return nil
}

// RegisterTemplate adds a URI template resource to the registry.
func RegisterTemplate(r *Registry, uriTemplate, name, description string, handler func(ctx context.Context, uri string) (Result, error), opts ...Option) error {
	tmpl := ResourceTemplate{
		Description: description,
		Handler:     handler,
		Name:        name,
		URITemplate: uriTemplate,
	}
	// Apply options via a temporary Resource for MimeType extraction.
	temp := Resource{}
	for _, opt := range opts {
		opt(&temp)
	}
	tmpl.MimeType = temp.MimeType

	r.templates = append(r.templates, tmpl)
	slices.SortFunc(r.templates, func(a, b ResourceTemplate) int {
		return strings.Compare(a.URITemplate, b.URITemplate)
	})
	return nil
}

// Resources returns a copy of all registered static resources in alphabetical order.
func (r *Registry) Resources() []Resource {
	return slices.Clone(r.resources)
}

// Templates returns a copy of all registered resource templates in alphabetical order.
func (r *Registry) Templates() []ResourceTemplate {
	return slices.Clone(r.templates)
}

// TextResult creates a Result with a single text content block.
func TextResult(uri, text string) Result {
	return Result{
		Contents: []ContentBlock{{Text: text, URI: uri}},
	}
}

// BlobResult creates a Result with a single base64-encoded content block.
func BlobResult(uri, blob, mimeType string) Result {
	return Result{
		Contents: []ContentBlock{{Blob: blob, MimeType: mimeType, URI: uri}},
	}
}

// WithMimeType returns an Option that sets the resource's MIME type.
func WithMimeType(mimeType string) Option {
	return func(r *Resource) {
		r.MimeType = mimeType
	}
}
