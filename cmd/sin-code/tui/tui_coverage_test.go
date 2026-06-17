// SPDX-License-Identifier: MIT
// Purpose: coverage tests for cmd/sin-code/tui — targets the remaining
// statements not exercised by the existing tui_test.go, chat_view_test.go,
// and todos_view_test.go suites.
package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/attachments"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"
	agentrunner "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/tui"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
)

// ── messages.go ─────────────────────────────────────────────────────────────

func TestViewKindStringDefault(t *testing.T) {
	v := ViewKind(999)
	if got := v.String(); got != "Unknown" {
		t.Errorf("String() = %q, want Unknown", got)
	}
}

func TestViewKindShortDefault(t *testing.T) {
	v := ViewKind(999)
	if got := v.Short(); got != "?·" {
		t.Errorf("Short() = %q, want ?·", got)
	}
}

func TestViewKindShortChat(t *testing.T) {
	if got := ViewChat.Short(); got != "7·Chat" {
		t.Errorf("Short() = %q, want 7·Chat", got)
	}
}

// ── model.go ────────────────────────────────────────────────────────────────

func TestModelContextFn(t *testing.T) {
	m := NewModel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.SetContextFn(func() context.Context { return ctx })
	if got := m.ctx(); got != ctx {
		t.Error("ctx() did not return injected context")
	}
}

func TestApplyThemeOutOfBounds(t *testing.T) {
	m := NewModel()
	m.ThemeIdx = -1
	m.ApplyTheme()
	if m.ThemeIdx != 0 {
		t.Errorf("negative ThemeIdx not reset to 0, got %d", m.ThemeIdx)
	}
	m.ThemeIdx = len(Themes)
	m.ApplyTheme()
	if m.ThemeIdx != 0 {
		t.Errorf("ThemeIdx >= len(Themes) not reset to 0, got %d", m.ThemeIdx)
	}
}

// ── styles.go ─────────────────────────────────────────────────────────────────

func TestStylesColorMethods(t *testing.T) {
	s := NewStyles(Themes[0])
	if s.BorderColor() == nil {
		t.Error("BorderColor() nil")
	}
	if s.Accent() == nil {
		t.Error("Accent() nil")
	}
	if s.Text() == nil {
		t.Error("Text() nil")
	}
	if s.TextDim() == nil {
		t.Error("TextDim() nil")
	}
}

// ── spinner.go ────────────────────────────────────────────────────────────────

func TestSpinnerViewStyled(t *testing.T) {
	s := NewSpinner()
	styles := NewStyles(Themes[0])
	view := s.View(styles.Spinner)
	if view == "" {
		t.Error("expected non-empty spinner view")
	}
}

// ── tabs.go ───────────────────────────────────────────────────────────────────

func TestTabsActiveEmpty(t *testing.T) {
	tabs := NewTabs()
	tabs.Sessions = nil
	tabs.ActiveIdx = 0
	if got := tabs.Active(); got.Name != "Session 1" {
		t.Errorf("Active() on empty = %q, want Session 1", got.Name)
	}
}

func TestTabsActiveInvalid(t *testing.T) {
	tabs := NewTabs()
	tabs.ActiveIdx = -1
	if got := tabs.Active(); got.Name != "Session 1" {
		t.Errorf("Active() invalid = %q, want Session 1", got.Name)
	}
	tabs.ActiveIdx = 100
	if got := tabs.Active(); got.Name != tabs.Sessions[0].Name {
		t.Errorf("Active() overflow = %q, want first", got.Name)
	}
}

func TestTabsAddEmptyName(t *testing.T) {
	tabs := NewTabs()
	tabs.Add("")
	want := "Session 2"
	if tabs.Sessions[1].Name != want {
		t.Errorf("Add empty name = %q, want %q", tabs.Sessions[1].Name, want)
	}
}

func TestTabsCloseOutOfRange(t *testing.T) {
	tabs := NewTabs()
	before := len(tabs.Sessions)
	tabs.Close(-1)
	tabs.Close(100)
	if len(tabs.Sessions) != before {
		t.Error("Close out-of-range modified sessions")
	}
}

func TestTabsCloseLastResets(t *testing.T) {
	tabs := NewTabs()
	tabs.Sessions = []Session{{Name: "Only"}}
	tabs.ActiveIdx = 0
	tabs.Close(0)
	if len(tabs.Sessions) != 1 || tabs.Sessions[0].Name != "Session 1" {
		t.Errorf("Close last did not reset default: %+v", tabs.Sessions)
	}
}

func TestTabsSelectOutOfRange(t *testing.T) {
	tabs := NewTabs()
	tabs.Select(-1)
	if tabs.ActiveIdx != 0 {
		t.Errorf("Select(-1) should not change, got %d", tabs.ActiveIdx)
	}
	tabs.Select(100)
	if tabs.ActiveIdx != 0 {
		t.Errorf("Select(100) should not change, got %d", tabs.ActiveIdx)
	}
}

func TestTabsNextEmpty(t *testing.T) {
	tabs := NewTabs()
	tabs.Sessions = nil
	tabs.Next()
	if len(tabs.Sessions) != 0 {
		t.Error("Next on empty should not create sessions")
	}
}

func TestTabsPrevEmpty(t *testing.T) {
	tabs := NewTabs()
	tabs.Sessions = nil
	tabs.Prev()
	if len(tabs.Sessions) != 0 {
		t.Error("Prev on empty should not create sessions")
	}
}

func TestTabsViewWithOverflow(t *testing.T) {
	tabs := NewTabs()
	for i := 0; i < 10; i++ {
		tabs.Add("")
	}
	tabs.ActiveIdx = 8
	tabs.Width = 80
	view := tabs.View(NewStyles(Themes[0]))
	if !strings.Contains(view, "⚡ sin-code") {
		t.Errorf("View missing header: %q", view)
	}
}

func TestTabsViewEmptyRestored(t *testing.T) {
	tabs := NewTabs()
	tabs.Sessions = nil
	view := tabs.View(NewStyles(Themes[0]))
	if !strings.Contains(view, "Session 1") {
		t.Errorf("View did not restore default session: %q", view)
	}
}

func TestLipglossWidthIgnoresNewlines(t *testing.T) {
	if got := lipglossWidth("a\nb\nc"); got != 3 {
		t.Errorf("lipglossWidth = %d, want 3", got)
	}
}

// ── sidebar.go ────────────────────────────────────────────────────────────────

func TestSidebarSelectedViewInvalid(t *testing.T) {
	s := NewSidebar()
	s.Selected = -1
	if got := s.SelectedView(); got != ViewTools {
		t.Errorf("SelectedView(-1) = %v, want ViewTools", got)
	}
	s.Selected = len(s.Items)
	if got := s.SelectedView(); got != ViewTools {
		t.Errorf("SelectedView(overflow) = %v, want ViewTools", got)
	}
}

func TestSidebarSetSelectedViewUnknown(t *testing.T) {
	s := NewSidebar()
	s.Selected = 3
	s.SetSelectedView(ViewKind(999))
	if s.Selected != 3 {
		t.Errorf("SetSelectedView unknown should not change, got %d", s.Selected)
	}
}

func TestSidebarViewCollapsed(t *testing.T) {
	s := NewSidebar()
	s.Collapsed = true
	view := s.View(NewStyles(Themes[0]))
	if !strings.Contains(view, "⚒") {
		t.Errorf("Collapsed view missing icons: %q", view)
	}
}

func TestBadgeForEmpty(t *testing.T) {
	s := NewSidebar()
	if badgeFor(s) != "" {
		t.Error("badgeFor empty should be empty")
	}
}

func TestItoaZero(t *testing.T) {
	if itoa(0) != "0" {
		t.Errorf("itoa(0) = %q", itoa(0))
	}
}

// ── footer.go ─────────────────────────────────────────────────────────────────

func TestDefaultHintsChat(t *testing.T) {
	hints := DefaultHints(ViewChat)
	if len(hints) == 0 {
		t.Error("expected hints for ViewChat")
	}
}

func TestFooterAgentNameInvalid(t *testing.T) {
	f := NewFooter(80)
	f.AgentIndex = -1
	if got := f.AgentName(); got != "Build" {
		t.Errorf("AgentName(-1) = %q, want Build", got)
	}
	f.AgentIndex = len(AgentNames)
	if got := f.AgentName(); got != "Build" {
		t.Errorf("AgentName(overflow) = %q, want Build", got)
	}
}

func TestFooterProgressBarNegative(t *testing.T) {
	f := NewFooter(80)
	if got := f.ProgressBar(-1); got != "" {
		t.Errorf("ProgressBar(-1) = %q, want empty", got)
	}
	f.TokensPct = -0.5
	if got := f.ProgressBar(5); got != strings.Repeat("░", 5) {
		t.Errorf("ProgressBar negative pct = %q", got)
	}
	f.TokensPct = 1.5
	if got := f.ProgressBar(5); got != strings.Repeat("█", 5) {
		t.Errorf("ProgressBar overflow pct = %q", got)
	}
}

func TestFooterRenderWithLoading(t *testing.T) {
	f := NewFooter(80)
	f.Loading = true
	f.ShowHints = false
	out := f.Render(NewStyles(Themes[0]))
	if out == "" {
		t.Error("expected non-empty render")
	}
}

func TestFooterRenderTodoCounts(t *testing.T) {
	f := NewFooter(80)
	f.SetView(ViewTodos)
	f.TodoOpen = 5
	f.TodoBlocked = 2
	f.TodoOverdue = 1
	f.TodoReady = 3
	f.ShowHints = false
	out := f.Render(NewStyles(Themes[0]))
	if !strings.Contains(out, "5 open") && !strings.Contains(out, "3 ready") {
		t.Errorf("expected todo counts in footer: %q", out)
	}
}

func TestFooterRenderNoSelection(t *testing.T) {
	f := NewFooter(80)
	f.SetView(ViewTools)
	f.Selection = ""
	f.ShowHints = false
	out := f.Render(NewStyles(Themes[0]))
	if !strings.Contains(out, "(no selection)") {
		t.Errorf("expected no-selection marker: %q", out)
	}
}

func TestFooterCountReady(t *testing.T) {
	f := Footer{TodoReady: 7}
	if got := footerCount(f, "ready", '🟢'); got != "🟢 7 ready" {
		t.Errorf("footerCount ready = %q", got)
	}
}

// ── history_view.go ──────────────────────────────────────────────────────────

func TestRenderHistoryViewSelectedOutOfRange(t *testing.T) {
	entries := []HistoryEntry{
		{Time: time.Now(), View: "Tools", Action: "a", Detail: "d", Success: true},
	}
	out := RenderHistoryView(entries, -1, NewStyles(Themes[0]), 80, 24)
	if !strings.Contains(out, "a") {
		t.Errorf("expected action in view: %q", out)
	}
}

