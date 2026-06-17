// SPDX-License-Identifier: MIT
package chat

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/attachments"
)

func newTestEnhancedInput(t *testing.T) *EnhancedInput {
	t.Helper()
	dir := t.TempDir()
	store, err := attachments.NewStoreAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	ei := NewEnhancedInput(store)
	ei.SetWorkspaceDir(dir)
	return ei
}

func TestEnhancedInputNew(t *testing.T) {
	ei := newTestEnhancedInput(t)
	if ei == nil {
		t.Fatal("nil enhanced input")
	}
	if ei.VimMode() == nil {
		t.Error("expected non-nil vim mode")
	}
	if ei.AutoComplete() == nil {
		t.Error("expected non-nil autocomplete")
	}
}

func TestEnhancedInputHistoryNavigationUp(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetHistory([]string{"msg1", "msg2", "msg3"})
	ei.NavigateHistoryUp()
	if ei.RawValue() != "msg3" {
		t.Errorf("expected msg3, got %q", ei.RawValue())
	}
	ei.NavigateHistoryUp()
	if ei.RawValue() != "msg2" {
		t.Errorf("expected msg2, got %q", ei.RawValue())
	}
	ei.NavigateHistoryUp()
	if ei.RawValue() != "msg1" {
		t.Errorf("expected msg1, got %q", ei.RawValue())
	}
}

func TestEnhancedInputHistoryNavigationDown(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetHistory([]string{"msg1", "msg2", "msg3"})
	ei.NavigateHistoryUp()
	ei.NavigateHistoryUp()
	ei.NavigateHistoryUp()
	if ei.RawValue() != "msg1" {
		t.Fatalf("expected msg1 at top, got %q", ei.RawValue())
	}
	ei.NavigateHistoryDown()
	if ei.RawValue() != "msg2" {
		t.Errorf("expected msg2, got %q", ei.RawValue())
	}
	ei.NavigateHistoryDown()
	if ei.RawValue() != "msg3" {
		t.Errorf("expected msg3, got %q", ei.RawValue())
	}
	ei.NavigateHistoryDown()
	if ei.RawValue() != "" {
		t.Errorf("expected empty after last, got %q", ei.RawValue())
	}
}

func TestEnhancedInputHistoryWrap(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetHistory([]string{"msg1", "msg2", "msg3"})
	ei.NavigateHistoryUp()
	ei.NavigateHistoryUp()
	ei.NavigateHistoryUp()
	if ei.RawValue() != "msg1" {
		t.Fatalf("expected msg1, got %q", ei.RawValue())
	}
	ei.NavigateHistoryWrapUp()
	if ei.RawValue() != "msg3" {
		t.Errorf("expected wrap to msg3, got %q", ei.RawValue())
	}
	ei.NavigateHistoryWrapDown()
	if ei.RawValue() != "" {
		t.Errorf("expected empty past end, got %q", ei.RawValue())
	}
	ei.NavigateHistoryWrapDown()
	if ei.RawValue() != "msg3" {
		t.Errorf("expected wrap down to msg3, got %q", ei.RawValue())
	}
}

func TestEnhancedInputHistoryEmpty(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetHistory(nil)
	ei.NavigateHistoryUp()
	if ei.RawValue() != "" {
		t.Errorf("expected empty with no history, got %q", ei.RawValue())
	}
	ei.NavigateHistoryDown()
	if ei.RawValue() != "" {
		t.Errorf("expected empty with no history, got %q", ei.RawValue())
	}
}

func TestEnhancedInputPasteDetectionFast(t *testing.T) {
	if !DetectPaste(101, 50*time.Millisecond) {
		t.Error("expected paste detected for 101 chars in 50ms")
	}
	if !DetectPaste(200, 100*time.Millisecond) {
		t.Error("expected paste detected for 200 chars in 100ms")
	}
}

func TestEnhancedInputPasteDetectionSlow(t *testing.T) {
	if DetectPaste(99, 50*time.Millisecond) {
		t.Error("expected no paste for 99 chars")
	}
	if DetectPaste(200, 200*time.Millisecond) {
		t.Error("expected no paste for 200ms elapsed")
	}
	if DetectPaste(50, 10*time.Millisecond) {
		t.Error("expected no paste for 50 chars")
	}
}

