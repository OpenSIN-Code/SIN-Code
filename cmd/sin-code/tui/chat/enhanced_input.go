// SPDX-License-Identifier: MIT
package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/attachments"
)

const (
	pasteCharThreshold   = 100
	pasteTimeWindow      = 100 * time.Millisecond
	maxInputChars         = 50_000
	maxInputWarnThreshold = 1_000
	historyMaxEntries    = 500
)

type AutoComplete struct {
	mu         sync.Mutex
	active     bool
	candidates []string
	selected   int
	query      string
	source     string
}

func NewAutoComplete() *AutoComplete {
	return &AutoComplete{}
}

func (a *AutoComplete) Active() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.active
}

func (a *AutoComplete) SetActive(b bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.active = b
	if !b {
		a.selected = 0
		a.candidates = nil
		a.query = ""
		a.source = ""
	}
}

func (a *AutoComplete) Candidates() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.candidates))
	copy(out, a.candidates)
	return out
}

func (a *AutoComplete) Selected() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.selected
}

func (a *AutoComplete) MoveUp() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.candidates) == 0 {
		return
	}
	a.selected--
	if a.selected < 0 {
		a.selected = len(a.candidates) - 1
	}
}

func (a *AutoComplete) MoveDown() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.candidates) == 0 {
		return
	}
	a.selected++
	if a.selected >= len(a.candidates) {
		a.selected = 0
	}
}

func (a *AutoComplete) Selection() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.candidates) == 0 || a.selected < 0 || a.selected >= len(a.candidates) {
		return ""
	}
	return a.candidates[a.selected]
}

func (a *AutoComplete) Source() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.source
}

type EnhancedInput struct {
	*Input

	mu             sync.Mutex
	typingStart    time.Time
	typingCount    int
	lastTyping     time.Time
	pasteIndicator string
	pasteDetected  bool
	pasteAt        time.Time

	autoComplete *AutoComplete
	vim          *VimMode

	workspaceDir  string
	slashCommands []string
	history       []string
	historyCursor int

	maxChars      int
	warnThreshold int
}

func NewEnhancedInput(store *attachments.Store) *EnhancedInput {
	return &EnhancedInput{
		Input:         NewInput(store),
		autoComplete:  NewAutoComplete(),
		vim:           NewVimMode(),
		slashCommands: defaultEnhancedSlashCommands(),
		maxChars:      maxInputChars,
		warnThreshold: maxInputWarnThreshold,
	}
}

func defaultEnhancedSlashCommands() []string {
	return []string{
		"/attach", "/attach-glob", "/clear", "/help", "/detach",
		"/search", "/btw", "/undercover", "/model", "/theme",
		"/compact", "/tools", "/sessions", "/dag", "/ctx-viz",
		"/dashboard",
	}
}

func (e *EnhancedInput) VimMode() *VimMode {
	return e.vim
}

func (e *EnhancedInput) AutoComplete() *AutoComplete {
	return e.autoComplete
}

func (e *EnhancedInput) SetWorkspaceDir(dir string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.workspaceDir = dir
}

func (e *EnhancedInput) WorkspaceDir() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.workspaceDir
}

func (e *EnhancedInput) SetSlashCommands(cmds []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.slashCommands = make([]string, len(cmds))
	copy(e.slashCommands, cmds)
}

func (e *EnhancedInput) SlashCommands() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.slashCommands))
	copy(out, e.slashCommands)
	return out
}

func (e *EnhancedInput) SetHistory(hist []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(hist) > historyMaxEntries {
		hist = hist[len(hist)-historyMaxEntries:]
	}
	e.history = make([]string, len(hist))
	copy(e.history, hist)
	e.historyCursor = len(e.history)
}

func (e *EnhancedInput) History() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.history))
	copy(out, e.history)
	return out
}

func (e *EnhancedInput) HistoryCursor() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.historyCursor
}

func (e *EnhancedInput) AddToHistory(text string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if text == "" {
		return
	}
	e.history = append(e.history, text)
	if len(e.history) > historyMaxEntries {
		e.history = e.history[len(e.history)-historyMaxEntries:]
	}
	e.historyCursor = len(e.history)
}

func (e *EnhancedInput) NavigateHistoryUp() {
	e.mu.Lock()
	hist := e.history
	cursor := e.historyCursor
	e.mu.Unlock()
	if len(hist) == 0 {
		return
	}
	if cursor > 0 {
		cursor--
		e.mu.Lock()
		e.historyCursor = cursor
		e.mu.Unlock()
		e.SetValue(hist[cursor])
	}
}