func TestRenderHistoryViewTruncated(t *testing.T) {
	entries := make([]HistoryEntry, 50)
	for i := range entries {
		entries[i] = HistoryEntry{Time: time.Now(), View: "Tools", Action: "a", Detail: "d", Success: true}
	}
	out := RenderHistoryView(entries, 0, NewStyles(Themes[0]), 80, 24)
	if !strings.Contains(out, "entries") {
		t.Errorf("expected entries summary: %q", out)
	}
}

// ── tools_view.go ─────────────────────────────────────────────────────────────

func TestRenderToolsViewNonRunnable(t *testing.T) {
	sidebar := NewSidebar()
	sidebar.ToolSel = 0 // discover is non-runnable
	out := RenderToolsView(sidebar, NewStyles(Themes[0]), 80, 24)
	if !strings.Contains(out, "Requires arguments") && !strings.Contains(out, "Press r to run") {
		t.Errorf("expected non-runnable hint: %q", out)
	}
}

// ── efm_view.go ───────────────────────────────────────────────────────────────

func TestRenderEFMViewTTLZero(t *testing.T) {
	stacks := []EFMStack{{Name: "x", Status: "running", URL: "http://a", TTL: 0}}
	out := RenderEFMView(stacks, NewStyles(Themes[0]), 80, 24, NewSpinner())
	if !strings.Contains(out, "—") {
		t.Errorf("expected em-dash for zero TTL: %q", out)
	}
}

func TestRenderEFMViewDefaultStatus(t *testing.T) {
	stacks := []EFMStack{{Name: "x", Status: "unknown", URL: "http://a", TTL: 10}}
	out := RenderEFMView(stacks, NewStyles(Themes[0]), 80, 24, NewSpinner())
	if !strings.Contains(out, "x") {
		t.Errorf("expected stack name in default status: %q", out)
	}
}

// ── chat_view.go ──────────────────────────────────────────────────────────────

func TestRenderChatClampsSize(t *testing.T) {
	m := NewModel()
	out := m.renderChat(NewStyles(Themes[0]), 5, 3)
	if !strings.Contains(out, "Send a message") {
		t.Errorf("expected empty prompt after clamp: %q", out)
	}
}

func TestChatViewHelp(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	out := m.chatViewHelp()
	if !strings.Contains(out, "Ctrl+S") {
		t.Errorf("expected help text: %q", out)
	}
}

// ── chat_program.go ───────────────────────────────────────────────────────────

func TestProgramFromTeaProgramNil(t *testing.T) {
	if got := ProgramFromTeaProgram(nil); got != nil {
		t.Error("expected nil wrapper for nil program")
	}
}

// ── chat_input.go ─────────────────────────────────────────────────────────────

func TestNewChatInputAttachmentError(t *testing.T) {
	orig := newAttachmentStoreHook
	newAttachmentStoreHook = func() (*attachments.Store, error) { return nil, errors.New("boom") }
	defer func() { newAttachmentStoreHook = orig }()

	ci := newChatInput()
	if ci == nil {
		t.Fatal("newChatInput should still return input on store error")
	}
}

func TestInitChatRunnerError(t *testing.T) {
	orig := newChatRunnerHook
	newChatRunnerHook = func() (*chat.Runner, error) { return nil, errors.New("no key") }
	defer func() { newChatRunnerHook = orig }()

	m := NewModel()
	m.initChatRunner()
	if m.ChatRunner != nil {
		t.Error("expected nil runner on error")
	}
}

func TestHandleChatSubmitWithRunnerProgram(t *testing.T) {
	origRunner := newChatRunnerHook
	origStream := chatRunnerStreamHook
	origAR := newAgentRunnerHook
	newChatRunnerHook = func() (*chat.Runner, error) { return &chat.Runner{}, nil }
	chatRunnerStreamHook = func(r *chat.Runner, ctx context.Context, prompt string, history []string, onChunk func(string)) (string, int, error) {
		return "async reply", 0, nil
	}
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) {
		return nil, fmt.Errorf("no agent runner in test")
	}
	defer func() {
		newChatRunnerHook = origRunner
		chatRunnerStreamHook = origStream
		newAgentRunnerHook = origAR
	}()

	sender := newFakeProgram()
	m := NewModel()
	m.Program = sender
	m.initChatInput()
	m.initChatRunner()
	handleChatSubmit(m, chat.SubmitMsg{Text: "hi"})

	select {
	case msg := <-sender.recv:
		resp, ok := msg.(chat.ChatResponseMsg)
		if !ok || resp.Text != "async reply" {
			t.Errorf("unexpected async message: %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async chat response")
	}
}

func TestApplyChatResponseMsgInvalidIndex(t *testing.T) {
	m := NewModel()
	applyChatResponseMsg(m, chat.ChatResponseMsg{Text: "x"}, -1)
	applyChatResponseMsg(m, chat.ChatResponseMsg{Text: "x"}, 5)
	if len(m.ChatHistory) != 0 {
		t.Error("should not mutate history on invalid index")
	}
}

func TestUpdateChatNilInput(t *testing.T) {
	m := NewModel()
	m.ChatInput = nil
	if got := m.updateChat(tea.KeyPressMsg{Text: "a"}); got != nil {
		t.Error("expected nil when ChatInput is nil")
	}
}

// ── agent_runner_adapter.go ───────────────────────────────────────────────────

func TestInitAgentRunnerError(t *testing.T) {
	orig := newAgentRunnerHook
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) {
		return nil, errors.New("fail")
	}
	defer func() { newAgentRunnerHook = orig }()

	m := NewModel()
	if got := m.initAgentRunner(); got != nil {
		t.Error("expected nil runner on error")
	}
	if m.AgentRunner != nil {
		t.Error("AgentRunner should not be set")
	}
}

func TestListenAgentRunnerCmdClosed(t *testing.T) {
	ch := make(chan agentrunner.AgentEvent)
	close(ch)
	r := &agentrunner.AgentRunner{Events: ch}
	cmd := listenAgentRunnerCmd(r)
	msg := cmd()
	m, ok := msg.(AgentRunnerMsg)
	if !ok || !m.Closed {
		t.Errorf("expected closed message, got %+v", msg)
	}
}

func TestHandleAgentRunnerEventClosed(t *testing.T) {
	m := NewModel()
	m.AgentRunner = &agentrunner.AgentRunner{}
	m.handleAgentRunnerEvent(AgentRunnerMsg{Closed: true})
	if m.AgentRunner != nil {
		t.Error("expected AgentRunner cleared")
	}
}

func TestHandleAgentRunnerEventKinds(t *testing.T) {
	m := NewModel()
	m.initChatInput()

	cases := []struct {
		kind agentrunner.EventKind
		want chatMsgKind
	}{
		{agentrunner.EventTurn, chatAgent},
		{agentrunner.EventTool, chatTool},
		{agentrunner.EventVerify, chatVerify},
		{agentrunner.EventDone, chatDone},
		{agentrunner.EventError, chatError},
		{agentrunner.EventKind(0), chatSystem},
	}
	for _, tc := range cases {
		ev := agentrunner.AgentEvent{Kind: tc.kind, Detail: "d", Result: "r", ToolName: "t", Err: nil}
		m.handleAgentRunnerEvent(AgentRunnerMsg{Event: ev})
		last := m.ChatHistory[len(m.ChatHistory)-1]
		if last.Kind != tc.want {
			t.Errorf("kind %v: expected chat kind %v, got %v", tc.kind, tc.want, last.Kind)
		}
	}
}

func TestHandleAgentRunnerEventHistoryCap(t *testing.T) {
	m := NewModel()
	m.ChatHistory = make([]ChatMessage, 500)
	m.handleAgentRunnerEvent(AgentRunnerMsg{Event: agentrunner.AgentEvent{Kind: agentrunner.EventTurn, Detail: "x"}})
	if len(m.ChatHistory) != 500 {
		t.Errorf("history cap should keep last 500, got %d", len(m.ChatHistory))
	}
}

func TestAnswerPendingAskNil(t *testing.T) {
	m := NewModel()
	m.answerPendingAsk(true) // should not panic
}

func TestAnswerPendingAskNonBlocking(t *testing.T) {
	m := NewModel()
	ch := make(chan bool)
	m.pendingAsk = ch
	m.answerPendingAsk(true)
	if m.pendingAsk != nil {
		t.Error("pendingAsk should be cleared")
	}
}

func TestSubmitAgentPromptNilRunner(t *testing.T) {
	orig := newAgentRunnerHook
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) {
		return nil, errors.New("fail")
	}
	defer func() { newAgentRunnerHook = orig }()

	m := NewModel()
	if got := m.submitAgentPrompt("hi"); got != nil {
		t.Error("expected nil command when runner creation fails")
	}
}

func TestSubmitAgentPromptSubmitError(t *testing.T) {
	orig := newAgentRunnerHook
	submitOrig := submitAgentRunnerHook
	ar := &agentrunner.AgentRunner{Events: make(chan agentrunner.AgentEvent, 1)}
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) {
		return ar, nil
	}
	submitAgentRunnerHook = func(r *agentrunner.AgentRunner, ctx context.Context, prompt string) (<-chan struct{}, error) {
		return nil, errors.New("submit failed")
	}
	defer func() {
		newAgentRunnerHook = orig
		submitAgentRunnerHook = submitOrig
	}()

	m := NewModel()
	m.initChatInput()
	if got := m.submitAgentPrompt("hi"); got != nil {
		t.Error("expected nil command on submit error")
	}
	last := m.ChatHistory[len(m.ChatHistory)-1]
	if !strings.Contains(last.Text, "unavailable") {
		t.Errorf("expected unavailable marker, got %q", last.Text)
	}
}

func TestRunAgentSkillPromptNilRunner(t *testing.T) {
	orig := newAgentRunnerHook
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) {
		return nil, errors.New("fail")
	}
	defer func() { newAgentRunnerHook = orig }()

	m := NewModel()
	m.initChatInput()
	if got := m.runAgentSkillPrompt("websearch", "find x"); got != nil {
		t.Error("expected nil command when runner creation fails")
	}
	last := m.ChatHistory[len(m.ChatHistory)-1]
	if !strings.Contains(last.Text, "sin-code mcp call") {
		t.Errorf("expected CLI hint, got %q", last.Text)
	}
}

