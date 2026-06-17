// SPDX-License-Identifier: MIT
package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/attachments"
)

var osUserHomeDirHook = os.UserHomeDir

type pickerEntry struct {
	name  string
	isDir bool
	isUp  bool
}

type filePickerState struct {
	active   bool
	items    []pickerEntry
	selected int
	cwd      string
	err      error
}

type Input struct {
	textarea      textarea.Model
	attachments   []*attachments.Attachment
	store         *attachments.Store
	width         int
	height        int
	placeholder   string
	history       []string
	historyCursor int
	filePicker    filePickerState
}

func NewInput(store *attachments.Store) *Input {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (Enter to send · Shift+Enter for newline · /attach for files)"
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.CharLimit = 100_000
	ta.Focus()

	styles := textarea.DefaultStyles(true)
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styles.Focused.CursorLine = lipgloss.NewStyle()
	styles.Focused.EndOfBuffer = lipgloss.NewStyle().Foreground(lipgloss.Color("236"))
	styles.Focused.Text = lipgloss.NewStyle()
	styles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	styles.Blurred.EndOfBuffer = lipgloss.NewStyle().Foreground(lipgloss.Color("236"))
	ta.SetStyles(styles)

	return &Input{
		textarea:    ta,
		store:       store,
		width:       80,
		height:      3,
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
	i.filePicker.active = false
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
		return false, nil
	case "/help":
		return false, nil
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
	case tea.PasteMsg:
		i.handlePaste(msg.Content)
		return nil, nil
	case tea.KeyPressMsg:
		if i.filePicker.active {
			switch msg.String() {
			case "up", "k":
				i.movePicker(-1)
				return nil, nil
			case "down", "j":
				i.movePicker(1)
				return nil, nil
			case "enter":
				i.confirmPicker()
				return nil, nil
			case "esc":
				i.filePicker.active = false
				return nil, nil
			}
			return nil, nil
		}
		switch msg.String() {
		case "ctrl+s", "ctrl+enter":
			return i.submit()
		case "enter":
			return i.submit()
		case "shift+enter":
			i.textarea.InsertString("\n")
			return nil, nil
		case "ctrl+d":
			if i.textarea.Value() == "" && len(i.attachments) == 0 {
				return tea.Quit, nil
			}
		case "up":
			if i.textarea.Line() == 0 && len(i.history) > 0 {
				if i.historyCursor > 0 {
					i.historyCursor--
					i.textarea.SetValue(i.history[i.historyCursor])
				}
				return nil, nil
			}
		case "down":
			if i.textarea.Line() >= i.textarea.LineCount()-1 && len(i.history) > 0 {
				if i.historyCursor < len(i.history)-1 {
					i.historyCursor++
					i.textarea.SetValue(i.history[i.historyCursor])
				} else {
					i.historyCursor = len(i.history)
					i.textarea.Reset()
				}
				return nil, nil
			}
		case "tab":
			if strings.TrimSpace(i.RawValue()) == "/attach" {
				i.enterFilePicker()
				return nil, nil
			}
		}
	}
	i.textarea, cmd = i.textarea.Update(msg)
	return cmd, nil
}

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