func (e *EnhancedInput) NavigateHistoryDown() {
	e.mu.Lock()
	hist := e.history
	cursor := e.historyCursor
	n := len(e.history)
	e.mu.Unlock()
	if len(hist) == 0 {
		return
	}
	if cursor < n-1 {
		cursor++
		e.mu.Lock()
		e.historyCursor = cursor
		e.mu.Unlock()
		e.SetValue(hist[cursor])
	} else {
		e.mu.Lock()
		e.historyCursor = n
		e.mu.Unlock()
		e.SetValue("")
	}
}

func (e *EnhancedInput) NavigateHistoryWrapUp() {
	e.mu.Lock()
	hist := e.history
	cursor := e.historyCursor
	n := len(e.history)
	e.mu.Unlock()
	if n == 0 {
		return
	}
	if cursor <= 0 {
		e.mu.Lock()
		e.historyCursor = n - 1
		e.mu.Unlock()
		e.SetValue(hist[n-1])
		return
	}
	e.NavigateHistoryUp()
}

func (e *EnhancedInput) NavigateHistoryWrapDown() {
	e.mu.Lock()
	hist := e.history
	cursor := e.historyCursor
	n := len(e.history)
	e.mu.Unlock()
	if n == 0 {
		return
	}
	if cursor >= n {
		e.mu.Lock()
		e.historyCursor = n - 1
		e.mu.Unlock()
		e.SetValue(hist[n-1])
		return
	}
	e.NavigateHistoryDown()
}

func (e *EnhancedInput) PasteIndicator() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pasteIndicator
}

func (e *EnhancedInput) WasPasteDetected() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pasteDetected
}

func DetectPaste(charCount int, elapsed time.Duration) bool {
	return charCount > pasteCharThreshold && elapsed <= pasteTimeWindow
}

func (e *EnhancedInput) RecordTyping(text string, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if text == "" {
		return
	}
	if e.lastTyping.IsZero() || now.Sub(e.lastTyping) > pasteTimeWindow {
		e.typingStart = now
		e.typingCount = len(text)
	} else {
		e.typingCount += len(text)
	}
	e.lastTyping = now

	elapsed := now.Sub(e.typingStart)
	if DetectPaste(e.typingCount, elapsed) {
		e.pasteDetected = true
		e.pasteIndicator = fmt.Sprintf("[pasted %d chars]", e.typingCount)
		e.pasteAt = now
	}
}

func (e *EnhancedInput) ClearPasteIndicator() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pasteDetected = false
	e.pasteIndicator = ""
	e.typingCount = 0
	e.typingStart = time.Time{}
	e.lastTyping = time.Time{}
}

func (e *EnhancedInput) InsertNewline() {
	e.Input.textarea.InsertString("\n")
}

func (e *EnhancedInput) MoveWordLeft() {
	updated, _ := e.Input.textarea.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	e.Input.textarea = updated
}

func (e *EnhancedInput) MoveWordRight() {
	updated, _ := e.Input.textarea.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
	e.Input.textarea = updated
}

func (e *EnhancedInput) DeleteWordBack() {
	updated, _ := e.Input.textarea.Update(tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModAlt})
	e.Input.textarea = updated
}

func (e *EnhancedInput) CharCount() int {
	return len([]rune(e.RawValue()))
}

func (e *EnhancedInput) MaxInputWarning() string {
	count := e.CharCount()
	if count > e.maxChars {
		return fmt.Sprintf("⚠ input exceeds %d chars (%d)", e.maxChars, count)
	}
	return ""
}

func (e *EnhancedInput) SetMaxChars(n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.maxChars = n
}

func (e *EnhancedInput) SetWarnThreshold(n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.warnThreshold = n
}

func (e *EnhancedInput) WarnThreshold() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.warnThreshold
}

func (e *EnhancedInput) Reset() {
	e.Input.Clear()
	e.mu.Lock()
	e.pasteDetected = false
	e.pasteIndicator = ""
	e.typingCount = 0
	e.typingStart = time.Time{}
	e.lastTyping = time.Time{}
	e.historyCursor = len(e.history)
	e.mu.Unlock()
	e.autoComplete.SetActive(false)
	e.vim.Reset()
}