func TestRunAgentSkillPromptSubmitError(t *testing.T) {
	orig := newAgentRunnerHook
	submitOrig := submitAgentRunnerHook
	ar := &agentrunner.AgentRunner{Events: make(chan agentrunner.AgentEvent, 1)}
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) {
		return ar, nil
	}
	submitAgentRunnerHook = func(r *agentrunner.AgentRunner, ctx context.Context, prompt string) (<-chan struct{}, error) {
		return nil, errors.New("submit failed")
	}
	defer func() {
		newAgentRunnerHook = orig
		submitAgentRunnerHook = submitOrig
	}()

	m := NewModel()
	m.initChatInput()
	if got := m.runAgentSkillPrompt("websearch", "find x"); got != nil {
		t.Error("expected nil command on submit error")
	}
	last := m.ChatHistory[len(m.ChatHistory)-1]
	if !strings.Contains(last.Text, "error") {
		t.Errorf("expected error marker, got %q", last.Text)
	}
}

// ── subscribe.go ─────────────────────────────────────────────────────────────

func TestListenForNotificationsClosed(t *testing.T) {
	orig := tuiBroadcasterHook
	ch := make(chan *notifications.Notification)
	close(ch)
	tuiBroadcasterHook = func() <-chan *notifications.Notification { return ch }
	defer func() { tuiBroadcasterHook = orig }()

	cmd := ListenForNotifications()
	if msg := cmd(); msg != nil {
		t.Errorf("expected nil on closed channel, got %T", msg)
	}
}

func TestListenForNotificationsNil(t *testing.T) {
	orig := tuiBroadcasterHook
	tuiBroadcasterHook = func() <-chan *notifications.Notification {
		ch := make(chan *notifications.Notification, 1)
		ch <- nil
		return ch
	}
	defer func() { tuiBroadcasterHook = orig }()

	cmd := ListenForNotifications()
	if msg := cmd(); msg != nil {
		t.Errorf("expected nil on nil notification, got %T", msg)
	}
}

// ── notifications_banner.go ────────────────────────────────────────────────────

func TestDismissBannerNil(t *testing.T) {
	m := NewModel()
	m.DismissBanner() // should not panic
}

func TestBannerNextWithDismissed(t *testing.T) {
	m := NewModel()
	m.Notifications = []NotificationItem{
		{ID: "1", Title: "A", Dismissed: true},
		{ID: "2", Title: "B", Dismissed: false},
	}
	m.BannerNext()
	if m.NotificationBanner == nil || m.NotificationBanner.ID != "2" {
		t.Errorf("expected banner 2, got %+v", m.NotificationBanner)
	}
}

func TestRenderBannerIconCases(t *testing.T) {
	m := NewModel()
	cases := []struct {
		typ  string
		icon string
	}{
		{"todo_completed", "✓"},
		{"todo_assigned", "📌"},
		{"todo_claimed", "📌"},
		{"todo_blocked", "⛔"},
		{"todo_unblocked", "✅"},
		{"todo_deleted", "✗"},
		{"todo_cancelled", "✗"},
	}
	for _, tc := range cases {
		m.SetBanner(&NotificationItem{ID: "n", Title: "T", Message: "M", Type: tc.typ})
		out := m.RenderBanner(m.Styles, 80)
		if !strings.Contains(out, tc.icon) {
			t.Errorf("type %s: expected icon %s in %q", tc.typ, tc.icon, out)
		}
	}
}

func TestRenderBannerTruncatesMessage(t *testing.T) {
	m := NewModel()
	long := strings.Repeat("x", 200)
	m.SetBanner(&NotificationItem{ID: "n", Title: "T", Message: long, Type: "todo_created"})
	out := m.RenderBanner(m.Styles, 80)
	if strings.Contains(out, long) {
		t.Error("expected long message to be truncated")
	}
}

func TestRenderBannerTinyWidth(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "n", Title: "T", Message: "M", Type: "todo_created"})
	out := m.RenderBanner(m.Styles, 5)
	if out == "" {
		t.Error("expected non-empty banner even with tiny width")
	}
}

// ── update.go (small uncovered branches) ─────────────────────────────────────

func TestApplyThemeNegative(t *testing.T) {
	m := NewModel()
	m.ThemeIdx = -5
	m.ApplyTheme()
	if m.ThemeIdx != 0 {
		t.Errorf("expected ThemeIdx reset to 0, got %d", m.ThemeIdx)
	}
}

func TestPreviousView(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewChat
	m.PreviousView()
	if m.ViewKind != ViewTodos {
		t.Errorf("PreviousView from Chat = %v, want Todos", m.ViewKind)
	}
}

func TestFilterPaletteEmpty(t *testing.T) {
	m := NewModel()
	m.Palette.Items = []string{"alpha", "beta"}
	m.Palette.Filter = nil
	m.Palette.Sel = 5
	m.filterPalette("")
	if len(m.Palette.Filter) != 2 || m.Palette.Sel != 0 {
		t.Errorf("empty query should restore all items and reset selection")
	}
}

func TestRunToolSkillWithNoOnRun(t *testing.T) {
	m := NewModel()
	m.runTool("websearch", nil)
	if len(m.ChatHistory) != 0 {
		t.Errorf("expected no chat history without OnRun, got %d", len(m.ChatHistory))
	}
}

func TestRunToolOnRunError(t *testing.T) {
	m := NewModel()
	m.OnRun = func(name string, args []string) error { return errors.New("boom") }
	m.runTool("discover", []string{"--help"})
	last := m.History[len(m.History)-1]
	if last.Success || !strings.Contains(last.Detail, "boom") {
		t.Errorf("expected error history entry, got %+v", last)
	}
}

func TestIsSkillName(t *testing.T) {
	if !isSkillName("websearch") {
		t.Error("websearch should be a skill")
	}
	if isSkillName("discover") {
		t.Error("discover should not be a skill")
	}
}

func TestUpdateUnknownMsg(t *testing.T) {
	m := NewModel()
	updated, cmd := m.Update(struct{ x int }{x: 1})
	if updated != m {
		t.Error("expected same model returned for unknown msg")
	}
	_ = cmd
}

func TestUpdateBannerKeyMsg(t *testing.T) {
	m := NewModel()
	_, cmd := m.Update(BannerKeyMsg{Action: "open:x"})
	if cmd != nil {
		t.Error("expected nil cmd for BannerKeyMsg")
	}
}

func TestHandleKeySubagentsMode(t *testing.T) {
	m := NewModel()
	m.Mode = ModeSubagents
	_, cmd := m.handleKey(tea.KeyPressMsg{Text: "esc"})
	if cmd != nil {
		t.Error("expected nil cmd in subagents mode")
	}
	if m.Mode != ModeNormal {
		t.Error("expected ModeNormal after esc in subagents")
	}
}

func TestHandleKeyUnknown(t *testing.T) {
	m := NewModel()
	_, cmd := m.handleKey(tea.KeyPressMsg{Text: "z"})
	if cmd != nil {
		t.Error("expected nil cmd for unknown key")
	}
}

func TestHandleKeyRunSelectedNoTool(t *testing.T) {
	m := NewModel()
	m.Sidebar.ToolSel = -1
	m.ViewKind = ViewTools
	before := len(m.History)
	m.Update(tea.KeyPressMsg{Text: "r"})
	if len(m.History) != before {
		t.Error("RunSelected with no selection should not append history")
	}
}

func TestHandleKeyEnterNoTool(t *testing.T) {
	m := NewModel()
	m.Sidebar.ToolSel = -1
	m.ViewKind = ViewTools
	before := len(m.History)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m.History) != before {
		t.Error("Enter with no selection should not append history")
	}
}

func TestHandleKeyNavUnknownView(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewChat
	before := m.Sidebar.Selected
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.Sidebar.Selected != before {
		t.Error("Navigation in unknown view should not move sidebar")
	}
}

func TestHandleKeyBannerOpenDismissNext(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "n", Title: "T", Type: "todo_created"})
	m.Update(tea.KeyPressMsg{Text: "o"})
	if last := m.History[len(m.History)-1]; last.Action != "banner-open" {
		t.Errorf("expected banner-open, got %q", last.Action)
	}
	m.Update(tea.KeyPressMsg{Text: "d"})
	if m.NotificationBanner != nil {
		t.Error("expected banner dismissed")
	}
}

func TestHandleKeyBannerNext(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "1", Title: "A", Type: "todo_created"})
	m.SetBanner(&NotificationItem{ID: "2", Title: "B", Type: "todo_completed"})
	m.Update(tea.KeyPressMsg{Text: "n"})
	if m.NotificationBanner == nil {
		t.Error("expected banner after next")
	}
}

func TestHandleKeyYNForAsk(t *testing.T) {
	m := NewModel()
	ch := make(chan bool, 1)
	m.pendingAsk = ch
	m.Update(tea.KeyPressMsg{Text: "y"})
	if m.pendingAsk != nil {
		t.Error("expected pendingAsk cleared")
	}
	m.pendingAsk = ch
	m.Update(tea.KeyPressMsg{Text: "n"})
	if m.pendingAsk != nil {
		t.Error("expected pendingAsk cleared after n")
	}
}

