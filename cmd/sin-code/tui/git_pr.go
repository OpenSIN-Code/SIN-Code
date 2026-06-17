// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type GHRunner func(args ...string) (string, error)

var defaultGHRunner GHRunner = func(args ...string) (string, error) {
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type PRInfo struct {
	Branch     string
	BaseBranch string
	Commits    []GitLogEntry
	Title      string
	Body       string
}

type PRCreateFlow struct {
	mu       sync.Mutex
	runner   GitRunner
	ghRunner GHRunner
	active   bool
	info     PRInfo
}

func NewPRCreateFlow() *PRCreateFlow {
	return &PRCreateFlow{
		runner:   defaultGitRunner,
		ghRunner: defaultGHRunner,
	}
}

func (f *PRCreateFlow) SetRunner(r GitRunner) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r != nil {
		f.runner = r
	}
}

func (f *PRCreateFlow) SetGHRunner(r GHRunner) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r != nil {
		f.ghRunner = r
	}
}

func (f *PRCreateFlow) Start() error {
	f.mu.Lock()
	f.active = true
	f.mu.Unlock()

	if err := f.DetectInfo(); err != nil {
		f.mu.Lock()
		f.active = false
		f.mu.Unlock()
		return err
	}

	f.mu.Lock()
	f.info.Title = f.generateTitleLocked()
	f.info.Body = f.generateBodyLocked()
	f.mu.Unlock()

	return nil
}

func (f *PRCreateFlow) DetectInfo() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	branch, err := f.runner("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	f.info.Branch = strings.TrimSpace(branch)

	base := f.detectBaseLocked()
	f.info.BaseBranch = base

	logOut, err := f.runner("log", "--oneline", "--format=%H|%an|%ad|%s", "--date=short", base+"..HEAD")
	if err != nil {
		f.info.Commits = nil
	} else {
		f.info.Commits = parseLogEntries(logOut)
	}

	return nil
}

func (f *PRCreateFlow) detectBaseLocked() string {
	for _, candidate := range []string{"main", "master", "develop"} {
		_, err := f.runner("rev-parse", "--verify", candidate)
		if err == nil {
			return candidate
		}
	}
	return "main"
}

func (f *PRCreateFlow) GenerateTitle() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.generateTitleLocked()
}

func (f *PRCreateFlow) generateTitleLocked() string {
	if len(f.info.Commits) == 0 {
		if f.info.Branch != "" {
			return f.info.Branch
		}
		return "Update"
	}

	msg := f.info.Commits[0].Message
	if len(msg) > 60 {
		msg = msg[:60]
	}
	return msg
}

func (f *PRCreateFlow) GenerateBody() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.generateBodyLocked()
}

func (f *PRCreateFlow) generateBodyLocked() string {
	if len(f.info.Commits) == 0 {
		return ""
	}

	var b strings.Builder
	for _, c := range f.info.Commits {
		hash := c.Hash
		if len(hash) > 7 {
			hash = hash[:7]
		}
		b.WriteString(fmt.Sprintf("- %s (%s)", c.Message, hash))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (f *PRCreateFlow) Render(styles Styles, width, height int) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.active {
		return ""
	}

	if width < 50 {
		width = 50
	}

	var b strings.Builder

	b.WriteString("┌─ Create PR ")
	b.WriteString(strings.Repeat("─", max(width-15, 4)))
	b.WriteString("┐\n")

	b.WriteString("│\n")
	b.WriteString(fmt.Sprintf("│  Branch: %s → %s\n",
		styles.AccentText.Render(f.info.Branch),
		styles.Muted.Render(f.info.BaseBranch)))
	b.WriteString(fmt.Sprintf("│  Commits: %d\n", len(f.info.Commits)))
	b.WriteString("│\n")

	b.WriteString("│  Title:\n")
	title := f.info.Title
	if title == "" {
		title = f.generateTitleLocked()
	}
	b.WriteString(fmt.Sprintf("│  %s\n", styles.AccentText.Render(title)))
	b.WriteString("│\n")

	b.WriteString("│  Body:\n")
	body := f.info.Body
	if body == "" {
		body = f.generateBodyLocked()
	}
	for _, line := range strings.Split(body, "\n") {
		b.WriteString(fmt.Sprintf("│  %s\n", line))
	}

	b.WriteString("│\n")
	b.WriteString("│  [Enter] Create  [Esc] Cancel\n")
	b.WriteString("└")
	b.WriteString(strings.Repeat("─", max(width-2, 4)))
	b.WriteString("┘")

	return b.String()
}

func (f *PRCreateFlow) Execute() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	title := f.info.Title
	if title == "" {
		title = f.generateTitleLocked()
	}
	body := f.info.Body
	if body == "" {
		body = f.generateBodyLocked()
	}

	_, err := f.ghRunner("pr", "create",
		"--title", title,
		"--body", body,
		"--base", f.info.BaseBranch,
		"--head", f.info.Branch,
	)
	if err != nil {
		return err
	}

	f.active = false
	return nil
}

func (f *PRCreateFlow) Active() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active
}

func (f *PRCreateFlow) Cancel() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active = false
}

func (f *PRCreateFlow) Info() PRInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.info
}

func (f *PRCreateFlow) SetTitle(title string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.info.Title = title
}

func (f *PRCreateFlow) SetBody(body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.info.Body = body
}
