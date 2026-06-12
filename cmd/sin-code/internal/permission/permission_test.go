// SPDX-License-Identifier: MIT
// Purpose: permission engine tests (mandate M4, AGENTS.md §8).
package permission

import "testing"

func TestCheck(t *testing.T) {
	e := New([]Rule{
		{Tool: "sin_read", Policy: "allow"},
		{Tool: "sckg_*", Policy: "allow"},
		{Tool: "sin_bash", Policy: "ask"},
		{Tool: "*", Policy: "ask"},
	})

	cases := []struct {
		tool string
		want Policy
	}{
		{"sin_read", Allow},
		{"sckg_query", Allow},
		{"sin_bash", Ask},
		{"unknown_tool", Ask},
	}
	for _, c := range cases {
		if got := e.Check(c.tool); got != c.want {
			t.Errorf("Check(%q) = %v, want %v", c.tool, got, c.want)
		}
	}
}

func TestHeadlessAskBecomesDeny(t *testing.T) {
	e := New([]Rule{{Tool: "sin_bash", Policy: "ask"}})
	e.Headless = true
	if e.Check("sin_bash") != Deny {
		t.Error("headless: ask must resolve to deny")
	}
}

func TestYoloBypassesAsk(t *testing.T) {
	e := New([]Rule{{Tool: "sin_bash", Policy: "ask"}})
	e.Yolo = true
	if e.Check("sin_bash") != Allow {
		t.Error("yolo: ask must resolve to allow")
	}
}

func TestYoloNeverBypassesDeny(t *testing.T) {
	e := New([]Rule{{Tool: "rm_*", Policy: "deny"}})
	e.Yolo = true
	if e.Check("rm_rf") != Deny {
		t.Error("yolo must never bypass deny")
	}
}

func TestFirstMatchWins(t *testing.T) {
	e := New([]Rule{
		{Tool: "sin_*", Policy: "allow"},
		{Tool: "sin_bash", Policy: "deny"},
	})
	if e.Check("sin_bash") != Allow {
		t.Error("first match (sin_*) should win over later sin_bash deny")
	}
}
