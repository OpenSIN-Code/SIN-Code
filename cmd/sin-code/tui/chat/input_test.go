// SPDX-License-Identifier: MIT
// Purpose: tests for the chat input widget — attach, slash commands, submit.
package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/attachments"
)

func newTestInput(t *testing.T) *Input {
	t.Helper()
	dir := t.TempDir()
	store, err := attachments.NewStoreAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	return NewInput(store)
}

func TestNewInput(t *testing.T) {
	i := newTestInput(t)
	if i == nil {
		t.Fatal("nil input")
	}
}

func TestInputInit(t *testing.T) {
	i := newTestInput(t)
	cmd := i.Init()
	if cmd == nil {
		t.Error("expected non-nil init cmd (textarea.Blink)")
	}
}

func TestInputSetSize(t *testing.T) {
	i := newTestInput(t)
	i.SetSize(100, 10)
	if i.width != 100 {
		t.Errorf("width: %d", i.width)
	}
	if i.height != 10 {
		t.Errorf("height: %d", i.height)
	}
}

func TestInputFocusBlur(t *testing.T) {
	i := newTestInput(t)
	cmd := i.Focus()
	if cmd == nil {
		t.Error("expected focus cmd")
	}
	i.Blur()
}

func TestInputClear(t *testing.T) {
	i := newTestInput(t)
	i.textarea.SetValue("some text")
	i.Clear()
	if i.RawValue() != "" {
		t.Errorf("expected empty, got %q", i.RawValue())
	}
	if len(i.Attachments()) != 0 {
		t.Errorf("expected no attachments")
	}
}

func TestInputAttach(t *testing.T) {
	i := newTestInput(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "test.png")
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 13}
	_ = os.WriteFile(src, png, 0o644)
	if err := i.Attach(src); err != nil {
		t.Fatal(err)
	}
	if len(i.Attachments()) != 1 {
		t.Errorf("expected 1 attachment, got %d", len(i.Attachments()))
	}
}

