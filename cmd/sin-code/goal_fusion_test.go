// SPDX-License-Identifier: MIT
// Purpose: tests for parseGoalID (issue #140 fusion helper).
package main

import "testing"

func TestParseGoalID(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		err  bool
	}{
		{"42", 42, false},
		{"#42", 42, false},
		{"  42  ", 42, false},
		{"  #42  ", 42, false},
		{"0", 0, false},
		{"9999999999", 9999999999, false},
		{"abc", 0, true},
		{"", 0, true},
		{"#abc", 0, true},
		{"-1", -1, false}, // negative is allowed (id is int64)
		{"3.14", 0, true},
	}
	for _, c := range cases {
		got, err := parseGoalID(c.in)
		if c.err {
			if err == nil {
				t.Errorf("parseGoalID(%q): expected error, got %d", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseGoalID(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseGoalID(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
