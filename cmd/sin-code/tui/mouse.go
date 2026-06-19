package tui

import (
	"sync"

	tea "charm.land/bubbletea/v2"
)

type MouseAction int

const (
	MouseNone MouseAction = iota
	MouseSelectItem
	MouseScrollUp
	MouseScrollDown
	MouseClickSidebar
	MouseClickInput
	MouseResize
)

func (a MouseAction) String() string {
	switch a {
	case MouseNone:
		return "none"
	case MouseSelectItem:
		return "select_item"
	case MouseScrollUp:
		return "scroll_up"
	case MouseScrollDown:
		return "scroll_down"
	case MouseClickSidebar:
		return "click_sidebar"
	case MouseClickInput:
		return "click_input"
	case MouseResize:
		return "resize"
	}
	return "unknown"
}

type MouseHandler struct {
	mu       sync.RWMutex
	enabled  bool
	width    int
	height   int
	sidebarW int
	rightW   int
}

func NewMouseHandler() *MouseHandler {
	return &MouseHandler{enabled: true, width: 80, height: 24, sidebarW: 22}
}

func (h *MouseHandler) SetLayout(width, height, sidebarWidth, rightPanelWidth int) {
	h.mu.Lock()
	h.width = width
	h.height = height
	h.sidebarW = sidebarWidth
	h.rightW = rightPanelWidth
	h.mu.Unlock()
}

func (h *MouseHandler) HandleClick(x, y int, view ViewKind) MouseAction {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.enabled {
		return MouseNone
	}
	const tabBarHeight = 3
	const footerHeight = 3
	if y < 0 || x < 0 {
		return MouseNone
	}
	if y < tabBarHeight {
		return MouseSelectItem
	}
	if h.height > tabBarHeight+footerHeight && y >= h.height-footerHeight {
		return MouseNone
	}
	if view == ViewChat {
		if h.sidebarW > 0 && x < h.sidebarW {
			return MouseClickSidebar
		}
		inputAreaStart := h.height - footerHeight - 6
		if inputAreaStart < tabBarHeight {
			inputAreaStart = tabBarHeight
		}
		if y >= inputAreaStart {
			return MouseClickInput
		}
		return MouseSelectItem
	}
	if h.sidebarW > 0 && x < h.sidebarW {
		return MouseClickSidebar
	}
	return MouseSelectItem
}

func (h *MouseHandler) HandleWheel(deltaY int) MouseAction {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.enabled {
		return MouseNone
	}
	if deltaY < 0 {
		return MouseScrollUp
	}
	if deltaY > 0 {
		return MouseScrollDown
	}
	return MouseNone
}

func (h *MouseHandler) HandleMotion(x, y int, view ViewKind) MouseAction {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return MouseNone
}
func (h *MouseHandler) Enabled() bool { h.mu.RLock(); defer h.mu.RUnlock(); return h.enabled }
func (h *MouseHandler) Enable()       { h.mu.Lock(); h.enabled = true; h.mu.Unlock() }
func (h *MouseHandler) Disable()      { h.mu.Lock(); h.enabled = false; h.mu.Unlock() }

func (m *Model) handleMouseClickMsg(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.Mouse != nil {
		m.Mouse.SetLayout(m.Width, m.Height, m.Sidebar.Width, m.rightWidth())
		action := m.Mouse.HandleClick(msg.X, msg.Y, m.ViewKind)
		switch action {
		case MouseClickSidebar:
			return m, m.handleSidebarClick(MouseResolution{Kind: "click", X: msg.X, Y: msg.Y, Target: "sidebar"})
		case MouseClickInput:
			if m.ChatInput != nil {
				return m, m.ChatInput.Focus()
			}
			return m, nil
		case MouseSelectItem:
			res := ResolveMouse(msg, m.Width, m.Height, m.Sidebar.Width, m.rightWidth())
			return m, m.handleMouseClick(res)
		}
	}
	res := ResolveMouse(msg, m.Width, m.Height, m.Sidebar.Width, m.rightWidth())
	return m, m.handleMouseAction(res)
}

func (m *Model) handleMouseWheelMsg(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.Mouse != nil {
		var deltaY int
		switch msg.Button {
		case tea.MouseWheelUp:
			deltaY = -1
		case tea.MouseWheelDown:
			deltaY = 1
		}
		action := m.Mouse.HandleWheel(deltaY)
		switch action {
		case MouseScrollUp:
			return m, m.handleMouseScrollUp(MouseResolution{Kind: "scroll_up", X: msg.X, Y: msg.Y})
		case MouseScrollDown:
			return m, m.handleMouseScrollDown(MouseResolution{Kind: "scroll_down", X: msg.X, Y: msg.Y})
		}
	}
	res := ResolveMouse(msg, m.Width, m.Height, m.Sidebar.Width, m.rightWidth())
	return m, m.handleMouseAction(res)
}

func (m *Model) handleMouseMotionMsg(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if m.Mouse != nil {
		_ = m.Mouse.HandleMotion(msg.X, msg.Y, m.ViewKind)
	}
	return m, nil
}
