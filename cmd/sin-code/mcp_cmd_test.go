// SPDX-License-Identifier: MIT
// Purpose: smoke tests for `sin-code mcp list` — verifies the default
// ecosystem registry surfaces through the CLI and JSON output is valid.
package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestMCPListJSONIncludesDefaults(t *testing.T) {
	cmd := NewMCPCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var cfgs []map[string]any
	if err := json.Unmarshal(out.Bytes(), &cfgs); err != nil {
		t.Fatalf("invalid JSON from `mcp list --json`: %v\n%s", err, out.String())
	}
	names := map[string]bool{}
	for _, c := range cfgs {
		if n, ok := c["name"].(string); ok {
			names[n] = true
		}
	}
	for _, want := range []string{"websearch", "browser", "scheduler", "honcho"} {
		if !names[want] {
			t.Errorf("default server %q missing from mcp list output", want)
		}
	}
}
