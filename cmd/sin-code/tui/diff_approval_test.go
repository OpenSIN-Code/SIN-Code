// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"testing"
)

const testDiffApproval = `--- a/main.go
+++ b/main.go
@@ -42,7 +42,21 @@
-func oldCode() {
+func newCode() {
+    // improved implementation
+    return nil
 }
`

func TestDiffApproval_Render(t *testing.T) {
	d := NewDiffApproval(testStyles())
	d.Width = 60
	d.Height = 15
	d.Show("cmd/sin-code/main.go", testDiffApproval)

	out := d.Render()
	if out == "" {
		t.Fatal("expected non-empty render output")
	}

	plain := stripANSI(out)

	if !strings.Contains(plain, "File Change") {
		t.Errorf("expected 'File Change' header, got:\n%s", plain)
	}
	if !strings.Contains(plain, "main.go") {
		t.Errorf("expected file path in output, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Approve") {
		t.Errorf("expected Approve button, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Reject") {
		t.Errorf("expected Reject button, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Edit") {
		t.Errorf("expected Edit button, got:\n%s", plain)
	}
}

func TestDiffApproval_RenderClosed(t *testing.T) {
	d := NewDiffApproval(testStyles())
	out := d.Render()
	if out != "" {
		t.Errorf("expected empty render when closed, got %q", out)
	}
}

func TestDiffApproval_Navigation(t *testing.T) {
	d := NewDiffApproval(testStyles())
	d.Show("file.go", testDiffApproval)

	if d.Selected != 0 {
		t.Fatalf("expected initial selection 0 (approve), got %d", d.Selected)
	}

	d.Next()
	if d.Selected != 1 {
		t.Errorf("after Next, expected 1 (reject), got %d", d.Selected)
	}

	d.Next()
	if d.Selected != 2 {
		t.Errorf("after 2x Next, expected 2 (edit), got %d", d.Selected)
	}

	d.Next()
	if d.Selected != 0 {
		t.Errorf("after 3x Next (wrap), expected 0 (approve), got %d", d.Selected)
	}

	d.Prev()
	if d.Selected != 2 {
		t.Errorf("after Prev from 0 (wrap), expected 2 (edit), got %d", d.Selected)
	}

	d.Prev()
	if d.Selected != 1 {
		t.Errorf("after Prev from 2, expected 1 (reject), got %d", d.Selected)
	}
}

func TestDiffApproval_Choice(t *testing.T) {
	d := NewDiffApproval(testStyles())
	d.Show("file.go", testDiffApproval)

	cases := []struct {
		selected int
		want     string
	}{
		{0, "approve"},
		{1, "reject"},
		{2, "edit"},
	}
	for _, tc := range cases {
		d.Selected = tc.selected
		if got := d.Choice(); got != tc.want {
			t.Errorf("Choice() with Selected=%d = %q, want %q", tc.selected, got, tc.want)
		}
	}
}

func TestDiffApproval_ShowCountsAdditionsDeletions(t *testing.T) {
	d := NewDiffApproval(testStyles())
	diff := `--- a/f.go
+++ b/f.go
@@ -1,3 +1,5 @@
 line1
-old1
-old2
+new1
+new2
+new3
 line3
`
	d.Show("f.go", diff)

	if d.Additions != 3 {
		t.Errorf("expected 3 additions, got %d", d.Additions)
	}
	if d.Deletions != 2 {
		t.Errorf("expected 2 deletions, got %d", d.Deletions)
	}
	if !d.Open {
		t.Error("expected Open=true after Show")
	}
}

func TestDiffApproval_Close(t *testing.T) {
	d := NewDiffApproval(testStyles())
	d.Show("f.go", testDiffApproval)
	d.Selected = 1

	d.Close()

	if d.Open {
		t.Error("expected Open=false after Close")
	}
	if d.FilePath != "" {
		t.Errorf("expected empty FilePath after Close, got %q", d.FilePath)
	}
	if d.Diff != "" {
		t.Error("expected empty Diff after Close")
	}
	if d.Selected != 0 {
		t.Errorf("expected Selected=0 after Close, got %d", d.Selected)
	}
}

func TestFooter_VerifyGateOff(t *testing.T) {
	f := NewFooter(80)
	f.SetVerifyGate(VerifyGateOff, "off")
	f.GitBranch = "main"

	out := f.renderChatFooter(testStyles())
	plain := stripANSI(out)

	if !strings.Contains(plain, "verify: off") {
		t.Errorf("expected 'verify: off' in footer, got:\n%s", plain)
	}
}

func TestFooter_VerifyGatePassed(t *testing.T) {
	f := NewFooter(80)
	f.SetVerifyGate(VerifyGatePassed, "poc")
	f.GitBranch = "main"

	out := f.renderChatFooter(testStyles())
	plain := stripANSI(out)

	if !strings.Contains(plain, "✓ verified") {
		t.Errorf("expected '✓ verified' in footer, got:\n%s", plain)
	}
}

func TestFooter_VerifyGateFailed(t *testing.T) {
	f := NewFooter(80)
	f.SetVerifyGate(VerifyGateFailed, "oracle")
	f.GitBranch = "main"

	out := f.renderChatFooter(testStyles())
	plain := stripANSI(out)

	if !strings.Contains(plain, "✗ failed") {
		t.Errorf("expected '✗ failed' in footer, got:\n%s", plain)
	}
}

func TestFooter_VerifyGateRunning(t *testing.T) {
	f := NewFooter(80)
	f.SetVerifyGate(VerifyGateRunning, "poc")
	f.GitBranch = "main"

	out := f.renderChatFooter(testStyles())
	plain := stripANSI(out)

	if !strings.Contains(plain, "verify: running") {
		t.Errorf("expected 'verify: running' in footer, got:\n%s", plain)
	}
}

func TestFooter_VerifyGateIdle(t *testing.T) {
	f := NewFooter(80)
	f.SetVerifyGate(VerifyGateIdle, "poc")

	out := f.renderChatFooter(testStyles())
	plain := stripANSI(out)

	if !strings.Contains(plain, "verify: ready") {
		t.Errorf("expected 'verify: ready' in footer, got:\n%s", plain)
	}
}

func TestFooter_VerifyGateIndicatorBetweenBranchAndTokens(t *testing.T) {
	f := NewFooter(80)
	f.SetVerifyGate(VerifyGatePassed, "poc")
	f.GitBranch = "main"
	f.Tokens = 2300

	out := f.renderChatFooter(testStyles())
	plain := stripANSI(out)

	branchIdx := strings.Index(plain, "main")
	verifyIdx := strings.Index(plain, "verified")
	tokenIdx := strings.Index(plain, "2300 tokens")

	if branchIdx < 0 {
		t.Fatal("expected git branch 'main' in footer")
	}
	if verifyIdx < 0 {
		t.Fatal("expected verify indicator in footer")
	}
	if tokenIdx < 0 {
		t.Fatal("expected tokens in footer")
	}

	if !(branchIdx < verifyIdx && verifyIdx < tokenIdx) {
		t.Errorf("expected order: branch < verify < tokens, got branch=%d verify=%d tokens=%d",
			branchIdx, verifyIdx, tokenIdx)
	}
}

func TestFooter_SetVerifyGate(t *testing.T) {
	f := NewFooter(80)

	f.SetVerifyGate(VerifyGateRunning, "oracle")
	if f.VerifyGate != VerifyGateRunning {
		t.Errorf("expected VerifyGate=VerifyGateRunning, got %d", f.VerifyGate)
	}
	if f.VerifyMode != "oracle" {
		t.Errorf("expected VerifyMode=oracle, got %q", f.VerifyMode)
	}
}

func TestVerifyGateStatusString(t *testing.T) {
	cases := map[VerifyGateStatus]string{
		VerifyGateOff:     "off",
		VerifyGateIdle:    "ready",
		VerifyGateRunning: "running",
		VerifyGatePassed:  "verified",
		VerifyGateFailed:  "failed",
	}
	for status, want := range cases {
		if got := status.String(); got != want {
			t.Errorf("VerifyGateStatus(%d).String() = %q, want %q", status, got, want)
		}
	}
}
