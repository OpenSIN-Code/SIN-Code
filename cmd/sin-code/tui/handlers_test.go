// SPDX-License-Identifier: MIT
package tui

import (
	"os"
	"strings"
	"testing"
)

func TestRenderDiffPopupViewEmpty(t *testing.T) {
	ClearDiffs()
	out := RenderDiffPopupView(NewStyles(Themes[0]), 80, 24)
	if !strings.Contains(out, "No recent") {
		t.Errorf("expected empty message, got %q", out)
	}
}

func TestRenderDiffPopupViewWithDiffs(t *testing.T) {
	ClearDiffs()
	RecordDiff("/test/file.go", "old", "new", "sin_edit")
	out := RenderDiffPopupView(NewStyles(Themes[0]), 80, 24)
	if !strings.Contains(out, "file.go") {
		t.Errorf("expected file name in popup, got %q", out)
	}
	ClearDiffs()
}

func TestHandleGitRefresh(t *testing.T) {
	m := NewModel()
	HandleGitRefresh(m, GitRefreshMsg{Status: GitStatus{Branch: "main", Ahead: 2}})
	if m.Footer.GitBranch == "" {
		t.Error("expected git branch to be set")
	}
	if !strings.Contains(m.Footer.GitBranch, "main") {
		t.Errorf("expected 'main' in branch, got %q", m.Footer.GitBranch)
	}
	if !strings.Contains(m.Footer.GitBranch, "↑2") {
		t.Errorf("expected ahead indicator, got %q", m.Footer.GitBranch)
	}
}

func TestHandleGitRefreshEmptyBranch(t *testing.T) {
	m := NewModel()
	HandleGitRefresh(m, GitRefreshMsg{Status: GitStatus{Branch: ""}})
	if m.Footer.GitBranch != "" {
		t.Errorf("expected empty branch, got %q", m.Footer.GitBranch)
	}
}

func TestInitGitRefresh(t *testing.T) {
	cmd := InitGitRefresh()
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
}

func TestHandleFilePreviewMsg(t *testing.T) {
	tmp := t.TempDir() + "/test.txt"
	os.WriteFile(tmp, []byte("hello world"), 0644)

	m := NewModel()
	HandleFilePreviewMsg(m, FilePreviewMsg{Path: tmp})
	if m.FilePreview == "" {
		t.Error("expected file preview to be loaded")
	}
	if !strings.Contains(m.FilePreview, "hello") {
		t.Errorf("expected file content, got %q", m.FilePreview)
	}
	if m.FilePreviewPath != tmp {
		t.Errorf("expected path %q, got %q", tmp, m.FilePreviewPath)
	}
}

func TestHandleFilePreviewMsgMissingFile(t *testing.T) {
	m := NewModel()
	HandleFilePreviewMsg(m, FilePreviewMsg{Path: "/nonexistent/path/file.txt"})
	if m.FilePreview == "" {
		t.Error("expected error message in preview")
	}
	if !strings.Contains(m.FilePreview, "Error") {
		t.Errorf("expected error message, got %q", m.FilePreview)
	}
}

func TestRenderFilePreviewEmpty(t *testing.T) {
	m := NewModel()
	out := RenderFilePreview(m, NewStyles(Themes[0]), 80, 24)
	if out != "" {
		t.Errorf("expected empty string when no preview, got %q", out)
	}
}

func TestRenderFilePreviewWithContent(t *testing.T) {
	m := NewModel()
	m.FilePreview = "hello world"
	m.FilePreviewPath = "/tmp/test.go"
	out := RenderFilePreview(m, NewStyles(Themes[0]), 80, 24)
	if !strings.Contains(out, "test.go") {
		t.Errorf("expected file path, got %q", out)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected content, got %q", out)
	}
}

func TestHandleDiffPopup(t *testing.T) {
	cmd := HandleDiffPopup()
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(DiffPopupMsg); !ok {
		t.Errorf("expected DiffPopupMsg, got %T", msg)
	}
}

func TestHandleFilePreviewCmd(t *testing.T) {
	cmd := HandleFilePreview("/some/path.go")
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
	msg := cmd()
	fpm, ok := msg.(FilePreviewMsg)
	if !ok {
		t.Errorf("expected FilePreviewMsg, got %T", msg)
	}
	if fpm.Path != "/some/path.go" {
		t.Errorf("expected path '/some/path.go', got %q", fpm.Path)
	}
}
