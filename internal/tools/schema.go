package tools

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

const maxSchemaDepth = 10

// deriveSchema reflects over struct T to build an InputSchema.
func deriveSchema[T any]() (InputSchema, error) {
	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	schema := InputSchema{
		Properties: make(map[string]Property),
		Type:       SchemaTypeObject,
	}

	if err := collectFields(t, schema.Properties, &schema.Required, 0); err != nil {
		return InputSchema{}, err
	}

	slices.Sort(schema.Required)
	return schema, nil
}

// collectFields iterates struct fields, promoting anonymous embedded structs
// and collecting named fields into the given property map and required slice.
func collectFields(t reflect.Type, props map[string]Property, required *[]string, depth int) error {
	for field := range t.Fields() {
		if field.Anonymous && shouldPromote(field) {
			ft := field.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if err := collectFields(ft, props, required, depth); err != nil {
				return err
			}
			continue
		}

		if err := collectField(field, props, required, depth); err != nil {
			return err
		}
	}
	return nil
}

// shouldPromote reports whether an anonymous field should have its fields
// promoted into the parent schema (untagged, exported, struct type).
func shouldPromote(field reflect.StructField) bool {
	jsonTag := field.Tag.Get("json")
	if jsonTag != "" && jsonTag != "-" {
		return false
	}
	if !field.IsExported() {
		return false
	}
	ft := field.Type
	if ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}
	return ft.Kind() == reflect.Struct
}

// collectField processes a single struct field into the property map.
func collectField(field reflect.StructField, props map[string]Property, required *[]string, depth int) error {
	jsonTag := field.Tag.Get("json")
	if jsonTag == "" || jsonTag == "-" {
		return nil
	}

	name, opts := parseJSONTag(jsonTag)
	if name == "" {
		return nil
	}

	prop, err := deriveProperty(field.Name, field.Type, depth)
	if err != nil {
		return err
	}

	if desc := field.Tag.Get("description"); desc != "" {
		prop.Description = desc
	}

	props[name] = prop

	if !strings.Contains(opts, "omitempty") {
		*required = append(*required, name)
	}
	return nil
}

// deriveProperty builds a Property for the given Go type. It handles
// primitives, slices, maps with string keys, and nested structs. Unsupported
// types (channels, funcs, etc.) return an error naming the field and type.
func deriveProperty(fieldName string, t reflect.Type, depth int) (Property, error) {
	if depth > maxSchemaDepth {
		return Property{}, fmt.Errorf("exceeded max depth for type %s", t)
	}
	if p, ok := derivePrimitive(t); ok {
		return p, nil
	}
	return deriveComposite(fieldName, t, depth)
}

// derivePrimitive returns a Property for Go primitive types (bool, int, float, string).
func derivePrimitive(t reflect.Type) (Property, bool) {
	switch t.Kind() {
	case reflect.Bool:
		return Property{Type: SchemaTypeBoolean}, true
	case reflect.Float32, reflect.Float64:
		return Property{Type: SchemaTypeNumber}, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Property{Type: SchemaTypeInteger}, true
	case reflect.String:
		return Property{Type: SchemaTypeString}, true
	default:
		return Property{}, false
	}
}

// deriveComposite handles map, pointer, slice, and struct types.
func deriveComposite(fieldName string, t reflect.Type, depth int) (Property, error) {
	switch t.Kind() {
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return Property{}, fmt.Errorf("unsupported map key type for field %q: %s (must be string)", fieldName, t.Key())
		}
		valProp, err := deriveProperty(fieldName, t.Elem(), depth+1)
		if err != nil {
			return Property{}, err
		}
		return Property{Type: SchemaTypeObject, AdditionalProperties: &valProp}, nil
	case reflect.Pointer:
		return deriveProperty(fieldName, t.Elem(), depth+1)
	case reflect.Slice:
		elemProp, err := deriveProperty(fieldName, t.Elem(), depth+1)
		if err != nil {
			return Property{}, err
		}
		return Property{Type: SchemaTypeArray, Items: &elemProp}, nil
	case reflect.Struct:
		return deriveStructProperty(t, depth+1)
	default:
		return Property{}, fmt.Errorf("unsupported type for field %q: %s", fieldName, t)
	}
}

// deriveStructProperty builds a Property with nested properties for a struct type.
func deriveStructProperty(t reflect.Type, depth int) (Property, error) {
	props := make(map[string]Property)
	var required []string

	if err := collectFields(t, props, &required, depth); err != nil {
		return Property{}, err
	}

	slices.Sort(required)
	p := Property{Type: SchemaTypeObject, Properties: props}
	if len(required) > 0 {
		p.Required = required
	}
	return p, nil
}

// parseJSONTag splits a json tag into its name and remaining options.
func parseJSONTag(tag string) (string, string) {
	name, opts, _ := strings.Cut(tag, ",")
	return name, opts
}
