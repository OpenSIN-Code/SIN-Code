// SPDX-License-Identifier: MIT
// Purpose: tests for the session branching tree (BuildSessionTree, Flatten,
// navigation, rendering, and session diff).
package tui

import (
	"strings"
	"testing"
	"time"
)

func sessionData(id, parent, name string, lastActive time.Time, msgs int, preview string) SessionNodeData {
	return SessionNodeData{
		ID:           id,
		Name:         name,
		ParentID:     parent,
		CreatedAt:    lastActive.Add(-time.Hour),
		LastActive:   lastActive,
		MessageCount: msgs,
		Preview:      preview,
	}
}

func TestBuildSessionTreeFlat(t *testing.T) {
	base := time.Now()
	tree := BuildSessionTree([]SessionNodeData{
		sessionData("s1", "", "Session 1", base, 5, "hello"),
		sessionData("s2", "", "Session 2", base.Add(time.Minute), 3, "world"),
	})
	if len(tree.Roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(tree.Roots))
	}
	if tree.Roots[0].ID != "s1" || tree.Roots[1].ID != "s2" {
		t.Errorf("root order mismatch: %s %s", tree.Roots[0].ID, tree.Roots[1].ID)
	}
	for _, r := range tree.Roots {
		if len(r.Children) != 0 {
			t.Errorf("root %s should have no children, got %d", r.ID, len(r.Children))
		}
	}
}

func TestBuildSessionTreeNested(t *testing.T) {
	base := time.Now()
	tree := BuildSessionTree([]SessionNodeData{
		sessionData("s1", "", "Root", base, 5, "parent msg"),
		sessionData("s2", "s1", "Child", base.Add(time.Minute), 3, "child msg"),
		sessionData("s3", "s2", "Grandchild", base.Add(2*time.Minute), 1, "grand msg"),
	})
	if len(tree.Roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree.Roots))
	}
	root := tree.Roots[0]
	if root.ID != "s1" {
		t.Fatalf("expected root s1, got %s", root.ID)
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	child := root.Children[0]
	if child.ID != "s2" {
		t.Errorf("expected child s2, got %s", child.ID)
	}
	if len(child.Children) != 1 {
		t.Fatalf("expected 1 grandchild, got %d", len(child.Children))
	}
	grand := child.Children[0]
	if grand.ID != "s3" {
		t.Errorf("expected grandchild s3, got %s", grand.ID)
	}
	if root.Depth != 0 || child.Depth != 1 || grand.Depth != 2 {
		t.Errorf("depths wrong: root=%d child=%d grand=%d", root.Depth, child.Depth, grand.Depth)
	}
}

func TestBuildSessionTreeOrphan(t *testing.T) {
	base := time.Now()
	tree := BuildSessionTree([]SessionNodeData{
		sessionData("s1", "", "Root", base, 5, "msg"),
		sessionData("s2", "missing-parent", "Orphan", base.Add(time.Minute), 2, "orphan msg"),
	})
	if len(tree.Roots) != 2 {
		t.Fatalf("expected 2 roots (1 root + 1 orphan), got %d", len(tree.Roots))
	}
	ids := map[string]bool{}
	for _, r := range tree.Roots {
		ids[r.ID] = true
	}
	if !ids["s2"] {
		t.Error("orphan s2 should be promoted to root")
	}
}

func TestSessionTreeFlatten(t *testing.T) {
	base := time.Now()
	tree := BuildSessionTree([]SessionNodeData{
		sessionData("a", "", "A", base, 1, ""),
		sessionData("b", "a", "B", base.Add(time.Minute), 1, ""),
		sessionData("c", "a", "C", base.Add(2*time.Minute), 1, ""),
		sessionData("d", "", "D", base.Add(3*time.Minute), 1, ""),
	})
	flat := tree.Flatten()
	if len(flat) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(flat))
	}
	wantOrder := []string{"a", "b", "c", "d"}
	for i, want := range wantOrder {
		if flat[i].ID != want {
			t.Errorf("position %d: expected %s, got %s", i, want, flat[i].ID)
		}
	}
}

func TestSessionTreeMoveUpDown(t *testing.T) {
	base := time.Now()
	tree := BuildSessionTree([]SessionNodeData{
		sessionData("a", "", "A", base, 1, ""),
		sessionData("b", "a", "B", base.Add(time.Minute), 1, ""),
		sessionData("c", "a", "C", base.Add(2*time.Minute), 1, ""),
	})
	if tree.FocusIdx != 0 {
		t.Errorf("expected initial focus 0, got %d", tree.FocusIdx)
	}
	tree.MoveDown()
	if tree.FocusIdx != 1 {
		t.Errorf("expected focus 1 after down, got %d", tree.FocusIdx)
	}
	tree.MoveDown()
	if tree.FocusIdx != 2 {
		t.Errorf("expected focus 2 after second down, got %d", tree.FocusIdx)
	}
	tree.MoveDown()
	if tree.FocusIdx != 2 {
		t.Errorf("expected focus to clamp at 2, got %d", tree.FocusIdx)
	}
	tree.MoveUp()
	if tree.FocusIdx != 1 {
		t.Errorf("expected focus 1 after up, got %d", tree.FocusIdx)
	}
	tree.MoveUp()
	if tree.FocusIdx != 0 {
		t.Errorf("expected focus 0 after second up, got %d", tree.FocusIdx)
	}
	tree.MoveUp()
	if tree.FocusIdx != 0 {
		t.Errorf("expected focus to clamp at 0, got %d", tree.FocusIdx)
	}
}