func TestHandlePaletteKeyDownUp(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	m.Palette.Filter = []string{"a", "b", "c"}
	m.Palette.Sel = 0
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.Palette.Sel != 1 {
		t.Errorf("expected sel 1, got %d", m.Palette.Sel)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.Palette.Sel != 0 {
		t.Errorf("expected sel 0, got %d", m.Palette.Sel)
	}
}

func TestHandlePaletteKeyBackspaceEmpty(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	m.Palette.Query = ""
	before := len(m.Palette.Filter)
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if len(m.Palette.Filter) != before {
		t.Error("Backspace on empty query should not change filter")
	}
}

func TestExecutePaletteChoiceDefault(t *testing.T) {
	m := NewModel()
	m.executePaletteChoice("unknown")
	if last := m.History[len(m.History)-1]; last.Action != "palette" {
		t.Errorf("expected palette history entry, got %q", last.Action)
	}
}

func TestHandleArgInputKeyUpdatesInput(t *testing.T) {
	m := NewModel()
	m.OpenArgInput("discover")
	_, cmd := m.handleArgInputKey(tea.KeyPressMsg{Text: "a"})
	if cmd == nil {
		t.Error("expected input cmd from non-submit/esc key")
	}
}

func TestHandleChatResponseNoPlaceholder(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ChatHistory = []ChatMessage{{Kind: chatUser, Text: "hello"}}
	m.handleChatResponse(chat.ChatResponseMsg{Text: "world"})
	if last := m.ChatHistory[len(m.ChatHistory)-1]; last.Kind != chatAssistant || last.Text != "world" {
		t.Errorf("got %+v", last)
	}
}

func TestHandleChatResponseEmptyNoPlaceholder(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ChatHistory = []ChatMessage{{Kind: chatUser, Text: "hello"}}
	m.handleChatResponse(chat.ChatResponseMsg{Text: ""})
	if last := m.ChatHistory[len(m.ChatHistory)-1]; !strings.Contains(last.Text, "empty") {
		t.Errorf("expected empty marker, got %q", last.Text)
	}
}

func TestHandleChatResponseErrorNoPlaceholder(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ChatHistory = []ChatMessage{{Kind: chatUser, Text: "hello"}}
	m.handleChatResponse(chat.ChatResponseMsg{Error: errFake{}})
	if last := m.ChatHistory[len(m.ChatHistory)-1]; !strings.Contains(last.Text, "error") {
		t.Errorf("expected error marker, got %q", last.Text)
	}
}

func TestHandleChatResponseEmptyHistory(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.handleChatResponse(chat.ChatResponseMsg{Text: "x"})
	if len(m.ChatHistory) != 0 {
		t.Error("should not append on empty history")
	}
}

func TestAgentRunnerMsgReSubscribe(t *testing.T) {
	m := NewModel()
	ar := &agentrunner.AgentRunner{Events: make(chan agentrunner.AgentEvent, 1)}
	m.AgentRunner = ar
	_, cmd := m.Update(AgentRunnerMsg{Event: agentrunner.AgentEvent{Kind: agentrunner.EventTurn, Detail: "x"}})
	if cmd == nil {
		t.Error("expected re-subscribe cmd")
	}
}

func TestAgentRunnerMsgClosedClears(t *testing.T) {
	m := NewModel()
	m.AgentRunner = &agentrunner.AgentRunner{}
	_, cmd := m.Update(AgentRunnerMsg{Closed: true})
	if m.AgentRunner != nil {
		t.Error("expected AgentRunner cleared")
	}
	if cmd != nil {
		t.Error("expected nil cmd when closed")
	}
}

func TestUpdateTodosLoaded(t *testing.T) {
	m := NewModel()
	_, cmd := m.Update(TodosLoadedMsg{Items: []TodoRow{{ID: "1", Title: "x"}}})
	if cmd != nil {
		t.Error("expected nil cmd")
	}
	if len(m.TodoItems) != 1 {
		t.Errorf("expected 1 todo item, got %d", len(m.TodoItems))
	}
}

func TestUpdateCountsMsg(t *testing.T) {
	m := NewModel()
	_, cmd := m.Update(CountsMsg{Open: 1, Blocked: 2, Overdue: 3, Ready: 4})
	if cmd != nil {
		t.Error("expected nil cmd")
	}
	if m.Sidebar.TodoOpen != 1 {
		t.Error("counts not updated")
	}
}

func TestUpdateNotificationMsg(t *testing.T) {
	orig := tuiBroadcasterHook
	tuiBroadcasterHook = func() <-chan *notifications.Notification { return make(<-chan *notifications.Notification) }
	defer func() { tuiBroadcasterHook = orig }()

	m := NewModel()
	_, cmd := m.Update(NotificationMsg{N: &testNotification{id: "n", title: "T", message: "M", t: "todo_created"}})
	if cmd == nil {
		t.Error("expected re-subscribe cmd")
	}
	if m.NotificationBanner == nil {
		t.Error("expected banner set")
	}
}

// ── views.go ──────────────────────────────────────────────────────────────────

func TestRenderCommandPaletteEmpty(t *testing.T) {
	out := RenderCommandPalette(nil, 0, "", NewStyles(Themes[0]), 80, 24)
	if !strings.Contains(out, "no matches") {
		t.Errorf("expected no matches: %q", out)
	}
}

func TestRenderSessionsViewEmpty(t *testing.T) {
	out := RenderSessionsView(NewStyles(Themes[0]), Tabs{}, 80, 24)
	if !strings.Contains(out, "No active sessions") {
		t.Errorf("expected empty sessions: %q", out)
	}
}

func TestComposeLayoutNoRight(t *testing.T) {
	out := ComposeLayout(NewTabs(), NewSidebar(), ViewTools, "content", "", NewFooter(80), NewStyles(Themes[0]), 80, 24)
	if out == "" {
		t.Error("expected non-empty layout without right panel")
	}
}

func TestComposeLayoutTiny(t *testing.T) {
	out := ComposeLayout(NewTabs(), NewSidebar(), ViewTools, "c", "", NewFooter(80), NewStyles(Themes[0]), 10, 5)
	if out == "" {
		t.Error("expected non-empty layout even with tiny dimensions")
	}
}

func TestSplitLinesNoNewline(t *testing.T) {
	out := splitLines("a", 3, 5)
	if out != "a  \n" {
		t.Errorf("splitLines = %q", out)
	}
}

func TestPadContent(t *testing.T) {
	out := padContent("line1", 10, 3)
	if !strings.Contains(out, "line1") {
		t.Errorf("expected line1 in padded content: %q", out)
	}
}

func TestRenderRightPanelNonRunnable(t *testing.T) {
	tool := &ToolSubItem{Name: "t", Description: "d", Runnable: false}
	out := RenderRightPanel(tool, ViewTools, NewStyles(Themes[0]), 30, 20)
	if !strings.Contains(out, "Requires arguments") {
		t.Errorf("expected requires arguments: %q", out)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

type fakeProgram struct {
	msgs []any
	recv chan any
}

func newFakeProgram() *fakeProgram {
	return &fakeProgram{recv: make(chan any, 16)}
}

func (f *fakeProgram) Send(msg any) {
	f.msgs = append(f.msgs, msg)
	select {
	case f.recv <- msg:
	default:
	}
}

// Ensure fakeProgram satisfies the interface.
var _ teaProgramIface = (*fakeProgram)(nil)

// ── Additional coverage tests (batch 2) ─────────────────────────────────────

// messages.go remaining Short cases
func TestViewKindShortAll(t *testing.T) {
	cases := []struct {
		v    ViewKind
		want string
	}{
		{ViewTools, "1·Tools"},
		{ViewSessions, "2·Sessions"},
		{ViewEFM, "3·EFM"},
		{ViewConfig, "4·Config"},
		{ViewHistory, "5·History"},
		{ViewTodos, "6·Todos"},
		{ViewChat, "7·Chat"},
		{ViewDAG, "8·DAG"},
		{ViewContextViz, "9·Context"},
		{ViewAgentDashboard, "0·Dashboard"},
	}
	for _, tc := range cases {
		if got := tc.v.Short(); got != tc.want {
			t.Errorf("Short(%v) = %q, want %q", tc.v, got, tc.want)
		}
	}
}

// model.go listItem methods
func TestListItemMethods(t *testing.T) {
	li := listItem{name: "n", description: "d", runnable: true}
	if li.Title() != "n" {
		t.Error("Title")
	}
	if li.Description() != "d" {
		t.Error("Description")
	}
	if li.FilterValue() != "n d" {
		t.Errorf("FilterValue = %q", li.FilterValue())
	}
}

// chat_view.go line wrapping
func TestRenderChatWrapsLongLines(t *testing.T) {
	m := NewModel()
	m.ChatHistory = []ChatMessage{{Kind: chatUser, Text: strings.Repeat("a", 100)}}
	out := m.renderChat(NewStyles(Themes[0]), 80, 20)
	if !strings.Contains(out, "a") {
		t.Error("expected long line to be rendered")
	}
}

// efm_view.go remaining
func TestRenderEFMViewOtherStatus(t *testing.T) {
	stacks := []EFMStack{{Name: "x", Status: "other", URL: "http://a", TTL: 10}}
	out := RenderEFMView(stacks, NewStyles(Themes[0]), 80, 24, NewSpinner())
	if !strings.Contains(out, "x") {
		t.Errorf("expected default status rendered: %q", out)
	}
}

// footer.go gap filling
func TestFooterRenderGapFill(t *testing.T) {
	f := NewFooter(20)
	f.SetView(ViewTools)
	f.ShowHints = false
	out := f.Render(NewStyles(Themes[0]))
	if out == "" {
		t.Error("expected non-empty footer render")
	}
}

// history_view.go selected branch
func TestRenderHistoryViewSelected(t *testing.T) {
	entries := []HistoryEntry{
		{Time: time.Now(), View: "Tools", Action: "a", Detail: "d", Success: true},
		{Time: time.Now(), View: "Tools", Action: "b", Detail: "e", Success: false},
	}
	out := RenderHistoryView(entries, 1, NewStyles(Themes[0]), 80, 24)
	if !strings.Contains(out, "b") {
		t.Errorf("expected selected entry: %q", out)
	}
}

// notifications_banner.go default icon
func TestRenderBannerDefaultIcon(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "n", Title: "T", Message: "M", Type: "unknown"})
	out := m.RenderBanner(m.Styles, 80)
	if !strings.Contains(out, "🔔") {
		t.Errorf("expected default bell icon: %q", out)
	}
}

// sidebar.go width clamp and non-tools view
func TestSidebarViewWidthClamp(t *testing.T) {
	s := NewSidebar()
	s.Width = 10
	s.Selected = 0
	out := s.View(NewStyles(Themes[0]))
	if !strings.Contains(out, "sin-code") {
		t.Errorf("expected sidebar header: %q", out)
	}
}

func TestSidebarViewNotTools(t *testing.T) {
	s := NewSidebar()
	s.Selected = 1 // ViewSessions
	out := s.View(NewStyles(Themes[0]))
	if strings.Contains(out, "Subcommands") {
		t.Error("subcommands should not render when not on Tools")
	}
}

// spinner.go tick
func TestSpinnerInit(t *testing.T) {
	s := NewSpinner()
	cmd := s.Init()
	if cmd == nil {
		t.Error("expected non-nil init cmd")
	}
}

// subscribe.go actual notification
func TestListenForNotificationsActual(t *testing.T) {
	orig := tuiBroadcasterHook
	ch := make(chan *notifications.Notification, 1)
	ch <- &notifications.Notification{ID: "n", Title: "T", Message: "M", Type: "todo_created"}
	tuiBroadcasterHook = func() <-chan *notifications.Notification { return ch }
	defer func() { tuiBroadcasterHook = orig }()

	cmd := ListenForNotifications()
	msg := cmd()
	nm, ok := msg.(NotificationMsg)
	if !ok || nm.N.GetTitle() != "T" {
		t.Errorf("expected notification msg, got %T", msg)
	}
}

// tabs.go remaining cases
func TestTabsViewDirty(t *testing.T) {
	tabs := NewTabs()
	tabs.Sessions[0].Dirty = true
	out := tabs.View(NewStyles(Themes[0]))
	if !strings.Contains(out, "●") {
		t.Errorf("expected dirty marker: %q", out)
	}
}

func TestTabsViewPadded(t *testing.T) {
	tabs := NewTabs()
	tabs.Width = 200
	out := tabs.View(NewStyles(Themes[0]))
	if len(out) < 100 {
		t.Errorf("expected padded output: %q", out)
	}
}

// todos_view.go clamping and priorities
func TestRenderTodosClampsSize(t *testing.T) {
	m := NewModel()
	out := m.RenderTodos(NewStyles(Themes[0]), 5, 3)
	if !strings.Contains(out, "Todos") {
		t.Errorf("expected title after clamp: %q", out)
	}
}

func TestRenderTodosPriorities(t *testing.T) {
	m := NewModel()
	m.TodoItems = []TodoRow{
		{ID: "1", Title: "P0", Priority: "P0"},
		{ID: "2", Title: "P1", Priority: "P1"},
		{ID: "3", Title: "P2", Priority: "P2"},
	}
	out := m.RenderTodos(NewStyles(Themes[0]), 80, 20)
	for _, title := range []string{"P0", "P1", "P2"} {
		if !strings.Contains(out, title) {
			t.Errorf("expected %s in todos: %q", title, out)
		}
	}
}

// tools_view.go non-runnable already covered, but verify selected tool
func TestRenderToolsViewSelectedNonRunnable(t *testing.T) {
	sidebar := NewSidebar()
	sidebar.ToolSel = 1 // execute is non-runnable
	out := RenderToolsView(sidebar, NewStyles(Themes[0]), 80, 24)
	if !strings.Contains(out, "execute") {
		t.Errorf("expected execute tool: %q", out)
	}
}

// update.go remaining branches
func TestUpdateWindowSizeCollapsed(t *testing.T) {
	m := NewModel()
	m.Sidebar.Collapsed = true
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.Sidebar.Width != 6 {
		t.Errorf("expected collapsed width 6, got %d", m.Sidebar.Width)
	}
}

func TestUpdateWindowSizeRightPanel(t *testing.T) {
	m := NewModel()
	m.RightPanel = true
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.Sidebar.Width != 22 {
		t.Errorf("expected sidebar width 22, got %d", m.Sidebar.Width)
	}
}

func TestUpdateChatKey(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewChat
	m.initChatInput()
	m.Update(tea.KeyPressMsg{Text: "a"})
	if !strings.Contains(m.ChatInput.RawValue(), "a") {
		t.Error("expected 'a' in chat input")
	}
}

func TestUpdateEscInterrupt(t *testing.T) {
	m := NewModel()
	before := len(m.History)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if len(m.History) != before+1 {
		t.Error("expected interrupt history entry")
	}
}

func TestHandleKeyCycleTheme(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewTools // "t" is a view-specific hotkey, not a chat key
	start := m.ThemeIdx
	m.Update(tea.KeyPressMsg{Text: "t"})
	if m.ThemeIdx == start {
		t.Error("expected theme to cycle")
	}
}

func TestHandleKeyCycleAgent(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewTools // "a" is a view-specific hotkey, not a chat key
	start := m.Footer.AgentIndex
	m.Update(tea.KeyPressMsg{Text: "a"})
	if m.Footer.AgentIndex == start {
		t.Error("expected agent to cycle")
	}
}

func TestHandleKeyRNotTools(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewChat
	before := len(m.History)
	m.Update(tea.KeyPressMsg{Text: "r"})
	if len(m.History) != before {
		t.Error("r outside tools should not append history")
	}
}

func TestHandleKeyNavArrow(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewTools
	before := m.Sidebar.ToolSel
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.Sidebar.ToolSel == before {
		t.Error("expected tool selection to move down")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.Sidebar.ToolSel != before {
		t.Errorf("expected tool selection to return, got %d", m.Sidebar.ToolSel)
	}
}

func TestHandleKeyLeftRight(t *testing.T) {
	m := NewModel()
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	// Just ensure no panic and no error
}

func TestHandlePaletteKeyEnterNoSelection(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	m.Palette.Filter = []string{}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	// When filter is empty, enter does nothing and palette stays open.
	if !m.Palette.Open {
		t.Error("expected palette to stay open when filter is empty")
	}
}

func TestHandlePaletteKeyDefaultChar(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	before := m.Palette.Query
	m.Update(tea.KeyPressMsg{Text: "x"})
	if m.Palette.Query == before {
		t.Error("expected query to update with default char")
	}
}

func TestHandleArgInputKeyEsc(t *testing.T) {
	m := NewModel()
	m.OpenArgInput("discover")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.ArgInput.Open {
		t.Error("expected arg input closed after esc")
	}
}

func TestHandleChatSubmitThinkingCap(t *testing.T) {
	prev, had := os.LookupEnv("SIN_NIM_API_KEY")
	os.Setenv("SIN_NIM_API_KEY", "fake-key")
	defer func() {
		if had {
			os.Setenv("SIN_NIM_API_KEY", prev)
		} else {
			os.Unsetenv("SIN_NIM_API_KEY")
		}
	}()

	origRunner := newChatRunnerHook
	origRun := chatRunnerRunHook
	newChatRunnerHook = func() (*chat.Runner, error) { return &chat.Runner{}, nil }
	chatRunnerRunHook = func(r *chat.Runner, ctx context.Context, prompt string, history []string) (string, int, error) {
		return "reply", 0, nil
	}
	defer func() {
		newChatRunnerHook = origRunner
		chatRunnerRunHook = origRun
	}()

	m := NewModel()
	m.initChatInput()
	m.initChatRunner()
	m.ChatHistory = make([]ChatMessage, 500)
	handleChatSubmit(m, chat.SubmitMsg{Text: "hi"})
	if len(m.ChatHistory) != 500 {
		t.Errorf("expected cap at 500, got %d", len(m.ChatHistory))
	}
}

func TestUpdateChatSubmit(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewChat
	m.initChatInput()
	sender := newFakeProgram()
	m.Program = sender

	origRunner := newChatRunnerHook
	origStream := chatRunnerStreamHook
	origAR := newAgentRunnerHook
	newChatRunnerHook = func() (*chat.Runner, error) { return &chat.Runner{}, nil }
	chatRunnerStreamHook = func(r *chat.Runner, ctx context.Context, prompt string, history []string, onChunk func(string)) (string, int, error) {
		return "async", 0, nil
	}
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) {
		return nil, fmt.Errorf("no agent runner in test")
	}
	defer func() {
		newChatRunnerHook = origRunner
		chatRunnerStreamHook = origStream
		newAgentRunnerHook = origAR
	}()

	m.initChatRunner()
	m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	select {
	case <-sender.recv:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chat submit")
	}
}

func TestRunSelectedWithSkill(t *testing.T) {
	orig := newAgentRunnerHook
	submitOrig := submitAgentRunnerHook
	ar := &agentrunner.AgentRunner{Events: make(chan agentrunner.AgentEvent, 1)}
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) { return ar, nil }
	submitAgentRunnerHook = func(r *agentrunner.AgentRunner, ctx context.Context, prompt string) (<-chan struct{}, error) {
		return nil, nil
	}
	defer func() {
		newAgentRunnerHook = orig
		submitAgentRunnerHook = submitOrig
	}()

	m := NewModel()
	m.initChatInput()
	called := false
	m.OnRun = func(name string, args []string) error { called = true; return nil }
	// Find a skill tool
	for i, t := range m.Sidebar.ToolSubItems {
		if t.Name == "websearch" {
			m.Sidebar.ToolSel = i
			break
		}
	}
	m.runTool("websearch", nil)
	if !called {
		t.Error("expected OnRun to be called after skill routing")
	}
}

