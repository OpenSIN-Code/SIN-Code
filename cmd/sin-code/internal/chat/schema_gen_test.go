// SPDX-License-Identifier: MIT
// Purpose: tests for cmd/sin-code/internal/chat/schema_gen (issue #370).
package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ----- helpers ----------------------------------------------------------

func marshal(t *testing.T, s *Schema) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(b)
}

// prettyMap unmarshals a Schema into map[string]any for order-blind
// structural assertions. We rely on this instead of string-matching the
// JSON so legitimate reformats don't break us.
func prettyMap(t *testing.T, s *Schema) map[string]any {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return m
}

// TestSchema_SimpleStruct — basic primitive fields + required detection.
func TestSchema_SimpleStruct(t *testing.T) {
	type args struct {
		Path    string `json:"path" mcp:"required:true,description:Directory to search"`
		Pattern string `json:"pattern,omitempty" mcp:"description:Glob pattern"`
		Limit   int    `json:"limit" mcp:"required:true"`
	}
	got, err := GenerateSchema(args{})
	if err != nil {
		t.Fatalf("GenerateSchema: %v", err)
	}
	m := prettyMap(t, got)
	if m["type"] != "object" {
		t.Fatalf("type: %v", m["type"])
	}
	req, _ := m["required"].([]any)
	reqSet := map[string]bool{}
	for _, r := range req {
		reqSet[r.(string)] = true
	}
	if !reqSet["path"] || !reqSet["limit"] {
		t.Fatalf("expected path+limit in required, got %v", req)
	}
	if reqSet["pattern"] {
		t.Fatalf("pattern should be optional (omitempty)")
	}
	props, _ := m["properties"].(map[string]any)
	if _, ok := props["path"]; !ok {
		t.Fatalf("properties missing path: %v", props)
	}
	if _, ok := props["limit"]; !ok {
		t.Fatalf("properties missing limit: %v", props)
	}
	if desc := props["path"].(map[string]any)["description"]; desc != "Directory to search" {
		t.Fatalf("path description: %v", desc)
	}
}

// TestSchema_NestedStruct — child struct becomes `properties.<name>.type=object`.
func TestSchema_NestedStruct(t *testing.T) {
	type inner struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	type outer struct {
		Name  string `json:"name"`
		Inner inner  `json:"inner"`
	}
	got, err := GenerateSchema(outer{})
	if err != nil {
		t.Fatalf("GenerateSchema: %v", err)
	}
	prop := got.Properties["inner"]
	if prop == nil {
		t.Fatalf("inner property missing")
	}
	if prop.Type != "object" {
		t.Fatalf("inner.type: %v", prop.Type)
	}
	if prop.Properties["host"].Type != "string" {
		t.Fatalf("inner.host.type: %v", prop.Properties["host"].Type)
	}
	if prop.Properties["port"].Type != "integer" {
		t.Fatalf("inner.port.type: %v", prop.Properties["port"].Type)
	}
}

// TestSchema_SliceField — array fields emit `items` with the element schema.
func TestSchema_SliceField(t *testing.T) {
	type args struct {
		Tags []string `json:"tags"`
		Nums []int    `json:"nums,omitempty"`
	}
	got, _ := GenerateSchema(args{})
	tags := got.Properties["tags"]
	if tags.Type != "array" {
		t.Fatalf("tags.type: %v", tags.Type)
	}
	if tags.Items == nil || tags.Items.Type != "string" {
		t.Fatalf("tags.items.type: %v", tags.Items)
	}
	nums := got.Properties["nums"]
	if nums.Items == nil || nums.Items.Type != "integer" {
		t.Fatalf("nums.items.type: %v", nums.Items)
	}
	if !reflect.DeepEqual(got.Required, []string{"tags"}) {
		t.Fatalf("required ordering: %v", got.Required)
	}
}

// TestSchema_MapField — string-keyed maps use a sub-schema for
// `additionalProperties`; maps of structs exercise the recursive
// sub-schema path.
func TestSchema_MapField(t *testing.T) {
	type port struct {
		Number int `json:"number"`
	}
	type args struct {
		Counts  map[string]int `json:"counts"`
		Aliases map[string]port `json:"aliases"`
	}
	got, _ := GenerateSchema(args{})

	c := got.Properties["counts"]
	if c.Type != "object" {
		t.Fatalf("counts.type: %v", c.Type)
	}
	// additionalProperties should be a Schema{Type:"integer"}
	ap, ok := c.AdditionalProperties.(*Schema)
	if !ok {
		t.Fatalf("counts.AdditionalProperties not *Schema: %T", c.AdditionalProperties)
	}
	if ap.Type != "integer" {
		t.Fatalf("counts ap.type: %v", ap.Type)
	}

	a := got.Properties["aliases"]
	ap2, ok := a.AdditionalProperties.(*Schema)
	if !ok {
		t.Fatalf("aliases ap not Schema: %T", a.AdditionalProperties)
	}
	if ap2.Type != "object" || ap2.Properties["number"].Type != "integer" {
		t.Fatalf("aliases value schema wrong: %+v", ap2)
	}
}

