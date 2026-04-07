package tools

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

const maxSchemaDepth = 10

// deriveSchema reflects over struct T to build an InputSchema.
func deriveSchema[T any]() InputSchema {
	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	schema := InputSchema{
		Properties: make(map[string]Property),
		Type:       "object",
	}

	collectFields(t, schema.Properties, &schema.Required, 0)

	slices.Sort(schema.Required)
	return schema
}

// collectFields iterates struct fields, promoting anonymous embedded structs
// and collecting named fields into the given property map and required slice.
func collectFields(t reflect.Type, props map[string]Property, required *[]string, depth int) {
	for field := range t.Fields() {
		if field.Anonymous && shouldPromote(field) {
			ft := field.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			collectFields(ft, props, required, depth)
			continue
		}

		collectField(field, props, required, depth)
	}
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
func collectField(field reflect.StructField, props map[string]Property, required *[]string, depth int) {
	jsonTag := field.Tag.Get("json")
	if jsonTag == "" || jsonTag == "-" {
		return
	}

	name, opts := parseJSONTag(jsonTag)
	if name == "" {
		return
	}

	prop := deriveProperty(field.Name, field.Type, depth)

	if desc := field.Tag.Get("description"); desc != "" {
		prop.Description = desc
	}

	props[name] = prop

	if !strings.Contains(opts, "omitempty") {
		*required = append(*required, name)
	}
}

// deriveProperty builds a Property for the given Go type. It handles
// primitives, slices, maps with string keys, and nested structs. Unsupported
// types (channels, funcs, etc.) cause a panic naming the field and type.
func deriveProperty(fieldName string, t reflect.Type, depth int) Property {
	if depth > maxSchemaDepth {
		panic(fmt.Sprintf("exceeded max depth for type %s", t))
	}
	switch t.Kind() {
	case reflect.Bool:
		return Property{Type: "boolean"}
	case reflect.Float32, reflect.Float64:
		return Property{Type: "number"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Property{Type: "integer"}
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			panic(fmt.Sprintf("unsupported map key type for field %q: %s (must be string)", fieldName, t.Key()))
		}
		valProp := deriveProperty(fieldName, t.Elem(), depth+1)
		return Property{Type: "object", AdditionalProperties: &valProp}
	case reflect.Pointer:
		return deriveProperty(fieldName, t.Elem(), depth+1)
	case reflect.Slice:
		elemProp := deriveProperty(fieldName, t.Elem(), depth+1)
		return Property{Type: "array", Items: &elemProp}
	case reflect.String:
		return Property{Type: "string"}
	case reflect.Struct:
		return deriveStructProperty(t, depth+1)
	default:
		panic(fmt.Sprintf("unsupported type for field %q: %s", fieldName, t))
	}
}

// deriveStructProperty builds a Property with nested properties for a struct type.
func deriveStructProperty(t reflect.Type, depth int) Property {
	props := make(map[string]Property)
	var required []string

	collectFields(t, props, &required, depth)

	slices.Sort(required)
	p := Property{Type: "object", Properties: props}
	if len(required) > 0 {
		p.Required = required
	}
	return p
}

// parseJSONTag splits a json tag into its name and remaining options.
func parseJSONTag(tag string) (string, string) {
	name, opts, _ := strings.Cut(tag, ",")
	return name, opts
}