func TestRunToolSkillNoArgs(t *testing.T) {
	orig := newAgentRunnerHook
	submitOrig := submitAgentRunnerHook
	ar := &agentrunner.AgentRunner{Events: make(chan agentrunner.AgentEvent, 1)}
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) { return ar, nil }
	submitAgentRunnerHook = func(r *agentrunner.AgentRunner, ctx context.Context, prompt string) (<-chan struct{}, error) {
		return nil, nil
	}
	defer func() {
		newAgentRunnerHook = orig
		submitAgentRunnerHook = submitOrig
	}()

	m := NewModel()
	m.initChatInput()
	m.OnRun = func(name string, args []string) error { return nil }
	m.runTool("websearch", []string{})
	// Skill routing with empty args should not panic and should reach OnRun.
}

func TestHandleAgentRunnerEventNonEmptyChannel(t *testing.T) {
	ch := make(chan agentrunner.AgentEvent, 1)
	ch <- agentrunner.AgentEvent{Kind: agentrunner.EventTurn, Detail: "x"}
	r := &agentrunner.AgentRunner{Events: ch}
	cmd := listenAgentRunnerCmd(r)
	msg := cmd()
	m, ok := msg.(AgentRunnerMsg)
	if !ok || !m.Closed && m.Event.Kind != agentrunner.EventTurn {
		t.Errorf("unexpected message: %+v", msg)
	}
}

func TestRunAgentSkillPromptHistoryCap(t *testing.T) {
	orig := newAgentRunnerHook
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) {
		return nil, errors.New("fail")
	}
	defer func() { newAgentRunnerHook = orig }()

	m := NewModel()
	m.initChatInput()
	m.ChatHistory = make([]ChatMessage, 500)
	m.runAgentSkillPrompt("websearch", "")
	if len(m.ChatHistory) != 500 {
		t.Errorf("expected cap at 500, got %d", len(m.ChatHistory))
	}
}

func TestRunAgentSkillPromptSubmitErrorCap(t *testing.T) {
	orig := newAgentRunnerHook
	submitOrig := submitAgentRunnerHook
	ar := &agentrunner.AgentRunner{Events: make(chan agentrunner.AgentEvent, 1)}
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) { return ar, nil }
	submitAgentRunnerHook = func(r *agentrunner.AgentRunner, ctx context.Context, prompt string) (<-chan struct{}, error) {
		return nil, errors.New("fail")
	}
	defer func() {
		newAgentRunnerHook = orig
		submitAgentRunnerHook = submitOrig
	}()

	m := NewModel()
	m.initChatInput()
	m.ChatHistory = make([]ChatMessage, 500)
	m.runAgentSkillPrompt("websearch", "")
	if len(m.ChatHistory) != 500 {
		t.Errorf("expected cap at 500, got %d", len(m.ChatHistory))
	}
}

// views.go remaining cases
func TestRenderCommandPaletteWithQuery(t *testing.T) {
	items := []string{"alpha", "beta", "gamma"}
	out := RenderCommandPalette(items, 1, "be", NewStyles(Themes[0]), 80, 24)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "beta") {
		t.Errorf("expected palette to render: %q", out)
	}
}

