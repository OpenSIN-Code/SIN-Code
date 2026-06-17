// SPDX-License-Identifier: MIT
// Purpose: tests for the expandable Tool Call Tree view.
package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func newTestNode(id, tool string) *ToolCallNode {
	return &ToolCallNode{
		ID:        id,
		Tool:      tool,
		Args:      "test args",
		Status:    "running",
		StartTime: time.Now(),
	}
}

func TestToolCallTreeAddRoot(t *testing.T) {
	tree := &ToolCallTree{}
	n := newTestNode("n1", "sin_bash")
	tree.AddNode("", n)

	if len(tree.Nodes) != 1 {
		t.Fatalf("expected 1 root node, got %d", len(tree.Nodes))
	}
	if tree.Nodes[0].ID != "n1" {
		t.Errorf("expected node ID 'n1', got %q", tree.Nodes[0].ID)
	}
	if tree.Nodes[0].Depth != 0 {
		t.Errorf("expected depth 0 for root, got %d", tree.Nodes[0].Depth)
	}
}

func TestToolCallTreeAddChild(t *testing.T) {
	tree := &ToolCallTree{}
	parent := newTestNode("p1", "sin_bash")
	tree.AddNode("", parent)

	child := newTestNode("c1", "sin_read")
	tree.AddNode("p1", child)

	if len(parent.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(parent.Children))
	}
	if parent.Children[0].ID != "c1" {
		t.Errorf("expected child ID 'c1', got %q", parent.Children[0].ID)
	}
	if parent.Children[0].Depth != 1 {
		t.Errorf("expected child depth 1, got %d", parent.Children[0].Depth)
	}
}

func TestToolCallTreeAddChildOrphanBecomesRoot(t *testing.T) {
	tree := &ToolCallTree{}
	orphan := newTestNode("o1", "sin_read")
	tree.AddNode("nonexistent", orphan)

	if len(tree.Nodes) != 1 {
		t.Fatalf("orphan with missing parent should become root, got %d roots", len(tree.Nodes))
	}
	if tree.Nodes[0].ID != "o1" {
		t.Errorf("expected orphan at root, got %q", tree.Nodes[0].ID)
	}
}

func TestToolCallTreeFindNode(t *testing.T) {
	tree := &ToolCallTree{}
	root := newTestNode("root", "sin_bash")
	tree.AddNode("", root)

	child := newTestNode("child", "sin_read")
	tree.AddNode("root", child)

	grand := newTestNode("grand", "sin_edit")
	tree.AddNode("child", grand)

	if tree.FindNode("root") == nil {
		t.Error("expected to find 'root'")
	}
	if tree.FindNode("child") == nil {
		t.Error("expected to find 'child'")
	}
	if tree.FindNode("grand") == nil {
		t.Error("expected to find 'grand' via DFS")
	}
	if tree.FindNode("missing") != nil {
		t.Error("expected nil for missing ID")
	}
}

func TestToolCallTreeUpdateNode(t *testing.T) {
	tree := &ToolCallTree{}
	n := newTestNode("n1", "sin_bash")
	tree.AddNode("", n)

	tree.UpdateNode("n1", "success", "build ok", 150*time.Millisecond, "")

	if n.Status != "success" {
		t.Errorf("expected status 'success', got %q", n.Status)
	}
	if n.Output != "build ok" {
		t.Errorf("expected output 'build ok', got %q", n.Output)
	}
	if n.Duration != 150*time.Millisecond {
		t.Errorf("expected duration 150ms, got %v", n.Duration)
	}
}

func TestToolCallTreeUpdateNodeMissingNoPanic(t *testing.T) {
	tree := &ToolCallTree{}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("UpdateNode on missing ID panicked: %v", r)
		}
	}()
	tree.UpdateNode("ghost", "success", "x", 0, "")
}

