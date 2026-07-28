// SPDX-License-Identifier: MIT
// Purpose: Command runner — executes `sin <args>` without a shell and returns
// bounded, redacted process output to the Bubbletea update loop.
// Docs: runner.doc.md

package tui

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const maxCommandOutputBytes = 4 << 20

// runFinishedMsg is sent to the model when the subprocess exits.
type runFinishedMsg struct {
	err     error
	elapsed time.Duration
	output  string
}

// runCommand executes an argv-based subprocess and returns its combined output
// as a Bubbletea message. The model is mutated only from Update, never from the
// command goroutine. No shell is involved: metacharacters remain literal argv.
func runCommand(argv []string) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		if len(argv) == 0 {
			err := fmt.Errorf("empty command")
			return runFinishedMsg{err: err, elapsed: time.Since(start), output: "✗ empty command\n"}
		}
		// #nosec G204 -- argv is produced by splitArguments and executed directly;
		// no shell is involved, so metacharacters remain ordinary argument bytes.
		cmd := exec.Command(argv[0], argv[1:]...)
		combined := newBoundedOutput(maxCommandOutputBytes)
		cmd.Stdout = combined
		cmd.Stderr = combined
		err := cmd.Run()
		output := "$ " + formatArgv(argv) + "\n" + combined.String()
		if combined.Truncated() {
			output += fmt.Sprintf("\n… output truncated after %d bytes …\n", maxCommandOutputBytes)
		}
		if err != nil && combined.Len() == 0 {
			output += fmt.Sprintf("✗ command failed: %s\n", err)
		}
		return runFinishedMsg{err: err, elapsed: time.Since(start), output: output}
	}
}

// splitArguments supports spaces, single/double quotes, and backslash escapes
// without shell expansion. Operators such as ;, |, $(), and redirects remain
// ordinary argument bytes.
func splitArguments(input string) ([]string, error) {
	var args []string
	var current strings.Builder
	var quote rune
	escaped := false
	started := false

	flush := func() {
		args = append(args, current.String())
		current.Reset()
		started = false
	}

	for _, r := range input {
		if escaped {
			current.WriteRune(r)
			escaped = false
			started = true
			continue
		}
		if quote != 0 {
			switch {
			case r == quote:
				quote = 0
				started = true
			case r == '\\' && quote == '"':
				escaped = true
			default:
				current.WriteRune(r)
				started = true
			}
			continue
		}
		switch r {
		case '\\':
			escaped = true
			started = true
		case '\'', '"':
			quote = r
			started = true
		case ' ', '\t', '\n', '\r':
			if started {
				flush()
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if escaped {
		return nil, fmt.Errorf("trailing escape")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if started {
		flush()
	}
	return args, nil
}

type boundedOutput struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	max       int
	truncated bool
}

func newBoundedOutput(max int) *boundedOutput {
	return &boundedOutput{max: max}
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(p)
	remaining := b.max - b.buf.Len()
	if remaining <= 0 {
		b.truncated = b.truncated || original > 0
		return original, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.buf.Write(p)
	return original, nil
}

func (b *boundedOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *boundedOutput) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func (b *boundedOutput) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

func formatArgv(argv []string) string {
	parts := make([]string, len(argv))
	redactNext := false
	for i, arg := range argv {
		display := arg
		if redactNext {
			display = "[REDACTED]"
			redactNext = false
		} else if key, value, ok := strings.Cut(arg, "="); ok && isSensitiveName(key) {
			display = key + "=[REDACTED]"
			_ = value
		} else if isSensitiveName(arg) {
			redactNext = true
		}
		if strings.ContainsAny(display, " \t\n\r\"'") {
			parts[i] = strconv.Quote(display)
		} else {
			parts[i] = display
		}
	}
	return strings.Join(parts, " ")
}

func isSensitiveName(raw string) bool {
	name := strings.ToLower(strings.TrimLeft(raw, "-"))
	if key, _, ok := strings.Cut(name, "="); ok {
		name = key
	}
	name = strings.ReplaceAll(name, "_", "-")
	for _, marker := range []string{
		"api-key", "access-key", "private-key", "password", "passwd",
		"secret", "token", "credential",
	} {
		if name == marker || strings.HasSuffix(name, "-"+marker) {
			return true
		}
	}
	return false
}
