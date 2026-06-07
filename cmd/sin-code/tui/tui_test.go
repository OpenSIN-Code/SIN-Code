package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModelDefaults(t *testing.T) {
	m := NewModel()
	if m == nil {
		t.Fatal("NewModel returned nil")
	}
	if m.ThemeIdx != 0 {
		t.Errorf("expected theme 0, got %d", m.ThemeIdx)
	}
	if m.ViewKind != ViewTools {
		t.Errorf("expected ViewTools, got %v", m.ViewKind)
	}
	if m.Mode != ModeNormal {
		t.Errorf("expected ModeNormal, got %v", m.Mode)
	}
	if m.Quitting {
		t.Error("expected Quitting false")
	}
	if m.Sidebar.Collapsed {
		t.Error("expected sidebar visible by default")
	}
	if len(m.Tabs.Sessions) != 1 {
		t.Errorf("expected 1 default session, got %d", len(m.Tabs.Sessions))
	}
	if len(m.Sidebar.Items) != 5 {
		t.Errorf("expected 5 sidebar items, got %d", len(m.Sidebar.Items))
	}
}

func TestThemesLength(t *testing.T) {
	if len(Themes) != 5 {
		t.Errorf("expected 5 themes, got %d", len(Themes))
	}
	expected := []string{"default", "Dracula", "Nord", "Solarized", "Monokai"}
	for i, want := range expected {
		if Themes[i].Name != want {
			t.Errorf("Themes[%d].Name = %q, want %q", i, Themes[i].Name, want)
		}
	}
}

func TestViewKindString(t *testing.T) {
	tests := []struct {
		v    ViewKind
		want string
	}{
		{ViewTools, "Tools"},
		{ViewSessions, "Sessions"},
		{ViewEFM, "EFM"},
		{ViewConfig, "Config"},
		{ViewHistory, "History"},
	}
	for _, tt := range tests {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("ViewKind(%d).String() = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestCycleTheme(t *testing.T) {
	m := NewModel()
	if m.ThemeIdx != 0 {
		t.Fatalf("expected initial theme 0, got %d", m.ThemeIdx)
	}
	originalAccent := m.Styles.Theme.Accent

	for i := 1; i < len(Themes); i++ {
		m.CycleTheme()
		if m.ThemeIdx != i {
			t.Errorf("after %d cycles, expected theme %d, got %d", i, i, m.ThemeIdx)
		}
		if m.Styles.Theme.Accent != Themes[i].Accent {
			t.Errorf("expected accent %q, got %q", Themes[i].Accent, m.Styles.Theme.Accent)
		}
	}

	m.CycleTheme()
	if m.ThemeIdx != 0 {
		t.Errorf("expected wrap-around to 0, got %d", m.ThemeIdx)
	}
	if m.Styles.Theme.Accent != originalAccent {
		t.Errorf("expected wrap-around accent %q, got %q", originalAccent, m.Styles.Theme.Accent)
	}
}

func TestSwitchView(t *testing.T) {
	m := NewModel()
	m.SwitchView(ViewEFM)
	if m.ViewKind != ViewEFM {
		t.Errorf("expected ViewEFM, got %v", m.ViewKind)
	}
	if m.Sidebar.SelectedView() != ViewEFM {
		t.Errorf("sidebar not synced: %v", m.Sidebar.SelectedView())
	}
	if m.Footer.View() != ViewEFM {
		t.Errorf("footer not synced: %v", m.Footer.View())
	}
}

func TestNextPrevView(t *testing.T) {
	m := NewModel()
	start := m.ViewKind

	m.NextView()
	if m.ViewKind == start {
		t.Error("expected view to change after NextView")
	}

	m.PreviousView()
	if m.ViewKind != start {
		t.Errorf("expected to return to %v, got %v", start, m.ViewKind)
	}
}

func TestViewJumpKeys(t *testing.T) {
	m := NewModel()
	cases := []struct {
		key  string
		want ViewKind
	}{
		{"1", ViewTools},
		{"2", ViewSessions},
		{"3", ViewEFM},
		{"4", ViewConfig},
		{"5", ViewHistory},
	}
	for _, tc := range cases {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)})
		if m.ViewKind != tc.want {
			t.Errorf("key %q: expected view %v, got %v", tc.key, tc.want, m.ViewKind)
		}
	}
}

func TestTabNextView(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	start := m.ViewKind
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.ViewKind == start {
		t.Error("expected view to change after Tab")
	}
}

func TestShiftTabPrevView(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	mid := m.ViewKind
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.ViewKind == mid {
		t.Error("expected view to change after Shift+Tab")
	}
}

