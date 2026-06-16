// SPDX-License-Identifier: MIT
// Purpose: JSON-Schema signature matching. A spec requirement that
// names a JSON object shape (e.g. `\`{"name": str, "id": int}\``)
// is checked against the .json files in the source tree.
//
// v0 is intentionally simple: we don't pull in a full JSON-Schema
// library (M2 says no new deps). Instead, we do structural
// equality: every key in the spec shape must exist in the JSON
// document with a compatible type, and the document may have
// additional keys (non-strict mode; strict mode is configurable
// via .sin-code.yml in a later PR).
//
// Docs: docs/SPEC-LAYER.md §"Drift detection (the hardening)" (JSON)
package spec

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// jsonShape is one parsed JSON object literal in a spec requirement.
// The map is "key" -> "type annotation" (e.g. "str", "int",
// "[]str", "object", etc.).
type jsonShape map[string]string

// jsonPattern matches a backtick-wrapped JSON object literal. Allows
// nested objects and arrays in the value position via a single
// level of nesting (the v0 limit). The `}` inside the pattern is
// followed by `[^`]*` to allow a trailing "strict!" marker (or any
// other spec-level annotation) before the closing backtick.
var jsonPattern = regexp.MustCompile("`(\\{(?:[^{}]|\\{[^{}]*\\})*\\}(?:[^`]*))`")

// jsonFunc is a normalized view of one JSON shape requirement.
type jsonFunc struct {
	Shape    jsonShape
	Raw      string // the original spec text
	Required bool   // true if the shape is required (vs optional)
	Strict   bool   // true if the JSON must not have keys beyond Shape
}

// parseJSONFiles walks the .json files under root and returns a flat
// list of (path, parsed) pairs.
func parseJSONFiles(root string) ([]jsonFile, error) {
	var out []jsonFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == ".git" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".json") {
			return nil
		}
		data, rerr := os.ReadFile(path) // #nosec G304
		if rerr != nil {
			return nil // skip unreadable
		}
		var v any
		if jerr := json.Unmarshal(data, &v); jerr != nil {
			return nil // skip malformed
		}
		out = append(out, jsonFile{Path: path, Value: v})
		return nil
	})
	return out, err
}

type jsonFile struct {
	Path  string
	Value any
}

// jsonMatch reports whether a JSON file's top-level object matches
// the spec's shape. In non-strict mode (the default), every key in
// the spec shape must exist in the JSON object with a compatible
// type; the JSON object may have additional keys. In strict mode,
// the JSON object must not have any key that is not in the spec
// shape (extras are reported as drift).
func jsonMatch(spec jsonShape, doc any, strict bool) (bool, string) {
	obj, ok := doc.(map[string]any)
	if !ok {
		return false, "top-level value is not a JSON object"
	}
	for k, want := range spec {
		got, present := obj[k]
		if !present {
			return false, fmt.Sprintf("missing key %q", k)
		}
		if !typeCompatible(want, got) {
			return false, fmt.Sprintf("key %q: spec type %q, got %T", k, want, got)
		}
	}
	if strict {
		// Build a set of spec keys for O(1) lookup.
		specSet := make(map[string]bool, len(spec))
		for k := range spec {
			specSet[k] = true
		}
		// Report the first extra key (cheaper than listing all).
		for k := range obj {
			if !specSet[k] {
				return false, fmt.Sprintf("strict mode: extra key %q not in spec", k)
			}
		}
	}
	return true, ""
}

// typeCompatible checks the spec's type-annotation string against
// the actual JSON value's Go type.
func typeCompatible(want string, got any) bool {
	if want == "" {
		return true
	}
	want = strings.TrimSuffix(want, "?")
	if strings.HasPrefix(want, "[]") {
		wantInner := strings.TrimPrefix(want, "[]")
		arr, ok := got.([]any)
		if !ok {
			return false
		}
		if wantInner == "" {
			return true
		}
		for _, v := range arr {
			if !typeCompatible(wantInner, v) {
				return false
			}
		}
		return true
	}
	if want == "object" || want == "{}" || strings.HasPrefix(want, "map[") {
		_, ok := got.(map[string]any)
		return ok
	}
	switch got.(type) {
	case string:
		return want == "string" || want == "str"
	case float64:
		return want == "number" || want == "int" || want == "float" || want == "num"
	case bool:
		return want == "bool" || want == "boolean"
	case nil:
		return want == "null" || want == "None"
	case []any:
		return want == "array" || want == "[]"
	case map[string]any:
		return want == "object" || want == "{}"
	}
	return false
}

// extractJSONShapes scans the spec for backtick-wrapped JSON
// object literals in requirements and returns them as jsonFunc
// records. The strict mode is set per-shape via a trailing
// "strict!" marker inside the body, e.g.:
//   `{"name": str, "id": int} strict!`
// (Useful for hand-edited specs that need extras forbidden.)
func extractJSONShapes(text string) []jsonFunc {
	var out []jsonFunc
	for _, m := range jsonPattern.FindAllStringSubmatch(text, -1) {
		body := strings.TrimSpace(m[1])
		strict := false
		// The "strict!" marker is a single trailing token inside
		// the body, recognized only when surrounded by whitespace.
		// We strip it before JSON-parse so the body remains a
		// valid JSON object literal.
		if strings.HasSuffix(body, "strict!") {
			strict = true
			body = strings.TrimSpace(strings.TrimSuffix(body, "strict!"))
		}
		shape, err := parseShapeBody(body)
		if err != nil {
			continue
		}
		out = append(out, jsonFunc{Shape: shape, Raw: m[0], Required: true, Strict: strict})
	}
	return out
}

// parseShapeBody parses a JSON object literal (`{"k": "type", ...}`)
// into a map. The body is already a complete JSON object; we just
// decode it directly.
func parseShapeBody(body string) (jsonShape, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, err
	}
	out := make(jsonShape, len(raw))
	for k, v := range raw {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			out[k] = s
		} else {
			// nested shape (e.g. {"meta": {}}) — store canonical form
			out[k] = strings.TrimSpace(string(v))
		}
	}
	return out, nil
}