func TestToolCallTreeFlatten(t *testing.T) {
	tree := &ToolCallTree{}
	root := newTestNode("r", "sin_bash")
	tree.AddNode("", root)

	c1 := newTestNode("c1", "sin_read")
	tree.AddNode("r", c1)

	c2 := newTestNode("c2", "sin_edit")
	tree.AddNode("r", c2)

	flat := tree.Flatten()
	if len(flat) != 1 {
		t.Fatalf("expected 1 node when collapsed, got %d", len(flat))
	}

	root.Expanded = true
	flat = tree.Flatten()
	if len(flat) != 3 {
		t.Fatalf("expected 3 nodes when expanded, got %d", len(flat))
	}
	if flat[0].ID != "r" || flat[1].ID != "c1" || flat[2].ID != "c2" {
		t.Errorf("unexpected flatten order: %s %s %s", flat[0].ID, flat[1].ID, flat[2].ID)
	}
}

func TestToolCallTreeToggleExpanded(t *testing.T) {
	tree := &ToolCallTree{}
	root := newTestNode("r", "sin_bash")
	tree.AddNode("", root)

	child := newTestNode("c", "sin_read")
	tree.AddNode("r", child)

	tree.FocusIdx = 0
	if root.Expanded {
		t.Fatal("root should start collapsed")
	}

	tree.ToggleExpanded()
	if !root.Expanded {
		t.Error("root should be expanded after toggle")
	}

	tree.ToggleExpanded()
	if root.Expanded {
		t.Error("root should be collapsed after second toggle")
	}
}

func TestToolCallTreeMoveUpDown(t *testing.T) {
	tree := &ToolCallTree{}
	n1 := newTestNode("n1", "sin_bash")
	n2 := newTestNode("n2", "sin_read")
	tree.AddNode("", n1)
	tree.AddNode("", n2)

	if tree.FocusIdx != 0 {
		t.Fatalf("expected initial focus 0, got %d", tree.FocusIdx)
	}

	tree.MoveDown()
	if tree.FocusIdx != 1 {
		t.Errorf("expected focus 1 after MoveDown, got %d", tree.FocusIdx)
	}

	tree.MoveUp()
	if tree.FocusIdx != 0 {
		t.Errorf("expected focus 0 after MoveUp, got %d", tree.FocusIdx)
	}

	tree.MoveUp()
	if tree.FocusIdx != 0 {
		t.Errorf("expected focus to stay 0 at top, got %d", tree.FocusIdx)
	}

	tree.MoveDown()
	tree.MoveDown()
	if tree.FocusIdx != 1 {
		t.Errorf("expected focus to stay 1 at bottom, got %d", tree.FocusIdx)
	}
}