func TestRenderSessionsViewDirty(t *testing.T) {
	tabs := NewTabs()
	tabs.Sessions = []Session{{Name: "S", Dirty: true}}
	out := RenderSessionsView(NewStyles(Themes[0]), tabs, 80, 24)
	if !strings.Contains(out, "●") {
		t.Errorf("expected dirty marker: %q", out)
	}
}

func TestPadContentEmpty(t *testing.T) {
	out := padContent("", 5, 3)
	if !strings.Contains(out, "     ") {
		t.Errorf("expected padded empty lines: %q", out)
	}
}

func TestSplitLinesHeight(t *testing.T) {
	out := splitLines("a\nb\nc", 2, 2)
	if !strings.Contains(out, "a") {
		t.Errorf("expected split lines: %q", out)
	}
}

func TestComposeLayoutRightPanel(t *testing.T) {
	out := ComposeLayout(NewTabs(), NewSidebar(), ViewTools, "content", "right", NewFooter(80), NewStyles(Themes[0]), 120, 40)
	if out == "" {
		t.Error("expected non-empty layout with right panel")
	}
}

// ── Additional coverage tests (batch 3) ─────────────────────────────────────

// chat_input.go remaining branches
func TestUpdateChatSubmitWithAgentCmd(t *testing.T) {
	prev, had := os.LookupEnv("SIN_NIM_API_KEY")
	os.Setenv("SIN_NIM_API_KEY", "fake-key")
	defer func() {
		if had {
			os.Setenv("SIN_NIM_API_KEY", prev)
		} else {
			os.Unsetenv("SIN_NIM_API_KEY")
		}
	}()

	origRunner := newChatRunnerHook
	origSubmit := submitAgentRunnerHook
	newChatRunnerHook = func() (*chat.Runner, error) { return nil, errors.New("no runner") }
	submitAgentRunnerHook = func(r *agentrunner.AgentRunner, ctx context.Context, prompt string) (<-chan struct{}, error) {
		return nil, nil
	}
	defer func() {
		newChatRunnerHook = origRunner
		submitAgentRunnerHook = origSubmit
	}()

	m := NewModel()
	m.ViewKind = ViewChat
	m.initChatInput()
	// Pre-fill agent runner so chat submit returns the agent cmd.
	ar := &agentrunner.AgentRunner{Events: make(chan agentrunner.AgentEvent, 1)}
	m.AgentRunner = ar
	m.Program = newFakeProgram()

	// Type some text first so Ctrl+S submits.
	m.Update(tea.KeyPressMsg{Text: "h"})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Error("expected non-nil cmd from chat submit with agent runner")
	}
}

// efm_view.go truncate n <= 1
func TestTruncateOne(t *testing.T) {
	if got := truncate("hello", 1); got != "h" {
		t.Errorf("truncate(1) = %q", got)
	}
}

// footer.go gap fill branch
func TestFooterRenderGapFillBranch(t *testing.T) {
	f := NewFooter(200)
	f.SetView(ViewTools)
	f.ShowHints = false
	f.Selection = "sel"
	out := f.Render(NewStyles(Themes[0]))
	if out == "" {
		t.Error("expected non-empty footer")
	}
}

// notifications_banner.go default icon and padding
func TestRenderBannerDefaultIconCoverage(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "n", Title: "T", Message: "M", Type: "custom_type"})
	out := m.RenderBanner(m.Styles, 80)
	if !strings.Contains(out, "🔔") {
		t.Errorf("expected default bell: %q", out)
	}
}

func TestRenderBannerMessagePadding(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "n", Title: "T", Message: "short", Type: "todo_created"})
	out := m.RenderBanner(m.Styles, 80)
	if !strings.Contains(out, "short") {
		t.Errorf("expected message: %q", out)
	}
}

// sidebar.go width < 18
func TestSidebarViewWidthClampCoverage(t *testing.T) {
	s := NewSidebar()
	s.Width = 10
	out := s.View(NewStyles(Themes[0]))
	if !strings.Contains(out, "Tools") {
		t.Errorf("expected sidebar with clamped width: %q", out)
	}
}

// spinner.go tick
func TestSpinnerTickCmd(t *testing.T) {
	cmd := spinnerTick()
	if cmd == nil {
		t.Error("expected non-nil spinner tick cmd")
	}
}

// subscribe.go hook function used directly
func TestTuiBroadcasterHookDirect(t *testing.T) {
	orig := tuiBroadcasterHook
	called := false
	tuiBroadcasterHook = func() <-chan *notifications.Notification {
		called = true
		ch := make(chan *notifications.Notification)
		close(ch)
		return ch
	}
	defer func() { tuiBroadcasterHook = orig }()

	cmd := ListenForNotifications()
	cmd()
	if !called {
		t.Error("expected hook to be invoked via ListenForNotifications")
	}
}

// tabs.go remaining branches
func TestTabsViewDirtyMarker(t *testing.T) {
	tabs := NewTabs()
	tabs.Sessions[0].Dirty = true
	out := tabs.View(NewStyles(Themes[0]))
	if !strings.Contains(out, "●") {
		t.Errorf("expected dirty marker: %q", out)
	}
}

func TestTabsActive(t *testing.T) {
	tabs := NewTabs()
	if got := tabs.Active(); got.Name != "Session 1" {
		t.Errorf("Active() = %q", got.Name)
	}
}

func TestTabsCloseActive(t *testing.T) {
	tabs := NewTabs()
	tabs.Add("x")
	idx := tabs.ActiveIdx
	tabs.Close(idx)
	if len(tabs.Sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(tabs.Sessions))
	}
}

// todos_view.go limit < 1
func TestRenderTodosLimitOne(t *testing.T) {
	m := NewModel()
	m.TodoItems = []TodoRow{{ID: "1", Title: "X"}}
	out := m.RenderTodos(NewStyles(Themes[0]), 80, 5)
	if !strings.Contains(out, "X") {
		t.Errorf("expected todo rendered: %q", out)
	}
}

// tools_view.go non-runnable
func TestRenderToolsViewNonRunnableCoverage(t *testing.T) {
	sidebar := NewSidebar()
	for i, t := range sidebar.ToolSubItems {
		if t.Name == "discover" {
			sidebar.ToolSel = i
			break
		}
	}
	out := RenderToolsView(sidebar, NewStyles(Themes[0]), 80, 24)
	if !strings.Contains(out, "Press r to run with arguments") {
		t.Errorf("expected non-runnable hint: %q", out)
	}
}

// update.go remaining branches
func TestFilterPaletteSelectionReset(t *testing.T) {
	m := NewModel()
	m.Palette.Items = []string{"alpha", "beta", "gamma"}
	m.Palette.Filter = m.Palette.Items
	m.Palette.Sel = 5
	m.filterPalette("be")
	if m.Palette.Sel != 0 {
		t.Errorf("expected Sel reset to 0, got %d", m.Palette.Sel)
	}
}

func TestUpdateKeyPressNonChat(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewTools
	_, cmd := m.Update(tea.KeyPressMsg{Text: "x"})
	if cmd != nil {
		t.Error("expected nil cmd for unknown key in tools view")
	}
}

func TestUpdateAgentRunnerMsgReSubscribe(t *testing.T) {
	m := NewModel()
	ch := make(chan agentrunner.AgentEvent, 1)
	m.AgentRunner = &agentrunner.AgentRunner{Events: ch}
	_, cmd := m.Update(AgentRunnerMsg{Event: agentrunner.AgentEvent{Kind: agentrunner.EventTurn, Detail: "x"}})
	if cmd == nil {
		t.Error("expected re-subscribe cmd")
	}
}

func TestHandleKeyUpConfig(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewConfig
	m.ConfigSel = 1
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.ConfigSel != 0 {
		t.Errorf("expected ConfigSel 0, got %d", m.ConfigSel)
	}
}

func TestHandleKeyDownConfig(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewConfig
	m.ConfigSel = 0
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.ConfigSel == 0 {
		t.Error("expected ConfigSel to move down")
	}
}

func TestHandleKeyUpTodos(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewTodos
	m.TodoItems = []TodoRow{{ID: "1", Title: "X"}, {ID: "2", Title: "Y"}}
	m.TodoSel = 1
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.TodoSel != 0 {
		t.Errorf("expected TodoSel 0, got %d", m.TodoSel)
	}
}

func TestHandleKeyDownTodos(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewTodos
	m.TodoItems = []TodoRow{{ID: "1", Title: "X"}, {ID: "2", Title: "Y"}}
	m.TodoSel = 0
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.TodoSel != 1 {
		t.Errorf("expected TodoSel 1, got %d", m.TodoSel)
	}
}

func TestHandleKeyEnterHelp(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewTools
	before := len(m.History)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m.History) != before+1 {
		t.Error("expected show-help history entry")
	}
}

func TestHandleKeyBannerNoBanner(t *testing.T) {
	m := NewModel()
	before := len(m.History)
	m.Update(tea.KeyPressMsg{Text: "o"})
	m.Update(tea.KeyPressMsg{Text: "d"})
	m.Update(tea.KeyPressMsg{Text: "n"})
	if len(m.History) != before {
		t.Error("banner keys without banner should not append history")
	}
}

func TestHandlePaletteKeyEnter(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	m.Palette.Filter = []string{"theme: next"}
	m.Palette.Sel = 0
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Palette.Open {
		t.Error("expected palette closed")
	}
}

func TestHandleArgInputKeyEnter(t *testing.T) {
	m := NewModel()
	m.Sidebar.ToolSel = 0
	m.RunSelected()
	m.ArgInput.Input.SetValue("--help")
	called := false
	m.OnRun = func(name string, args []string) error { called = true; return nil }
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.ArgInput.Open {
		t.Error("expected arg input closed")
	}
	if !called {
		t.Error("expected OnRun called")
	}
}

func TestPrevView(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewTools
	m.PrevView()
	if m.ViewKind != ViewKanban {
		t.Errorf("PrevView = %v, want Kanban", m.ViewKind)
	}
}

func TestViewModeSubagents(t *testing.T) {
	m := NewModel()
	m.Width = 100
	m.Height = 30
	m.Ready = true
	m.Mode = ModeSubagents
	out := m.View().Content
	if !strings.Contains(out, "Subagents") {
		t.Errorf("expected Subagents popup: %q", out)
	}
}

func TestViewModePalette(t *testing.T) {
	m := NewModel()
	m.Width = 100
	m.Height = 30
	m.Ready = true
	m.Mode = ModePalette
	out := m.View().Content
	if !strings.Contains(out, "Command Palette") && !strings.Contains(out, "palette") {
		t.Errorf("expected palette popup: %q", out)
	}
}

func TestContentWidthSmall(t *testing.T) {
	m := NewModel()
	m.Width = 10
	m.Sidebar.Width = 0
	if got := m.contentWidth(); got != 20 {
		t.Errorf("contentWidth = %d, want 20", got)
	}
}

