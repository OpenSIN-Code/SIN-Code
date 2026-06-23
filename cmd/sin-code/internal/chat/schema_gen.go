// SPDX-License-Identifier: MIT
// Purpose: emit JSON Schema (draft-07-compatible fragments) for arbitrary
// Go struct values via reflection. Replaces hand-written map[string]any
// schema literals in serve.go (issue #370).
//
// Determinism: outputs are byte-stable per (input struct, tag set) pair.
// Field order is source-declaration order — never reflection iteration
// order. Map keys are alphabetically sorted. nil slices marshal as `[]`,
// nil maps marshal as `{}`, so two calls on equal inputs produce equal
// bytes without depending on Go runtime layout choices.
//
// Supported Go types: primitive scalars (string/bool/int*/uint*/float*),
// time.Time (rendered as `{"type":"string","format":"date-time"}`), slices,
// arrays, maps (string-keyed only — non-string-keyed maps fall back to
// `{"type":"object","additionalProperties":true}`), pointers (followed to
// the underlying type), and nested structs (inlined with `allOf`).
package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Schema is the JSON-Schema fragment we emit. Every field uses the
// default JSON encoding (no custom MarshalJSON needed for *Schema
// itself — only its inner maps need ordering, which we enforce by
// populating with sorted keys).
type Schema struct {
	// Type is the JSON-schema "type" — "object", "string", "array",
	// "integer", "number", "boolean", or "null". For composite outputs
	// (allOf, $ref) the Type field is left blank and the composite
	// fields are populated instead.
	Type string `json:"type,omitempty"`

	// Description is the human-readable description pulled from the
	// `mcp:"description:…"` tag, the const block, or the struct field
	// doc-comment (whichever the caller supplies last).
	Description string `json:"description,omitempty"`

	// Properties maps a JSON name → its Schema. Populated for object and
	// composite outputs. Keys are sorted at marshal time so the bytes
	// are stable.
	Properties map[string]*Schema `json:"properties,omitempty"`

	// Required is the sorted list of JSON field names that the caller
	// must supply. Sorted at marshal time so the bytes are stable.
	Required []string `json:"required,omitempty"`

	// Items points at the element schema for array outputs.
	Items *Schema `json:"items,omitempty"`

	// AdditionalProperties allows extra keys on maps/objects. JSON
	// schema permits either a boolean or a sub-schema here, so we model
	// it as `any` — callers and tests should not cast it.
	AdditionalProperties any `json:"additionalProperties,omitempty"`

	// Enum constrains string/int outputs to a closed set.
	Enum []any `json:"enum,omitempty"`

	// Format hints the JSON-schema "format" — only "date-time" is
	// emitted by GenerateSchema (time.Time recognition).
	Format string `json:"format,omitempty"`

	// AllOf is used to express embedded structs: the outer object is
	// the parent, each embedded struct contributes one allOf entry.
	// AllOf entries are emitted in source-declaration order.
	AllOf []*Schema `json:"allOf,omitempty"`

	// Ref is populated for the inline-`$ref` style: sub-schemas that
	// would otherwise repeat verbatim receive a stable identifier like
	// "#/$defs/TypeName" so downstream generators can deduplicate.
	// GenerateSchema does not deduplicate by default (de-dup is opt-in
	// in `Config.UseDefs`) — but the Ref field is set on every nested
	// reuse when that flag is on.
	Ref string `json:"$ref,omitempty"`

	// Defs is the top-level `$defs` block — populated only on the
	// root schema when Config.UseDefs is true.
	Defs map[string]*Schema `json:"$defs,omitempty"`
}

// GenerateSchema returns the JSON-Schema fragment for v. v must be a
// struct (or a pointer to a struct) — anything else yields an error
// because the schema protocol is meaningless for non-record types.
func GenerateSchema(v any) (*Schema, error) {
	return GenerateSchemaWithConfig(v, Config{})
}

// Config tunes GenerateSchema. Zero value (Config{}) gives the canonical
// behaviour: object schema + required list + sorted keys + allOf for
// embeds. Use UseDefs=true to opt into the inline-$ref style.
type Config struct {
	// UseDefs switches the generator to emit a top-level $defs block
	// and use $ref for every nested struct (instead of inlining). This
	// is opt-in because $ref trips up older clients that don't resolve
	// the $ref target.
	UseDefs bool
}

// GenerateSchemaWithConfig is the configurable form of GenerateSchema.
func GenerateSchemaWithConfig(v any, cfg Config) (*Schema, error) {
	if v == nil {
		return nil, errors.New("chat: GenerateSchema: nil value")
	}
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("chat: GenerateSchema: expected struct, got %s", t.Kind())
	}
	seen := map[string]*Schema{}
	s, err := buildStruct(t, cfg, seen)
	if err != nil {
		return nil, err
	}
	if cfg.UseDefs && len(seen) > 0 {
		s.Defs = seen
	}
	return s, nil
}

