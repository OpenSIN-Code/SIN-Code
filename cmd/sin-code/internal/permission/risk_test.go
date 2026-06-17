// SPDX-License-Identifier: MIT
// Purpose: tests for the YOLO risk classifier (issue #272, M7).
package permission

import (
	"sync"
	"testing"
)

func TestClassify_LowRiskTools(t *testing.T) {
	c := NewRiskClassifier()
	tools := []string{"read", "sin_read", "scout", "sin_scout", "discover",
		"sin_discover", "map", "grasp", "harvest", "sckg", "oracle", "efm"}
	for _, tool := range tools {
		if got := c.Classify(tool, nil); got != RiskLow {
			t.Errorf("Classify(%q) = %s, want low", tool, got)
		}
	}
}

func TestClassify_MediumRiskTools(t *testing.T) {
	c := NewRiskClassifier()
	tools := []string{"edit", "sin_edit", "write", "sin_write", "sin_test", "test"}
	for _, tool := range tools {
		if got := c.Classify(tool, nil); got != RiskMedium {
			t.Errorf("Classify(%q) = %s, want medium", tool, got)
		}
	}
}

func TestClassify_HighRiskTools(t *testing.T) {
	c := NewRiskClassifier()
	tools := []string{"execute", "bash", "sin_bash", "sin_git_commit",
		"sin_test_generate", "shell"}
	for _, tool := range tools {
		if got := c.Classify(tool, nil); got != RiskHigh {
			t.Errorf("Classify(%q) = %s, want high", tool, got)
		}
	}
}

func TestClassify_CriticalRiskTools(t *testing.T) {
	c := NewRiskClassifier()
	tools := []string{"sin_browser_navigate", "rm", "delete", "remove",
		"sin_git_push", "git_push"}
	for _, tool := range tools {
		if got := c.Classify(tool, nil); got != RiskCritical {
			t.Errorf("Classify(%q) = %s, want critical", tool, got)
		}
	}
}

func TestClassify_ForceAndResetElevateToCritical(t *testing.T) {
	c := NewRiskClassifier()
	if got := c.Classify("sin_bash_force", nil); got != RiskCritical {
		t.Errorf("tool with 'force' should be critical, got %s", got)
	}
	if got := c.Classify("git_reset_hard", nil); got != RiskCritical {
		t.Errorf("tool with 'reset' should be critical, got %s", got)
	}
}

func TestClassify_UnknownToolDefaultsToMedium(t *testing.T) {
	c := NewRiskClassifier()
	if got := c.Classify("some_unknown_tool", nil); got != RiskMedium {
		t.Errorf("unknown tool should default to medium, got %s", got)
	}
	if got := c.Classify("", nil); got != RiskMedium {
		t.Errorf("empty tool name should default to medium, got %s", got)
	}
}

func TestClassify_DangerousArgsElevateToCritical(t *testing.T) {
	c := NewRiskClassifier()
	args := map[string]any{"command": "rm -rf /tmp/important"}
	if got := c.Classify("sin_bash", args); got != RiskCritical {
		t.Errorf("sin_bash with 'rm -rf' args should be critical, got %s", got)
	}
	args2 := map[string]any{"cmd": "git push --force origin main"}
	if got := c.Classify("sin_bash", args2); got != RiskCritical {
		t.Errorf("sin_bash with 'git push --force' should be critical, got %s", got)
	}
	args3 := map[string]any{"script": "sudo chmod 777 /etc/passwd"}
	if got := c.Classify("bash", args3); got != RiskCritical {
		t.Errorf("bash with sudo should be critical, got %s", got)
	}
}

func TestClassify_SafeArgsDoNotElevate(t *testing.T) {
	c := NewRiskClassifier()
	args := map[string]any{"command": "ls -la /tmp"}
	if got := c.Classify("sin_bash", args); got != RiskHigh {
		t.Errorf("sin_bash with safe args should stay high, got %s", got)
	}
}

