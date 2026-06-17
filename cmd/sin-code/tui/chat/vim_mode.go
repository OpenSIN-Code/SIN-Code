// SPDX-License-Identifier: MIT
package chat

import (
	"strings"
	"sync"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

type VimState int

const (
	VimNormal VimState = iota
	VimInsert
	VimVisual
)

const (
	vimNormalStr = "[NORMAL]"
	vimInsertStr = "[INSERT]"
	vimVisualStr = "[VISUAL]"
)

type VimMode struct {
	mu      sync.Mutex
	active  bool
	state   VimState
	pending string
}

func NewVimMode() *VimMode {
	return &VimMode{
		state: VimNormal,
	}
}

func (v *VimMode) Active() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active
}

func (v *VimMode) Toggle() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.active = !v.active
	v.state = VimNormal
	v.pending = ""
}

func (v *VimMode) Enable() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.active = true
	v.state = VimNormal
	v.pending = ""
}

func (v *VimMode) Disable() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.active = false
	v.state = VimNormal
	v.pending = ""
}

func (v *VimMode) State() VimState {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.state
}

func (v *VimMode) SetState(s VimState) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.state = s
	if s != VimNormal {
		v.pending = ""
	}
}

func (v *VimMode) ModeIndicator() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	switch v.state {
	case VimInsert:
		return vimInsertStr
	case VimVisual:
		return vimVisualStr
	default:
		return vimNormalStr
	}
}

func (v *VimMode) Pending() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.pending
}

func (v *VimMode) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.state = VimNormal
	v.pending = ""
}

func (v *VimMode) setPendingLocked(p string) {
	v.pending = p
}

func (v *VimMode) clearPending() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.pending = ""
}

func (v *VimMode) HandleKey(msg tea.KeyPressMsg, ta *textarea.Model) (bool, tea.Cmd) {
	v.mu.Lock()
	if !v.active {
		v.mu.Unlock()
		return false, nil
	}
	state := v.state
	v.mu.Unlock()

	if state == VimInsert {
		if msg.Code == tea.KeyEscape {
			v.SetState(VimNormal)
			return true, nil
		}
		return false, nil
	}

	if state == VimVisual {
		if msg.Code == tea.KeyEscape {
			v.SetState(VimNormal)
			return true, nil
		}
		return false, nil
	}

	return v.handleNormal(msg, ta)
}

func (v *VimMode) handleNormal(msg tea.KeyPressMsg, ta *textarea.Model) (bool, tea.Cmd) {
	text := msg.Text
	code := msg.Code

	pending := v.Pending()

	if pending == "d" {
		switch text {
		case "d":
			v.deleteLine(ta)
			v.clearPending()
			return true, nil
		case "w":
			v.deleteWord(ta)
			v.clearPending()
			return true, nil
		default:
			v.clearPending()
			return true, nil
		}
	}

	switch {
	case text == "i":
		v.SetState(VimInsert)
		return true, nil
	case text == "a":
		v.moveRight(ta)
		v.SetState(VimInsert)
		return true, nil
	case text == "o":
		v.moveToEndOfLine(ta)
		ta.InsertString("\n")
		v.SetState(VimInsert)
		return true, nil
	case text == "h" || code == tea.KeyLeft:
		v.moveLeft(ta)
		return true, nil
	case text == "l" || code == tea.KeyRight:
		v.moveRight(ta)
		return true, nil
	case text == "j" || code == tea.KeyDown:
		v.moveDown(ta)
		return true, nil
	case text == "k" || code == tea.KeyUp:
		v.moveUp(ta)
		return true, nil
	case text == "w":
		v.moveWordForward(ta)
		return true, nil
	case text == "b":
		v.moveWordBackward(ta)
		return true, nil
	case text == "0" || code == tea.KeyHome:
		ta.CursorStart()
		return true, nil
	case text == "$" || code == tea.KeyEnd:
		ta.CursorEnd()
		return true, nil
	case text == "d":
		v.mu.Lock()
		v.setPendingLocked("d")
		v.mu.Unlock()
		return true, nil
	case code == tea.KeyEscape:
		v.clearPending()
		v.SetState(VimNormal)
		return true, nil
	}

	return false, nil
}

func (v *VimMode) moveLeft(ta *textarea.Model) {
	col := ta.Column()
	if col > 0 {
		ta.SetCursorColumn(col - 1)
		return
	}
	line := ta.Line()
	if line > 0 {
		ta.CursorUp()
		ta.CursorEnd()
	}
}

func (v *VimMode) moveRight(ta *textarea.Model) {
	col := ta.Column()
	line := ta.Line()
	val := ta.Value()
	lines := strings.Split(val, "\n")
	if line < len(lines) {
		lineLen := len([]rune(lines[line]))
		if col < lineLen {
			ta.SetCursorColumn(col + 1)
			return
		}
	}
	if line < len(lines)-1 {
		ta.CursorDown()
		ta.CursorStart()
	}
}

func (v *VimMode) moveDown(ta *textarea.Model) {
	line := ta.Line()
	val := ta.Value()
	lines := strings.Split(val, "\n")
	if line < len(lines)-1 {
		ta.CursorDown()
	}
}

func (v *VimMode) moveUp(ta *textarea.Model) {
	line := ta.Line()
	if line > 0 {
		ta.CursorUp()
	}
}

func (v *VimMode) moveWordForward(ta *textarea.Model) {
	val := ta.Value()
	lines := strings.Split(val, "\n")
	line := ta.Line()
	col := ta.Column()
	if line >= len(lines) {
		return
	}
	runes := []rune(lines[line])
	pos := col
	for pos < len(runes) && isWordChar(runes[pos]) {
		pos++
	}
	for pos < len(runes) && !isWordChar(runes[pos]) {
		pos++
	}
	ta.SetCursorColumn(pos)
}

func (v *VimMode) moveWordBackward(ta *textarea.Model) {
	val := ta.Value()
	lines := strings.Split(val, "\n")
	line := ta.Line()
	col := ta.Column()
	if line >= len(lines) {
		return
	}
	runes := []rune(lines[line])
	pos := col
	if pos > 0 {
		pos--
	}
	for pos > 0 && !isWordChar(runes[pos]) {
		pos--
	}
	for pos > 0 && isWordChar(runes[pos-1]) {
		pos--
	}
	ta.SetCursorColumn(pos)
}

func (v *VimMode) moveToEndOfLine(ta *textarea.Model) {
	ta.CursorEnd()
}

func (v *VimMode) deleteLine(ta *textarea.Model) {
	val := ta.Value()
	lines := strings.Split(val, "\n")
	line := ta.Line()
	if line < 0 || line >= len(lines) {
		return
	}
	lines = append(lines[:line], lines[line+1:]...)
	if len(lines) == 0 {
		lines = []string{""}
	}
	ta.SetValue(strings.Join(lines, "\n"))
	if line >= len(lines) {
		line = len(lines) - 1
	}
	ta.CursorStart()
}

func (v *VimMode) deleteWord(ta *textarea.Model) {
	val := ta.Value()
	lines := strings.Split(val, "\n")
	line := ta.Line()
	col := ta.Column()
	if line < 0 || line >= len(lines) {
		return
	}
	runes := []rune(lines[line])
	if col >= len(runes) {
		return
	}
	pos := col
	for pos < len(runes) && !isWordChar(runes[pos]) {
		pos++
	}
	for pos < len(runes) && isWordChar(runes[pos]) {
		pos++
	}
	newLine := string(runes[:col]) + string(runes[pos:])
	lines[line] = newLine
	ta.SetValue(strings.Join(lines, "\n"))
	ta.SetCursorColumn(col)
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}
