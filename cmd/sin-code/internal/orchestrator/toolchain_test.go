// SPDX-License-Identifier: MIT
// Purpose: tests for the intent tool-chain registry.
package orchestrator

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestToolChainForIntentSecurity(t *testing.T) {
	chain := ToolChainForIntent(IntentSecurity)
	want := []string{ToolSecurityScan, ToolSBOMGenerate, ToolOracle}
	if !slices.Equal(chain.Required, want) {
		t.Errorf("security required tools: got %v, want %v", chain.Required, want)
	}
}

func TestToolChainForIntentReview(t *testing.T) {
	chain := ToolChainForIntent(IntentReview)
	want := []string{ToolADW, ToolOracle, ToolPoC}
	if !slices.Equal(chain.Required, want) {
		t.Errorf("review required tools: got %v, want %v", chain.Required, want)
	}
}

func TestToolChainForIntentArchitecture(t *testing.T) {
	chain := ToolChainForIntent(IntentArchitecture)
	want := []string{ToolMap, ToolSCKG, ToolOracle}
	if !slices.Equal(chain.Required, want) {
		t.Errorf("architecture required tools: got %v, want %v", chain.Required, want)
	}
}

func TestToolChainForIntentTest(t *testing.T) {
	chain := ToolChainForIntent(IntentTest)
	want := []string{ToolTest, ToolOracle}
	if !slices.Equal(chain.Required, want) {
		t.Errorf("test required tools: got %v, want %v", chain.Required, want)
	}
}

func TestToolChainForIntentCodebase(t *testing.T) {
	chain := ToolChainForIntent(IntentCodebase)
	want := []string{ToolScout, ToolMap}
	if !slices.Equal(chain.Required, want) {
		t.Errorf("codebase required tools: got %v, want %v", chain.Required, want)
	}
}

func TestToolChainForIntentDocs(t *testing.T) {
	chain := ToolChainForIntent(IntentDocs)
	want := []string{ToolHarvest, ToolRead}
	if !slices.Equal(chain.Required, want) {
		t.Errorf("docs required tools: got %v, want %v", chain.Required, want)
	}
}

func TestToolChainForIntentGeneralEmpty(t *testing.T) {
	chain := ToolChainForIntent(IntentGeneral)
	if len(chain.Required) != 0 || len(chain.Optional) != 0 || len(chain.Forbidden) != 0 {
		t.Errorf("general intent should have empty chain, got %+v", chain)
	}
}

func TestToolChainForIntentUnknownEmpty(t *testing.T) {
	chain := ToolChainForIntent(Intent("unknown_intent"))
	if len(chain.Required) != 0 || len(chain.Optional) != 0 || len(chain.Forbidden) != 0 {
		t.Errorf("unknown intent should have empty chain, got %+v", chain)
	}
}

func TestToolChainCopyIsIndependent(t *testing.T) {
	chain := ToolChainForIntent(IntentCodebase)
	chain.Required[0] = "mutated"
	fresh := ToolChainForIntent(IntentCodebase)
	if fresh.Required[0] == "mutated" {
		t.Error("ToolChainForIntent must return an independent copy")
	}
}

func TestRequiredToolsForIntent(t *testing.T) {
	got := RequiredToolsForIntent(IntentSecurity)
	want := []string{ToolSecurityScan, ToolSBOMGenerate, ToolOracle}
	if !slices.Equal(got, want) {
		t.Errorf("RequiredToolsForIntent: got %v, want %v", got, want)
	}
}

func TestPlanToolChainJSON(t *testing.T) {
	p := NewPlanner(DefaultAgents())
	plan := p.BuildPlan("Audit for XSS and SQL injection vulnerabilities")
	if plan.ToolChain == nil {
		t.Fatal("expected plan to have a tool chain")
	}
	if !slices.Equal(plan.ToolChain.Required, []string{ToolSecurityScan, ToolSBOMGenerate, ToolOracle}) {
		t.Errorf("security plan tool chain: got %v", plan.ToolChain.Required)
	}

	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["tool_chain"] == nil {
		t.Error("tool_chain should be present in JSON output")
	}
}

func TestPlannerToolChainPerIntent(t *testing.T) {
	cases := []struct {
		prompt string
		intent Intent
		want   []string
	}{
		{"Add user authentication", IntentCodebase, []string{ToolScout, ToolMap}},
		{"Write unit tests for auth", IntentTest, []string{ToolTest, ToolOracle}},
		{"Review the pull request", IntentReview, []string{ToolADW, ToolOracle, ToolPoC}},
		{"Add documentation to the README", IntentDocs, []string{ToolHarvest, ToolRead}},
		{"Audit for XSS and SQL injection vulnerabilities", IntentSecurity, []string{ToolSecurityScan, ToolSBOMGenerate, ToolOracle}},
		{"Design the system architecture and data model", IntentArchitecture, []string{ToolMap, ToolSCKG, ToolOracle}},
	}
	for _, c := range cases {
		c := c
		t.Run(string(c.intent), func(t *testing.T) {
			p := NewPlanner(DefaultAgents())
			plan := p.BuildPlan(c.prompt)
			if plan.Intent != c.intent {
				t.Errorf("intent: got %s, want %s", plan.Intent, c.intent)
			}
			if plan.ToolChain == nil {
				t.Fatal("expected non-nil tool chain")
			}
			if !slices.Equal(plan.ToolChain.Required, c.want) {
				t.Errorf("required tools: got %v, want %v", plan.ToolChain.Required, c.want)
			}
		})
	}
}

func TestPlannerToolChainDeterministic(t *testing.T) {
	p := NewPlanner(DefaultAgents())
	var first *Plan
	for i := 0; i < 20; i++ {
		plan := p.BuildPlan("Audit for XSS and SQL injection vulnerabilities")
		if plan.ToolChain == nil {
			t.Fatal("nil tool chain")
		}
		if first == nil {
			first = plan
			continue
		}
		if !slices.Equal(first.ToolChain.Required, plan.ToolChain.Required) {
			t.Errorf("tool chain changed across calls: %v vs %v", first.ToolChain.Required, plan.ToolChain.Required)
		}
	}
}