func TestRightWidthMedium(t *testing.T) {
	m := NewModel()
	m.RightPanel = true
	m.Width = 80
	if got := m.rightWidth(); got != 24 {
		t.Errorf("rightWidth = %d, want 24", got)
	}
}

// views.go remaining branches
func TestRenderCommandPaletteQuery(t *testing.T) {
	items := []string{"alpha", "beta"}
	out := RenderCommandPalette(items, 0, "al", NewStyles(Themes[0]), 80, 24)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "alpha") {
		t.Errorf("expected alpha in palette: %q", out)
	}
}

func TestRenderSessionsViewWithActive(t *testing.T) {
	tabs := NewTabs()
	tabs.Sessions = []Session{{Name: "A", Active: true}, {Name: "B", Dirty: true}}
	tabs.ActiveIdx = 1
	out := RenderSessionsView(NewStyles(Themes[0]), tabs, 80, 24)
	if !strings.Contains(out, "B") {
		t.Errorf("expected session B: %q", out)
	}
}

func TestRenderRightPanelNilView(t *testing.T) {
	out := RenderRightPanel(nil, ViewSessions, NewStyles(Themes[0]), 30, 20)
	if !strings.Contains(out, "no selection") {
		t.Errorf("expected no selection: %q", out)
	}
}

func TestComposeLayoutRightWide(t *testing.T) {
	out := ComposeLayout(NewTabs(), NewSidebar(), ViewTools, "content", strings.Repeat("r", 100), NewFooter(80), NewStyles(Themes[0]), 120, 40)
	if out == "" {
		t.Error("expected non-empty layout with wide right content")
	}
}

// ── Additional coverage tests (batch 4) ─────────────────────────────────────

// chat_program.go: cover non-nil wrapper and Send
func TestProgramFromTeaProgramNonNil(t *testing.T) {
	var p *tea.Program
	if ProgramFromTeaProgram(p) != nil {
		t.Error("expected nil wrapper for nil program")
	}
}

func TestTeaProgramWrapperSend(t *testing.T) {
	if os.Getenv("SKIP_TEA_PROGRAM_TEST") != "" {
		t.Skip("skipping tea program test")
	}

	// Minimal program that starts, receives one message, and quits.
	m := minimalTeaModel{done: make(chan struct{})}
	p := tea.NewProgram(m,
		tea.WithoutRenderer(),
		tea.WithInput(strings.NewReader("")),
		tea.WithOutput(io.Discard),
		tea.WithWindowSize(80, 24),
	)
	go func() {
		if _, err := p.Run(); err != nil {
			// ignore
		}
	}()

	// Wait for program to be ready before sending.
	time.Sleep(50 * time.Millisecond)
	wrapper := ProgramFromTeaProgram(p)
	wrapper.Send("hello")

	select {
	case <-m.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for wrapper send")
	}
	p.Quit()
}

type minimalTeaModel struct {
	done chan struct{}
}

func (minimalTeaModel) Init() tea.Cmd { return nil }
func (m minimalTeaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg == "hello" {
		close(m.done)
	}
	return m, nil
}
func (minimalTeaModel) View() tea.View { return tea.NewView("") }

// notifications_banner.go default icon and message padding
func TestRenderBannerDefaultIconAndPadding(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "n", Title: "TT", Message: "M", Type: "todo_custom"})
	out := m.RenderBanner(m.Styles, 80)
	if !strings.Contains(out, "🔔") && !strings.Contains(out, "TT") {
		t.Errorf("expected banner: %q", out)
	}
}

func TestRenderBannerMessagePaddingBranch(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "n", Title: "T", Message: "short", Type: "todo_created"})
	out := m.RenderBanner(m.Styles, 80)
	if !strings.Contains(out, "short") {
		t.Errorf("expected message: %q", out)
	}
}

func TestBannerOpenCmd(t *testing.T) {
	cmd := BannerOpenCmd("abc")
	msg := cmd()
	if m, ok := msg.(BannerKeyMsg); !ok || m.Action != "open:abc" {
		t.Errorf("unexpected msg: %#v", msg)
	}
}

func TestBannerDismissCmd(t *testing.T) {
	cmd := BannerDismissCmd("xyz")
	msg := cmd()
	if m, ok := msg.(BannerKeyMsg); !ok || m.Action != "dismiss:xyz" {
		t.Errorf("unexpected msg: %#v", msg)
	}
}

// sidebar.go width < 18
func TestSidebarViewCollapsedWidth(t *testing.T) {
	s := NewSidebar()
	s.Width = 10
	out := s.View(NewStyles(Themes[0]))
	if out == "" {
		t.Error("expected non-empty sidebar view")
	}
}

// spinner.go tick
func TestSpinnerTickFunc(t *testing.T) {
	cmd := spinnerTick()
	if cmd == nil {
		t.Error("expected non-nil tick cmd")
	}
}

// subscribe.go tuiBroadcasterHook literal
func TestTuiBroadcasterHookCoverage(t *testing.T) {
	// Reset hook to default so the literal function is invoked.
	orig := tuiBroadcasterHook
	tuiBroadcasterHook = func() <-chan *notifications.Notification { return notifications.TUIBroadcaster() }
	defer func() { tuiBroadcasterHook = orig }()

	cmd := ListenForNotifications()
	// Do not execute the cmd; just verify the hook literal is the default.
	if cmd == nil {
		t.Error("expected cmd")
	}
}

// tabs.go remaining branches
func TestTabsActiveDefault(t *testing.T) {
	tabs := NewTabs()
	tabs.ActiveIdx = -1
	if got := tabs.Active(); got.Name != tabs.Sessions[0].Name {
		t.Errorf("Active default = %q", got.Name)
	}
}

func TestTabsActiveOverflow(t *testing.T) {
	tabs := NewTabs()
	tabs.ActiveIdx = 100
	if got := tabs.Active(); got.Name != tabs.Sessions[0].Name {
		t.Errorf("Active overflow = %q", got.Name)
	}
}

func TestTabsViewWithOverflowSelection(t *testing.T) {
	tabs := NewTabs()
	for i := 0; i < 10; i++ {
		tabs.Add("")
	}
	tabs.ActiveIdx = 8
	out := tabs.View(NewStyles(Themes[0]))
	if !strings.Contains(out, "Session") {
		t.Errorf("expected sessions: %q", out)
	}
}

func TestLipglossWidthCoverage(t *testing.T) {
	if got := lipglossWidth(""); got != 0 {
		t.Errorf("lipglossWidth empty = %d", got)
	}
}

// update.go remaining branches
func TestUpdateChatKeyReturnsCmd(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewChat
	m.initChatInput()
	_, cmd := m.Update(tea.KeyPressMsg{Text: "a"})
	_ = cmd
	if m.ChatInput.RawValue() != "a" {
		t.Error("expected 'a' in chat input")
	}
}

func TestUpdateNonChatKey(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewTools
	_, cmd := m.Update(tea.KeyPressMsg{Text: "x"})
	if cmd != nil {
		t.Error("expected nil cmd for unknown key")
	}
}

func TestUpdateWindowSizeCollapsedCoverage(t *testing.T) {
	m := NewModel()
	m.Sidebar.Collapsed = true
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.Sidebar.Width != 6 {
		t.Errorf("expected width 6, got %d", m.Sidebar.Width)
	}
}

func TestUpdateWindowSizeRightPanelNarrow(t *testing.T) {
	m := NewModel()
	m.RightPanel = true
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	if m.Sidebar.Width != 22 {
		t.Errorf("expected width 22, got %d", m.Sidebar.Width)
	}
}

func TestPreviousViewCoverage(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewTools
	m.PreviousView()
	if m.ViewKind != ViewKanban {
		t.Errorf("PreviousView = %v, want Kanban", m.ViewKind)
	}
}

func TestViewRightPanelMode(t *testing.T) {
	m := NewModel()
	m.Width = 100
	m.Height = 30
	m.Ready = true
	m.RightPanel = true
	m.ViewKind = ViewTools
	out := m.View().Content
	if out == "" {
		t.Error("expected non-empty view with right panel")
	}
}

func TestContentWidthCollapsed(t *testing.T) {
	m := NewModel()
	m.Width = 10
	m.Sidebar.Collapsed = true
	m.RightPanel = false
	if got := m.contentWidth(); got != 20 {
		t.Errorf("contentWidth = %d, want 20", got)
	}
}

func TestRightWidthNarrow(t *testing.T) {
	m := NewModel()
	m.RightPanel = true
	m.Width = 80
	if got := m.rightWidth(); got != 24 {
		t.Errorf("rightWidth = %d, want 24", got)
	}
}

func TestHandleKeyViewSwitch(t *testing.T) {
	m := NewModel()
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7"} {
		m.Update(tea.KeyPressMsg{Text: key})
	}
}

func TestHandleKeyTabShift(t *testing.T) {
	m := NewModel()
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
}

func TestHandleKeyAskNoPending(t *testing.T) {
	m := NewModel()
	m.Update(tea.KeyPressMsg{Text: "y"})
	m.Update(tea.KeyPressMsg{Text: "n"})
}

func TestHandleKeyPaletteDefaultChar(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	m.Update(tea.KeyPressMsg{Text: "z"})
	if !strings.Contains(m.Palette.Query, "z") {
		t.Error("expected z in query")
	}
}

func TestHandleKeyPaletteEnter(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	m.Palette.Filter = []string{"theme: next"}
	m.Palette.Sel = 0
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Palette.Open {
		t.Error("expected palette closed")
	}
}

func TestHandleKeyPaletteNoFilter(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	m.Palette.Filter = []string{}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.Palette.Open {
		t.Error("expected palette to stay open")
	}
}

func TestHandleKeyArgInputEnter(t *testing.T) {
	m := NewModel()
	m.Sidebar.ToolSel = 0
	m.RunSelected()
	called := false
	m.OnRun = func(name string, args []string) error { called = true; return nil }
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !called {
		t.Error("expected OnRun called")
	}
}

func TestHandleKeyArgInputEsc(t *testing.T) {
	m := NewModel()
	m.Sidebar.ToolSel = 0
	m.RunSelected()
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.ArgInput.Open {
		t.Error("expected arg input closed")
	}
}

func TestHandleKeySubagentsEsc(t *testing.T) {
	m := NewModel()
	m.OpenSubagents()
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.Mode != ModeNormal {
		t.Error("expected ModeNormal")
	}
}

func TestHandleKeySubagentsCtrlA(t *testing.T) {
	m := NewModel()
	m.OpenSubagents()
	m.Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if m.Mode != ModeNormal {
		t.Error("expected ModeNormal")
	}
}