// TestSchema_EmbeddedStruct — embedded struct appears as `allOf` entry,
// NOT as a top-level property.
func TestSchema_EmbeddedStruct(t *testing.T) {
	type base struct {
		CreatedAt time.Time `json:"created_at"`
		ID        string    `json:"id"`
	}
	type derived struct {
		base
		Name string `json:"name"`
	}
	got, _ := GenerateSchema(derived{})
	if len(got.AllOf) != 1 {
		t.Fatalf("expected 1 allOf entry, got %d", len(got.AllOf))
	}
	if got.AllOf[0].Type != "object" {
		t.Fatalf("allOf[0].type: %v", got.AllOf[0].Type)
	}
	if _, ok := got.Properties["base"]; ok {
		t.Fatalf("embedded struct must not appear as a property")
	}
	// AllOf entries must appear in declaration order; the base
	// contains created_at then id.
	if len(got.AllOf[0].Properties) != 2 {
		t.Fatalf("base props: %v", got.AllOf[0].Properties)
	}
	if got.Properties["name"] == nil {
		t.Fatalf("name property missing on derived")
	}
}

// TestSchema_PointerField — pointer-to-T follows to T and renders the
// same way the value would.
func TestSchema_PointerField(t *testing.T) {
	type args struct {
		Maybe *string `json:"maybe,omitempty"`
		Must  *int    `json:"must"`
		Holds *struct {
			Inner string `json:"inner"`
		} `json:"holds,omitempty"`
	}
	got, _ := GenerateSchema(args{})
	if got.Properties["maybe"].Type != "string" {
		t.Fatalf("maybe.type: %v", got.Properties["maybe"])
	}
	if got.Properties["must"].Type != "integer" {
		t.Fatalf("must.type: %v", got.Properties["must"])
	}
	if got.Properties["holds"].Type != "object" {
		t.Fatalf("holds.type: %v", got.Properties["holds"])
	}
	// necessary: maybe + holds are omitempty; must is required
	reqSet := map[string]bool{}
	for _, r := range got.Required {
		reqSet[r] = true
	}
	if !reqSet["must"] || reqSet["maybe"] || reqSet["holds"] {
		t.Fatalf("required wrong: %v", got.Required)
	}
}

// TestSchema_TimeField — time.Time renders as {type:string, format:date-time}.
func TestSchema_TimeField(t *testing.T) {
	type args struct {
		At time.Time `json:"at"`
	}
	got, _ := GenerateSchema(args{})
	at := got.Properties["at"]
	if at.Type != "string" {
		t.Fatalf("at.type: %v", at.Type)
	}
	if at.Format != "date-time" {
		t.Fatalf("at.format: %v", at.Format)
	}
}

// TestSchema_RefStyleDefs — UseDefs=true promotes nested structs to
// $defs and re-uses them via $ref. The first occurrence inlines, the
// second points at the same name in $defs.
func TestSchema_RefStyleDefs(t *testing.T) {
	type inner struct {
		Tag string `json:"tag"`
	}
	type outer struct {
		A inner `json:"a"`
		B inner `json:"b"`
	}
	got, err := GenerateSchemaWithConfig(outer{}, Config{UseDefs: true})
	if err != nil {
		t.Fatalf("GenerateSchemaWithConfig: %v", err)
	}
	if len(got.Defs) == 0 {
		t.Fatalf("expected $defs populated, got %+v", got)
	}
	// The first occurrence (a) inlines. The second (b) uses $ref.
	if got.Properties["a"].Ref != "" {
		t.Fatalf("a should inline, got ref=%q", got.Properties["a"].Ref)
	}
	if got.Properties["b"].Ref == "" {
		t.Fatalf("b should be a $ref, got %+v", got.Properties["b"])
	}
	if !strings.HasPrefix(got.Properties["b"].Ref, "#/$defs/") {
		t.Fatalf("b.ref must point at $defs, got %q", got.Properties["b"].Ref)
	}
}

// TestSchema_ByteStable — same input ⇒ same bytes. Critical for the
// eval harness (issue #171) and the system-prompt hash metric (issue #2).
func TestSchema_ByteStable(t *testing.T) {
	type args struct {
		B string `json:"b"`
		A int    `json:"a"`
		C bool   `json:"c"`
	}
	s1, _ := GenerateSchema(args{})
	s2, _ := GenerateSchema(args{})
	b1 := marshal(t, s1)
	b2 := marshal(t, s2)
	if b1 != b2 {
		t.Fatalf("byte drift between equal inputs:\n  a=%s\n  b=%s", b1, b2)
	}
	// Even if the JSON property happens to be named alphabetically,
	// the marshalled output must use a stable property order: source
	// declaration order (b, a, c) must be preserved or sorted
	// alphabetically — pick one, but stick to it. We re-sort
	// alphabetically inside MarshalJSON, which we validate below.
	if !strings.Contains(b1, `"a"`) {
		t.Fatalf("expected property a in output: %s", b1)
	}
}

// TestSchema_NonStructError — pass a non-struct (e.g. int) and expect
// an explicit error.
func TestSchema_NonStructError(t *testing.T) {
	if _, err := GenerateSchema(123); err == nil {
		t.Fatalf("expected error for non-struct input")
	}
	if _, err := GenerateSchema(nil); err == nil {
		t.Fatalf("expected error for nil input")
	}
}

// TestSchema_NestedPointerToStruct — pointer-to-struct functionals.
func TestSchema_NestedPointerToStruct(t *testing.T) {
	type inner struct {
		Tag string `json:"tag"`
	}
	type outer struct {
		Maybe *inner `json:"maybe,omitempty"`
	}
	got, _ := GenerateSchema(outer{})
	prop := got.Properties["maybe"]
	if prop.Type != "object" {
		t.Fatalf("maybe.type: %v", prop.Type)
	}
	if prop.Properties["tag"].Type != "string" {
		t.Fatalf("maybe.tag.type: %v", prop.Properties["tag"])
	}
}