func (e *EnhancedInput) currentWord() (word string, prefix string, isSlash bool, isPath bool) {
	raw := e.RawValue()
	if raw == "" {
		return "", "", false, false
	}
	runes := []rune(raw)
	col := e.Input.textarea.Column()
	if col > len(runes) {
		col = len(runes)
	}
	start := col
	for start > 0 && runes[start-1] != ' ' && runes[start-1] != '\n' && runes[start-1] != '\t' {
		start--
	}
	prefix = string(runes[start:col])
	isSlash = strings.HasPrefix(prefix, "/")
	isPath = strings.HasPrefix(prefix, "./") || strings.HasPrefix(prefix, "/") || strings.HasPrefix(prefix, "../")
	return prefix, prefix, isSlash, isPath
}

func (e *EnhancedInput) completeSlash(prefix string) []string {
	e.mu.Lock()
	cmds := make([]string, len(e.slashCommands))
	copy(cmds, e.slashCommands)
	e.mu.Unlock()
	prefix = strings.ToLower(prefix)
	var matches []string
	for _, cmd := range cmds {
		if strings.HasPrefix(strings.ToLower(cmd), prefix) {
			matches = append(matches, cmd)
		}
	}
	sort.Strings(matches)
	return matches
}

func (e *EnhancedInput) completeFilePaths(prefix string) []string {
	e.mu.Lock()
	dir := e.workspaceDir
	e.mu.Unlock()
	if dir == "" {
		return nil
	}
	var searchDir, filePrefix string
	if strings.HasPrefix(prefix, "/") {
		searchDir = filepath.Dir(prefix)
		filePrefix = filepath.Base(prefix)
	} else if strings.HasPrefix(prefix, "./") {
		searchDir = filepath.Join(dir, filepath.Dir(prefix))
		filePrefix = filepath.Base(prefix)
	} else {
		searchDir = dir
		filePrefix = prefix
	}
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil
	}
	var matches []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(filePrefix, ".") {
			continue
		}
		if strings.HasPrefix(name, filePrefix) {
			full := filepath.Join(searchDir, name)
			if entry.IsDir() {
				full += "/"
			}
			matches = append(matches, full)
		}
	}
	sort.Strings(matches)
	return matches
}

func (e *EnhancedInput) completeHistory(prefix string) []string {
	e.mu.Lock()
	hist := make([]string, len(e.history))
	copy(hist, e.history)
	e.mu.Unlock()
	prefix = strings.ToLower(prefix)
	var matches []string
	seen := make(map[string]bool)
	for i := len(hist) - 1; i >= 0; i-- {
		entry := hist[i]
		if prefix == "" || strings.HasPrefix(strings.ToLower(entry), prefix) {
			if !seen[entry] {
				seen[entry] = true
				matches = append(matches, entry)
			}
		}
		if len(matches) >= 10 {
			break
		}
	}
	return matches
}

func (e *EnhancedInput) Complete() bool {
	_, prefix, isSlash, isPath := e.currentWord()
	if prefix == "" {
		return false
	}

	var candidates []string
	source := ""

	switch {
	case isSlash:
		candidates = e.completeSlash(prefix)
		source = "commands"
	case isPath:
		candidates = e.completeFilePaths(prefix)
		source = "files"
	default:
		candidates = e.completeHistory(prefix)
		source = "history"
	}

	if len(candidates) == 0 {
		e.autoComplete.SetActive(false)
		return false
	}

	if len(candidates) == 1 {
		e.applyCompletion(candidates[0], prefix)
		e.autoComplete.SetActive(false)
		return true
	}

	e.autoComplete.mu.Lock()
	e.autoComplete.candidates = candidates
	e.autoComplete.selected = 0
	e.autoComplete.query = prefix
	e.autoComplete.source = source
	e.autoComplete.active = true
	e.autoComplete.mu.Unlock()
	return true
}

func (e *EnhancedInput) applyCompletion(completion, prefix string) {
	raw := e.RawValue()
	runes := []rune(raw)
	col := e.Input.textarea.Column()
	if col > len(runes) {
		col = len(runes)
	}
	start := col
	for start > 0 && runes[start-1] != ' ' && runes[start-1] != '\n' && runes[start-1] != '\t' {
		start--
	}
	newValue := string(runes[:start]) + completion + string(runes[col:])
	e.SetValue(newValue)
	newCol := start + len([]rune(completion))
	e.Input.textarea.SetCursorColumn(newCol)
}

