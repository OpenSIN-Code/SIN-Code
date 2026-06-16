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
// level of nesting (the v0 limit).
var jsonPattern = regexp.MustCompile("`(\\{(?:[^{}]|\\{[^{}]*\\})*\\})`")

// jsonFunc is a normalized view of one JSON shape requirement.
type jsonFunc struct {
	Shape    jsonShape
	Raw      string // the original spec text
	Required bool   // true if the shape is required (vs optional)
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
// the spec's shape. Every key in the spec shape must exist in the
// JSON object with a compatible type; the JSON object may have
// additional keys (non-strict mode).
func jsonMatch(spec jsonShape, doc any) (bool, string) {
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
// records.
func extractJSONShapes(text string) []jsonFunc {
	var out []jsonFunc
	for _, m := range jsonPattern.FindAllStringSubmatch(text, -1) {
		body := m[1]
		shape, err := parseShapeBody(body)
		if err != nil {
			continue
		}
		out = append(out, jsonFunc{Shape: shape, Raw: m[0], Required: true})
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