func TestToolCallTreeFocusedNode(t *testing.T) {
	tree := &ToolCallTree{}
	n1 := newTestNode("n1", "sin_bash")
	n2 := newTestNode("n2", "sin_read")
	tree.AddNode("", n1)
	tree.AddNode("", n2)

	tree.FocusIdx = 1
	got := tree.FocusedNode()
	if got == nil || got.ID != "n2" {
		t.Errorf("expected focused node 'n2', got %v", got)
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

func TestRenderToolCallTreeEmpty(t *testing.T) {
	styles := NewStyles(Themes[0])
	out := RenderToolCallTree(nil, styles, 80)
	if !strings.Contains(out, "No tool calls yet") {
		t.Errorf("expected 'No tool calls yet', got: %s", out)
	}

	tree := &ToolCallTree{}
	out2 := RenderToolCallTree(tree, styles, 80)
	if !strings.Contains(out2, "No tool calls yet") {
		t.Errorf("expected 'No tool calls yet' for empty tree, got: %s", out2)
	}
}

func TestRenderToolCallTreeWithNodes(t *testing.T) {
	styles := NewStyles(Themes[0])
	tree := &ToolCallTree{}
	n := newTestNode("n1", "sin_bash")
	n.Status = "success"
	n.Duration = 250 * time.Millisecond
	tree.AddNode("", n)

	out := RenderToolCallTree(tree, styles, 80)

	if !strings.Contains(out, "Tool Call Tree") {
		t.Error("expected header 'Tool Call Tree'")
	}
	if !strings.Contains(out, "sin_bash") {
		t.Error("expected tool name 'sin_bash' in output")
	}
	if !strings.Contains(out, "test args") {
		t.Error("expected args 'test args' in output")
	}
}

func TestRenderToolCallTreeExpanded(t *testing.T) {
	styles := NewStyles(Themes[0])
	tree := &ToolCallTree{}
	n := newTestNode("n1", "sin_bash")
	n.Status = "success"
	n.Output = "Build successful\nAll tests passed\nExtra line"
	n.Expanded = true
	tree.AddNode("", n)

	out := RenderToolCallTree(tree, styles, 80)

	if !strings.Contains(out, "Build successful") {
		t.Error("expected first output line when expanded")
	}
	if !strings.Contains(out, "All tests passed") {
		t.Error("expected second output line when expanded")
	}
	if !strings.Contains(out, "(truncated)") {
		t.Error("expected truncation indicator for >2 output lines")
	}
	if strings.Contains(out, "Extra line") {
		t.Error("third output line should be truncated, not shown")
	}
}

func TestRenderToolCallTreeExpandedError(t *testing.T) {
	styles := NewStyles(Themes[0])
	tree := &ToolCallTree{}
	n := newTestNode("n1", "sin_bash")
	n.Status = "error"
	n.Error = "command not found"
	n.Expanded = true
	tree.AddNode("", n)

	out := RenderToolCallTree(tree, styles, 80)
	if !strings.Contains(out, "command not found") {
		t.Error("expected error text when expanded")
	}
}

func TestRenderToolCallTreeCollapsedHidesOutput(t *testing.T) {
	styles := NewStyles(Themes[0])
	tree := &ToolCallTree{}
	n := newTestNode("n1", "sin_bash")
	n.Status = "success"
	n.Output = "secret output"
	n.Expanded = false
	tree.AddNode("", n)

	out := RenderToolCallTree(tree, styles, 80)
	if strings.Contains(out, "secret output") {
		t.Error("output should be hidden when collapsed")
	}
}

func TestRenderToolCallTreeFocused(t *testing.T) {
	styles := NewStyles(Themes[0])
	tree := &ToolCallTree{}
	n1 := newTestNode("n1", "sin_bash")
	n2 := newTestNode("n2", "sin_read")
	tree.AddNode("", n1)
	tree.AddNode("", n2)

	tree.FocusIdx = 1
	out := RenderToolCallTree(tree, styles, 80)

	if !strings.Contains(out, "sin_read") {
		t.Error("expected focused node 'sin_read' in output")
	}
}

func TestRenderToolCallTreeNested(t *testing.T) {
	styles := NewStyles(Themes[0])
	tree := &ToolCallTree{}

	root := newTestNode("root", "sin_bash")
	root.Status = "success"
	tree.AddNode("", root)

	child := newTestNode("child", "sin_read")
	child.Status = "success"
	tree.AddNode("root", child)

	grand := newTestNode("grand", "sin_edit")
	grand.Status = "running"
	tree.AddNode("child", grand)

	root.Expanded = true
	child.Expanded = true

	out := RenderToolCallTree(tree, styles, 80)

	if !strings.Contains(out, "sin_bash") {
		t.Error("expected root tool 'sin_bash'")
	}
	if !strings.Contains(out, "sin_read") {
		t.Error("expected child tool 'sin_read' when parent expanded")
	}
	if !strings.Contains(out, "sin_edit") {
		t.Error("expected grandchild tool 'sin_edit' when ancestors expanded")
	}

	root.Expanded = false
	out2 := RenderToolCallTree(tree, styles, 80)
	if strings.Contains(out2, "sin_read") {
		t.Error("child should be hidden when root collapsed")
	}
}

func TestRenderToolCallTreeExpandIndicator(t *testing.T) {
	styles := NewStyles(Themes[0])
	tree := &ToolCallTree{}

	root := newTestNode("root", "sin_bash")
	root.Status = "success"
	tree.AddNode("", root)
	tree.AddNode("root", newTestNode("c", "sin_read"))

	out := RenderToolCallTree(tree, styles, 80)
	if !strings.Contains(out, "▶") {
		t.Error("expected ▶ indicator when collapsed with children")
	}

	root.Expanded = true
	out2 := RenderToolCallTree(tree, styles, 80)
	if !strings.Contains(out2, "▼") {
		t.Error("expected ▼ indicator when expanded with children")
	}
}

func TestRenderToolCallTreeArgsTruncated(t *testing.T) {
	styles := NewStyles(Themes[0])
	tree := &ToolCallTree{}
	n := newTestNode("n1", "sin_bash")
	n.Args = strings.Repeat("x", 50)
	tree.AddNode("", n)

	out := RenderToolCallTree(tree, styles, 80)
	if !strings.Contains(out, "...") {
		t.Error("expected '...' truncation for long args")
	}
}

func TestStatusIcon(t *testing.T) {
	cases := map[string]string{
		"running": "⟳",
		"success": "✓",
		"error":   "✗",
		"denied":  "⛔",
		"unknown": "○",
	}
	for status, expected := range cases {
		if got := statusIcon(status); got != expected {
			t.Errorf("statusIcon(%q) = %q, want %q", status, got, expected)
		}
	}
}

func TestStyleForStatus(t *testing.T) {
	styles := NewStyles(Themes[0])
	probe := "X"
	cases := map[string]lipgloss.Style{
		"success": styles.StatusOK,
		"error":   styles.StatusErr,
		"denied":  styles.StatusWarn,
		"running": styles.AccentText,
		"unknown": styles.Muted,
	}
	for status, expected := range cases {
		got := styleForStatus(status, styles)
		if got.Render(probe) != expected.Render(probe) {
			t.Errorf("styleForStatus(%q) does not match expected style", status)
		}
	}
}

func TestUpdateNodeFallbackByToolName(t *testing.T) {
	tree := &ToolCallTree{}
	n := &ToolCallNode{
		ID:        "tool-9999999999-sin_bash",
		Tool:      "sin_bash",
		Status:    "running",
		StartTime: time.Now(),
	}
	tree.AddNode("", n)

	tree.UpdateNode("sin_bash", "success", "build ok", 42*time.Millisecond, "")

	if n.Status != "success" {
		t.Errorf("expected fallback to match by tool name, got status %q", n.Status)
	}
	if n.Output != "build ok" {
		t.Errorf("expected output 'build ok', got %q", n.Output)
	}
}

func TestUpdateNodePrefersRunning(t *testing.T) {
	tree := &ToolCallTree{}
	old := newTestNode("n1", "sin_read")
	old.Status = "success"
	tree.AddNode("", old)
	running := newTestNode("n2", "sin_read")
	running.Status = "running"
	tree.AddNode("", running)

	tree.UpdateNode("sin_read", "success", "done", 0, "")

	if old.Status != "success" {
		t.Error("old node should remain success")
	}
	if running.Status != "success" {
		t.Errorf("running node should be updated, got %q", running.Status)
	}
}

func TestRenderToolCallTreeInViewModel(t *testing.T) {
	m := NewModel()
	m.ToolTree = &ToolCallTree{}
	m.ToolTree.AddNode("", &ToolCallNode{
		ID:     "t1",
		Tool:   "sin_bash",
		Status: "success",
		Args:   "go build",
		Output: "ok",
	})
	m.ToolTreeVisible = true
	m.ViewKind = ViewChat
	m.Width = 80
	m.Height = 24

	out := m.View()
	if !strings.Contains(out.Content, "sin_bash") {
		t.Error("View() should render tool tree when ToolTreeVisible=true")
	}
	m.ToolTreeVisible = false
	out2 := m.View()
	if strings.Contains(out2.Content, "Tool Call Tree") {
		t.Error("View() should not render tool tree when ToolTreeVisible=false")
	}
}
