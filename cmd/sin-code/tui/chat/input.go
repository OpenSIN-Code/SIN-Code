// SPDX-License-Identifier: MIT
// Purpose: chat input widget — textarea with attachment support and slash
// commands. Used by the TUI 2.0 chat mode (Phase 5 of st-3t5v).
package chat

import (
	"fmt"
	"os"
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
		if msg.Paste {
			i.handlePaste(string(msg.Runes))
			return nil, nil
		}
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

// handlePaste inspects a pasted payload and dispatches it as an attachment
// when possible (raw image bytes or an existing file path). Otherwise the
// payload is inserted as text in the textarea.
//
// This works on Bubbletea v1.3.10 by intercepting the bracketed-paste
// KeyMsg (Type=KeyRunes, Paste=true, Runes=payload) and short-circuiting
// before the textarea swallows it. When sin-code eventually upgrades to
// Bubbletea v2, the same logic can be triggered by a public tea.PasteMsg.
// See docs/bubbletea-v2-migration.md.
//
// TODO(st-bvm3): Bubbletea v2 has a native tea.PasteMsg that carries raw
// bytes (no UTF-8 corruption). When we migrate to v2, this synthetic Paste
// flag detection in the KeyMsg Update() path (line ~179) should be replaced
// with `case tea.PasteMsg:`.
// Track at: docs/issues/st-bvm3-bubbletea-v2-migration.md
// Plan:      docs/plans/bubbletea-v2-upgrade.md
// Target:    v3.0.0 — pure migration, no functional change.
func (i *Input) handlePaste(content string) {
	if i.isImageBytes(content) {
		name := "pasted-" + imageExt(content)
		if err := i.AttachBytes([]byte(content), name); err == nil {
			return
		}
	}
	if i.isFilePath(content) {
		if err := i.Attach(content); err == nil {
			return
		}
	}
	i.textarea.InsertString(content)
}

// HandlePasteBytes is a public entry-point for paste events that carry raw
// bytes (e.g. an image dragged into the terminal, or a future Bubbletea v2
// tea.PasteMsg adapter). The Bubbletea v1 KeyMsg.Paste path runs the
// payload through utf8.DecodeRune which corrupts non-UTF-8 image data, so
// programmatic callers (tests, future drag-and-drop handlers) should use
// this method to preserve byte fidelity.
func (i *Input) HandlePasteBytes(data []byte) {
	if i.isImageBytes(string(data)) {
		name := "pasted-" + imageExt(string(data))
		if err := i.AttachBytes(data, name); err == nil {
			return
		}
	}
	if i.isFilePath(string(data)) {
		if err := i.Attach(string(data)); err == nil {
			return
		}
	}
	i.textarea.InsertString(string(data))
}

// isImageBytes reports whether the payload starts with the magic bytes
// of a known image format (PNG, JPEG, GIF, WebP).
func (i *Input) isImageBytes(content string) bool {
	b := []byte(content)
	if len(b) >= 4 {
		if b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G' {
			return true
		}
		if b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
			return true
		}
		if string(b[:4]) == "RIFF" && len(b) >= 12 && string(b[8:12]) == "WEBP" {
			return true
		}
	}
	if len(b) >= 6 {
		if string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a" {
			return true
		}
	}
	return false
}

// isFilePath reports whether content looks like a single filesystem path
// (starts with /, ~/, or ./, contains no newlines) and resolves to an
// existing regular file.
func (i *Input) isFilePath(content string) bool {
	trimmed := strings.TrimRight(content, "\r\n\t ")
	if trimmed == "" || strings.ContainsAny(trimmed, "\r\n") {
		return false
	}
	if !(strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "~/") || strings.HasPrefix(trimmed, "./")) {
		return false
	}
	if strings.HasPrefix(trimmed, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		trimmed = filepath.Join(home, trimmed[2:])
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func imageExt(content string) string {
	b := []byte(content)
	if len(b) >= 4 && b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G' {
		return "png"
	}
	if len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
		return "jpg"
	}
	if len(b) >= 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a") {
		return "gif"
	}
	if len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return "webp"
	}
	return "bin"
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
