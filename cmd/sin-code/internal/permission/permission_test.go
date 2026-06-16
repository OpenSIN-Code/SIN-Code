// SPDX-License-Identifier: MIT
// Purpose: tests for issue #193 — session-wide permission modes.
package permission

import "testing"

func TestEngine_ModeDefault_NoOp(t *testing.T) {
	e := New([]Rule{{Tool: "Bash", Policy: "allow"}})
	if e.Check("Bash") != Allow {
		t.Error("default mode should not change rule-based Allow")
	}
	if e.Check("unknown") != Ask {
		t.Error("default mode should keep Ask for unknown tools")
	}
}

func TestEngine_ModePlan_ForcesAskOnMutating(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Policy: "allow"},
		{Tool: "Edit", Policy: "allow"},
		{Tool: "Read", Policy: "allow"},
		{Tool: "ls", Policy: "allow"},
	}
	e := New(rules)
	if err := e.SetMode(ModePlan); err != nil {
		t.Fatal(err)
	}
	if e.Check("Bash") != Ask {
		t.Errorf("plan mode: Bash should be Ask, got %s", e.Check("Bash"))
	}
	if e.Check("Edit") != Ask {
		t.Errorf("plan mode: Edit should be Ask, got %s", e.Check("Edit"))
	}
	if e.Check("Read") != Allow {
		t.Errorf("plan mode: Read should be Allow, got %s", e.Check("Read"))
	}
	if e.Check("ls") != Allow {
		t.Errorf("plan mode: ls should be Allow, got %s", e.Check("ls"))
	}
}

func TestEngine_ModeAcceptEdits(t *testing.T) {
	rules := []Rule{
		{Tool: "Edit", Policy: "ask"},
		{Tool: "Bash", Policy: "ask"},
		{Tool: "Read", Policy: "allow"},
	}
	e := New(rules)
	if err := e.SetMode(ModeAcceptEdits); err != nil {
		t.Fatal(err)
	}
	if e.Check("Edit") != Allow {
		t.Errorf("acceptEdits: Edit should be Allow, got %s", e.Check("Edit"))
	}
	if e.Check("Write") != Allow {
		t.Errorf("acceptEdits: Write should be Allow, got %s", e.Check("Write"))
	}
	if e.Check("Bash") != Ask {
		t.Errorf("acceptEdits: Bash should stay Ask, got %s", e.Check("Bash"))
	}
	if e.Check("Read") != Allow {
		t.Errorf("acceptEdits: Read should be Allow, got %s", e.Check("Read"))
	}
}

func TestEngine_ModeBypass(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Policy: "ask"},
		{Tool: "Edit", Policy: "ask"},
		{Tool: "danger", Policy: "deny"},
	}
	e := New(rules)
	if err := e.SetMode(ModeBypass); err != nil {
		t.Fatal(err)
	}
	if e.Check("Bash") != Allow {
		t.Errorf("bypass: Bash should be Allow, got %s", e.Check("Bash"))
	}
	if e.Check("Edit") != Allow {
		t.Errorf("bypass: Edit should be Allow, got %s", e.Check("Edit"))
	}
	if e.Check("danger") != Deny {
		t.Errorf("bypass: Deny must NEVER be overridden, got %s", e.Check("danger"))
	}
}

func TestEngine_SetMode_Invalid(t *testing.T) {
	e := New(nil)
	if err := e.SetMode(Mode("garbage")); err == nil {
		t.Error("expected error for unknown mode")
	}
}

func TestEngine_ModeHeadlessAskToDeny(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Policy: "ask"}}
	e := New(rules)
	e.Headless = true
	if err := e.SetMode(ModeDefault); err != nil {
		t.Fatal(err)
	}
	if e.Check("Bash") != Deny {
		t.Errorf("headless: Ask should resolve to Deny, got %s", e.Check("Bash"))
	}
}

func TestEngine_ModeYoloBypassesAsk(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Policy: "ask"}}
	e := New(rules)
	e.Yolo = true
	if err := e.SetMode(ModeDefault); err != nil {
		t.Fatal(err)
	}
	if e.Check("Bash") != Allow {
		t.Errorf("yolo: Ask should resolve to Allow, got %s", e.Check("Bash"))
	}
}

func TestPolicyString(t *testing.T) {
	cases := []struct {
		p    Policy
		want string
	}{
		{Allow, "allow"},
		{Ask, "ask"},
		{Deny, "deny"},
		{Policy(99), "deny"},
	}
	for _, c := range cases {
		if got := c.p.String(); got != c.want {
			t.Errorf("%v.String() = %q, want %q", c.p, got, c.want)
		}
	}
}
