// SPDX-License-Identifier: MIT
package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type FilePicker struct {
	rootDir string
	sel     int
	open    bool
	files   []string
}

func NewFilePicker(rootDir string) *FilePicker {
	return &FilePicker{rootDir: rootDir}
}

func (f *FilePicker) Open() {
	f.open = true
	f.sel = 0
}

func (f *FilePicker) Close() {
	f.open = false
}

func (f *FilePicker) IsOpen() bool {
	return f.open
}

func (f *FilePicker) Render(styles Styles, width, height int) string {
	if !f.open {
		return ""
	}
	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" Open File"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("-", width-6)))
	b.WriteString("\n\n")
	if len(f.files) == 0 {
		b.WriteString(styles.Muted.Render("  No files found"))
		b.WriteString("\n")
	} else {
		for i, file := range f.files {
			label := "  " + file
			if i == f.sel {
				b.WriteString(styles.PopupSel.Render(padRight(label, width-6)))
			} else {
				b.WriteString(styles.PopupItem.Render(padRight(label, width-6)))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(" up/down navigate | enter open | esc close"))
	b.WriteString("\n")
	return styles.Popup.Render(b.String())
}

func (f *FilePicker) MoveUp() {
	if f.sel > 0 {
		f.sel--
	}
}

func (f *FilePicker) MoveDown() {
	if f.sel < len(f.files)-1 {
		f.sel++
	}
}

func (m *Model) OpenFilePicker() {
	if m.FilePicker == nil {
		m.FilePicker = NewFilePicker(m.Workspace)
	}
	m.FilePicker.Open()
	m.Mode = ModeFilePicker
}

func (m *Model) handleFilePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		if m.FilePicker != nil {
			m.FilePicker.Close()
		}
		m.Mode = ModeNormal
		return m, nil
	case "enter":
		if m.FilePicker != nil && m.FilePicker.sel < len(m.FilePicker.files) {
			m.ShowFilePreview(m.FilePicker.files[m.FilePicker.sel])
			m.FilePicker.Close()
		}
		m.Mode = ModeNormal
		return m, nil
	case "up":
		if m.FilePicker != nil {
			m.FilePicker.MoveUp()
		}
		return m, nil
	case "down":
		if m.FilePicker != nil {
			m.FilePicker.MoveDown()
		}
		return m, nil
	}
	return m, nil
}