// buildStruct walks a struct type once and returns its object schema.
// seen tracks structs we've already promoted to $defs so we emit a
// stable Ref-only pointer to the second occurrence.
func buildStruct(t reflect.Type, cfg Config, seen map[string]*Schema) (*Schema, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("chat: buildStruct: expected struct, got %s", t.Kind())
	}

	// De-dup when UseDefs is on: identical struct name ⇒ same `$defs`
	// entry, so subsequent occurrences become a `{ "$ref": ... }`.
	if cfg.UseDefs {
		if existing, ok := seen[t.String()]; ok && existing != nil {
			return &Schema{Ref: "#/$defs/" + defKey(t)}, nil
		}
	}

	props := map[string]*Schema{}
	required := []string{}
	allOf := []*Schema{}

	// Iterate the source types directly so we preserve declaration
	// order. reflect.VisibleFields flattens embedded struct fields into
	// the parent — that's exactly what we want for embeds, so we get
	// them inline by default and as allOf entries when UseDefs is on.
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		ft := f.Type
		// Anonymous (embedded) struct → allOf entry, not a property.
		// We process these even when the field name itself is
		// unexported: an embedded type's properties are promoted into
		// the parent in JSON marshaling, so the embed must be recorded
		// in the schema regardless of casing.
		if f.Anonymous && ft.Kind() == reflect.Struct {
			inner, err := buildStructForEmbed(ft, cfg, seen)
			if err != nil {
				return nil, fmt.Errorf("chat: embed %s: %w", f.Name, err)
			}
			allOf = append(allOf, inner)
			continue
		}
		// Anonymous embedded pointer-to-struct — same treatment.
		if f.Anonymous && ft.Kind() == reflect.Ptr && ft.Elem().Kind() == reflect.Struct {
			inner, err := buildStructForEmbed(ft.Elem(), cfg, seen)
			if err != nil {
				return nil, fmt.Errorf("chat: embed %s: %w", f.Name, err)
			}
			allOf = append(allOf, inner)
			continue
		}

		if !f.IsExported() {
			continue
		}
		name, omit, desc := parseFieldTag(f)
		if name == "-" {
			continue
		}
		propSchema, err := buildField(ft, cfg, seen)
		if err != nil {
			return nil, fmt.Errorf("chat: field %s.%s: %w", t.Name(), f.Name, err)
		}
		if desc != "" {
			propSchema.Description = desc
		}
		props[name] = propSchema
		if !omit {
			required = append(required, name)
		}
	}

	sort.Strings(required)

	s := &Schema{
		Type:       "object",
		Properties: props,
		Required:   required,
	}
	if len(allOf) > 0 {
		s.AllOf = allOf
	}

	if cfg.UseDefs {
		seen[t.String()] = s
	}
	return s, nil
}

// buildStructForEmbed is buildStruct's embed sibling. We do not add
// the embedded struct's properties into the parent — Go's embedding
// would normally flatten them, but JSON schema conventions encourage
// explicit allOf so reviewers can tell embeds apart from real fields.
func buildStructForEmbed(t reflect.Type, cfg Config, seen map[string]*Schema) (*Schema, error) {
	return buildStruct(t, cfg, seen)
}

