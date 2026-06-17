// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"
	"sync"
)

type LSPStatus struct {
	mu       sync.Mutex
	server   string
	running  bool
	diags    int
	errors   int
	warnings int
}

func NewLSPStatus() *LSPStatus {
	return &LSPStatus{}
}

func (s *LSPStatus) Update(server string, running bool, diags int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.server = server
	s.running = running
	s.diags = diags
	s.errors = 0
	s.warnings = 0
}

func (s *LSPStatus) UpdateDetailed(server string, running bool, errors, warnings int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.server = server
	s.running = running
	s.errors = errors
	s.warnings = warnings
	s.diags = errors + warnings
}

func (s *LSPStatus) Render(styles Styles, width int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if width < 10 {
		width = 10
	}

	var b strings.Builder
	server := s.server
	if server == "" {
		server = "lsp"
	}

	if s.running {
		b.WriteString(styles.StatusOK.Render("●"))
		b.WriteString(" ")
		b.WriteString(styles.Muted.Render(server))
		if s.errors > 0 {
			b.WriteString("  ")
			b.WriteString(styles.StatusErr.Render(fmt.Sprintf("%d errors", s.errors)))
		}
		if s.warnings > 0 {
			b.WriteString("  ")
			b.WriteString(styles.StatusWarn.Render(fmt.Sprintf("%d warnings", s.warnings)))
		}
		if s.errors == 0 && s.warnings == 0 && s.diags == 0 {
			b.WriteString("  ")
			b.WriteString(styles.StatusOK.Render("clean"))
		} else if s.diags > 0 && s.errors == 0 && s.warnings == 0 {
			b.WriteString("  ")
			b.WriteString(styles.Muted.Render(fmt.Sprintf("%d diagnostics", s.diags)))
		}
	} else {
		b.WriteString(styles.StatusErr.Render("○"))
		b.WriteString(" ")
		b.WriteString(styles.Muted.Render(server))
		b.WriteString("  ")
		b.WriteString(styles.Muted.Render("offline"))
	}

	out := b.String()
	out = strings.TrimSpace(out)
	if visibleWidth(out) > width {
		out = truncateVisible(out, width)
	}
	return out
}

func (s *LSPStatus) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *LSPStatus) Server() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.server
}

func (s *LSPStatus) DiagCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.diags
}

func visibleWidth(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		n++
	}
	return n
}

func truncateVisible(s string, width int) string {
	if visibleWidth(s) <= width {
		return s
	}
	var b strings.Builder
	n := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			b.WriteRune(r)
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			b.WriteRune(r)
			inEsc = true
			continue
		}
		if n >= width-3 {
			break
		}
		b.WriteRune(r)
		n++
	}
	b.WriteString("...")
	return b.String()
}