func TestSidebarToggle(t *testing.T) {
	m := NewModel()
	if m.Sidebar.Collapsed {
		t.Fatal("expected sidebar visible initially")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	if !m.Sidebar.Collapsed {
		t.Error("expected sidebar collapsed after ctrl+b")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.Sidebar.Collapsed {
		t.Error("expected sidebar visible after second ctrl+b")
	}
}

func TestCommandPaletteOpenClose(t *testing.T) {
	m := NewModel()
	if m.Palette.Open {
		t.Fatal("expected palette closed")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if !m.Palette.Open {
		t.Error("expected palette open after ctrl+p")
	}
	if m.Mode != ModePalette {
		t.Errorf("expected ModePalette, got %v", m.Mode)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.Palette.Open {
		t.Error("expected palette closed after esc")
	}
	if m.Mode != ModeNormal {
		t.Errorf("expected ModeNormal, got %v", m.Mode)
	}
}

func TestCommandPaletteFilter(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	m.Palette.Query = "disc"
	m.filterPalette(m.Palette.Query)
	if len(m.Palette.Filter) == 0 {
		t.Fatal("expected at least one match for 'disc'")
	}
	found := false
	for _, s := range m.Palette.Filter {
		if strings.Contains(strings.ToLower(s), "disc") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'disc' in filtered results, got %v", m.Palette.Filter)
	}
}

func TestCommandPaletteSelect(t *testing.T) {
	m := NewModel()
	m.OpenPalette()
	if len(m.Palette.Filter) == 0 {
		t.Fatal("expected palette to have items")
	}
	original := len(m.History)
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Palette.Open {
		t.Error("expected palette to close after enter")
	}
	if len(m.History) <= original {
		t.Error("expected history to grow after palette selection")
	}
}

func TestSubagentsPopup(t *testing.T) {
	m := NewModel()
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if m.Mode != ModeSubagents {
		t.Errorf("expected ModeSubagents, got %v", m.Mode)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.Mode != ModeNormal {
		t.Errorf("expected ModeNormal after esc, got %v", m.Mode)
	}
}

func TestQuitKeys(t *testing.T) {
	m := NewModel()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !m.Quitting {
		t.Error("expected Quitting after 'q'")
	}

	m2 := NewModel()
	m2.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m2.Quitting {
		t.Error("expected Quitting after ctrl+c")
	}
}

func TestSpinnerTick(t *testing.T) {
	m := NewModel()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return a non-nil cmd (spinner tick)")
	}
	initialFrame := m.Spinner.Frame()
	updated, _ := m.Spinner.Update(SpinnerTickMsg(time.Now()))
	m.Spinner = updated
	if m.Spinner.Frame() == initialFrame {
		t.Error("expected spinner frame to advance after tick")
	}
}

func TestSpinnerView(t *testing.T) {
	s := NewSpinner()
	view := s.ViewPlain()
	if view == "" {
		t.Error("expected non-empty spinner view")
	}
	if !strings.Contains(view, "⚡") && !strings.Contains(view, "✦") && !strings.Contains(view, "✧") {
		t.Errorf("expected bolt character in spinner view, got %q", view)
	}
}

func TestSpinnerCyclesFrames(t *testing.T) {
	s := NewSpinner()
	frames := map[int]bool{}
	for i := 0; i < 30; i++ {
		frames[s.Frame()] = true
		var cmd tea.Cmd
		s, cmd = s.Update(SpinnerTickMsg(time.Now()))
		_ = cmd
	}
	if len(frames) < 2 {
		t.Error("expected spinner to cycle through multiple frames")
	}
}

func TestFooterSelection(t *testing.T) {
	m := NewModel()
	if m.Footer.Selection != "" {
		t.Errorf("expected empty selection, got %q", m.Footer.Selection)
	}
}

func TestFooterCycleAgent(t *testing.T) {
	m := NewModel()
	start := m.Footer.AgentIndex
	m.Footer.CycleAgent()
	if m.Footer.AgentIndex == start {
		t.Error("expected agent index to change")
	}
	if m.Footer.AgentName() == "" {
		t.Error("expected non-empty agent name")
	}
}

func TestFooterProgressBar(t *testing.T) {
	f := NewFooter(80)
	bar := f.ProgressBar(10)
	runeCount := 0
	for range bar {
		runeCount++
	}
	if runeCount != 10 {
		t.Errorf("expected 10-rune bar, got %d runes (%d bytes): %q", runeCount, len(bar), bar)
	}

	f.TokensPct = 0.5
	bar = f.ProgressBar(10)
	if !strings.Contains(bar, "█") || !strings.Contains(bar, "░") {
		t.Errorf("expected mix of filled/empty in 50%% bar, got %q", bar)
	}
}

func TestWindowResize(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.Width != 120 {
		t.Errorf("expected width 120, got %d", m.Width)
	}
	if m.Height != 40 {
		t.Errorf("expected height 40, got %d", m.Height)
	}
	if !m.Ready {
		t.Error("expected Ready=true after WindowSizeMsg")
	}
}

func TestHistoryAppend(t *testing.T) {
	m := NewModel()
	start := len(m.History)
	m.AppendHistory(ViewTools.String(), "test", "detail", true)
	if len(m.History) != start+1 {
		t.Errorf("expected history to grow by 1, got %d", len(m.History))
	}
	last := m.History[len(m.History)-1]
	if last.View != ViewTools.String() {
		t.Errorf("expected last view %q, got %q", ViewTools.String(), last.View)
	}
	if !last.Success {
		t.Error("expected success=true")
	}
}

func TestHistoryCap(t *testing.T) {
	m := NewModel()
	for i := 0; i < 250; i++ {
		m.AppendHistory("Tools", "x", "y", true)
	}
	if len(m.History) > 200 {
		t.Errorf("expected history capped at 200, got %d", len(m.History))
	}
}

func TestToolsViewRenders(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := m.View()
	if !strings.Contains(view, "Tools") {
		t.Errorf("expected Tools in view, got:\n%s", view)
	}
	if !strings.Contains(view, "discover") {
		t.Errorf("expected 'discover' in view, got:\n%s", view)
	}
}

func TestEFMViewEmpty(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.SwitchView(ViewEFM)
	view := m.View()
	if !strings.Contains(view, "EFM") {
		t.Errorf("expected 'EFM' in view, got:\n%s", view)
	}
}

func TestEFMViewWithStacks(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.EFMStks = []EFMStack{
		{Name: "test-stack-1", Status: "running", URL: "http://localhost:1234", TTL: 60},
		{Name: "test-stack-2", Status: "down", URL: "http://localhost:5678", TTL: 0},
	}
	m.SwitchView(ViewEFM)
	view := m.View()
	if !strings.Contains(view, "test-stack-1") {
		t.Errorf("expected 'test-stack-1' in view, got:\n%s", view)
	}
}

func TestConfigViewRenders(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.SwitchView(ViewConfig)
	view := m.View()
	if !strings.Contains(view, "Config") {
		t.Errorf("expected 'Config' in view, got:\n%s", view)
	}
	if !strings.Contains(view, "theme") {
		t.Errorf("expected 'theme' key in view, got:\n%s", view)
	}
}

func TestHistoryViewRenders(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.AppendHistory("Tools", "test-action", "test-detail", true)
	m.SwitchView(ViewHistory)
	view := m.View()
	if !strings.Contains(view, "test-action") {
		t.Errorf("expected 'test-action' in view, got:\n%s", view)
	}
}

func TestSessionsViewRenders(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.SwitchView(ViewSessions)
	view := m.View()
	if !strings.Contains(view, "Session") {
		t.Errorf("expected 'Session' in view, got:\n%s", view)
	}
}

func TestRunSelectedRunnable(t *testing.T) {
	m := NewModel()
	called := false
	m.OnRun = func(name string, args []string) error {
		called = true
		return nil
	}
	for i := 0; i < 20; i++ {
		m.Sidebar.ToolMoveDown()
	}
	tool := m.Sidebar.SelectedTool()
	if tool == nil {
		t.Fatal("expected a tool to be selected")
	}
	if tool.Runnable {
		m.RunSelected()
		if !called {
			t.Errorf("expected OnRun to be called for runnable tool %q", tool.Name)
		}
	}
}

func TestRunSelectedNeedsArgs(t *testing.T) {
	m := NewModel()
	m.Sidebar.ToolSel = 0
	tool := m.Sidebar.SelectedTool()
	if tool == nil {
		t.Fatal("expected a tool to be selected")
	}
	if !tool.Runnable {
		m.RunSelected()
		if !m.ArgInput.Open {
			t.Error("expected arg input to open for non-runnable tool")
		}
		if m.ArgInput.Cmd != tool.Name {
			t.Errorf("expected cmd %q, got %q", tool.Name, m.ArgInput.Cmd)
		}
	}
}

func TestArgInputEsc(t *testing.T) {
	m := NewModel()
	m.Sidebar.ToolSel = 0
	tool := m.Sidebar.SelectedTool()
	if tool == nil && !tool.Runnable {
		m.RunSelected()
		m.Update(tea.KeyMsg{Type: tea.KeyEscape})
		if m.ArgInput.Open {
			t.Error("expected arg input closed after esc")
		}
	}
}

func TestAddSession(t *testing.T) {
	tabs := NewTabs()
	tabs.Add("")
	if len(tabs.Sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(tabs.Sessions))
	}
	tabs.Add("My Custom")
	if tabs.Sessions[2].Name != "My Custom" {
		t.Errorf("expected 'My Custom', got %q", tabs.Sessions[2].Name)
	}
	if tabs.ActiveIdx != 2 {
		t.Errorf("expected active index 2, got %d", tabs.ActiveIdx)
	}
}

func TestCloseSession(t *testing.T) {
	tabs := NewTabs()
	tabs.Add("test")
	tabs.Close(0)
	if len(tabs.Sessions) != 1 {
		t.Errorf("expected 1 session after close, got %d", len(tabs.Sessions))
	}
	if tabs.Sessions[0].Name != "test" {
		t.Errorf("expected remaining session 'test', got %q", tabs.Sessions[0].Name)
	}
}

func TestSidebarMoveUpDown(t *testing.T) {
	s := NewSidebar()
	start := s.Selected
	s.MoveDown()
	if s.Selected != start+1 {
		t.Errorf("expected selected to increment, got %d", s.Selected)
	}
	s.MoveUp()
	if s.Selected != start {
		t.Errorf("expected selected to return, got %d", s.Selected)
	}
}

func TestSidebarToolMoveUpDown(t *testing.T) {
	s := NewSidebar()
	start := s.ToolSel
	s.ToolMoveDown()
	if s.ToolSel != start+1 {
		t.Errorf("expected toolsel to increment, got %d", s.ToolSel)
	}
	s.ToolMoveUp()
	if s.ToolSel != start {
		t.Errorf("expected toolsel to return, got %d", s.ToolSel)
	}
}

func TestViewTooSmall(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 10, Height: 4})
	view := m.View()
	if !strings.Contains(view, "too small") && !strings.Contains(view, "Tools") {
		t.Errorf("expected 'too small' message or fallback, got:\n%s", view)
	}
}

func TestViewQuitting(t *testing.T) {
	m := NewModel()
	m.Quitting = true
	if view := m.View(); view != "" {
		t.Errorf("expected empty view when quitting, got %q", view)
	}
}

func TestNewStylesApplies(t *testing.T) {
	s := NewStyles(Themes[2])
	if s.Theme.Name != "Nord" {
		t.Errorf("expected Nord theme, got %q", s.Theme.Name)
	}
}

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"foo bar", []string{"foo", "bar"}},
		{`"foo bar" baz`, []string{`"foo bar"`, "baz"}},
		{"", nil},
		{"  a  b  ", []string{"a", "b"}},
	}
	for _, tc := range tests {
		got := splitArgs(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("splitArgs(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitArgs(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestDefaultHints(t *testing.T) {
	for _, v := range []ViewKind{ViewTools, ViewSessions, ViewEFM, ViewConfig, ViewHistory} {
		hints := DefaultHints(v)
		if len(hints) == 0 {
			t.Errorf("expected hints for view %v, got 0", v)
		}
	}
}

func TestTabNext(t *testing.T) {
	tabs := NewTabs()
	tabs.Add("two")
	start := tabs.ActiveIdx
	tabs.Next()
	if tabs.ActiveIdx == start {
		t.Error("expected ActiveIdx to change")
	}
}

func TestTabPrev(t *testing.T) {
	tabs := NewTabs()
	tabs.Prev()
	if tabs.ActiveIdx != len(tabs.Sessions)-1 {
		t.Errorf("expected wrap to last, got %d", len(tabs.Sessions))
	}
}

func TestRenderToolsViewEmpty(t *testing.T) {
	s := NewStyles(Themes[0])
	s2 := Sidebar{ToolSubItems: nil, ToolSel: 0}
	view := RenderToolsView(s2, s, 80, 24)
	if !strings.Contains(view, "No tool") {
		t.Errorf("expected 'No tool' message, got %q", view)
	}
}

func TestRenderHistoryViewEmpty(t *testing.T) {
	s := NewStyles(Themes[0])
	view := RenderHistoryView([]HistoryEntry{}, 0, s, 80, 24)
	if !strings.Contains(view, "No actions") {
		t.Errorf("expected 'No actions' message, got %q", view)
	}
}

func TestRenderEFMViewEmpty(t *testing.T) {
	s := NewStyles(Themes[0])
	view := RenderEFMView([]EFMStack{}, s, 80, 24, NewSpinner())
	if !strings.Contains(view, "No active stacks") {
		t.Errorf("expected 'No active stacks' message, got %q", view)
	}
}

func TestAgentNamesList(t *testing.T) {
	if len(AgentNames) != 3 {
		t.Errorf("expected 3 agents, got %d", len(AgentNames))
	}
}

func TestToolSubItemCount(t *testing.T) {
	items := DefaultToolSubItems()
	if len(items) != 19 {
		t.Errorf("expected 19 tools, got %d", len(items))
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate shorter: got %q", got)
	}
	if got := truncate("hello world", 5); got != "hell…" {
		t.Errorf("truncate longer: got %q", got)
	}
	if got := truncate("hi", 0); got != "" {
		t.Errorf("truncate zero: got %q", got)
	}
}

func TestEFMStacksDisplayColors(t *testing.T) {
	s := NewStyles(Themes[0])
	stacks := []EFMStack{
		{Name: "running-one", Status: "running", URL: "http://a", TTL: 60},
		{Name: "starting-one", Status: "starting", URL: "http://b", TTL: 30},
		{Name: "down-one", Status: "down", URL: "http://c", TTL: 0},
	}
	view := RenderEFMView(stacks, s, 100, 24, NewSpinner())
	if !strings.Contains(view, "running-one") {
		t.Error("expected running-one in view")
	}
	if !strings.Contains(view, "starting-one") {
		t.Error("expected starting-one in view")
	}
	if !strings.Contains(view, "down-one") {
		t.Error("expected down-one in view")
	}
}

func TestComposeLayout(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := m.View()
	if len(view) < 100 {
		t.Errorf("expected substantial view output, got %d bytes", len(view))
	}
}

func TestComposeLayoutSmall(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view even at small sizes")
	}
}

func TestDefaultConfigEntries(t *testing.T) {
	entries := DefaultConfigEntries()
	if len(entries) < 5 {
		t.Errorf("expected at least 5 config entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Key == "" {
			t.Error("expected non-empty key")
		}
		if e.Kind == "" {
			t.Errorf("expected non-empty kind for %q", e.Key)
		}
	}
}

func TestConfigViewSelectionMoves(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.SwitchView(ViewConfig)
	start := m.ConfigSel
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.ConfigSel == start {
		t.Error("expected config selection to move down")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.ConfigSel != start {
		t.Error("expected config selection to return")
	}
}

func TestToolListSelectionMoves(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	start := m.Sidebar.ToolSel
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.Sidebar.ToolSel == start {
		t.Error("expected tool selection to move down")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.Sidebar.ToolSel != start {
		t.Error("expected tool selection to return")
	}
}

func TestSpinnerUpdateIgnoresUnknownMsgs(t *testing.T) {
	s := NewSpinner()
	startFrame := s.Frame()
	s2, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	s = s2
	if s.Frame() != startFrame {
		t.Error("expected spinner frame to be unchanged on non-tick msg")
	}
}

func TestRenderRightPanel(t *testing.T) {
	s := NewStyles(Themes[0])
	tool := &ToolSubItem{Name: "test", Description: "desc", Runnable: true}
	view := RenderRightPanel(tool, ViewTools, s, 30, 20)
	if !strings.Contains(view, "test") {
		t.Errorf("expected 'test' in panel, got %q", view)
	}
}

func TestRenderRightPanelNil(t *testing.T) {
	s := NewStyles(Themes[0])
	view := RenderRightPanel(nil, ViewTools, s, 30, 20)
	if !strings.Contains(view, "no selection") {
		t.Errorf("expected 'no selection' message, got %q", view)
	}
}

func TestRenderSubagentsPopup(t *testing.T) {
	s := NewStyles(Themes[0])
	view := RenderSubagentsPopup(s, 80, 20)
	if !strings.Contains(view, "Subagents") {
		t.Errorf("expected 'Subagents' in popup, got %q", view)
	}
}

func TestRunSelectedArgFlow(t *testing.T) {
	m := NewModel()
	called := ""
	m.OnRun = func(name string, args []string) error {
		called = name
		return nil
	}
	m.Sidebar.ToolSel = 0
	tool := m.Sidebar.SelectedTool()
	if tool == nil || tool.Runnable {
		t.Skip("first tool is runnable; test only for arg-input flow")
	}
	m.RunSelected()
	if !m.ArgInput.Open {
		t.Fatal("expected arg input to open")
	}
	m.ArgInput.Input.SetValue("--help")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.ArgInput.Open {
		t.Error("expected arg input to close after enter")
	}
	if called != tool.Name {
		t.Errorf("expected OnRun called with %q, got %q", tool.Name, called)
	}
}
