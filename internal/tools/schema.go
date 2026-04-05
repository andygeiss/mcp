package tools

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

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

	for field := range t.Fields() {
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		name, opts := parseJSONTag(jsonTag)
		if name == "" {
			continue
		}

		prop := deriveProperty(field.Name, field.Type)

		if desc := field.Tag.Get("description"); desc != "" {
			prop.Description = desc
		}

		schema.Properties[name] = prop

		if !strings.Contains(opts, "omitempty") {
			schema.Required = append(schema.Required, name)
		}
	}

	slices.Sort(schema.Required)
	return schema
}

// deriveProperty builds a Property for the given Go type. It handles
// primitives, slices, maps with string keys, and nested structs. Unsupported
// types (channels, funcs, etc.) cause a panic naming the field and type.
func deriveProperty(fieldName string, t reflect.Type) Property {
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
		valProp := deriveProperty(fieldName, t.Elem())
		return Property{Type: "object", AdditionalProperties: &valProp}
	case reflect.Pointer:
		return deriveProperty(fieldName, t.Elem())
	case reflect.Slice:
		elemProp := deriveProperty(fieldName, t.Elem())
		return Property{Type: "array", Items: &elemProp}
	case reflect.String:
		return Property{Type: "string"}
	case reflect.Struct:
		return deriveStructProperty(t)
	default:
		panic(fmt.Sprintf("unsupported type for field %q: %s", fieldName, t))
	}
}

// deriveStructProperty builds a Property with nested properties for a struct type.
func deriveStructProperty(t reflect.Type) Property {
	props := make(map[string]Property)
	var required []string

	for field := range t.Fields() {
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		name, opts := parseJSONTag(jsonTag)
		if name == "" {
			continue
		}
		prop := deriveProperty(field.Name, field.Type)
		if desc := field.Tag.Get("description"); desc != "" {
			prop.Description = desc
		}
		props[name] = prop
		if !strings.Contains(opts, "omitempty") {
			required = append(required, name)
		}
	}

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
