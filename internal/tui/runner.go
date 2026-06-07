// SPDX-License-Identifier: MIT
// Purpose: Command runner — executes `sin <args>` in a subprocess, streams
// stdout/stderr line-by-line into the TUI.
// Docs: runner.doc.md

package tui

import (
	"bufio"
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// runFinishedMsg is sent to the model when the subprocess exits.
type runFinishedMsg struct {
	err     error
	elapsed time.Duration
}

// runCommand starts `sh -c <command>` in a goroutine, streams output via
// `stream(line)`, and returns a tea.Cmd that fires runFinishedMsg on exit.
func runCommand(command string, stream func(string)) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		cmd := exec.Command("sh", "-c", command)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			stream(fmt.Sprintf("✗ cannot capture stdout: %s\n", err))
			return runFinishedMsg{err: err, elapsed: time.Since(start)}
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			stream(fmt.Sprintf("✗ cannot capture stderr: %s\n", err))
			return runFinishedMsg{err: err, elapsed: time.Since(start)}
		}
		if err := cmd.Start(); err != nil {
			stream(fmt.Sprintf("✗ failed to start: %s\n", err))
			return runFinishedMsg{err: err, elapsed: time.Since(start)}
		}

		// Notify the user.
		stream(fmt.Sprintf("$ %s\n", command))

		// Stream stdout.
		go scanLines(stdout, stream)
		// Stream stderr (interleaved).
		go scanLines(stderr, func(line string) {
			stream("│ " + line + "\n")
		})

		err = cmd.Wait()
		return runFinishedMsg{err: err, elapsed: time.Since(start)}
	}
}

func scanLines(r interface{ Read(p []byte) (int, error) }, stream func(string)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // up to 1MB lines
	for scanner.Scan() {
		stream(scanner.Text() + "\n")
	}
}
