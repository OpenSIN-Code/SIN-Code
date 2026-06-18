// SPDX-License-Identifier: MIT
// Purpose: tests for the reflect-based JSON-schema generator (issue #370).
package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

type simpleUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

type nestedProfile struct {
	User   simpleUser `json:"user"`
	Status string     `json:"status"`
}

type sliceHolder struct {
	Tags []string `json:"tags"`
	IDs  []int    `json:"ids,omitempty"`
}

type mapHolder struct {
	Meta map[string]int `json:"meta"`
}

type baseFields struct {
	ID      int    `json:"id"`
	Created string `json:"created"`
}

type embeddedUser struct {
	baseFields
	Name string `json:"name"`
}

type pointerHolder struct {
	Name  *string `json:"name"`
	Count *int    `json:"count"`
}

type timeHolder struct {
	Created time.Time `json:"created"`
}

func schemaType(t *testing.T, v any) map[string]*Schema {
	t.Helper()
	s, err := GenerateSchema(v)
	if err != nil {
		t.Fatalf("GenerateSchema: %v", err)
	}
	if s.Type != "object" {
		t.Fatalf("root type = %q; want object", s.Type)
	}
	return s.Properties
}

func TestGenerateSchema_Simple(t *testing.T) {
	p := schemaType(t, simpleUser{})
	if p["name"] == nil || p["name"].Type != "string" {
		t.Fatalf("name field wrong: %+v", p["name"])
	}
	if p["age"] == nil || p["age"].Type != "integer" {
		t.Fatalf("age field wrong: %+v", p["age"])
	}
}

func TestGenerateSchema_Nested(t *testing.T) {
	s, err := GenerateSchema(nestedProfile{})
	if err != nil {
		t.Fatal(err)
	}
	user := s.Properties["user"]
	if user == nil || user.Type != "object" {
		t.Fatalf("nested user field wrong: %+v", user)
	}
	if user.Properties["email"] == nil {
		t.Fatal("nested user.email missing")
	}
}

func TestGenerateSchema_Slice(t *testing.T) {
	p := schemaType(t, sliceHolder{})
	tags := p["tags"]
	if tags == nil || tags.Type != "array" {
		t.Fatalf("tags wrong: %+v", tags)
	}
	if tags.Items == nil || tags.Items.Type != "string" {
		t.Fatalf("tags items wrong: %+v", tags.Items)
	}
}

func TestGenerateSchema_Map(t *testing.T) {
	p := schemaType(t, mapHolder{})
	meta := p["meta"]
	if meta == nil || meta.Type != "object" {
		t.Fatalf("meta wrong: %+v", meta)
	}
	if meta.AdditionalProps == nil || meta.AdditionalProps.Type != "integer" {
		t.Fatalf("meta additionalProperties wrong: %+v", meta.AdditionalProps)
	}
}

func TestGenerateSchema_Embedded(t *testing.T) {
	p := schemaType(t, embeddedUser{})
	// hoisted from embedded baseFields
	if p["id"] == nil {
		t.Fatal("embedded field 'id' not hoisted")
	}
	if p["created"] == nil {
		t.Fatal("embedded field 'created' not hoisted")
	}
	// own field
	if p["name"] == nil {
		t.Fatal("own field 'name' missing")
	}
}

func TestGenerateSchema_Pointer(t *testing.T) {
	s, err := GenerateSchema(pointerHolder{})
	if err != nil {
		t.Fatal(err)
	}
	name := s.Properties["name"]
	if name == nil || name.Type != "string" {
		t.Fatalf("pointer field name wrong: %+v", name)
	}
	// pointers should not be required
	for _, r := range s.Required {
		if r == "name" || r == "count" {
			t.Fatalf("pointer field %q should not be required", r)
		}
	}
}

func TestGenerateSchema_TimeTime(t *testing.T) {
	p := schemaType(t, timeHolder{})
	created := p["created"]
	if created == nil || created.Type != "string" {
		t.Fatalf("time.Time field wrong: %+v", created)
	}
	if !strings.Contains(created.Description, "RFC 3339") {
		t.Fatalf("time.Time description wrong: %q", created.Description)
	}
}

func TestGenerateSchema_ByteStability(t *testing.T) {
	a, _ := GenerateSchema(nestedProfile{})
	b, _ := GenerateSchema(nestedProfile{})
	j1, _ := SchemaToJSON(a)
	j2, _ := SchemaToJSON(b)
	if !reflect.DeepEqual(j1, j2) {
		t.Fatal("schema output not byte-stable for same type")
	}
	// also verify it round-trips through json.Unmarshal
	var raw1, raw2 map[string]any
	if err := json.Unmarshal(j1, &raw1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(j2, &raw2); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(raw1, raw2) {
		t.Fatal("schema round-trip mismatch")
	}
}