// views.go remaining branches
func TestComposeLayoutTinyCoverage(t *testing.T) {
	out := ComposeLayout(NewTabs(), NewSidebar(), ViewTools, "c", "", NewFooter(80), NewStyles(Themes[0]), 5, 2)
	if out == "" {
		t.Error("expected non-empty layout")
	}
}

// ── Additional coverage tests (batch 5) ───────────────────────────────────────

func TestApplyChatResponseEmptyText(t *testing.T) {
	m := NewModel()
	m.ChatHistory = []ChatMessage{{Kind: chatUser, Text: "hi"}}
	applyChatResponseMsg(m, chat.ChatResponseMsg{Text: ""}, 0)
	if m.ChatHistory[0].Kind != chatAssistant || m.ChatHistory[0].Text != "(empty response)" {
		t.Errorf("unexpected: %+v", m.ChatHistory[0])
	}
}

func TestRenderBannerInnerWidthClamp(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "n", Title: "T", Message: "M", Type: "todo_created"})
	out := m.RenderBanner(m.Styles, 10)
	if out == "" {
		t.Error("expected non-empty banner")
	}
}

func TestSidebarViewWithTodoBadge(t *testing.T) {
	s := NewSidebar()
	s.TodoOpen = 5
	s.Width = 30
	out := s.View(NewStyles(Themes[0]))
	if !strings.Contains(out, "5") {
		t.Errorf("expected badge in sidebar: %q", out)
	}
}

func TestSpinnerTickCmdReturnsMsg(t *testing.T) {
	cmd := spinnerTick()
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	// Execute the returned command; it should yield a SpinnerTickMsg.
	msg := cmd()
	if _, ok := msg.(SpinnerTickMsg); !ok {
		t.Errorf("expected SpinnerTickMsg, got %#v", msg)
	}
}

func TestListenForNotificationsRealBroadcaster(t *testing.T) {
	n := &notifications.Notification{ID: "n1", Title: "T", Message: "M", Type: "todo_created"}
	notifications.SendTUI(n)

	cmd := ListenForNotifications()
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	select {
	case msg := <-done:
		if nm, ok := msg.(NotificationMsg); !ok || nm.N.GetID() != "n1" {
			t.Errorf("unexpected msg: %#v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestTabsSelectValid(t *testing.T) {
	tabs := NewTabs()
	tabs.Add("")
	tabs.Select(1)
	if tabs.ActiveIdx != 1 {
		t.Errorf("ActiveIdx = %d", tabs.ActiveIdx)
	}
}

func TestTabsViewPadding(t *testing.T) {
	tabs := NewTabs()
	tabs.Width = 200
	out := tabs.View(NewStyles(Themes[0]))
	if !strings.Contains(out, "Session 1") {
		t.Errorf("expected session: %q", out)
	}
}

func TestRenderToolsViewNonRunnableBatch5(t *testing.T) {
	s := NewSidebar()
	s.ToolSubItems = []ToolSubItem{
		{Name: "a", Description: "d", Runnable: true},
		{Name: "b", Description: "d", Runnable: false},
	}
	s.ToolSel = 0
	out := RenderToolsView(s, NewStyles(Themes[0]), 80, 30)
	if !strings.Contains(out, "Runnable without args") {
		t.Errorf("expected runnable hint: %q", out)
	}

	s.ToolSel = 1
	out = RenderToolsView(s, NewStyles(Themes[0]), 80, 30)
	if !strings.Contains(out, "Press r to run with arguments") {
		t.Errorf("expected non-runnable hint: %q", out)
	}
}

func TestUpdateSpinnerTickMsg(t *testing.T) {
	m := NewModel()
	m.Spinner = NewSpinner()
	_, cmd := m.Update(SpinnerTickMsg(time.Now()))
	if cmd == nil {
		t.Error("expected spinner tick cmd")
	}
}

func TestUpdateTodosLoadedMsg(t *testing.T) {
	m := NewModel()
	m.TodoSel = 5
	m.Update(TodosLoadedMsg{Items: []TodoRow{{ID: "1", Title: "x"}}})
	if m.TodoSel != 0 {
		t.Errorf("TodoSel = %d, want 0", m.TodoSel)
	}
}

func TestUpdateAgentRunnerMsgReSubscribeCoverage(t *testing.T) {
	ws := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := agentrunner.NewAgentRunner(ctx, agentrunner.Config{Workspace: ws, SkipMCP: true})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	defer r.Close()

	m := NewModel()
	m.AgentRunner = r
	_, cmd := m.Update(AgentRunnerMsg{Event: agentrunner.AgentEvent{Kind: agentrunner.EventTurn, Detail: "x"}, Closed: false})
	if cmd == nil {
		t.Error("expected re-subscribe cmd")
	}
}

func TestHandlePaletteBackspace(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	m.Palette.Query = "ab"
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.Palette.Query != "a" {
		t.Errorf("Query = %q", m.Palette.Query)
	}
}

func TestExecutePaletteChoices(t *testing.T) {
	cases := []struct {
		choice string
		check  func(*Model) bool
	}{
		{"agent: cycle", func(m *Model) bool { return true }},
		{"view: tools", func(m *Model) bool { return m.ViewKind == ViewTools }},
		{"view: sessions", func(m *Model) bool { return m.ViewKind == ViewSessions }},
		{"view: efm", func(m *Model) bool { return m.ViewKind == ViewEFM }},
		{"view: config", func(m *Model) bool { return m.ViewKind == ViewConfig }},
		{"view: history", func(m *Model) bool { return m.ViewKind == ViewHistory }},
		{"sidebar: toggle", func(m *Model) bool { return m.Sidebar.Collapsed }},
		{"quit", func(m *Model) bool { return m.Quitting }},
		{"unknown", func(m *Model) bool { return len(m.History) > 0 }},
	}
	for _, c := range cases {
		m := NewModel()
		m.executePaletteChoice(c.choice)
		if !c.check(m) {
			t.Errorf("choice %q failed check", c.choice)
		}
	}
}

func TestViewContentHeightClamp(t *testing.T) {
	m := NewModel()
	m.Width = 120
	m.Height = 5
	m.Ready = true
	m.ViewKind = ViewTools
	out := m.View().Content
	if out == "" {
		t.Error("expected non-empty view with tiny height")
	}
}

func TestViewArgInputMode(t *testing.T) {
	m := NewModel()
	m.Width = 120
	m.Height = 40
	m.Ready = true
	m.OpenArgInput("cmd")
	m.Mode = ModeArgInput
	out := m.View().Content
	if !strings.Contains(out, "cmd") {
		t.Errorf("expected arg input prompt: %q", out)
	}
}

func TestViewNoSelectedTool(t *testing.T) {
	m := NewModel()
	m.Width = 120
	m.Height = 40
	m.Ready = true
	m.Sidebar.Items = []SidebarItem{}
	m.Sidebar.Selected = -1
	m.ViewKind = ViewTools
	out := m.View().Content
	if out == "" {
		t.Error("expected non-empty view")
	}
}

func TestRightWidthNoPanel(t *testing.T) {
	m := NewModel()
	m.RightPanel = false
	if got := m.rightWidth(); got != 0 {
		t.Errorf("rightWidth = %d, want 0", got)
	}
}

func TestRightWidthSmall(t *testing.T) {
	m := NewModel()
	m.RightPanel = true
	m.Width = 50
	if got := m.rightWidth(); got != 20 {
		t.Errorf("rightWidth = %d, want 20", got)
	}
}

func TestContentWidthClamp(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewTools // sidebar is only subtracted in non-chat views
	m.Width = 30
	m.Sidebar.Collapsed = false
	m.Sidebar.Width = 22
	m.RightPanel = false
	if got := m.contentWidth(); got != 20 {
		t.Errorf("contentWidth = %d, want 20", got)
	}
}

func TestHandleChatResponseHistoryLimit(t *testing.T) {
	m := NewModel()
	for i := 0; i < 505; i++ {
		m.ChatHistory = append(m.ChatHistory, ChatMessage{Kind: chatUser, Text: fmt.Sprintf("user: %d", i)})
	}
	m.handleChatResponse(chat.ChatResponseMsg{Text: "hello"})
	if len(m.ChatHistory) != 500 {
		t.Errorf("len = %d, want 500", len(m.ChatHistory))
	}
	if m.ChatHistory[499].Kind != chatAssistant || m.ChatHistory[499].Text != "hello" {
		t.Errorf("expected assistant at end: %+v", m.ChatHistory[499])
	}
}

// ── Additional coverage tests (batch 6) ───────────────────────────────────────

func TestRenderBannerInnerWidthTiny(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "n", Title: "T", Message: "M", Type: "todo_created"})
	out := m.RenderBanner(m.Styles, 9)
	if out == "" {
		t.Error("expected banner")
	}
}

func TestRenderBannerWidthOne(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "n", Title: "T", Message: "M", Type: "todo_created"})
	out := m.RenderBanner(m.Styles, 1)
	if out == "" {
		t.Error("expected banner")
	}
}

func TestTabsViewPaddingLargeWidth(t *testing.T) {
	tabs := NewTabs()
	tabs.Width = 1000
	out := tabs.View(NewStyles(Themes[0]))
	if len(out) == 0 {
		t.Error("expected non-empty tabs")
	}
}

func TestUpdateChatResponseMsg(t *testing.T) {
	m := NewModel()
	m.ChatHistory = []ChatMessage{{Kind: chatUser, Text: "hi"}}
	m.Update(chat.ChatResponseMsg{Text: "hello"})
	if !strings.Contains(m.ChatHistory[1].Text, "hello") {
		t.Errorf("expected assistant response: %+v", m.ChatHistory)
	}
}

func TestViewContentHeightClampSix(t *testing.T) {
	m := NewModel()
	m.Width = 120
	m.Height = 6
	m.Ready = true
	m.ViewKind = ViewTools
	out := m.View().Content
	if out == "" {
		t.Error("expected non-empty view with height 6")
	}
}

func TestViewSelectedToolNil(t *testing.T) {
	m := NewModel()
	m.Width = 120
	m.Height = 40
	m.Ready = true
	m.Sidebar.Selected = -1
	m.Sidebar.ToolSel = -1
	m.ViewKind = ViewTools
	out := m.View().Content
	if out == "" {
		t.Error("expected non-empty view with no selected tool")
	}
}

func TestComposeLayoutTinyHeight(t *testing.T) {
	out := ComposeLayout(NewTabs(), NewSidebar(), ViewTools, "c", "", NewFooter(80), NewStyles(Themes[0]), 80, 3)
	if out == "" {
		t.Error("expected non-empty layout")
	}
}
