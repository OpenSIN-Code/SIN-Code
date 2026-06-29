// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when testgen is refactored
package testgen

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func simpleType(t string) bool {
	switch t {
	case "int", "int64", "int32", "string", "bool", "float64", "float32", "uint", "uint64":
		return true
	}
	return false
}

func zeroValue(t string) string {
	switch t {
	case "int", "int64", "int32", "uint", "uint64":
		return "0"
	case "string":
		return `""`
	case "bool":
		return "false"
	case "float64", "float32":
		return "0.0"
	default:
		return "nil"
	}
}

// jsonLiteral renders a JSON-decoded value as a Go struct/array/map
// literal. Supported types: nil, bool, float64, float32, int, int64,
// string, []any, map[string]any, json.Number, time.Time, time.Duration,
// []byte (base64). Unsupported types fall back to nil so a bad field
// never breaks the entire generated test.
func jsonLiteral(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if float64(int(x)) == x {
			return fmt.Sprintf("%d", int(x))
		}
		return fmt.Sprintf("%v", x)
	case float32:
		return fmt.Sprintf("%v", x)
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case string:
		return fmt.Sprintf("%q", x)
	case json.Number:
		s := x.String()
		if i, err := x.Int64(); err == nil {
			return fmt.Sprintf("%d", i)
		}
		if f, err := x.Float64(); err == nil {
			if float64(int64(f)) == f {
				return fmt.Sprintf("%d", int64(f))
			}
			return fmt.Sprintf("%v", f)
		}
		return fmt.Sprintf("%q", s)
	case time.Time:
		return "time.Date(" + timeFormat(x) + ")"
	case time.Duration:
		return fmt.Sprintf("%v*time.Nanosecond", int64(x))
	case []byte:
		return fmt.Sprintf("[]byte(%q)", string(x))
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, jsonLiteral(e))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(x))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%q: %s", k, jsonLiteral(x[k])))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return "nil"
	}
}

// timeFormat returns the year/month/day/h/m/s/n,location arguments for
// time.Date() from a time.Time. Centralised so the unit test and the
// generator agree on byte-stable output.
func timeFormat(t time.Time) string {
	return fmt.Sprintf("%d, time.%s, %d, %d, %d, %d, %d, time.UTC",
		t.Year(), t.Month().String(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond())
}

// testKey is the map key used to look up per-function LLM cases. It
// matches both method (`Receiver_Name`) and free-function (`Name`)
// spellings of describeFunc output.
func testKey(fn FuncInfo) string {
	if fn.IsMethod {
		return fn.Receiver + "_" + fn.Name
	}
	return fn.Name
}

// renderCaseRow writes one `tests := []struct{...}{...}` element for
// the LLM-supplied TestCase. Unknown arg names or want names produce a
// zero-value fallback so the partial row still compiles.
func renderCaseRow(fn FuncInfo, tc TestCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\t\t{\n")
	fmt.Fprintf(&b, "\t\t\tname: %q,\n", tc.Name)
	if len(fn.Args) > 0 {
		fmt.Fprintf(&b, "\t\t\targs: args{\n")
		for _, a := range fn.Args {
			if v, ok := tc.Args[a.Name]; ok {
				fmt.Fprintf(&b, "\t\t\t\t%s: %s,\n", a.Name, jsonLiteral(v))
			} else {
				fmt.Fprintf(&b, "\t\t\t\t%s: %s,\n", a.Name, zeroValue(a.Type))
			}
		}
		fmt.Fprintf(&b, "\t\t\t},\n")
	}
	for _, r := range fn.Returns {
		if r.Type == "error" {
			continue
		}
		field := "want" + strings.Title(r.Name)
		if v, ok := tc.Wants[r.Name]; ok {
			fmt.Fprintf(&b, "\t\t\t%s: %s,\n", field, jsonLiteral(v))
		} else {
			fmt.Fprintf(&b, "\t\t\t%s: %s,\n", field, zeroValue(r.Type))
		}
	}
	fmt.Fprintf(&b, "\t\t\twantErr: %v,\n", tc.WantErr)
	fmt.Fprintf(&b, "\t\t},\n")
	return b.String()
}