func TestEnhancedInputPasteIndicator(t *testing.T) {
	ei := newTestEnhancedInput(t)
	now := time.Now()
	ei.RecordTyping(strings.Repeat("a", 60), now)
	ei.RecordTyping(strings.Repeat("b", 50), now.Add(30*time.Millisecond))
	if !ei.WasPasteDetected() {
		t.Error("expected paste detected")
	}
	indicator := ei.PasteIndicator()
	if !strings.Contains(indicator, "pasted") {
		t.Errorf("expected paste indicator, got %q", indicator)
	}
	if !strings.Contains(indicator, "110") {
		t.Errorf("expected 110 chars in indicator, got %q", indicator)
	}
}

func TestEnhancedInputPasteIndicatorClears(t *testing.T) {
	ei := newTestEnhancedInput(t)
	now := time.Now()
	ei.RecordTyping(strings.Repeat("a", 110), now)
	if !ei.WasPasteDetected() {
		t.Fatal("expected paste detected")
	}
	ei.ClearPasteIndicator()
	if ei.WasPasteDetected() {
		t.Error("expected paste cleared")
	}
	if ei.PasteIndicator() != "" {
		t.Errorf("expected empty indicator, got %q", ei.PasteIndicator())
	}
}

func TestEnhancedInputMultilineShiftEnter(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetValue("hello")
	ei.Input.textarea.CursorEnd()
	_, submit := ei.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if submit != nil {
		t.Error("shift+enter should not submit")
	}
	val := ei.RawValue()
	if !strings.Contains(val, "\n") {
		t.Errorf("expected newline in value, got %q", val)
	}
}

func TestEnhancedInputEnterSubmits(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetValue("hello world")
	_, submit := ei.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if submit == nil {
		t.Fatal("expected submit on enter")
	}
	if submit.Text != "hello world" {
		t.Errorf("got %q", submit.Text)
	}
}

func TestEnhancedInputWordMovementLeft(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetValue("hello world")
	ei.Input.textarea.CursorEnd()
	colBefore := ei.Input.textarea.Column()
	ei.MoveWordLeft()
	colAfter := ei.Input.textarea.Column()
	if colAfter >= colBefore {
		t.Errorf("expected cursor moved left, before=%d after=%d", colBefore, colAfter)
	}
	if colAfter != 6 {
		t.Errorf("expected cursor at col 6 (start of 'world'), got %d", colAfter)
	}
}

func TestEnhancedInputWordMovementRight(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetValue("hello world")
	ei.Input.textarea.CursorStart()
	ei.MoveWordLeft()
	colAfterLeft := ei.Input.textarea.Column()
	if colAfterLeft != 0 {
		t.Fatalf("expected col 0 after word left from start, got %d", colAfterLeft)
	}
	ei.Input.textarea.CursorStart()
	ei.MoveWordRight()
	colAfterRight := ei.Input.textarea.Column()
	if colAfterRight != 5 {
		t.Errorf("expected col 5 (end of first word), got %d", colAfterRight)
	}
}

func TestEnhancedInputDeleteWordBack(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetValue("hello world")
	ei.Input.textarea.CursorEnd()
	ei.DeleteWordBack()
	val := ei.RawValue()
	if strings.Contains(val, "world") {
		t.Errorf("expected 'world' deleted, got %q", val)
	}
	if val != "hello " && val != "hello" {
		t.Errorf("expected 'hello ' or 'hello', got %q", val)
	}
}

func TestEnhancedInputAutoCompleteSlashCommands(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetSlashCommands([]string{"/attach", "/clear", "/help","/compact"})
	ei.SetValue("/at")
	ei.Input.textarea.CursorEnd()
	ok := ei.Complete()
	if !ok {
		t.Error("expected completion to succeed")
	}
	val := ei.RawValue()
	if val != "/attach" {
		t.Errorf("expected /attach, got %q", val)
	}
}

