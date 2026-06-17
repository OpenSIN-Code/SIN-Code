// SPDX-License-Identifier: MIT
// Purpose: expandable hierarchical Tool Call Tree view for the TUI.
// Shows the causal relationship between tool calls (parent → child),
// better than opencode's flat list and Claude Code's collapsible panels.
package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// ToolCallNode represents one tool call in the tree.
type ToolCallNode struct {
	ID        string // unique ID
	Tool      string // tool name (sin_bash, sin_read, etc.)
	Args      string // input arguments (truncated)
	Output    string // tool output (truncated)
	Status    string // "running", "success", "error", "denied"
	StartTime time.Time
	Duration  time.Duration
	Error     string
	Children  []*ToolCallNode // nested tool calls (if any)
	Expanded  bool            // whether this node is expanded in the UI
	Depth     int             // indentation level
}

// ToolCallTree holds the root-level tool calls for one agent run.
type ToolCallTree struct {
	Nodes    []*ToolCallNode
	FocusIdx int // which node is focused (for expand/collapse)
}

// ToolCallTreeMsg adds a tool call to the tree.
type ToolCallTreeMsg struct {
	ParentID string // empty = root level
	Node     *ToolCallNode
}

// ToolCallUpdateMsg updates an existing tool call.
type ToolCallUpdateMsg struct {
	ID       string
	Status   string
	Output   string
	Duration time.Duration
	Error    string
}

// AddNode adds a tool call node to the tree, either at root or under a parent.
func (t *ToolCallTree) AddNode(parentID string, node *ToolCallNode) {
	node.Depth = 0
	if parentID == "" {
		t.Nodes = append(t.Nodes, node)
		return
	}
	parent := t.FindNode(parentID)
	if parent != nil {
		node.Depth = parent.Depth + 1
		parent.Children = append(parent.Children, node)
	} else {
		t.Nodes = append(t.Nodes, node)
	}
}

// UpdateNode updates a tool call by ID, with fallback to tool-name matching.
func (t *ToolCallTree) UpdateNode(id, status, output string, duration time.Duration, errMsg string) {
	node := t.FindNode(id)
	if node == nil {
		node = t.FindNodeByTool(id)
	}
	if node == nil {
		return
	}
	node.Status = status
	node.Output = output
	node.Duration = duration
	node.Error = errMsg
}

// FindNodeByTool finds the most recent running node with the given tool name.
// Falls back to the last node with that name if none are running.
func (t *ToolCallTree) FindNodeByTool(name string) *ToolCallNode {
	var running *ToolCallNode
	var last *ToolCallNode
	for _, n := range t.Nodes {
		scanNodeByTool(n, name, &running, &last)
	}
	if running != nil {
		return running
	}
	return last
}

func scanNodeByTool(node *ToolCallNode, name string, running **ToolCallNode, last **ToolCallNode) {
	if node.Tool == name {
		*last = node
		if node.Status == "running" {
			*running = node
		}
	}
	for _, c := range node.Children {
		scanNodeByTool(c, name, running, last)
	}
}

// FindNode searches the tree for a node by ID (DFS).
func (t *ToolCallTree) FindNode(id string) *ToolCallNode {
	for _, n := range t.Nodes {
		if found := findNodeDFS(n, id); found != nil {
			return found
		}
	}
	return nil
}

func findNodeDFS(node *ToolCallNode, id string) *ToolCallNode {
	if node.ID == id {
		return node
	}
	for _, c := range node.Children {
		if found := findNodeDFS(c, id); found != nil {
			return found
		}
	}
	return nil
}

// ToggleExpanded flips the expanded state of the focused node.
func (t *ToolCallTree) ToggleExpanded() {
	node := t.FocusedNode()
	if node != nil {
		node.Expanded = !node.Expanded
	}
}

// FocusedNode returns the currently focused node.
func (t *ToolCallTree) FocusedNode() *ToolCallNode {
	flat := t.Flatten()
	if t.FocusIdx < 0 || t.FocusIdx >= len(flat) {
		return nil
	}
	return flat[t.FocusIdx]
}