func (e *EnhancedInput) ConfirmCompletion() bool {
	if !e.autoComplete.Active() {
		return false
	}
	sel := e.autoComplete.Selection()
	query := e.autoComplete.query
	if sel == "" {
		e.autoComplete.SetActive(false)
		return false
	}
	e.applyCompletion(sel, query)
	e.autoComplete.SetActive(false)
	return true
}

func (e *EnhancedInput) Update(msg tea.Msg) (tea.Cmd, *SubmitMsg) {
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		if e.vim.Active() {
			handled, cmd := e.vim.HandleKey(kp, &e.Input.textarea)
			if handled {
				return cmd, nil
			}
			if e.vim.State() == VimNormal {
				return nil, nil
			}
		}

		if e.autoComplete.Active() {
			switch kp.String() {
			case "up", "k":
				e.autoComplete.MoveUp()
				return nil, nil
			case "down", "j":
				e.autoComplete.MoveDown()
				return nil, nil
			case "tab", "enter":
				if e.ConfirmCompletion() {
					return nil, nil
				}
			case "esc":
				e.autoComplete.SetActive(false)
				return nil, nil
			}
		}

		if kp.Code == tea.KeyTab {
			raw := e.RawValue()
			trimmed := strings.TrimSpace(raw)
			if strings.HasPrefix(trimmed, "/attach") && trimmed == "/attach" {
			} else if e.Complete() {
				return nil, nil
			}
		}

		if kp.Mod&tea.ModAlt != 0 {
			switch kp.Code {
			case tea.KeyLeft:
				e.MoveWordLeft()
				return nil, nil
			case tea.KeyRight:
				e.MoveWordRight()
				return nil, nil
			case tea.KeyBackspace:
				e.DeleteWordBack()
				return nil, nil
			}
		}

		if kp.Code == tea.KeyUp {
			line := e.Input.textarea.Line()
			if line == 0 {
				e.NavigateHistoryWrapUp()
				return nil, nil
			}
		}
		if kp.Code == tea.KeyDown {
			line := e.Input.textarea.Line()
			if line >= e.Input.textarea.LineCount()-1 {
				e.NavigateHistoryWrapDown()
				return nil, nil
			}
		}

		if kp.Code == tea.KeyEnter && kp.Mod&tea.ModShift != 0 {
			e.InsertNewline()
			return nil, nil
		}

		if kp.Text != "" && kp.Code != tea.KeyEnter && kp.Code != tea.KeyTab && kp.Code != tea.KeyEscape &&
			kp.Code != tea.KeyBackspace && kp.Code != tea.KeyDelete &&
			kp.Code != tea.KeyUp && kp.Code != tea.KeyDown &&
			kp.Code != tea.KeyLeft && kp.Code != tea.KeyRight {
			e.RecordTyping(kp.Text, time.Now())
		}
	}

	cmd, submit := e.Input.Update(msg)
	if submit != nil {
		e.AddToHistory(submit.Text)
	}
	return cmd, submit
}

func (e *EnhancedInput) View() string {
	var b strings.Builder

	if e.vim.Active() {
		b.WriteString(e.vim.ModeIndicator())
		b.WriteString(" ")
	}

	if e.WasPasteDetected() {
		b.WriteString(e.PasteIndicator())
		b.WriteString(" ")
	}

	count := e.CharCount()
	if count > e.WarnThreshold() {
		b.WriteString(fmt.Sprintf("%d chars", count))
		b.WriteString(" ")
	}

	warning := e.MaxInputWarning()
	if warning != "" {
		b.WriteString(warning)
		b.WriteString(" ")
	}

	header := strings.TrimRight(b.String(), " ")
	if header != "" {
		b.Reset()
		b.WriteString(header)
		b.WriteString("\n")
	} else {
		b.Reset()
	}

	if e.autoComplete.Active() {
		cands := e.autoComplete.Candidates()
		if len(cands) > 0 {
			b.WriteString(fmt.Sprintf("  %s (%d matches)\n", e.autoComplete.Source(), len(cands)))
			for i, c := range cands {
				marker := "  "
				if i == e.autoComplete.Selected() {
					marker = "❯ "
				}
				b.WriteString(marker + c + "\n")
			}
			b.WriteString("  ↑↓ select · Tab/Enter confirm · Esc cancel\n")
			return b.String()
		}
	}

	b.WriteString(e.Input.View())
	return b.String()
}
