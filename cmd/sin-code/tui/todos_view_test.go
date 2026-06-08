// SPDX-License-Identifier: MIT
// Purpose: tests for the Todos view, notification banner, and TUI subscription.
package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestViewTodosAdded(t *testing.T) {
	if ViewTodos.String() != "Todos" {
		t.Errorf("expected 'Todos', got %q", ViewTodos.String())
	}
	if ViewTodos.Short() != "6·Todos" {
		t.Errorf("expected '6·Todos', got %q", ViewTodos.Short())
	}
}

func TestSidebarHasTodos(t *testing.T) {
	items := DefaultSidebarItems()
	hasTodos := false
	for _, it := range items {
		if it.View == ViewTodos {
			hasTodos = true
			if it.Shortcut != "6" {
				t.Errorf("expected shortcut 6, got %q", it.Shortcut)
			}
		}
	}
	if !hasTodos {
		t.Error("expected Todos in default sidebar items")
	}
}

func TestNextViewCyclesAll6(t *testing.T) {
	m := NewModel()
	seen := map[ViewKind]bool{}
	for i := 0; i < 6; i++ {
		seen[m.ViewKind] = true
		m.NextView()
	}
	if len(seen) != 6 {
		t.Errorf("expected 6 unique views, got %d", len(seen))
	}
}

func TestPrevViewCyclesAll6(t *testing.T) {
	m := NewModel()
	seen := map[ViewKind]bool{}
	for i := 0; i < 6; i++ {
		seen[m.ViewKind] = true
		m.PrevView()
	}
	if len(seen) != 6 {
		t.Errorf("expected 6 unique views, got %d", len(seen))
	}
}

func TestSwitchToTodosVia6(t *testing.T) {
	m := NewModel()
	m.Update(tea.KeyPressMsg{Text: "6"})
	if m.ViewKind != ViewTodos {
		t.Errorf("expected ViewTodos, got %v", m.ViewKind)
	}
}

func TestTodoItemsAndSelection(t *testing.T) {
	m := NewModel()
	m.SwitchView(ViewTodos)
	m.TodoItems = []TodoRow{
		{ID: "st-aaaa", Title: "Fix bug", Priority: "P0", Status: "open", Type: "bug"},
		{ID: "st-bbbb", Title: "Add dark mode", Priority: "P2", Status: "open", Type: "feature"},
	}
	m.TodoSel = 0
	if m.TodoSel != 0 {
		t.Error("expected sel 0")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.TodoSel != 1 {
		t.Errorf("expected sel 1, got %d", m.TodoSel)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.TodoSel != 1 {
		t.Error("expected sel to clamp at last")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.TodoSel != 0 {
		t.Errorf("expected sel 0, got %d", m.TodoSel)
	}
}

func TestRenderTodosEmpty(t *testing.T) {
	m := NewModel()
	out := m.RenderTodos(m.Styles, 80, 20)
	if !strings.Contains(out, "Todos") {
		t.Error("expected title")
	}
	if !strings.Contains(out, "no todos yet") {
		t.Error("expected empty message")
	}
}

func TestRenderTodosWithItems(t *testing.T) {
	m := NewModel()
	m.TodoItems = []TodoRow{
		{ID: "st-aaaa", Title: "Fix bug", Priority: "P0", Status: "open", Type: "bug"},
		{ID: "st-bbbb", Title: "Add dark mode", Priority: "P2", Status: "in_progress", Type: "feature"},
	}
	out := m.RenderTodos(m.Styles, 80, 20)
	if !strings.Contains(out, "st-aaaa") {
		t.Error("expected st-aaaa in output")
	}
	if !strings.Contains(out, "st-bbbb") {
		t.Error("expected st-bbbb in output")
	}
	if !strings.Contains(out, "Fix bug") {
		t.Error("expected title in output")
	}
	if !strings.Contains(out, "open") {
		t.Error("expected status in output")
	}
}

func TestRenderTodosShowsCounts(t *testing.T) {
	m := NewModel()
	m.Sidebar.TodoOpen = 5
	m.Sidebar.TodoReady = 3
	m.Sidebar.TodoBlocked = 2
	m.Sidebar.TodoOverdue = 1
	out := m.RenderTodos(m.Styles, 80, 20)
	if !strings.Contains(out, "5 open") {
		t.Error("expected open count")
	}
	if !strings.Contains(out, "3 ready") {
		t.Error("expected ready count")
	}
	if !strings.Contains(out, "2 blocked") {
		t.Error("expected blocked count")
	}
	if !strings.Contains(out, "1 overdue") {
		t.Error("expected overdue count")
	}
}

func TestSidebarBadgeRenders(t *testing.T) {
	s := NewSidebar()
	s.TodoOpen = 5
	s.TodoBlocked = 2
	s.TodoOverdue = 1
	badge := badgeFor(s)
	if !strings.Contains(badge, "5") {
		t.Errorf("expected 5 in badge, got %q", badge)
	}
	if !strings.Contains(badge, "2") {
		t.Errorf("expected 2 in badge, got %q", badge)
	}
	if !strings.Contains(badge, "1") {
		t.Errorf("expected 1 in badge, got %q", badge)
	}
	if !strings.Contains(badge, "🔵") {
		t.Errorf("expected 🔵 in badge, got %q", badge)
	}
}

func TestSidebarBadgeEmpty(t *testing.T) {
	s := NewSidebar()
	if badge := badgeFor(s); badge != "" {
		t.Errorf("expected empty badge, got %q", badge)
	}
}

func TestItoa(t *testing.T) {
	cases := map[int]string{0: "0", 1: "1", 9: "9", 10: "10", 99: "99", 100: "100", -5: "-5"}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestSetBanner(t *testing.T) {
	m := NewModel()
	if m.NotificationBanner != nil {
		t.Error("expected nil banner initially")
	}
	m.SetBanner(&NotificationItem{ID: "nt-1", Title: "Hello", Message: "world", Type: "todo_created"})
	if m.NotificationBanner == nil {
		t.Fatal("banner should be set")
	}
	if m.NotificationBanner.Title != "Hello" {
		t.Errorf("got %q", m.NotificationBanner.Title)
	}
	if len(m.Notifications) != 1 {
		t.Errorf("expected 1 in history, got %d", len(m.Notifications))
	}
}

func TestDismissBanner(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "nt-1", Title: "Hello", Type: "todo_created"})
	m.SetBanner(&NotificationItem{ID: "nt-2", Title: "Second", Type: "todo_completed"})
	if m.NotificationBanner.ID != "nt-2" {
		t.Errorf("expected latest banner, got %q", m.NotificationBanner.ID)
	}
	m.DismissBanner()
	if m.Notifications[1].Dismissed != true {
		t.Error("expected dismissed=true")
	}
}

func TestBannerNext(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "nt-1", Title: "First", Type: "todo_created"})
	m.SetBanner(&NotificationItem{ID: "nt-2", Title: "Second", Type: "todo_completed"})
	m.DismissBanner()
	if m.NotificationBanner == nil {
		t.Error("expected next non-dismissed banner")
	}
	if m.NotificationBanner.ID != "nt-1" {
		t.Errorf("expected nt-1, got %q", m.NotificationBanner.ID)
	}
}