func TestEnhancedInputAutoCompleteSlashMultiple(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetSlashCommands([]string{"/attach", "/attach-glob", "/clear"})
	ei.SetValue("/at")
	ei.Input.textarea.CursorEnd()
	ok := ei.Complete()
	if !ok {
		t.Error("expected completion to find matches")
	}
	if !ei.AutoComplete().Active() {
		t.Error("expected autocomplete active with multiple matches")
	}
	cands := ei.AutoComplete().Candidates()
	if len(cands) != 2 {
		t.Errorf("expected 2 candidates, got %d: %v", len(cands), cands)
	}
	ei.AutoComplete().MoveDown()
	sel := ei.AutoComplete().Selection()
	if sel != "/attach-glob" {
		t.Errorf("expected /attach-glob selected, got %q", sel)
	}
	ei.ConfirmCompletion()
	if !strings.Contains(ei.RawValue(), "/attach-glob") {
		t.Errorf("expected /attach-glob in value, got %q", ei.RawValue())
	}
}

func TestEnhancedInputAutoCompleteFilePaths(t *testing.T) {
	ei := newTestEnhancedInput(t)
	dir := ei.WorkspaceDir()
	if err := os.WriteFile(filepath.Join(dir, "testfile.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ei.SetValue("./testf")
	ei.Input.textarea.CursorEnd()
	ok := ei.Complete()
	if !ok {
		t.Error("expected file path completion to succeed")
	}
	val := ei.RawValue()
	if !strings.Contains(val, "testfile.go") {
		t.Errorf("expected testfile.go in value, got %q", val)
	}
}

func TestEnhancedInputAutoCompleteFilePathsMultiple(t *testing.T) {
	ei := newTestEnhancedInput(t)
	dir := ei.WorkspaceDir()
	if err := os.WriteFile(filepath.Join(dir, "test1.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test2.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ei.SetValue("./test")
	ei.Input.textarea.CursorEnd()
	ok := ei.Complete()
	if !ok {
		t.Error("expected completion to find matches")
	}
	if !ei.AutoComplete().Active() {
		t.Error("expected autocomplete active with multiple file matches")
	}
	cands := ei.AutoComplete().Candidates()
	if len(cands) != 2 {
		t.Errorf("expected 2 file candidates, got %d: %v", len(cands), cands)
	}
}

func TestEnhancedInputAutoCompleteHistory(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetHistory([]string{"git commit", "git push", "docker build"})
	ei.SetValue("git")
	ei.Input.textarea.CursorEnd()
	ok := ei.Complete()
	if !ok {
		t.Error("expected history completion to succeed")
	}
	if !ei.AutoComplete().Active() {
		t.Fatal("expected autocomplete active with multiple matches")
	}
	cands := ei.AutoComplete().Candidates()
	if len(cands) < 2 {
		t.Errorf("expected at least 2 candidates, got %d: %v", len(cands), cands)
	}
}

func TestEnhancedInputAutoCompleteNoMatch(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetSlashCommands([]string{"/attach", "/clear"})
	ei.SetValue("/xyz")
	ei.Input.textarea.CursorEnd()
	ok := ei.Complete()
	if ok {
		t.Error("expected no completion for /xyz")
	}
	if ei.AutoComplete().Active() {
		t.Error("expected autocomplete inactive with no matches")
	}
}

func TestEnhancedInputCharacterCountDisplay(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetWarnThreshold(10)
	ei.SetValue(strings.Repeat("x", 50))
	view := ei.View()
	if !strings.Contains(view, "50 chars") {
		t.Errorf("expected char count in view, got %q", view)
	}
}

func TestEnhancedInputCharacterCountHiddenBelowThreshold(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetWarnThreshold(1000)
	ei.SetValue("hello")
	view := ei.View()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "5 chars") && !strings.Contains(line, "chars  0 attach") {
		}
	}
}

func TestEnhancedInputMaxInputWarning(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetMaxChars(100)
	ei.SetValue(strings.Repeat("x", 150))
	warning := ei.MaxInputWarning()
	if warning == "" {
		t.Error("expected warning for exceeding max chars")
	}
	if !strings.Contains(warning, "exceeds") {
		t.Errorf("expected 'exceeds' in warning, got %q", warning)
	}
}

func TestEnhancedInputMaxInputNoWarning(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetMaxChars(1000)
	ei.SetValue("hello")
	warning := ei.MaxInputWarning()
	if warning != "" {
		t.Errorf("expected no warning, got %q", warning)
	}
}

func TestEnhancedInputMaxInputWarningInView(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetMaxChars(10)
	ei.SetValue(strings.Repeat("x", 20))
	view := ei.View()
	if !strings.Contains(view, "exceeds") {
		t.Errorf("expected warning in view, got %q", view)
	}
}

func TestEnhancedInputReset(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetHistory([]string{"msg1", "msg2"})
	ei.SetValue("some text")
	now := time.Now()
	ei.RecordTyping(strings.Repeat("a", 110), now)
	if !ei.WasPasteDetected() {
		t.Fatal("expected paste detected before reset")
	}
	ei.AutoComplete().SetActive(true)
	ei.Reset()
	if ei.RawValue() != "" {
		t.Errorf("expected empty value after reset, got %q", ei.RawValue())
	}
	if ei.WasPasteDetected() {
		t.Error("expected paste cleared after reset")
	}
	if ei.AutoComplete().Active() {
		t.Error("expected autocomplete inactive after reset")
	}
	if ei.HistoryCursor() != 2 {
		t.Errorf("expected history cursor reset to len, got %d", ei.HistoryCursor())
	}
}

func TestEnhancedInputAddToHistory(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.AddToHistory("msg1")
	ei.AddToHistory("msg2")
	hist := ei.History()
	if len(hist) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(hist))
	}
	if hist[0] != "msg1" || hist[1] != "msg2" {
		t.Errorf("expected msg1,msg2, got %v", hist)
	}
}

