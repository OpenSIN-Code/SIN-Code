// SPDX-License-Identifier: MIT
package tui

import "sync"

type PaneKind int

const (
	PaneNone PaneKind = iota
	PaneChat
	PaneFileViewer
	PaneDAG
	PaneDiff
)

func (p PaneKind) String() string {
	switch p {
	case PaneNone:
		return "None"
	case PaneChat:
		return "Chat"
	case PaneFileViewer:
		return "FileViewer"
	case PaneDAG:
		return "DAG"
	case PaneDiff:
		return "Diff"
	}
	return "Unknown"
}

type SplitPane struct {
	mu       sync.Mutex
	active   bool
	side     PaneKind
	sidePct  int
	minWidth int
	maxWidth int
}

func NewSplitPane() *SplitPane {
	return &SplitPane{
		side:     PaneFileViewer,
		sidePct:  40,
		minWidth: 30,
		maxWidth: 60,
	}
}

func (sp *SplitPane) Toggle() {
	sp.mu.Lock()
	sp.active = !sp.active
	sp.mu.Unlock()
}

func (sp *SplitPane) SetActive(active bool) {
	sp.mu.Lock()
	sp.active = active
	sp.mu.Unlock()
}

func (sp *SplitPane) SetSide(pane PaneKind) {
	sp.mu.Lock()
	sp.side = pane
	sp.mu.Unlock()
}

func (sp *SplitPane) Active() bool {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.active
}

func (sp *SplitPane) SideKind() PaneKind {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.side
}

func (sp *SplitPane) SideWidth(totalWidth int) int {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	w := totalWidth * sp.sidePct / 100
	if w < sp.minWidth {
		w = sp.minWidth
	}
	if w > sp.maxWidth {
		w = sp.maxWidth
	}
	if w > totalWidth-20 {
		w = totalWidth - 20
	}
	if w < 0 {
		w = 0
	}
	return w
}

func (sp *SplitPane) MainWidth(totalWidth int) int {
	return totalWidth - sp.SideWidth(totalWidth) - 1
}
