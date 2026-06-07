// SPDX-License-Identifier: MIT
// Purpose: Dispatch notifications to TUI channel, stderr, macOS, and webhooks.
package notifications

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

var (
	tuiChanMu sync.RWMutex
	tuiChan   = make(chan *Notification, 100)
)

// TUIBroadcaster returns the channel that the TUI subscribes to.
func TUIBroadcaster() <-chan *Notification {
	tuiChanMu.RLock()
	defer tuiChanMu.RUnlock()
	return tuiChan
}

// SendTUI is the internal non-blocking send used by Dispatcher.
func SendTUI(n *Notification) {
	tuiChanMu.RLock()
	ch := tuiChan
	tuiChanMu.RUnlock()
	select {
	case ch <- n:
	default:
	}
}

// Dispatcher delivers a notification through all enabled channels.
type Dispatcher struct {
	Store     *Store
	WebhookURL string
	Stderr    bool
	MacOS     bool
	HTTPClient *http.Client
}

func NewDispatcher(store *Store) *Dispatcher {
	return &Dispatcher{
		Store:      store,
		Stderr:     true,
		MacOS:      true,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (d *Dispatcher) Send(n *Notification) error {
	if d == nil || n == nil {
		return fmt.Errorf("nil dispatcher or notification")
	}
	if d.Store != nil {
		_ = d.Store.Add(n)
	}
	d.sendTUI(n)
	if d.Stderr {
		d.sendStderr(n)
	}
	if d.MacOS {
		d.sendMacOS(n)
	}
	if d.WebhookURL != "" {
		d.sendWebhook(n)
	}
	return nil
}

func (d *Dispatcher) sendTUI(n *Notification) {
	SendTUI(n)
}

func (d *Dispatcher) sendStderr(n *Notification) {
	fmt.Fprintf(os.Stderr, "\n🔔 [%s] %s\n  %s\n", n.Type, n.Title, n.Message)
}

func (d *Dispatcher) sendMacOS(n *Notification) {
	if _, err := exec.LookPath("osascript"); err != nil {
		return
	}
	script := fmt.Sprintf(`display notification "%s" with title "sin-code" subtitle "%s"`, escape(n.Message), escape(n.Title))
	cmd := exec.Command("osascript", "-e", script)
	_ = cmd.Run()
}

func (d *Dispatcher) sendWebhook(n *Notification) {
	if d.HTTPClient == nil {
		return
	}
	body, _ := jsonMarshal(n)
	req, err := http.NewRequest("POST", d.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Sin-Code-Event", string(n.Type))
	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

func escape(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' {
			out = append(out, '\\', c)
		} else if c == '\n' {
			out = append(out, ' ', ' ')
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}

// jsonMarshal is a small wrapper to keep the dispatch import surface small.
func jsonMarshal(v interface{}) ([]byte, error) {
	return jsonEncode(v)
}