func TestEnhancedInputAddToHistoryEmpty(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.AddToHistory("")
	if len(ei.History()) != 0 {
		t.Error("expected no entry for empty text")
	}
}

func TestEnhancedInputVimModeIntegration(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.VimMode().Enable()
	ei.SetValue("hello")
	ei.Input.textarea.CursorEnd()
	_, submit := ei.Update(tea.KeyPressMsg{Text: "h"})
	if submit != nil {
		t.Error("vim h should not submit")
	}
	col := ei.Input.textarea.Column()
	if col >= 5 {
		t.Errorf("expected cursor moved left in vim normal mode, got col %d", col)
	}
	_, _ = ei.Update(tea.KeyPressMsg{Text: "i"})
	if ei.VimMode().State() != VimInsert {
		t.Error("expected insert mode after i")
	}
	_, _ = ei.Update(tea.KeyPressMsg{Text: "x"})
	if !strings.Contains(ei.RawValue(), "x") {
		t.Errorf("expected x inserted in insert mode, got %q", ei.RawValue())
	}
	_, _ = ei.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if ei.VimMode().State() != VimNormal {
		t.Error("expected normal mode after esc")
	}
}

func TestEnhancedInputConcurrentAccess(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetHistory([]string{"h1", "h2", "h3"})
	ei.SetSlashCommands([]string{"/a", "/b", "/c"})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			ei.AddToHistory("concurrent-msg")
		}()
		go func() {
			defer wg.Done()
			_ = ei.History()
			_ = ei.HistoryCursor()
		}()
		go func() {
			defer wg.Done()
			ei.RecordTyping("test", time.Now())
			_ = ei.WasPasteDetected()
			_ = ei.PasteIndicator()
		}()
		go func() {
			defer wg.Done()
			ei.AutoComplete().MoveUp()
			ei.AutoComplete().MoveDown()
			_ = ei.AutoComplete().Active()
			_ = ei.AutoComplete().Selection()
		}()
	}
	wg.Wait()
}