func (i *Input) isFilePath(content string) bool {
	trimmed := strings.TrimRight(content, "\r\n\t ")
	if trimmed == "" || strings.ContainsAny(trimmed, "\r\n") {
		return false
	}
	if !(strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "~/") || strings.HasPrefix(trimmed, "./")) {
		return false
	}
	if strings.HasPrefix(trimmed, "~/") {
		home, err := osUserHomeDirHook()
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

func (i *Input) submit() (tea.Cmd, *SubmitMsg) {
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
	text := i.RawValue()
	if text != "" {
		i.history = append(i.history, text)
		i.historyCursor = len(i.history)
	}
	return nil, &SubmitMsg{
		Text:        text,
		Attachments: i.attachments,
	}
}

func (i *Input) View() string {
	var b strings.Builder
	if len(i.attachments) > 0 {
		b.WriteString(i.renderAttachmentChips())
		b.WriteString("\n")
	}
	if i.filePicker.active {
		b.WriteString(i.filePickerView())
		return b.String()
	}
	raw := i.textarea.Value()
	lineCount := strings.Count(raw, "\n") + 1
	b.WriteString("  ")
	if lineCount > 1 {
		b.WriteString(fmt.Sprintf("%d lines · ", lineCount))
	}
	b.WriteString(fmt.Sprintf("%d chars", len(raw)))
	if len(i.attachments) > 0 {
		b.WriteString(fmt.Sprintf(" · %d attach", len(i.attachments)))
	}
	b.WriteString("\n")
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

func formatAttachmentSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
}

func attachmentIcon(name, mime string) string {
	if strings.HasPrefix(mime, "image/") {
		return "🖼"
	}
	if strings.HasPrefix(mime, "text/") {
		return "📄"
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return "🖼"
	case ".go", ".py", ".js", ".ts", ".rs", ".java", ".c", ".cpp", ".txt", ".md":
		return "📄"
	case ".zip", ".tar", ".gz", ".tgz", ".bin", ".exe", ".o", ".a", ".so", ".dylib", ".pdf":
		return "📦"
	}
	if mime != "" {
		return "📦"
	}
	return "📎"
}

func (i *Input) renderAttachmentChips() string {
	var b strings.Builder
	n := len(i.attachments)
	limit := n
	if n > 3 {
		limit = 2
	}
	for idx := 0; idx < limit; idx++ {
		if idx > 0 {
			b.WriteString("  ")
		}
		a := i.attachments[idx]
		b.WriteString(fmt.Sprintf("%s %s (%s)", attachmentIcon(a.Name, a.MIME), a.Name, formatAttachmentSize(a.Size)))
	}
	if n > 3 {
		b.WriteString(fmt.Sprintf("  and %d more...", n-limit))
	}
	return b.String()
}

func (i *Input) enterFilePicker() {
	i.textarea.Reset()
	fp := &i.filePicker
	fp.active = true
	fp.selected = 0
	if fp.cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			fp.cwd = wd
		} else {
			fp.cwd = "."
		}
	}
	i.loadPickerItems()
}

func (i *Input) loadPickerItems() {
	fp := &i.filePicker
	entries, err := os.ReadDir(fp.cwd)
	if err != nil {
		fp.items = nil
		fp.err = err
		return
	}
	fp.err = nil
	items := make([]pickerEntry, 0, len(entries)+1)
	if filepath.Dir(fp.cwd) != fp.cwd {
		items = append(items, pickerEntry{name: "..", isUp: true})
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		items = append(items, pickerEntry{name: e.Name(), isDir: e.IsDir()})
	}
	fp.items = items
	if fp.selected >= len(items) {
		fp.selected = len(items) - 1
	}
	if fp.selected < 0 {
		fp.selected = 0
	}
}

func (i *Input) movePicker(delta int) {
	n := len(i.filePicker.items)
	if n == 0 {
		return
	}
	i.filePicker.selected += delta
	if i.filePicker.selected < 0 {
		i.filePicker.selected = 0
	}
	if i.filePicker.selected >= n {
		i.filePicker.selected = n - 1
	}
}

func (i *Input) confirmPicker() {
	fp := &i.filePicker
	if fp.selected < 0 || fp.selected >= len(fp.items) {
		fp.active = false
		return
	}
	it := fp.items[fp.selected]
	if it.isUp {
		fp.cwd = filepath.Dir(fp.cwd)
		fp.selected = 0
		i.loadPickerItems()
		return
	}
	if it.isDir {
		fp.cwd = filepath.Join(fp.cwd, it.name)
		fp.selected = 0
		i.loadPickerItems()
		return
	}
	path := filepath.Join(fp.cwd, it.name)
	_ = i.Attach(path)
	fp.active = false
}

func (i *Input) filePickerView() string {
	var b strings.Builder
	fp := &i.filePicker
	b.WriteString("Attach a file — ↑↓ navigate · Enter select · Esc cancel\n")
	b.WriteString(fmt.Sprintf("  %s\n", fp.cwd))
	if fp.err != nil {
		b.WriteString(fmt.Sprintf("  (error: %v)\n", fp.err))
		return b.String()
	}
	items := fp.items
	if len(items) == 0 {
		b.WriteString("  (no files)\n")
		return b.String()
	}
	maxRows := 12
	if i.height > maxRows {
		maxRows = i.height
	}
	start := 0
	if fp.selected >= maxRows {
		start = fp.selected - maxRows + 1
	}
	end := start + maxRows
	if end > len(items) {
		end = len(items)
	}
	for idx := start; idx < end; idx++ {
		it := items[idx]
		marker := "  "
		if idx == fp.selected {
			marker = "❯ "
		}
		suffix := ""
		if it.isUp {
			suffix = " (up)"
		} else if it.isDir {
			suffix = "/"
		}
		b.WriteString(fmt.Sprintf("%s%s%s\n", marker, it.name, suffix))
	}
	return b.String()
}

