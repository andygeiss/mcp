package tools

import "github.com/andygeiss/mcp/internal/schema"

// deriveSchema reflects over struct T to build an InputSchema.
func deriveSchema[T any]() (InputSchema, error) {
	s, err := schema.DeriveInputSchema[T]()
	if err != nil {
		return InputSchema{}, err
	}
	return InputSchema{
		Properties: convertProperties(s.Properties),
		Required:   s.Required,
		Type:       s.Type,
	}, nil
}

// convertProperties converts schema.Property map to tools.Property map.
func convertProperties(src map[string]schema.Property) map[string]Property {
	if src == nil {
		return nil
	}
	dst := make(map[string]Property, len(src))
	for k, v := range src {
		dst[k] = convertProperty(v)
	}
	return dst
}

// convertProperty converts a single schema.Property to tools.Property.
func convertProperty(src schema.Property) Property {
	p := Property{
		Description: src.Description,
		Properties:  convertProperties(src.Properties),
		Required:    src.Required,
		Type:        src.Type,
	}
	if src.AdditionalProperties != nil {
		cp := convertProperty(*src.AdditionalProperties)
		p.AdditionalProperties = &cp
	}
	if src.Items != nil {
		cp := convertProperty(*src.Items)
		p.Items = &cp
	}
	return p
}
