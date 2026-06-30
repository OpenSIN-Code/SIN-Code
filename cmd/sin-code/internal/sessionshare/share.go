// SPDX-License-Identifier: MIT
// Purpose: Session sharing via portable export (issue #482).
// Exports sessions to self-contained JSON or HTML files.
package sessionshare

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Export struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Title     string    `json:"title"`
	Messages  []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Time    string `json:"time,omitempty"`
}

func (e *Export) ToJSON() ([]byte, error) {
	return json.MarshalIndent(e, "", "  ")
}

func (e *Export) ToHTML() (string, error) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Title}} — SIN-Code Session</title>
<style>
:root { --bg: #0d1117; --fg: #e6edf3; --accent: #58a6ff; --muted: #8b949e; --border: #30363d; }
* { margin: 0; padding: 0; box-sizing: border-box; }
body { background: var(--bg); color: var(--fg); font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; line-height: 1.6; padding: 20px; max-width: 900px; margin: 0 auto; }
h1 { color: var(--accent); margin-bottom: 8px; font-size: 1.4em; }
.meta { color: var(--muted); font-size: 0.85em; margin-bottom: 24px; }
.msg { margin-bottom: 16px; padding: 12px 16px; border-radius: 8px; border: 1px solid var(--border); }
.msg-user { background: #161b22; }
.msg-assistant { background: #1c2128; border-color: var(--accent); }
.msg-tool { background: #0d1117; border-style: dashed; }
.msg pre { background: #161b22; padding: 12px; border-radius: 6px; overflow-x: auto; margin-top: 8px; font-size: 0.85em; }
.role { font-weight: 600; font-size: 0.8em; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 4px; }
.role-user { color: var(--accent); }
.role-assistant { color: #3fb950; }
.role-tool { color: var(--muted); }
</style>
</head>
<body>
<h1>{{.Title}}</h1>
<div class="meta">Exported {{.CreatedAt.Format "2006-01-02 15:04:05"}} · SIN-Code Session Share</div>
{{range .Messages}}
<div class="msg msg-{{.Role}}">
<div class="role role-{{.Role}}">{{.Role}}</div>
<div>{{.Content}}</div>
</div>
{{end}}
</body>
</html>`

	t, err := template.New("session").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := t.Execute(&buf, e); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (e *Export) WriteFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		data, err := e.ToJSON()
		if err != nil {
			return err
		}
		return os.WriteFile(path, data, 0o644)
	case ".html", ".htm":
		html, err := e.ToHTML()
		if err != nil {
			return err
		}
		return os.WriteFile(path, []byte(html), 0o644)
	default:
		return fmt.Errorf("unsupported format: %s (use .json or .html)", ext)
	}
}

func FromJSON(data []byte) (*Export, error) {
	var e Export
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	if e.Version == 0 {
		e.Version = 1
	}
	return &e, nil
}

func FromFile(path string) (*Export, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return FromJSON(data)
}
