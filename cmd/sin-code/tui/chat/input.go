// SPDX-License-Identifier: MIT
// Purpose: chat input widget — textarea with attachment support and slash
// commands. Used by the TUI 2.0 chat mode (Phase 5 of st-3t5v).
package chat

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textarea"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/attachments"
)

type Input struct {
	textarea    textarea.Model
	attachments []*attachments.Attachment
	store       *attachments.Store
	width       int
	height      int
	placeholder string
}

func NewInput(store *attachments.Store) *Input {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (/attach <path>, /clear, Ctrl+S submit, Ctrl+C quit)"
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(5)
	ta.CharLimit = 100_000
	ta.Focus()
	return &Input{
		textarea:    ta,
		store:       store,
		width:       80,
		height:      5,
		placeholder: "Type a message...",
	}
}

func (i *Input) Init() tea.Cmd {
	return textarea.Blink
}

func (i *Input) SetSize(w, h int) {
	i.width = w
	i.height = h
	i.textarea.SetWidth(w)
	i.textarea.SetHeight(h)
}

func (i *Input) Focus() tea.Cmd {
	return i.textarea.Focus()
}

func (i *Input) Blur() {
	i.textarea.Blur()
}

func (i *Input) Value() string {
	val := i.textarea.Value()
	for _, a := range i.attachments {
		val += "\n" + a.Marker()
	}
	return val
}

func (i *Input) RawValue() string {
	return i.textarea.Value()
}

func (i *Input) Attachments() []*attachments.Attachment {
	return i.attachments
}

func (i *Input) Clear() {
	i.textarea.Reset()
	i.attachments = nil
}

func (i *Input) Attach(path string) error {
	a, err := i.store.Attach(path)
	if err != nil {
		return err
	}
	i.attachments = append(i.attachments, a)
	return nil
}

func (i *Input) AttachBytes(data []byte, name string) error {
	a, err := i.store.AttachReader(strings.NewReader(string(data)), name, int64(len(data)))
	if err != nil {
		return err
	}
	i.attachments = append(i.attachments, a)
	return nil
}

func (i *Input) HandleSlashCommand(line string) (handled bool, err error) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "/") {
		return false, nil
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return false, nil
	}
	switch parts[0] {
	case "/attach":
		if len(parts) < 2 {
			return true, fmt.Errorf("usage: /attach <path>")
		}
		for _, p := range parts[1:] {
			if err := i.Attach(p); err != nil {
				return true, err
			}
		}
		return true, nil
	case "/attach-glob":
		if len(parts) < 2 {
			return true, fmt.Errorf("usage: /attach-glob <pattern>")
		}
		matches, err := filepath.Glob(parts[1])
		if err != nil {
			return true, err
		}
		for _, m := range matches {
			if err := i.Attach(m); err != nil {
				return true, err
			}
		}
		return true, nil
	case "/clear":
		i.Clear()
		return true, nil
	case "/detach":
		if len(parts) < 2 {
			return true, fmt.Errorf("usage: /detach <name|index>")
		}
		if err := i.detachByNameOrIndex(parts[1]); err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

func (i *Input) detachByNameOrIndex(ref string) error {
	if len(i.attachments) == 0 {
		return fmt.Errorf("no attachments")
	}
	var idx int = -1
	if n, err := fmt.Sscanf(ref, "%d", &idx); err == nil && n == 1 {
		if idx < 0 || idx >= len(i.attachments) {
			return fmt.Errorf("index out of range")
		}
	} else {
		for j, a := range i.attachments {
			if a.Name == ref {
				idx = j
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("attachment not found: %s", ref)
		}
	}
	i.attachments = append(i.attachments[:idx], i.attachments[idx+1:]...)
	return nil
}

func (i *Input) Update(msg tea.Msg) (tea.Cmd, *SubmitMsg) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+s", "ctrl+enter":
			val := strings.TrimSpace(i.RawValue())
			if strings.HasPrefix(val, "/") {
				handled, err := i.HandleSlashCommand(val)
				if err != nil {
					i.textarea.SetValue("[error: " + err.Error() + "]")
				}
				if handled {
					return nil, nil
				}
			}
			return nil, &SubmitMsg{
				Text:        i.RawValue(),
				Attachments: i.attachments,
			}
		case "ctrl+d":
			if i.textarea.Value() == "" && len(i.attachments) == 0 {
				return tea.Quit, nil
			}
		}
	}
	i.textarea, cmd = i.textarea.Update(msg)
	return cmd, nil
}

type SubmitMsg struct {
	Text        string
	Attachments []*attachments.Attachment
}

func (i *Input) View() string {
	var b strings.Builder
	if len(i.attachments) > 0 {
		b.WriteString("[")
		for idx, a := range i.attachments {
			if idx > 0 {
				b.WriteString(", ")
			}
			b.WriteString(a.Name)
		}
		b.WriteString("]\n")
	}
	b.WriteString(i.textarea.View())
	return b.String()
}

func (i *Input) RenderStatus() string {
	count := len(i.attachments)
	if count == 0 {
		return fmt.Sprintf(" %d chars  0 attachments", len(i.textarea.Value()))
	}
	return fmt.Sprintf(" %d chars  %d attachment(s)", len(i.textarea.Value()), count)
}