func TestSessionTreeFocusedNode(t *testing.T) {
	base := time.Now()
	tree := BuildSessionTree([]SessionNodeData{
		sessionData("a", "", "A", base, 1, ""),
		sessionData("b", "a", "B", base.Add(time.Minute), 1, ""),
	})
	tree.FocusIdx = 1
	node := tree.FocusedNode()
	if node == nil {
		t.Fatal("expected non-nil focused node")
	}
	if node.ID != "b" {
		t.Errorf("expected focused node b, got %s", node.ID)
	}
	tree.FocusIdx = 99
	if tree.FocusedNode() != nil {
		t.Error("expected nil for out-of-range focus")
	}
	tree.FocusIdx = -1
	if tree.FocusedNode() != nil {
		t.Error("expected nil for negative focus")
	}
}

func TestRenderSessionTreeEmpty(t *testing.T) {
	styles := NewStyles(Themes[0])
	out := RenderSessionTree(nil, styles, 80)
	if !strings.Contains(out, "No sessions") {
		t.Errorf("expected empty message, got %q", out)
	}
	tree := &SessionTree{}
	out2 := RenderSessionTree(tree, styles, 80)
	if !strings.Contains(out2, "No sessions") {
		t.Errorf("expected empty message for empty tree, got %q", out2)
	}
}

func TestRenderSessionTreeWithRoots(t *testing.T) {
	styles := NewStyles(Themes[0])
	base := time.Now()
	tree := BuildSessionTree([]SessionNodeData{
		sessionData("s1", "", "My Session", base, 5, "hello world"),
		sessionData("s2", "", "Other", base.Add(time.Minute), 3, "foo"),
	})
	out := RenderSessionTree(tree, styles, 80)
	if !strings.Contains(out, "Session Tree") {
		t.Error("expected title in output")
	}
	if !strings.Contains(out, "My Session") {
		t.Error("expected session name in output")
	}
	if !strings.Contains(out, "Other") {
		t.Error("expected second session name in output")
	}
	if !strings.Contains(out, "hello world") {
		t.Error("expected preview in output")
	}
	if !strings.Contains(out, "2 session(s)") {
		t.Error("expected session count in output")
	}
}

func TestRenderSessionTreeNested(t *testing.T) {
	styles := NewStyles(Themes[0])
	base := time.Now()
	tree := BuildSessionTree([]SessionNodeData{
		sessionData("root", "", "Root Session", base, 5, "parent"),
		sessionData("child", "root", "Child Session", base.Add(time.Minute), 3, "child msg"),
	})
	out := RenderSessionTree(tree, styles, 80)
	if !strings.Contains(out, "Root Session") {
		t.Error("expected root session name")
	}
	if !strings.Contains(out, "Child Session") {
		t.Error("expected child session name")
	}
	if !strings.Contains(out, "⑂1") {
		t.Error("expected fork indicator for root with 1 child")
	}
}

func TestRenderSessionTreeFocused(t *testing.T) {
	styles := NewStyles(Themes[0])
	base := time.Now()
	tree := BuildSessionTree([]SessionNodeData{
		sessionData("s1", "", "First", base, 1, ""),
		sessionData("s2", "", "Second", base.Add(time.Minute), 1, ""),
	})
	tree.FocusIdx = 0
	out0 := RenderSessionTree(tree, styles, 80)
	if !strings.Contains(out0, "First") {
		t.Error("expected focused session name in output")
	}
	tree.FocusIdx = 1
	out1 := RenderSessionTree(tree, styles, 80)
	if !strings.Contains(out1, "Second") {
		t.Error("expected second session name when focused")
	}
	if out0 == out1 {
		t.Error("focused output should differ when focus changes")
	}
}

func TestRenderSessionDiff(t *testing.T) {
	styles := NewStyles(Themes[0])
	diff := SessionDiff{
		SessionA:    "session-alpha",
		SessionB:    "session-beta",
		AddedMsgs:   7,
		RemovedMsgs: 2,
		SharedMsgs:  15,
		DivergeIdx:  15,
	}
	out := RenderSessionDiff(diff, styles, 80)
	if !strings.Contains(out, "Session Diff") {
		t.Error("expected diff title")
	}
	if !strings.Contains(out, "Branch point: message #") {
		t.Error("expected branch point label in output")
	}
	if !strings.Contains(out, "15") {
		t.Error("expected diverge index 15 in output")
	}
	if !strings.Contains(out, "+ 7 new in session-beta") {
		t.Error("expected added messages line")
	}
	if !strings.Contains(out, "- 2 only in session-alpha") {
		t.Error("expected removed messages line")
	}
	if !strings.Contains(out, "= 15 shared messages") {
		t.Error("expected shared messages line")
	}
}

func TestMarkActiveSession(t *testing.T) {
	base := time.Now()
	tree := BuildSessionTree([]SessionNodeData{
		sessionData("old", "", "Old", base, 1, ""),
		sessionData("newest", "", "Newest", base.Add(2*time.Minute), 1, ""),
		sessionData("mid", "", "Mid", base.Add(time.Minute), 1, ""),
	})
	flat := tree.Flatten()
	var active *SessionNode
	for _, n := range flat {
		if n.Status == "active" {
			if active != nil {
				t.Fatal("expected exactly one active session")
			}
			active = n
		}
	}
	if active == nil {
		t.Fatal("expected one session marked active")
	}
	if active.ID != "newest" {
		t.Errorf("expected newest to be active, got %s", active.ID)
	}
}
