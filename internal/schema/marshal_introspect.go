package schema

import (
	"reflect"
	"strings"
)

// FindUnmarshalable returns the dotted struct-field path of the first
// non-marshalable element in v, plus ok=true when a path was found. It is
// the introspection helper FR4 (Story 1.4) wires into the tools dispatch
// path so that a tool whose Out value contains a runtime-unmarshalable
// type (chan, func, cyclic struct) yields a -32603 response carrying
// error.data.field rather than a generic Internal error.
//
// Conventions:
//   - chan and func at any depth are reported by the field they sit under.
//   - cycle detection uses the address of pointer values; a pointer that
//     re-enters itself is reported as a cycle.
//   - slice/array/map elements are walked as a representative-of-type:
//     the path strips the index ("Items.<field>", not "Items[3].<field>").
//   - nested struct fields are joined with "." ("Outer.Inner.Bad").
//   - top-level v that is itself a chan or func returns "" with ok=true,
//     so callers using the empty path know the failure was at the root.
//
// The walker is depth-bounded by maxSchemaDepth (already used elsewhere in
// the package) to prevent runaway recursion on hostile inputs.
func FindUnmarshalable(v any) (string, bool) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return "", false
	}
	visited := map[uintptr]bool{}
	var path []string
	if walkUnmarshalable(rv, &path, visited, 0) {
		return strings.Join(path, "."), true
	}
	return "", false
}

func walkUnmarshalable(v reflect.Value, path *[]string, visited map[uintptr]bool, depth int) bool {
	if depth > maxSchemaDepth {
		return false
	}
	switch v.Kind() {
	case reflect.Chan, reflect.Func:
		return true
	case reflect.Pointer:
		return walkPointer(v, path, visited, depth)
	case reflect.Interface:
		if v.IsNil() {
			return false
		}
		return walkUnmarshalable(v.Elem(), path, visited, depth+1)
	case reflect.Struct:
		return walkStruct(v, path, visited, depth)
	case reflect.Slice, reflect.Array:
		// Empty slices/arrays marshal cleanly regardless of element type;
		// only walk the first element to surface cycles inside element
		// structs.
		if v.Len() == 0 {
			return false
		}
		return walkUnmarshalable(v.Index(0), path, visited, depth+1)
	case reflect.Map:
		iter := v.MapRange()
		if iter.Next() {
			return walkUnmarshalable(iter.Value(), path, visited, depth+1)
		}
		return false
	default:
		return false
	}
}

func walkPointer(v reflect.Value, path *[]string, visited map[uintptr]bool, depth int) bool {
	if v.IsNil() {
		return false
	}
	ptr := v.Pointer()
	if visited[ptr] {
		return true
	}
	visited[ptr] = true
	defer delete(visited, ptr)
	return walkUnmarshalable(v.Elem(), path, visited, depth+1)
}

func walkStruct(v reflect.Value, path *[]string, visited map[uintptr]bool, depth int) bool {
	t := v.Type()
	if t == timeType || t == rawMessageType {
		return false
	}
	for i := range v.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		*path = append(*path, fieldJSONName(f))
		if walkUnmarshalable(v.Field(i), path, visited, depth+1) {
			return true
		}
		*path = (*path)[:len(*path)-1]
	}
	return false
}

// fieldJSONName returns the JSON-tagged name of a struct field when present,
// falling back to the Go field name. Matches the visibility rules tool
// authors see in the wire format so error.data.field is recognizable.
func fieldJSONName(f reflect.StructField) string {
	if tag := f.Tag.Get("json"); tag != "" && tag != "-" {
		name, _, _ := strings.Cut(tag, ",")
		if name != "" {
			return name
		}
	}
	return f.Name
}
