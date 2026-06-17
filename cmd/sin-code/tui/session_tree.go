// SPDX-License-Identifier: MIT
// Purpose: Session branching visualization — fork-point tree display and
// branch diff.  Shows session forks as a tree rather than a flat list.
package tui

import (
	"fmt"
	"strings"
	"time"
)

// SessionNode represents one session in the branching tree.
type SessionNode struct {
	ID           string
	Name         string
	ParentID     string // empty = root session
	CreatedAt    time.Time
	LastActive   time.Time
	MessageCount int
	Status       string // "active", "idle", "archived"
	Preview      string // last assistant message preview
	Children     []*SessionNode
	Depth        int
}

// SessionTree holds the root sessions and provides tree operations.
type SessionTree struct {
	Roots    []*SessionNode
	FocusIdx int
}

// SessionTreeMsg is sent when the session tree should be rebuilt.
type SessionTreeMsg struct {
	Sessions []SessionNodeData
}

// SessionNodeData is the flat session data from the session store.
type SessionNodeData struct {
	ID           string
	Name         string
	ParentID     string
	CreatedAt    time.Time
	LastActive   time.Time
	MessageCount int
	Preview      string
}

// BuildSessionTree constructs a tree from flat session data.
func BuildSessionTree(sessions []SessionNodeData) *SessionTree {
	tree := &SessionTree{}
	nodeMap := make(map[string]*SessionNode)

	for _, s := range sessions {
		node := &SessionNode{
			ID:           s.ID,
			Name:         s.Name,
			ParentID:     s.ParentID,
			CreatedAt:    s.CreatedAt,
			LastActive:   s.LastActive,
			MessageCount: s.MessageCount,
			Preview:      s.Preview,
			Status:       "idle",
		}
		nodeMap[s.ID] = node
	}

	for _, s := range sessions {
		node := nodeMap[s.ID]
		if s.ParentID != "" {
			if parent, ok := nodeMap[s.ParentID]; ok {
				parent.Children = append(parent.Children, node)
			} else {
				tree.Roots = append(tree.Roots, node) // orphan → root
			}
		} else {
			tree.Roots = append(tree.Roots, node)
		}
	}

	setDepths(tree.Roots, 0)
	markActiveSession(tree)

	return tree
}

func setDepths(nodes []*SessionNode, depth int) {
	for _, n := range nodes {
		n.Depth = depth
		setDepths(n.Children, depth+1)
	}
}

func markActiveSession(tree *SessionTree) {
	var mostRecent *SessionNode
	for _, root := range tree.Roots {
		findMostRecent(root, &mostRecent)
	}
	if mostRecent != nil {
		mostRecent.Status = "active"
	}
}

func findMostRecent(node *SessionNode, best **SessionNode) {
	if *best == nil || node.LastActive.After((*best).LastActive) {
		*best = node
	}
	for _, c := range node.Children {
		findMostRecent(c, best)
	}
}

// RenderSessionTree renders the session branching tree.
func RenderSessionTree(tree *SessionTree, styles Styles, width int) string {
	if tree == nil || len(tree.Roots) == 0 {
		return styles.Muted.Render("  No sessions. Start chatting to create one.")
	}

	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("🌿 Session Tree"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n")

	flat := tree.Flatten()
	for i, node := range flat {
		b.WriteString(renderSessionNode(node, i == tree.FocusIdx, styles, width))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(fmt.Sprintf("  %d session(s) · ↑/↓ navigate · enter switch · f fork · d diff", len(flat))))
	b.WriteString("\n")

	return b.String()
}

func renderSessionNode(node *SessionNode, focused bool, styles Styles, width int) string {
	var b strings.Builder

	indent := strings.Repeat("  ", node.Depth)

	connector := "├─ "
	if node.Depth == 0 {
		connector = ""
	}

	icon := "○"
	style := styles.Muted
	switch node.Status {
	case "active":
		icon = "●"
		style = styles.StatusOK
	case "idle":
		icon = "○"
		style = styles.Muted
	}

	forkIndicator := ""
	if len(node.Children) > 0 {
		forkIndicator = fmt.Sprintf(" ⑂%d", len(node.Children))
	}

	name := node.Name
	preview := ""
	if node.Preview != "" {
		p := node.Preview
		if len(p) > 30 {
			p = p[:27] + "..."
		}
		preview = styles.Muted.Render(" " + p)
	}

	msgCount := styles.Muted.Render(fmt.Sprintf(" (%d)", node.MessageCount))

	line := fmt.Sprintf("%s%s%s %s %s%s%s%s",
		indent, connector,
		style.Render(icon),
		styles.Bold.Render(name),
		forkIndicator,
		msgCount,
		preview,
		"")

	if focused {
		b.WriteString(styles.SidebarSel.Render(padRight(line, max(width-2, 0))))
	} else {
		b.WriteString(styles.Content.Render(line))
	}

	return b.String()
}

// SessionDiff represents the difference between two sessions.
type SessionDiff struct {
	SessionA    string
	SessionB    string
	AddedMsgs   int // messages in B not in A
	RemovedMsgs int // messages in A not in B
	SharedMsgs  int // common messages
	DivergeIdx  int // message index where they diverge
}

// RenderSessionDiff renders a visual diff between two sessions.
func RenderSessionDiff(diff SessionDiff, styles Styles, width int) string {
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("🔄 Session Diff"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n\n")

	b.WriteString(styles.Muted.Render("  Branch point: message #"))
	b.WriteString(styles.Bold.Render(fmt.Sprintf("%d", diff.DivergeIdx)))
	b.WriteString("\n\n")

	b.WriteString(styles.StatusOK.Render(fmt.Sprintf("  + %d new in %s", diff.AddedMsgs, diff.SessionB)))
	b.WriteString("\n")
	b.WriteString(styles.StatusErr.Render(fmt.Sprintf("  - %d only in %s", diff.RemovedMsgs, diff.SessionA)))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(fmt.Sprintf("  = %d shared messages", diff.SharedMsgs)))
	b.WriteString("\n")

	return b.String()
}

// Flatten returns all nodes in display order (DFS pre-order).
func (t *SessionTree) Flatten() []*SessionNode {
	var out []*SessionNode
	for _, n := range t.Roots {
		flattenSessionDFS(n, &out)
	}
	return out
}

func flattenSessionDFS(node *SessionNode, out *[]*SessionNode) {
	*out = append(*out, node)
	for _, c := range node.Children {
		flattenSessionDFS(c, out)
	}
}

func (t *SessionTree) MoveUp() {
	if t.FocusIdx > 0 {
		t.FocusIdx--
	}
}

func (t *SessionTree) MoveDown() {
	flat := t.Flatten()
	if t.FocusIdx < len(flat)-1 {
		t.FocusIdx++
	}
}

func (t *SessionTree) FocusedNode() *SessionNode {
	flat := t.Flatten()
	if t.FocusIdx < 0 || t.FocusIdx >= len(flat) {
		return nil
	}
	return flat[t.FocusIdx]
}