func TestRiskLevel_String(t *testing.T) {
	cases := []struct {
		level RiskLevel
		want  string
	}{
		{RiskLow, "low"},
		{RiskMedium, "medium"},
		{RiskHigh, "high"},
		{RiskCritical, "critical"},
		{RiskLevel(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.level.String(); got != c.want {
			t.Errorf("%d.String() = %q, want %q", c.level, got, c.want)
		}
	}
}

func TestParseRiskLevel(t *testing.T) {
	cases := []struct {
		input string
		want  RiskLevel
		err   bool
	}{
		{"low", RiskLow, false},
		{"medium", RiskMedium, false},
		{"high", RiskHigh, false},
		{"critical", RiskCritical, false},
		{"", RiskMedium, false},
		{"LOW", RiskLow, false},
		{"garbage", RiskMedium, true},
	}
	for _, c := range cases {
		got, err := ParseRiskLevel(c.input)
		if c.err && err == nil {
			t.Errorf("ParseRiskLevel(%q): expected error, got nil", c.input)
		}
		if !c.err && err != nil {
			t.Errorf("ParseRiskLevel(%q): unexpected error: %v", c.input, err)
		}
		if got != c.want {
			t.Errorf("ParseRiskLevel(%q) = %s, want %s", c.input, got, c.want)
		}
	}
}

func TestRiskClassifier_ThresholdFiltering(t *testing.T) {
	c := NewRiskClassifier()
	c.SetThreshold(RiskLow)
	if c.Threshold() != RiskLow {
		t.Fatal("threshold not set to low")
	}
	if c.Classify("sin_bash", nil) <= c.Threshold() {
		t.Error("high-risk sin_bash should NOT pass low threshold")
	}
	if !(c.Classify("read", nil) <= c.Threshold()) {
		t.Error("low-risk read should pass low threshold")
	}

	c.SetThreshold(RiskCritical)
	if !(c.Classify("rm", nil) <= c.Threshold()) {
		t.Error("critical-risk rm should pass critical threshold")
	}
	if !(c.Classify("sin_bash", nil) <= c.Threshold()) {
		t.Error("high-risk sin_bash should pass critical threshold")
	}
}

func TestEngine_YoloWithRisk_LowThreshold(t *testing.T) {
	rules := []Rule{
		{Tool: "sin_bash", Policy: "ask"},
		{Tool: "sin_read", Policy: "ask"},
		{Tool: "danger", Policy: "deny"},
	}
	e := New(rules)
	e.Yolo = true
	e.Risk = NewRiskClassifier()
	e.Risk.SetThreshold(RiskLow)

	if e.Check("sin_read") != Allow {
		t.Error("low-risk sin_read should be auto-approved under yolo+low-threshold")
	}
	if e.Check("sin_bash") != Ask {
		t.Error("high-risk sin_bash should stay Ask (not auto-approved) under low threshold")
	}
	if e.Check("danger") != Deny {
		t.Error("deny rule must NEVER be overridden by yolo+risk")
	}
}

func TestEngine_YoloWithRisk_MediumThreshold(t *testing.T) {
	rules := []Rule{
		{Tool: "sin_bash", Policy: "ask"},
		{Tool: "sin_edit", Policy: "ask"},
		{Tool: "rm", Policy: "ask"},
	}
	e := New(rules)
	e.Yolo = true
	e.Risk = NewRiskClassifier()
	e.Risk.SetThreshold(RiskMedium)

	if e.Check("sin_edit") != Allow {
		t.Error("medium-risk sin_edit should be auto-approved under medium threshold")
	}
	if e.Check("sin_bash") != Ask {
		t.Error("high-risk sin_bash should stay Ask under medium threshold")
	}
	if e.Check("rm") != Ask {
		t.Error("critical-risk rm should stay Ask under medium threshold")
	}
}

func TestEngine_YoloWithRisk_HeadlessAboveThreshold_Denies(t *testing.T) {
	rules := []Rule{{Tool: "sin_bash", Policy: "ask"}}
	e := New(rules)
	e.Yolo = true
	e.Headless = true
	e.Risk = NewRiskClassifier()
	e.Risk.SetThreshold(RiskLow)

	if e.Check("sin_bash") != Deny {
		t.Error("headless + yolo + risk above threshold should deny, not ask")
	}
	if e.Check("sin_read") != Allow {
		t.Error("headless + yolo + risk at/below threshold should allow")
	}
}

func TestEngine_YoloWithRisk_HighThreshold(t *testing.T) {
	rules := []Rule{
		{Tool: "sin_bash", Policy: "ask"},
		{Tool: "rm", Policy: "ask"},
	}
	e := New(rules)
	e.Yolo = true
	e.Risk = NewRiskClassifier()
	e.Risk.SetThreshold(RiskHigh)

	if e.Check("sin_bash") != Allow {
		t.Error("high-risk sin_bash should be auto-approved under high threshold")
	}
	if e.Check("rm") != Ask {
		t.Error("critical-risk rm should stay Ask under high threshold")
	}
}

func TestEngine_YoloWithoutRisk_BlanketAllow(t *testing.T) {
	rules := []Rule{{Tool: "sin_bash", Policy: "ask"}}
	e := New(rules)
	e.Yolo = true
	if e.Check("sin_bash") != Allow {
		t.Error("yolo without risk classifier should blanket-approve (legacy behavior)")
	}
}

func TestEngine_CheckWithArgs_DangerousElevates(t *testing.T) {
	rules := []Rule{{Tool: "sin_bash", Policy: "ask"}}
	e := New(rules)
	e.Yolo = true
	e.Risk = NewRiskClassifier()
	e.Risk.SetThreshold(RiskHigh)

	safeArgs := map[string]any{"command": "ls -la"}
	if e.CheckWithArgs("sin_bash", safeArgs) != Allow {
		t.Error("sin_bash with safe args should be allowed under high threshold")
	}

	dangerousArgs := map[string]any{"command": "rm -rf /"}
	if e.CheckWithArgs("sin_bash", dangerousArgs) != Ask {
		t.Error("sin_bash with rm -rf args should be critical → Ask (above high threshold)")
	}
}

func TestEngine_CheckWithArgs_HeadlessDangerous_Denies(t *testing.T) {
	rules := []Rule{{Tool: "sin_bash", Policy: "ask"}}
	e := New(rules)
	e.Yolo = true
	e.Headless = true
	e.Risk = NewRiskClassifier()
	e.Risk.SetThreshold(RiskHigh)

	dangerousArgs := map[string]any{"command": "git reset --hard origin/main"}
	if e.CheckWithArgs("sin_bash", dangerousArgs) != Deny {
		t.Error("headless + dangerous args above threshold should deny")
	}
}

func TestRiskClassifier_RaceSafe(t *testing.T) {
	c := NewRiskClassifier()
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			switch n % 4 {
			case 0:
				_ = c.Classify("sin_bash", map[string]any{"command": "ls"})
			case 1:
				_ = c.Classify("read", nil)
			case 2:
				c.SetThreshold(RiskLevel(n % 4))
			case 3:
				_ = c.Threshold()
			}
		}(i)
	}
	wg.Wait()
}

func TestRiskClassifier_NilSafe(t *testing.T) {
	var c *RiskClassifier
	if got := c.Classify("sin_bash", nil); got != RiskMedium {
		t.Errorf("nil classifier should return medium, got %s", got)
	}
	if got := c.Threshold(); got != RiskMedium {
		t.Errorf("nil classifier threshold should be medium, got %s", got)
	}
	c.SetThreshold(RiskHigh)
}
