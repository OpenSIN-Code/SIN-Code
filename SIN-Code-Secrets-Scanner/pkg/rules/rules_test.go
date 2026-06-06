package rules

import (
	"testing"
)

func TestAllRules(t *testing.T) {
	rules := AllRules()
	if len(rules) == 0 {
		t.Fatal("expected rules to be non-empty")
	}
	if len(rules) < 20 {
		t.Fatalf("expected at least 20 rules, got %d", len(rules))
	}

	// Check IDs are unique
	ids := make(map[string]bool)
	for _, r := range rules {
		if ids[r.ID] {
			t.Fatalf("duplicate rule ID: %s", r.ID)
		}
		ids[r.ID] = true
	}
}

func TestFilterRulesByType(t *testing.T) {
	all := AllRules()
	filtered := FilterRulesByType(all, []string{"api-key"})
	if len(filtered) == 0 {
		t.Fatal("expected filtered rules to be non-empty")
	}
	for _, r := range filtered {
		if r.SecretType != "api-key" {
			t.Fatalf("rule %s has type %s, expected api-key", r.ID, r.SecretType)
		}
	}
}

func TestFilterRulesBySeverity(t *testing.T) {
	all := AllRules()
	filtered := FilterRulesBySeverity(all, "high")
	if len(filtered) == 0 {
		t.Fatal("expected filtered rules to be non-empty")
	}
	for _, r := range filtered {
		if r.Severity != "critical" && r.Severity != "high" {
			t.Fatalf("rule %s has severity %s, expected >= high", r.ID, r.Severity)
		}
	}
}

func TestRuleStructure(t *testing.T) {
	all := AllRules()
	for _, r := range all {
		if r.ID == "" {
			t.Fatal("rule ID is empty")
		}
		if r.Name == "" {
			t.Fatalf("rule %s has empty name", r.ID)
		}
		if r.Severity == "" {
			t.Fatalf("rule %s has empty severity", r.ID)
		}
		if r.SecretType == "" {
			t.Fatalf("rule %s has empty secret type", r.ID)
		}
		if len(r.Patterns) == 0 {
			t.Fatalf("rule %s has no patterns", r.ID)
		}
		if r.Remediation == "" {
			t.Fatalf("rule %s has empty remediation", r.ID)
		}
	}
}

func TestSecretTypes(t *testing.T) {
	all := AllRules()
	types := make(map[string]int)
	for _, r := range all {
		types[r.SecretType]++
	}
	expectedTypes := []string{"api-key", "token", "password", "private-key", "certificate", "config-file"}
	for _, st := range expectedTypes {
		if types[st] == 0 {
			t.Fatalf("expected secret type %s to have at least one rule", st)
		}
	}
}
