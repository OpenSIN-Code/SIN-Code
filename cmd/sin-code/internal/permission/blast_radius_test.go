// SPDX-License-Identifier: MIT
// Purpose: unit tests for blast-radius estimation (issue #322, M7).
package permission

import (
	"sync"
	"testing"
)

func TestBlastRadius_ReadOnly_None(t *testing.T) {
	b := NewBlastRadius()
	tools := []string{"sin_read", "sin_scout", "sin_discover", "sin_map",
		"sin_grasp", "sin_harvest", "read", "scout", "glob", "grep"}
	for _, tool := range tools {
		if got := b.Estimate(tool, nil); got != RadiusNone {
			t.Errorf("Estimate(%q) = %s, want none", tool, got)
		}
	}
}

func TestBlastRadius_EditWrite_File(t *testing.T) {
	b := NewBlastRadius()
	tools := []string{"sin_edit", "sin_write", "edit", "write", "multi_edit"}
	for _, tool := range tools {
		if got := b.Estimate(tool, nil); got != RadiusFile {
			t.Errorf("Estimate(%q) = %s, want file", tool, got)
		}
	}
}

func TestBlastRadius_Bash_GoTest_Package(t *testing.T) {
	b := NewBlastRadius()
	args := map[string]any{"command": "go test ./..."}
	if got := b.Estimate("sin_bash", args); got != RadiusPackage {
		t.Errorf("sin_bash with 'go test' = %s, want package", got)
	}
}

func TestBlastRadius_Bash_GoBuild_Module(t *testing.T) {
	b := NewBlastRadius()
	args := map[string]any{"command": "go build ./..."}
	if got := b.Estimate("sin_bash", args); got != RadiusModule {
		t.Errorf("sin_bash with 'go build' = %s, want module", got)
	}
}

func TestBlastRadius_Bash_RmRf_System(t *testing.T) {
	b := NewBlastRadius()
	args := map[string]any{"command": "rm -rf /tmp/important"}
	if got := b.Estimate("sin_bash", args); got != RadiusSystem {
		t.Errorf("sin_bash with 'rm -rf' = %s, want system", got)
	}
}

func TestBlastRadius_GitCommit_Module(t *testing.T) {
	b := NewBlastRadius()
	if got := b.Estimate("sin_git_commit", nil); got != RadiusModule {
		t.Errorf("sin_git_commit = %s, want module", got)
	}
}

func TestBlastRadius_GitPush_System(t *testing.T) {
	b := NewBlastRadius()
	if got := b.Estimate("sin_git_push", nil); got != RadiusSystem {
		t.Errorf("sin_git_push = %s, want system", got)
	}
}

func TestBlastRadius_Score(t *testing.T) {
	b := NewBlastRadius()
	cases := []struct {
		level RadiusLevel
		want  float64
	}{
		{RadiusNone, 0.0},
		{RadiusFile, 0.2},
		{RadiusPackage, 0.4},
		{RadiusModule, 0.7},
		{RadiusSystem, 0.95},
	}
	for _, c := range cases {
		if got := b.Score(c.level); got != c.want {
			t.Errorf("Score(%s) = %.2f, want %.2f", c.level, got, c.want)
		}
	}
}

func TestBlastRadius_Description(t *testing.T) {
	b := NewBlastRadius()
	for level := RadiusNone; level <= RadiusSystem; level++ {
		desc := b.Description(level)
		if desc == "" {
			t.Errorf("Description(%s) should not be empty", level)
		}
	}
	if desc := b.Description(RadiusNone); desc != "read-only, no side effects" {
		t.Errorf("Description(none) = %q, want 'read-only, no side effects'", desc)
	}
}

func TestBlastRadius_ToRiskLevel(t *testing.T) {
	b := NewBlastRadius()
	cases := []struct {
		radius RadiusLevel
		want   RiskLevel
	}{
		{RadiusNone, RiskLow},
		{RadiusFile, RiskMedium},
		{RadiusPackage, RiskMedium},
		{RadiusModule, RiskHigh},
		{RadiusSystem, RiskCritical},
	}
	for _, c := range cases {
		if got := b.ToRiskLevel(c.radius); got != c.want {
			t.Errorf("ToRiskLevel(%s) = %s, want %s", c.radius, got, c.want)
		}
	}
}

func TestBlastRadius_ClassifyWithBlast_Elevates(t *testing.T) {
	rc := NewRiskClassifier()
	br := NewBlastRadius()
	got := ClassifyWithBlast(rc, br, "sin_bash", map[string]any{"command": "rm -rf /"})
	if got != RiskCritical {
		t.Errorf("ClassifyWithBlast(rm -rf) = %s, want critical", got)
	}
	got = ClassifyWithBlast(rc, br, "sin_read", nil)
	if got != RiskLow {
		t.Errorf("ClassifyWithBlast(sin_read) = %s, want low", got)
	}
}

func TestBlastRadius_ScoreWithBlast(t *testing.T) {
	rc := NewRiskClassifier()
	br := NewBlastRadius()
	score := ScoreWithBlast(rc, br, "sin_bash", map[string]any{"command": "rm -rf /"})
	if score < 0.9 {
		t.Errorf("ScoreWithBlast(rm -rf) = %.2f, want >= 0.9", score)
	}
	score = ScoreWithBlast(rc, br, "sin_read", nil)
	if score > 0.2 {
		t.Errorf("ScoreWithBlast(sin_read) = %.2f, want <= 0.2", score)
	}
}

func TestBlastRadius_RaceSafe(t *testing.T) {
	b := NewBlastRadius()
	rc := NewRiskClassifier()
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			args := map[string]any{"command": "go test"}
			if n%5 == 0 {
				args["command"] = "rm -rf /tmp"
			}
			_ = b.Estimate("sin_bash", args)
			_ = b.Score(b.Estimate("sin_edit", nil))
			_ = b.Description(b.Estimate("sin_read", nil))
			_ = ClassifyWithBlast(rc, b, "sin_bash", args)
			_ = ScoreWithBlast(rc, b, "sin_bash", args)
		}(i)
	}
	wg.Wait()
}
