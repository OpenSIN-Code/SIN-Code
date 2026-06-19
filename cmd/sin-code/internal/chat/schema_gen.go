// SPDX-License-Identifier: MIT
// Purpose: reflect-based JSON-schema generator from Go structs (issue #370).
// Produces a deterministic JSON Schema (draft-07 subset) for any Go value,
// honouring `json:"name"` tags, `omitempty`, pointers, slices, maps,
// embedded structs, and time.Time (serialised as a string). Byte-stable:
// the same Go type always emits byte-identical JSON so downstream golden
// tests can pin the schema.
package chat

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Schema is a minimal JSON-Schema (draft-07 subset) representation.
type Schema struct {
	Type            string             `json:"type,omitempty"`
	Properties      map[string]*Schema `json:"properties,omitempty"`
	Required        []string           `json:"required,omitempty"`
	Items           *Schema            `json:"items,omitempty"`
	AdditionalProps *Schema            `json:"additionalProperties,omitempty"`
	Description     string             `json:"description,omitempty"`
}

// MarshalJSON serialises Schema with deterministic key ordering for the
// Properties map and a sorted Required slice, guaranteeing byte-stability
// for the same logical schema.
func (s *Schema) MarshalJSON() ([]byte, error) {
	type alias Schema
	tmp := alias(*s)
	if tmp.Properties != nil {
		// encoding/json sorts map keys natively, but we make it explicit:
		// there is nothing to do here — Go's json sorts string map keys.
	}
	if len(tmp.Required) > 1 {
		sort.Strings(tmp.Required)
	}
	return json.Marshal(tmp)
}

// GenerateSchema builds a JSON Schema for the Go type of v.
// v may be a zero value; only its type is inspected.
func GenerateSchema(v any) (*Schema, error) {
	if v == nil {
		return &Schema{Type: "null"}, nil
	}
	t := reflect.TypeOf(v)
	return schemaFromType(t, map[reflect.Type]bool{})
}

// schemaFromType recursively builds a Schema from a reflect.Type. The
// `seen` map guards against infinite recursion on self-referential types
// (e.g. linked lists / trees) by emitting a stub once a type is in-flight.
func schemaFromType(t reflect.Type, seen map[reflect.Type]bool) (*Schema, error) {
	// time.Time is a struct under the hood but serialises to a string.
	if t == reflect.TypeOf(time.Time{}) {
		return &Schema{Type: "string", Description: "RFC 3339 timestamp"}, nil
	}

	switch t.Kind() {
	case reflect.Pointer:
		return schemaFromType(t.Elem(), seen)
	case reflect.String:
		return &Schema{Type: "string"}, nil
	case reflect.Bool:
		return &Schema{Type: "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Schema{Type: "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return &Schema{Type: "number"}, nil
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			// []byte / []uint8 serialises as a base64 string in JSON.
			return &Schema{Type: "string", Description: "base64-encoded bytes"}, nil
		}
		item, err := schemaFromType(t.Elem(), seen)
		if err != nil {
			return nil, err
		}
		return &Schema{Type: "array", Items: item}, nil
	case reflect.Map:
		val, err := schemaFromType(t.Elem(), seen)
		if err != nil {
			return nil, err
		}
		return &Schema{Type: "object", AdditionalProps: val}, nil
	case reflect.Struct:
		return schemaFromStruct(t, seen)
	case reflect.Interface:
		return &Schema{Type: "object", AdditionalProps: &Schema{}}, nil
	default:
		return &Schema{Type: "object", AdditionalProps: &Schema{}}, nil
	}
}

// schemaFromStruct builds an object schema from a struct type, honouring
// json tags, omitempty, and embedded (anonymous) fields which are
// hoisted into the parent's properties (matching encoding/json behaviour).
func schemaFromStruct(t reflect.Type, seen map[reflect.Type]bool) (*Schema, error) {
	if seen[t] {
		// Self-referential type already being processed — emit a stub.
		return &Schema{Type: "object"}, nil
	}
	seen[t] = true
	defer delete(seen, t)

	props := map[string]*Schema{}
	var required []string

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		// Embedded (anonymous) field: hoist its properties into the parent.
		// We process anonymous fields even when the type name is unexported
		// (e.g. `baseFields`), matching encoding/json behaviour — only the
		// inner struct's own exported fields are promoted.
		if f.Anonymous {
			inner, err := schemaFromType(f.Type, seen)
			if err != nil {
				return nil, err
			}
			if inner.Type == "object" && inner.Properties != nil {
				for k, v := range inner.Properties {
					props[k] = v
				}
				innerReq := inner.Required
				if f.Type.Kind() == reflect.Pointer {
					// Pointer-embedded fields are never required.
					innerReq = nil
				}
				required = append(required, innerReq...)
			}
			continue
		}

		if !f.IsExported() {
			continue
		}

		name, omitempty, skip := parseJSONTag(f.Tag.Get("json"))
		if skip {
			continue
		}

		fieldSchema, err := schemaFromType(f.Type, seen)
		if err != nil {
			return nil, err
		}
		props[name] = fieldSchema

		isPtr := f.Type.Kind() == reflect.Pointer
		if !omitempty && !isPtr {
			required = append(required, name)
		}
	}

	sch := &Schema{
		Type:       "object",
		Properties: props,
	}
	if len(required) > 0 {
		sch.Required = required
	}
	return sch, nil
}

// parseJSONTag extracts the field name, omitempty flag, and skip flag
// from a `json:"..."` struct tag.
func parseJSONTag(tag string) (name string, omitempty bool, skip bool) {
	if tag == "-" {
		return "", false, true
	}
	if tag == "" {
		return "", false, false
	}
	parts := strings.Split(tag, ",")
	name = strings.TrimSpace(parts[0])
	for _, opt := range parts[1:] {
		if strings.TrimSpace(opt) == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty, false
}

// SchemaToJSON is a convenience helper returning pretty-printed, determinist
// JSON for a schema. Useful for golden tests.
func SchemaToJSON(s *Schema) ([]byte, error) {
	if s == nil {
		return []byte("null"), nil
	}
	b, err := s.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}
	return b, nil
}
