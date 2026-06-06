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

func TestFilterRulesByLanguage(t *testing.T) {
	all := AllRules()
	filtered := FilterRulesByLanguage(all, []string{"python"})
	if len(filtered) == 0 {
		t.Fatal("expected filtered rules to be non-empty")
	}
	for _, r := range filtered {
		found := false
		for _, l := range r.Languages {
			if l == "python" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("rule %s does not support python", r.ID)
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
		if r.CWE == "" {
			t.Fatalf("rule %s has empty CWE", r.ID)
		}
		if r.OWASP == "" {
			t.Fatalf("rule %s has empty OWASP", r.ID)
		}
		if len(r.Languages) == 0 {
			t.Fatalf("rule %s has no languages", r.ID)
		}
		if len(r.Patterns) == 0 {
			t.Fatalf("rule %s has no patterns", r.ID)
		}
		if r.Remediation == "" {
			t.Fatalf("rule %s has empty remediation", r.ID)
		}
	}
}

func TestRuleCategories(t *testing.T) {
	all := AllRules()
	categories := make(map[string]int)
	for _, r := range all {
		categories[r.Category]++
	}
	expectedCategories := []string{"injection", "secrets", "crypto", "access-control", "deserialization", "ssrf", "misconfiguration", "logging", "concurrency"}
	for _, cat := range expectedCategories {
		if categories[cat] == 0 {
			t.Fatalf("expected category %s to have at least one rule", cat)
		}
	}
}