// Flatten returns all nodes in display order (DFS pre-order).
func (t *ToolCallTree) Flatten() []*ToolCallNode {
	var out []*ToolCallNode
	for _, n := range t.Nodes {
		flattenDFS(n, &out)
	}
	return out
}

func flattenDFS(node *ToolCallNode, out *[]*ToolCallNode) {
	*out = append(*out, node)
	if node.Expanded {
		for _, c := range node.Children {
			flattenDFS(c, out)
		}
	}
}

// MoveUp moves the focus index up.
func (t *ToolCallTree) MoveUp() {
	if t.FocusIdx > 0 {
		t.FocusIdx--
	}
}

// MoveDown moves the focus index down.
func (t *ToolCallTree) MoveDown() {
	flat := t.Flatten()
	if t.FocusIdx < len(flat)-1 {
		t.FocusIdx++
	}
}

// RenderToolCallTree renders the tree as an indented list with tree connectors.
func RenderToolCallTree(tree *ToolCallTree, styles Styles, width int) string {
	if tree == nil || len(tree.Nodes) == 0 {
		return styles.Muted.Render("  No tool calls yet")
	}

	flat := tree.Flatten()
	var b strings.Builder

	b.WriteString(styles.ContentHdr.Render("🔧 Tool Call Tree"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n")

	for i, node := range flat {
		b.WriteString(renderTreeNode(node, i == tree.FocusIdx, styles, width))
		b.WriteString("\n")
	}

	b.WriteString(styles.Muted.Render("  ↑/↓ navigate · enter expand/collapse · esc back"))
	b.WriteString("\n")

	return b.String()
}

func renderTreeNode(node *ToolCallNode, focused bool, styles Styles, width int) string {
	var b strings.Builder

	indent := strings.Repeat("  ", node.Depth)

	connector := "├─ "
	if node.Depth == 0 {
		connector = "▸ "
	}

	icon := statusIcon(node.Status)
	iconStyle := styleForStatus(node.Status, styles)

	args := node.Args
	if len(args) > 40 {
		args = args[:37] + "..."
	}

	duration := ""
	if node.Duration > 0 {
		duration = styles.Muted.Render(fmt.Sprintf(" (%s)", formatDuration(node.Duration)))
	}

	expandIndicator := ""
	if len(node.Children) > 0 {
		if node.Expanded {
			expandIndicator = "▼ "
		} else {
			expandIndicator = "▶ "
		}
	}

	line := fmt.Sprintf("%s%s%s%s %s %s%s",
		indent, connector, expandIndicator,
		iconStyle.Render(icon),
		styles.Bold.Render(node.Tool),
		styles.Muted.Render(args),
		duration)

	if focused {
		b.WriteString(styles.SidebarSel.Render(padRight(line, width-2)))
	} else {
		b.WriteString(styles.Content.Render(line))
	}

	if node.Expanded && node.Output != "" {
		outLines := strings.Split(node.Output, "\n")
		for i, ol := range outLines {
			if i >= 2 {
				b.WriteString("\n" + indent + "    " + styles.Muted.Render("... (truncated)"))
				break
			}
			b.WriteString("\n" + indent + "    " + styles.Content.Render(ol))
		}
	}

	if node.Expanded && node.Error != "" {
		b.WriteString("\n" + indent + "    " + styles.StatusErr.Render("❌ "+node.Error))
	}

	return b.String()
}

func statusIcon(status string) string {
	switch status {
	case "running":
		return "⟳"
	case "success":
		return "✓"
	case "error":
		return "✗"
	case "denied":
		return "⛔"
	default:
		return "○"
	}
}

func styleForStatus(status string, styles Styles) lipgloss.Style {
	switch status {
	case "success":
		return styles.StatusOK
	case "error":
		return styles.StatusErr
	case "denied":
		return styles.StatusWarn
	case "running":
		return styles.AccentText
	default:
		return styles.Muted
	}
}
