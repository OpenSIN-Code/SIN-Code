// SPDX-License-Identifier: MIT
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

type ProgramOptions struct {
	ExternalMode  bool
	Port          int
	Hostname      string
	MDNS          bool
	Sigusr2Reload bool
}

type ReloadMsg struct{}

func RunProgram(model *Model, opts ProgramOptions) error {
	if opts.ExternalMode {
		return runExternalMode(model, opts)
	}
	if !isTerminal(os.Stdin) {
		return fmt.Errorf("no TTY available — run 'sin-code tui' in a terminal, or use 'sin-code chat -p \"prompt\"' for headless mode")
	}
	prog := tea.NewProgram(model)
	model.Program = ProgramFromTeaProgram(prog)
	if opts.Sigusr2Reload {
		setupHotReload(model, prog)
	}
	_, err := prog.Run()
	return err
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func ReloadCmd() tea.Cmd {
	return func() tea.Msg { return ReloadMsg{} }
}

func setupHotReload(model *Model, prog *tea.Program) {
	sig := reloadSignal()
	if sig == nil {
		return
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sig)
	go func() {
		for range ch {
			prog.Send(ReloadMsg{})
		}
	}()
}

func reloadSignal() os.Signal {
	switch runtime.GOOS {
	case "darwin":
		return syscall.Signal(31)
	case "linux":
		return syscall.Signal(12)
	default:
		return nil
	}
}

func HandleReload(m *Model) {
	reloadConfig()
	m.ApplyTheme()
	m.SetBanner(&NotificationItem{
		ID:      "reload-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		Title:   "Reloaded",
		Message: "Configuration reloaded",
		Type:    "info",
	})
}

func reloadConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".config", "sin-code", "config.toml")
	_, err = os.ReadFile(path)
	return path, err
}

type externalRenderMsg struct{}