func TestInputAttachMissingFile(t *testing.T) {
	i := newTestInput(t)
	if err := i.Attach("/nonexistent/file"); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestInputAttachBytes(t *testing.T) {
	i := newTestInput(t)
	data := []byte("hello bytes")
	if err := i.AttachBytes(data, "x.txt"); err != nil {
		t.Fatal(err)
	}
	if len(i.Attachments()) != 1 {
		t.Errorf("got %d", len(i.Attachments()))
	}
}

func TestInputValueWithAttachments(t *testing.T) {
	i := newTestInput(t)
	i.textarea.SetValue("hello")
	i.AttachBytes([]byte("x"), "x.txt")
	val := i.Value()
	if !strings.Contains(val, "hello") {
		t.Error("expected text in value")
	}
	if !strings.Contains(val, "x.txt") {
		t.Error("expected attachment marker in value")
	}
}

func TestInputRawValueExcludesAttachments(t *testing.T) {
	i := newTestInput(t)
	i.textarea.SetValue("hello")
	i.AttachBytes([]byte("x"), "x.txt")
	raw := i.RawValue()
	if strings.Contains(raw, "[file:") {
		t.Error("raw value should not include attachment markers")
	}
}

func TestInputSlashAttach(t *testing.T) {
	i := newTestInput(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(src, []byte("hi"), 0o644)
	handled, err := i.HandleSlashCommand("/attach " + src)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Error("expected handled=true")
	}
	if len(i.Attachments()) != 1 {
		t.Errorf("got %d", len(i.Attachments()))
	}
}

func TestInputSlashAttachMultiple(t *testing.T) {
	i := newTestInput(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	_ = os.WriteFile(a, []byte("a"), 0o644)
	_ = os.WriteFile(b, []byte("b"), 0o644)
	_, err := i.HandleSlashCommand("/attach " + a + " " + b)
	if err != nil {
		t.Fatal(err)
	}
	if len(i.Attachments()) != 2 {
		t.Errorf("got %d", len(i.Attachments()))
	}
}

func TestInputSlashAttachMissingArg(t *testing.T) {
	i := newTestInput(t)
	_, err := i.HandleSlashCommand("/attach")
	if err == nil {
		t.Error("expected error for missing arg")
	}
}

func TestInputSlashAttachGlob(t *testing.T) {
	i := newTestInput(t)
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt", "c.bin"} {
		_ = os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644)
	}
	_, err := i.HandleSlashCommand("/attach-glob " + filepath.Join(dir, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(i.Attachments()) != 2 {
		t.Errorf("expected 2 .txt files, got %d", len(i.Attachments()))
	}
}

func TestInputSlashClear(t *testing.T) {
	i := newTestInput(t)
	i.textarea.SetValue("some text")
	handled, err := i.HandleSlashCommand("/clear")
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Error("expected handled")
	}
	if i.RawValue() != "" {
		t.Error("expected cleared")
	}
}

func TestInputSlashDetachByIndex(t *testing.T) {
	i := newTestInput(t)
	i.AttachBytes([]byte("a"), "a.txt")
	i.AttachBytes([]byte("b"), "b.txt")
	_, err := i.HandleSlashCommand("/detach 0")
	if err != nil {
		t.Fatal(err)
	}
	if len(i.Attachments()) != 1 {
		t.Errorf("expected 1, got %d", len(i.Attachments()))
	}
}

func TestInputSlashDetachByName(t *testing.T) {
	i := newTestInput(t)
	i.AttachBytes([]byte("a"), "a.txt")
	i.AttachBytes([]byte("b"), "b.txt")
	_, err := i.HandleSlashCommand("/detach a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(i.Attachments()) != 1 {
		t.Errorf("expected 1, got %d", len(i.Attachments()))
	}
}

func TestInputSlashDetachInvalidIndex(t *testing.T) {
	i := newTestInput(t)
	i.AttachBytes([]byte("a"), "a.txt")
	_, err := i.HandleSlashCommand("/detach 5")
	if err == nil {
		t.Error("expected error for out of range")
	}
}

func TestInputSlashDetachMissingName(t *testing.T) {
	i := newTestInput(t)
	i.AttachBytes([]byte("a"), "a.txt")
	_, err := i.HandleSlashCommand("/detach notfound.txt")
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestInputSlashDetachNoAttachments(t *testing.T) {
	i := newTestInput(t)
	_, err := i.HandleSlashCommand("/detach 0")
	if err == nil {
		t.Error("expected error when no attachments")
	}
}

func TestInputSlashUnknown(t *testing.T) {
	i := newTestInput(t)
	handled, _ := i.HandleSlashCommand("/notacommand")
	if handled {
		t.Error("expected not handled for unknown command")
	}
}

func TestInputSlashNotACommand(t *testing.T) {
	i := newTestInput(t)
	handled, _ := i.HandleSlashCommand("hello world")
	if handled {
		t.Error("expected not handled for non-slash text")
	}
}

func TestInputViewWithAttachments(t *testing.T) {
	i := newTestInput(t)
	i.textarea.SetValue("hello")
	i.AttachBytes([]byte("x"), "x.txt")
	view := i.View()
	if !strings.Contains(view, "hello") {
		t.Error("view should contain text")
	}
	if !strings.Contains(view, "x.txt") {
		t.Error("view should show attachment")
	}
}

func TestInputViewEmpty(t *testing.T) {
	i := newTestInput(t)
	view := i.View()
	if view == "" {
		t.Error("view should not be empty")
	}
}

func TestInputRenderStatus(t *testing.T) {
	i := newTestInput(t)
	i.textarea.SetValue("hi")
	status := i.RenderStatus()
	if !strings.Contains(status, "2 chars") {
		t.Errorf("expected chars count, got %q", status)
	}
	if !strings.Contains(status, "0 attachments") {
		t.Errorf("expected attachments count, got %q", status)
	}
}

func TestInputRenderStatusWithAttachments(t *testing.T) {
	i := newTestInput(t)
	i.AttachBytes([]byte("x"), "x.txt")
	status := i.RenderStatus()
	if !strings.Contains(status, "1 attachment") {
		t.Errorf("got %q", status)
	}
}

func TestInputUpdateSubmit(t *testing.T) {
	i := newTestInput(t)
	i.textarea.SetValue("hello world")
	_, submit := i.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if submit == nil {
		t.Fatal("expected submit msg")
	}
	if submit.Text != "hello world" {
		t.Errorf("got %q", submit.Text)
	}
}

func TestInputUpdateSlashHandled(t *testing.T) {
	i := newTestInput(t)
	i.textarea.SetValue("/clear")
	_, submit := i.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if submit != nil {
		t.Error("expected no submit for slash command")
	}
	if i.RawValue() != "" {
		t.Error("expected cleared after /clear")
	}
}

func TestInputUpdateOtherKey(t *testing.T) {
	i := newTestInput(t)
	cmd, _ := i.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	_ = cmd
	if !strings.Contains(i.RawValue(), "a") {
		t.Errorf("expected 'a' in value, got %q", i.RawValue())
	}
}