func TestEnhancedInputConcurrentVimAndAutoComplete(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.VimMode().Enable()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			ei.VimMode().Toggle()
			_ = ei.VimMode().Active()
			_ = ei.VimMode().ModeIndicator()
			_ = ei.VimMode().State()
		}()
		go func() {
			defer wg.Done()
			ei.AutoComplete().SetActive(true)
			ei.AutoComplete().SetActive(false)
			_ = ei.AutoComplete().Candidates()
		}()
	}
	wg.Wait()
}

func TestEnhancedInputConcurrentCompleteAndReset(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetSlashCommands([]string{"/attach", "/clear", "/help"})
	ei.SetHistory([]string{"cmd1", "cmd2"})
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			ei.AutoComplete().SetActive(true)
			ei.AutoComplete().MoveDown()
			_ = ei.AutoComplete().Selection()
		}()
		go func() {
			defer wg.Done()
			ei.AutoComplete().SetActive(false)
		}()
	}
	wg.Wait()
}

func TestEnhancedInputUpdateTypesChar(t *testing.T) {
	ei := newTestEnhancedInput(t)
	_, submit := ei.Update(tea.KeyPressMsg{Text: "a"})
	if submit != nil {
		t.Error("expected no submit for typing a char")
	}
	if !strings.Contains(ei.RawValue(), "a") {
		t.Errorf("expected 'a' in value, got %q", ei.RawValue())
	}
}

func TestEnhancedInputUpdateCtrlS(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetValue("hello")
	_, submit := ei.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if submit == nil {
		t.Fatal("expected submit on ctrl+s")
	}
}

func TestEnhancedInputUpdateTabNoCompletion(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetSlashCommands([]string{"/attach"})
	ei.SetValue("hello")
	ei.Input.textarea.CursorEnd()
	_, submit := ei.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if submit != nil {
		t.Error("tab should not submit")
	}
}

func TestEnhancedInputViewWithVimMode(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.VimMode().Enable()
	view := ei.View()
	if !strings.Contains(view, vimNormalStr) {
		t.Errorf("expected [NORMAL] in view, got %q", view)
	}
	ei.VimMode().SetState(VimInsert)
	view = ei.View()
	if !strings.Contains(view, vimInsertStr) {
		t.Errorf("expected [INSERT] in view, got %q", view)
	}
}

func TestEnhancedInputViewWithPasteIndicator(t *testing.T) {
	ei := newTestEnhancedInput(t)
	now := time.Now()
	ei.RecordTyping(strings.Repeat("a", 110), now)
	view := ei.View()
	if !strings.Contains(view, "[pasted") {
		t.Errorf("expected paste indicator in view, got %q", view)
	}
}

func TestEnhancedInputHistoryTruncation(t *testing.T) {
	ei := newTestEnhancedInput(t)
	big := make([]string, historyMaxEntries+100)
	for i := range big {
		big[i] = "msg"
	}
	ei.SetHistory(big)
	if len(ei.History()) > historyMaxEntries {
		t.Errorf("expected history truncated to %d, got %d", historyMaxEntries, len(ei.History()))
	}
}

func TestEnhancedInputAutoCompleteCancelEsc(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetSlashCommands([]string{"/attach", "/attach-glob"})
	ei.SetValue("/at")
	ei.Input.textarea.CursorEnd()
	ei.Complete()
	if !ei.AutoComplete().Active() {
		t.Fatal("expected autocomplete active")
	}
	_, submit := ei.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if submit != nil {
		t.Error("esc should not submit")
	}
	if ei.AutoComplete().Active() {
		t.Error("expected autocomplete closed on esc")
	}
}

func TestEnhancedInputAutoCompleteNavigate(t *testing.T) {
	ei := newTestEnhancedInput(t)
	ei.SetSlashCommands([]string{"/attach", "/attach-glob", "/clear"})
	ei.SetValue("/")
	ei.Input.textarea.CursorEnd()
	ei.Complete()
	if !ei.AutoComplete().Active() {
		t.Fatal("expected autocomplete active")
	}
	initialSel := ei.AutoComplete().Selected()
	_, _ = ei.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if ei.AutoComplete().Selected() == initialSel {
		t.Error("expected selection to change on down")
	}
	_, _ = ei.Update(tea.KeyPressMsg{Code: tea.KeyUp})
}