type externalInput struct {
	Type   string `json:"type"`
	Key    string `json:"key"`
	Code   int    `json:"code"`
	Mod    int    `json:"mod"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Button int    `json:"button"`
}

func runExternalMode(model *Model, opts ProgramOptions) error {
	var frameMu sync.RWMutex
	currentFrame := renderExternalFrame(model)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prog := tea.NewProgram(model,
		tea.WithoutRenderer(),
		tea.WithInput(nil),
		tea.WithWindowSize(120, 40),
		tea.WithFilter(func(m tea.Model, msg tea.Msg) tea.Msg {
			if mdl, ok := m.(*Model); ok {
				frame := renderExternalFrame(mdl)
				frameMu.Lock()
				currentFrame = frame
				frameMu.Unlock()
			}
			if _, ok := msg.(externalRenderMsg); ok {
				return nil
			}
			return msg
		}),
	)
	model.Program = ProgramFromTeaProgram(prog)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(externalTUIHTML))
	})
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		frameMu.RLock()
		writeSSEFrame(w, currentFrame)
		frameMu.RUnlock()
		flusher.Flush()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				frameMu.RLock()
				frame := currentFrame
				frameMu.RUnlock()
				writeSSEFrame(w, frame)
				flusher.Flush()
			}
		}
	})
	mux.HandleFunc("/input", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var input externalInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		msg := browserInputToMsg(input)
		if msg == nil {
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}
		prog.Send(msg)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	mux.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, ch := range payload.Text {
			if ch == '\n' {
				prog.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
				continue
			}
			prog.Send(tea.KeyPressMsg{
				Text: string(ch),
				Code: ch,
			})
		}
		prog.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				prog.Send(externalRenderMsg{})
			}
		}
	}()

	addr := fmt.Sprintf("%s:%d", opts.Hostname, opts.Port)
	log.Printf("sin-code tui external mode on http://%s", addr)
	server := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("external mode server error: %v", err)
		}
	}()

	_, err := prog.Run()
	cancel()
	_ = server.Shutdown(context.Background())
	return err
}

func browserInputToMsg(input externalInput) tea.Msg {
	if input.Type == "mouse" {
		return tea.MouseClickMsg{
			X:      input.X,
			Y:      input.Y,
			Button: tea.MouseButton(input.Button),
			Mod:    tea.KeyMod(input.Mod),
		}
	}
	if input.Type != "key" && input.Type != "" {
		return nil
	}
	code, isSpecial := mapBrowserKeyCode(input.Key)
	if code == 0 && !isSpecial {
		return nil
	}
	mod := tea.KeyMod(input.Mod)
	text := ""
	if !isSpecial && mod&(tea.ModCtrl|tea.ModAlt) == 0 {
		text = input.Key
	}
	return tea.KeyPressMsg{
		Text: text,
		Code: code,
		Mod:  mod,
	}
}

func mapBrowserKeyCode(keyName string) (rune, bool) {
	base := keyName
	for _, p := range []string{"ctrl+", "alt+", "shift+", "meta+"} {
		if strings.HasPrefix(base, p) {
			base = strings.TrimPrefix(base, p)
			break
		}
	}
	switch base {
	case "enter":
		return tea.KeyEnter, true
	case "tab":
		return tea.KeyTab, true
	case "esc", "escape":
		return tea.KeyEscape, true
	case "backspace":
		return tea.KeyBackspace, true
	case "space":
		return tea.KeySpace, true
	case "up":
		return tea.KeyUp, true
	case "down":
		return tea.KeyDown, true
	case "left":
		return tea.KeyLeft, true
	case "right":
		return tea.KeyRight, true
	case "delete":
		return tea.KeyDelete, true
	case "pgup":
		return tea.KeyPgUp, true
	case "pgdown":
		return tea.KeyPgDown, true
	case "home":
		return tea.KeyHome, true
	case "end":
		return tea.KeyEnd, true
	default:
		if r, sz := utf8.DecodeRuneInString(base); sz > 0 {
			return r, false
		}
		return 0, false
	}
}

func renderExternalFrame(m *Model) string {
	var b strings.Builder
	b.WriteString("sin-code tui — external mode\n")
	b.WriteString(strings.Repeat("─", 40))
	b.WriteString("\n")
	fmt.Fprintf(&b, "view:      %s\n", m.ViewKind.String())
	fmt.Fprintf(&b, "theme:     %s\n", Themes[m.ThemeIdx].Name)
	if m.Workspace != "" {
		fmt.Fprintf(&b, "workspace: %s\n", m.Workspace)
	}
	if m.NotificationBanner != nil {
		fmt.Fprintf(&b, "\n🔔 %s: %s\n", m.NotificationBanner.Title, m.NotificationBanner.Message)
	}
	b.WriteString("\n(Use the input bar below to interact.)\n")
	return b.String()
}

func writeSSEFrame(w http.ResponseWriter, frame string) {
	for _, line := range strings.Split(frame, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

const externalTUIHTML = `<!DOCTYPE html>
<html>
<head><title>sin-code tui</title>
<style>
body { margin: 0; background: #1e1e2e; color: #cdd6f4; font-family: monospace; }
#output { white-space: pre-wrap; font-size: 14px; padding: 8px; height: calc(100vh - 80px); overflow-y: auto; }
#input-bar { display: flex; gap: 8px; padding: 8px; background: #181825; border-top: 1px solid #313244; }
#input { flex: 1; background: #313244; color: #cdd6f4; border: 1px solid #45475a; border-radius: 4px; padding: 8px; font-family: monospace; font-size: 14px; resize: vertical; min-height: 40px; max-height: 200px; box-sizing: border-box; }
#input:focus { outline: none; border-color: #7D56F4; }
#send-btn { background: #7D56F4; color: white; border: none; border-radius: 4px; padding: 8px 16px; font-family: monospace; cursor: pointer; white-space: nowrap; }
#send-btn:hover { background: #6B46E0; }
</style>
</head>
<body>
<pre id="output">Connecting...</pre>
<div id="input-bar">
  <textarea id="input" placeholder="Type or paste text here... (Ctrl+Enter sends)" rows="3" autofocus></textarea>
  <button id="send-btn">Send (Ctrl+Enter)</button>
</div>
<script>
const output = document.getElementById('output');
const input = document.getElementById('input');
const sendBtn = document.getElementById('send-btn');

const evt = new EventSource("/stream");
evt.onmessage = function(e) {
    output.textContent = e.data;
    output.scrollTop = output.scrollHeight;
};
evt.onerror = function() {
    output.textContent = "Disconnected. Reconnecting...";
};

function submitText() {
    const text = input.value;
    if (text.trim()) {
        fetch('/submit', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({text: text}),
        });
        input.value = '';
    }
}

sendBtn.addEventListener('click', submitText);

const specialKeys = {
    'ArrowUp': 'up', 'ArrowDown': 'down',
    'ArrowLeft': 'left', 'ArrowRight': 'right',
    'Escape': 'esc', 'PageUp': 'pgup', 'PageDown': 'pgdown',
    'Home': 'home', 'End': 'end', 'Delete': 'delete',
    'Tab': 'tab', 'Backspace': 'backspace',
};

input.addEventListener('keydown', function(e) {
    // Ctrl+Enter submits the full text
    if (e.key === 'Enter' && e.ctrlKey) {
        e.preventDefault();
        submitText();
        return;
    }

    // Ctrl+C sends quit
    if (e.key === 'c' && e.ctrlKey) {
        e.preventDefault();
        fetch('/input', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({type: 'key', key: 'ctrl+c', code: e.keyCode, mod: 4}),
        });
        return;
    }

    // Special keys (arrows, escape, etc.) go to /input directly
    if (specialKeys[e.key]) {
        e.preventDefault();
        let mod = 0;
        if (e.shiftKey) mod |= 1;
        if (e.altKey) mod |= 2;
        if (e.ctrlKey) mod |= 4;
        fetch('/input', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({type: 'key', key: specialKeys[e.key], code: e.keyCode, mod: mod}),
        });
        return;
    }

    // Regular typing: let the textarea handle it naturally.
    // User types text, then Ctrl+Enter to submit.
});
</script>
</body>
</html>`