func TestBannerNextEmpty(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "nt-1", Title: "X", Type: "todo_created"})
	m.DismissBanner()
	if m.NotificationBanner != nil {
		t.Error("expected no more banners")
	}
}

func TestRenderBanner(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "nt-1", Title: "Todo assigned", Message: "st-aaaa", Type: "todo_assigned"})
	out := m.RenderBanner(m.Styles, 80)
	if !strings.Contains(out, "Todo assigned") {
		t.Error("expected title in banner")
	}
	if !strings.Contains(out, "st-aaaa") {
		t.Error("expected message in banner")
	}
	if !strings.Contains(out, "[o] open") {
		t.Error("expected [o] open in banner")
	}
	if !strings.Contains(out, "[d] dismiss") {
		t.Error("expected [d] dismiss in banner")
	}
	if !strings.Contains(out, "[n] next") {
		t.Error("expected [n] next in banner")
	}
}

func TestRenderBannerEmpty(t *testing.T) {
	m := NewModel()
	out := m.RenderBanner(m.Styles, 80)
	if out != "" {
		t.Errorf("expected empty, got %q", out)
	}
}

func TestRenderBannerIconForCompleted(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "nt-1", Title: "Done", Message: "msg", Type: "todo_completed"})
	out := m.RenderBanner(m.Styles, 80)
	if !strings.Contains(out, "✓") {
		t.Error("expected ✓ icon for completed")
	}
}

func TestBannerKeyOpen(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "nt-1", Title: "T", Type: "todo_created"})
	_, _ = m.Update(tea.KeyPressMsg{Text: "o"})
	if len(m.History) == 0 {
		t.Error("expected history entry")
	}
	last := m.History[len(m.History)-1]
	if last.Action != "banner-open" {
		t.Errorf("expected banner-open, got %q", last.Action)
	}
}

func TestBannerKeyDismiss(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "nt-1", Title: "T", Type: "todo_created"})
	_, _ = m.Update(tea.KeyPressMsg{Text: "d"})
	if m.NotificationBanner != nil {
		t.Error("banner should be dismissed")
	}
}

func TestBannerKeyNext(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "nt-1", Title: "First", Type: "todo_created"})
	m.SetBanner(&NotificationItem{ID: "nt-2", Title: "Second", Type: "todo_completed"})
	_, _ = m.Update(tea.KeyPressMsg{Text: "n"})
	if m.NotificationBanner == nil {
		t.Fatal("banner should be set after next")
	}
}

