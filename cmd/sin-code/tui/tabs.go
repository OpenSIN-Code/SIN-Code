package tui

import (
	"fmt"
	"strings"
)

type Session struct {
	Name    string
	Active  bool
	Dirty   bool
}

type Tabs struct {
	Sessions   []Session
	ActiveIdx  int
	MaxVisible int
	Width      int
}

func NewTabs() Tabs {
	return Tabs{
		Sessions: []Session{
			{Name: "Session 1", Active: true},
		},
		ActiveIdx:  0,
		MaxVisible: 6,
		Width:      80,
	}
}

func (t Tabs) Active() Session {
	if len(t.Sessions) == 0 {
		return Session{Name: "Session 1"}
	}
	if t.ActiveIdx < 0 || t.ActiveIdx >= len(t.Sessions) {
		return t.Sessions[0]
	}
	return t.Sessions[t.ActiveIdx]
}

func (t *Tabs) Add(name string) {
	if name == "" {
		name = fmt.Sprintf("Session %d", len(t.Sessions)+1)
	}
	t.Sessions = append(t.Sessions, Session{Name: name})
	t.ActiveIdx = len(t.Sessions) - 1
}

func (t *Tabs) Close(idx int) {
	if idx < 0 || idx >= len(t.Sessions) {
		return
	}
	t.Sessions = append(t.Sessions[:idx], t.Sessions[idx+1:]...)
	if t.ActiveIdx >= len(t.Sessions) {
		t.ActiveIdx = len(t.Sessions) - 1
	}
	if t.ActiveIdx < 0 {
		t.ActiveIdx = 0
		t.Sessions = []Session{{Name: "Session 1", Active: true}}
	}
}

func (t *Tabs) Select(idx int) {
	if idx < 0 || idx >= len(t.Sessions) {
		return
	}
	t.ActiveIdx = idx
}

func (t *Tabs) Next() {
	if len(t.Sessions) == 0 {
		return
	}
	t.ActiveIdx = (t.ActiveIdx + 1) % len(t.Sessions)
}

func (t *Tabs) Prev() {
	if len(t.Sessions) == 0 {
		return
	}
	t.ActiveIdx = (t.ActiveIdx - 1 + len(t.Sessions)) % len(t.Sessions)
}

func (t Tabs) View(s Styles) string {
	if len(t.Sessions) == 0 {
		t.Sessions = []Session{{Name: "Session 1", Active: true}}
		t.ActiveIdx = 0
	}

	var b strings.Builder
	b.WriteString(s.Header.Render("⚡ sin-code"))
	b.WriteString(" ")

	start := 0
	end := len(t.Sessions)
	if end > t.MaxVisible {
		start = t.ActiveIdx
		if start+t.MaxVisible > end {
			start = end - t.MaxVisible
		}
		end = start + t.MaxVisible
	}

	for i := start; i < end; i++ {
		sess := t.Sessions[i]
		label := sess.Name
		if sess.Dirty {
			label = "● " + label
		}
		if i == t.ActiveIdx {
			b.WriteString(s.TabActive.Render(" " + label + " "))
		} else {
			b.WriteString(s.TabIdle.Render(" " + label + " "))
		}
		b.WriteString(" ")
	}

	b.WriteString(s.TabAdd.Render(" + "))

	used := lipglossWidth(b.String())
	if used < t.Width {
		b.WriteString(strings.Repeat(" ", t.Width-used))
	}

	return b.String()
}

func lipglossWidth(s string) int {
	w := 0
	for _, r := range s {
		if r == '\n' {
			continue
		}
		w++
	}
	return w
}