// buildField dispatches by kind/time to the typed sub-schemas. The
// "anonymous" and "validatable" cases (time.Time, custom stringers)
// are detected first because they have Kind()==Struct but render as
// scalars.
func buildField(t reflect.Type, cfg Config, seen map[string]*Schema) (*Schema, error) {
	// time.Time has Kind()==Struct but is a logical scalar.
	if t == reflect.TypeOf(time.Time{}) {
		return &Schema{Type: "string", Format: "date-time"}, nil
	}

	// Follow pointer-to-T until we reach a concrete type or nil.
	if t.Kind() == reflect.Ptr {
		inner, err := buildField(t.Elem(), cfg, seen)
		if err != nil {
			return nil, err
		}
		return inner, nil
	}

	switch t.Kind() {
	case reflect.String:
		return &Schema{Type: "string"}, nil
	case reflect.Bool:
		return &Schema{Type: "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &Schema{Type: "integer"}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Schema{Type: "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return &Schema{Type: "number"}, nil
	case reflect.Slice, reflect.Array:
		items, err := buildField(t.Elem(), cfg, seen)
		if err != nil {
			return nil, err
		}
		return &Schema{Type: "array", Items: items}, nil
	case reflect.Map:
		// We only fully model string-keyed maps. Non-string keys fall
		// back to additionalProperties: true.
		if t.Key().Kind() == reflect.String {
			valueSchema, err := buildField(t.Elem(), cfg, seen)
			if err != nil {
				return nil, err
			}
			return &Schema{
				Type:                 "object",
				AdditionalProperties: valueSchema,
			}, nil
		}
		return &Schema{Type: "object", AdditionalProperties: true}, nil
	case reflect.Struct:
		return buildStruct(t, cfg, seen)
	default:
		return nil, fmt.Errorf("chat: unsupported field kind %s", t.Kind())
	}
}

// parseFieldTag decrypts the field's `json:"…"` and `mcp:"…"` tags.
//
// json:"name[,omitempty]" → JSON property name + optional omission.
//
//	mcp:"description:…[,required:true|false]" — accepted separators inside
//	  the mcp tag are `;` OR `,`. The comma variant is the rule-of-thumb
//	  because struct-tag composers rarely escape semicolons.
//
// The mcp tag is private to this package. Adopting it across the rest
// of the codebase is part of issue #370's rollout.
func parseFieldTag(f reflect.StructField) (name string, omit bool, desc string) {
	jsonTag := strings.TrimSpace(f.Tag.Get("json"))
	if jsonTag == "-" {
		return "-", false, ""
	}
	if jsonTag == "" {
		name = lowerFirst(f.Name)
	} else {
		// Support `json:",omitempty"` (name fell back to lowerFirst).
		parts := strings.Split(jsonTag, ",")
		if parts[0] != "" {
			name = parts[0]
		} else {
			name = lowerFirst(f.Name)
		}
		for _, p := range parts[1:] {
			if p == "omitempty" {
				omit = true
			}
		}
	}
	// Accept both `;` and `,` as sub-key separators inside the mcp tag.
	mcpTag := f.Tag.Get("mcp")
	for _, raw := range strings.FieldsFunc(mcpTag, func(r rune) bool {
		return r == ';' || r == ','
	}) {
		part := strings.TrimSpace(raw)
		switch {
		case part == "":
			// skip
		case strings.HasPrefix(part, "description:"):
			desc = strings.TrimPrefix(part, "description:")
		case part == "required:true":
			omit = false // explicit required, override omitempty
		case part == "required:false":
			omit = true // explicit optional, override anything else
		}
	}
	return name, omit, desc
}

// lowerFirst converts the first rune to lower-case (so a Go field
// `FilePath` → `filePath`). We do not camel-case the rest — the struct
// author is responsible for the canonical JSON name in the tag.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// defKey is the canonical name used in `#/$defs/<key>` Refs. We hash
// the qualified type name so structurally-identical local types that
// share a name don't collide, but full qualification keeps
// cross-package identities distinct.
func defKey(t reflect.Type) string {
	return strings.ReplaceAll(t.String(), ".", "_")
}

// MarshalJSON renders the Schema to canonical, byte-stable JSON.
// Properties and Required are emitted in alphabetical order so two
// calls on equal inputs produce equal bytes. AllOf preserves
// source-declaration order — that order is intentional and structural.
func (s *Schema) MarshalJSON() ([]byte, error) {
	type alias Schema
	out := map[string]any{}

	if len(s.Defs) > 0 {
		defKeys := make([]string, 0, len(s.Defs))
		for k := range s.Defs {
			defKeys = append(defKeys, k)
		}
		sort.Strings(defKeys)
		defs := make(map[string]*Schema, len(s.Defs))
		for _, k := range defKeys {
			defs[k] = s.Defs[k]
		}
		out["$defs"] = defs
	}

	// Composite (ref-only) schemas emit just the $ref. We use the alias
	// type to invoke the default encoder for everything else.
	if s.Ref != "" {
		out["$ref"] = s.Ref
		// Some JSON-schema validators reject extra fields on a $ref
		// node, so we intentionally do NOT carry over other fields
		// when Ref is set.
		return json.Marshal(out)
	}

	if len(s.AllOf) > 0 {
		allOf := make([]any, 0, len(s.AllOf))
		for _, a := range s.AllOf {
			allOf = append(allOf, a)
		}
		out["allOf"] = allOf
	}

	if s.Type != "" {
		out["type"] = s.Type
	}
	if s.Description != "" {
		out["description"] = s.Description
	}
	if s.Format != "" {
		out["format"] = s.Format
	}
	if len(s.Properties) > 0 {
		keys := make([]string, 0, len(s.Properties))
		for k := range s.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		props := make(map[string]any, len(s.Properties))
		for _, k := range keys {
			props[k] = s.Properties[k]
		}
		out["properties"] = props
	}
	if len(s.Required) > 0 {
		req := append([]string(nil), s.Required...)
		sort.Strings(req)
		out["required"] = req
	}
	if s.Items != nil {
		out["items"] = s.Items
	}
	if s.AdditionalProperties != nil {
		out["additionalProperties"] = s.AdditionalProperties
	}
	if len(s.Enum) > 0 {
		out["enum"] = s.Enum
	}

	return json.Marshal(out)
}