func TestCountsMsgUpdates(t *testing.T) {
	m := NewModel()
	_, _ = m.Update(CountsMsg{Open: 5, Ready: 3, Blocked: 2, Overdue: 1})
	if m.Sidebar.TodoOpen != 5 {
		t.Errorf("got %d", m.Sidebar.TodoOpen)
	}
	if m.Sidebar.TodoReady != 3 {
		t.Errorf("got %d", m.Sidebar.TodoReady)
	}
	if m.Sidebar.TodoBlocked != 2 {
		t.Errorf("got %d", m.Sidebar.TodoBlocked)
	}
	if m.Sidebar.TodoOverdue != 1 {
		t.Errorf("got %d", m.Sidebar.TodoOverdue)
	}
}

func TestTodosLoadedMsg(t *testing.T) {
	m := NewModel()
	_, _ = m.Update(TodosLoadedMsg{Items: []TodoRow{
		{ID: "st-1", Title: "X"},
		{ID: "st-2", Title: "Y"},
	}})
	if len(m.TodoItems) != 2 {
		t.Errorf("got %d", len(m.TodoItems))
	}
}

func TestTodosLoadedClampsSel(t *testing.T) {
	m := NewModel()
	m.TodoSel = 5
	_, _ = m.Update(TodosLoadedMsg{Items: []TodoRow{{ID: "st-1", Title: "X"}}})
	if m.TodoSel != 0 {
		t.Errorf("expected sel 0, got %d", m.TodoSel)
	}
}

func TestNotificationMsgFiresBanner(t *testing.T) {
	m := NewModel()
	_, cmd := m.Update(NotificationMsg{N: &testNotification{id: "nt-1", title: "Hello", message: "world", t: "todo_created"}})
	if m.NotificationBanner == nil {
		t.Fatal("banner should be set")
	}
	if m.NotificationBanner.Title != "Hello" {
		t.Errorf("got %q", m.NotificationBanner.Title)
	}
	if cmd == nil {
		t.Error("expected re-subscribe cmd")
	}
}

func TestViewIncludesTodosView(t *testing.T) {
	m := NewModel()
	m.Width = 100
	m.Height = 30
	m.Ready = true
	m.ViewKind = ViewTodos
	out := m.View().Content
	if !strings.Contains(out, "Todos") {
		t.Error("expected 'Todos' in view")
	}
}

func TestViewIncludesBanner(t *testing.T) {
	m := NewModel()
	m.Width = 100
	m.Height = 30
	m.Ready = true
	m.ViewKind = ViewTools
	m.SetBanner(&NotificationItem{ID: "nt-1", Title: "BANNER-TEST", Message: "msg", Type: "todo_created"})
	out := m.View().Content
	if !strings.Contains(out, "BANNER-TEST") {
		t.Error("expected banner in view")
	}
}

func TestDefaultHintsForTodos(t *testing.T) {
	hints := DefaultHints(ViewTodos)
	has := func(label string) bool {
		for _, h := range hints {
			if h.Label == label {
				return true
			}
		}
		return false
	}
	if !has("open") || !has("dismiss") || !has("next") {
		t.Error("expected open/dismiss/next hints")
	}
}

func TestNotificationItemGetters(t *testing.T) {
	n := NotificationItem{ID: "nt-1", Title: "T", Message: "M", Type: "todo_created"}
	if n.GetID() != "nt-1" {
		t.Error("GetID")
	}
	if n.GetTitle() != "T" {
		t.Error("GetTitle")
	}
	if n.GetMessage() != "M" {
		t.Error("GetMessage")
	}
	if n.GetType() != "todo_created" {
		t.Error("GetType")
	}
}

func TestNotificationAdapterGetters(t *testing.T) {
	raw := &testNotification{id: "nt-1", title: "T", message: "M", t: "todo_created"}
	var a NotificationSource = raw
	if a.GetID() != "nt-1" || a.GetTitle() != "T" || a.GetMessage() != "M" || a.GetType() != "todo_created" {
		t.Error("interface getters failed")
	}
}

func TestListenForNotificationsReturnsCmd(t *testing.T) {
	cmd := ListenForNotifications()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	_ = cmd
}

func TestRefreshTodosCmd(t *testing.T) {
	cmd := RefreshTodosCmd()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	c, ok := msg.(CountsMsg)
	if !ok {
		t.Fatalf("expected CountsMsg, got %T", msg)
	}
	_ = c
}

func TestBannerDismissBySetNil(t *testing.T) {
	m := NewModel()
	m.SetBanner(&NotificationItem{ID: "nt-1", Title: "X", Type: "todo_created"})
	m.DismissBanner()
	if m.NotificationBanner != nil {
		t.Error("should be nil after dismiss")
	}
}

func TestNextWithoutBanner(t *testing.T) {
	m := NewModel()
	m.BannerNext()
	if m.NotificationBanner != nil {
		t.Error("expected nil")
	}
}

type testNotification struct {
	id      string
	title   string
	message string
	t       string
}

func (t *testNotification) GetID() string      { return t.id }
func (t *testNotification) GetTitle() string   { return t.title }
func (t *testNotification) GetMessage() string { return t.message }
func (t *testNotification) GetType() string    { return t.t }

// Ensure testNotification satisfies the interface
var _ NotificationSource = (*testNotification)(nil)

var _ = time.Second
