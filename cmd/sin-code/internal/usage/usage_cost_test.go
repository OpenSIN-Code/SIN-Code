// SPDX-License-Identifier: MIT
// Purpose: tests for the package-level ComputeCost helper.
package usage

import "testing"

func TestComputeCost(t *testing.T) {
	cases := []struct {
		model  string
		tokens int
		want   float64
	}{
		{"gpt-4o", 1000, 0.0050},
		{"meta/llama-3.3-70b-instruct", 1000, 0.0009},
		{"my-custom-llama-3.3-70b-thing", 1000, 0.0009},
		{"", 1000, 0},
		{"gpt-4o", 0, 0},
		{"gpt-4o", -5, 0},
		{"totally-unknown-model", 1000, 0},
	}
	for _, c := range cases {
		got := ComputeCost(c.model, c.tokens)
		if absFloat(got-c.want) > 1e-9 {
			t.Errorf("ComputeCost(%q, %d) = %v, want %v", c.model, c.tokens, got, c.want)
		}
	}
}

func TestComputeCostLongestMatchWins(t *testing.T) {
	// "gpt-4o-mini" matches both "gpt-4o-mini" (0.0002) and "gpt-4o" (0.0050).
	// The longest match must win so the price is deterministic.
	got := ComputeCost("gpt-4o-mini", 1000)
	want := 0.0002
	if absFloat(got-want) > 1e-9 {
		t.Errorf("ComputeCost(gpt-4o-mini) = %v, want %v", got, want)
	}
}
