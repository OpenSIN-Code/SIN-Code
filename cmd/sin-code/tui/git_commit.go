// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

type CommitFilter interface {
	FilterCommitMessage(msg string) string
}

type noopCommitFilter struct{}

func (noopCommitFilter) FilterCommitMessage(msg string) string { return msg }

type GitCommitFlow struct {
	mu          sync.Mutex
	runner      GitRunner
	filter      CommitFilter
	active      bool
	stagedCount int
	added       int
	removed     int
	message     string
	diff        string
}

func NewGitCommitFlow() *GitCommitFlow {
	return &GitCommitFlow{
		runner: defaultGitRunner,
		filter: noopCommitFilter{},
	}
}

func (f *GitCommitFlow) SetRunner(r GitRunner) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r != nil {
		f.runner = r
	}
}

func (f *GitCommitFlow) SetFilter(filter CommitFilter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if filter != nil {
		f.filter = filter
	} else {
		f.filter = noopCommitFilter{}
	}
}

func (f *GitCommitFlow) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	porcelain, err := f.runner("status", "--porcelain")
	if err != nil {
		return err
	}

	staged := 0
	for _, line := range strings.Split(porcelain, "\n") {
		if len(line) < 2 {
			continue
		}
		if line[0] != ' ' && line[0] != '?' {
			staged++
		}
	}

	diff, err := f.runner("diff", "--cached")
	if err != nil {
		return err
	}

	f.active = true
	f.stagedCount = staged
	f.diff = diff
	f.message = f.generateMessageLocked(diff)
	f.added, f.removed = countDiffLines(diff)
	return nil
}

func (f *GitCommitFlow) GenerateMessage(diff string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.generateMessageLocked(diff)
}

func (f *GitCommitFlow) generateMessageLocked(diff string) string {
	stats := parseFileStats(diff)
	if len(stats) == 0 {
		return "chore: update files"
	}

	commitType := detectCommitType(stats)
	scope := detectScope(stats)
	description := detectDescription(stats, diff)

	msg := commitType
	if scope != "" {
		msg += "(" + scope + ")"
	}
	msg += ": " + description

	if f.filter != nil {
		msg = f.filter.FilterCommitMessage(msg)
	}

	return msg
}

func (f *GitCommitFlow) Render(styles Styles, width, height int) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.active {
		return ""
	}

	if width < 40 {
		width = 40
	}

	var b strings.Builder

	b.WriteString("┌─ Commit ")
	b.WriteString(strings.Repeat("─", max(width-12, 4)))
	b.WriteString("┐\n")

	b.WriteString("│\n")
	b.WriteString(fmt.Sprintf("│  Staged: %d %s (+%d -%d)\n",
		f.stagedCount, pluralFiles(f.stagedCount), f.added, f.removed))
	b.WriteString("│\n")
	b.WriteString("│  Message:\n")

	msgLines := strings.Split(f.message, "\n")
	for _, ml := range msgLines {
		b.WriteString(fmt.Sprintf("│  %s\n", styles.AccentText.Render(ml)))
	}

	b.WriteString("│\n")
	b.WriteString("│  [Enter] Commit  [Esc] Cancel\n")
	b.WriteString("└")
	b.WriteString(strings.Repeat("─", max(width-2, 4)))
	b.WriteString("┘")

	return b.String()
}

func (f *GitCommitFlow) Execute(message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	_, err := f.runner("commit", "-m", message)
	if err != nil {
		return err
	}

	f.active = false
	f.message = ""
	f.diff = ""
	f.stagedCount = 0
	f.added = 0
	f.removed = 0
	return nil
}

func (f *GitCommitFlow) Active() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active
}

func (f *GitCommitFlow) Cancel() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active = false
}

func (f *GitCommitFlow) Message() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.message
}

func (f *GitCommitFlow) SetMessage(msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.message = msg
}

func (f *GitCommitFlow) StagedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stagedCount
}

func detectCommitType(stats []FileDiffStat) string {
	hasTest := false
	hasGo := false
	hasMd := false
	hasConfig := false
	hasStyle := false
	hasNonConfig := false

	for _, s := range stats {
		ext := filepath.Ext(s.Path)
		base := filepath.Base(s.Path)

		if strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, ".test.go") {
			hasTest = true
		} else if ext == ".go" {
			hasGo = true
			hasNonConfig = true
		} else if ext == ".md" {
			hasMd = true
			hasNonConfig = true
		} else if ext == ".css" || ext == ".html" || ext == ".scss" {
			hasStyle = true
			hasNonConfig = true
		} else if ext == ".yml" || ext == ".yaml" || ext == ".toml" || ext == ".json" || ext == ".mod" || ext == ".sum" {
			hasConfig = true
		} else {
			hasNonConfig = true
		}
	}

	if hasTest && !hasGo && !hasMd {
		return "test"
	}
	if hasMd && !hasGo && !hasNonConfig {
		return "docs"
	}
	if hasStyle && !hasGo {
		return "style"
	}
	if hasConfig && !hasNonConfig {
		return "chore"
	}
	if hasGo {
		return "feat"
	}
	if hasMd {
		return "docs"
	}
	return "chore"
}

func detectScope(stats []FileDiffStat) string {
	if len(stats) == 0 {
		return ""
	}

	var dirs []string
	for _, s := range stats {
		dir := filepath.Dir(s.Path)
		if dir == "." || dir == "/" {
			continue
		}
		dirs = append(dirs, dir)
	}

	if len(dirs) == 0 {
		return ""
	}

	common := longestCommonPrefix(dirs)
	parts := strings.Split(common, "/")
	if len(parts) == 0 {
		return ""
	}

	last := parts[len(parts)-1]
	if last == "" {
		return ""
	}

	if len(stats) == 1 {
		dir := filepath.Dir(stats[0].Path)
		parts := strings.Split(dir, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-1]
		}
		return last
	}

	return last
}

func detectDescription(stats []FileDiffStat, diff string) string {
	if len(stats) == 0 {
		return "update files"
	}

	if len(stats) == 1 {
		base := filepath.Base(stats[0].Path)
		ext := filepath.Ext(base)
		name := strings.TrimSuffix(base, ext)
		name = strings.TrimSuffix(name, "_test")

		verb := "update"
		if stats[0].Status == "added" {
			verb = "add"
		} else if stats[0].Status == "deleted" {
			verb = "remove"
		}

		return fmt.Sprintf("%s %s", verb, name)
	}

	main := stats[0]
	verb := "update"
	if main.Status == "added" {
		verb = "add"
	} else if main.Status == "deleted" {
		verb = "remove"
	}

	return fmt.Sprintf("%s %s and %d more", verb, filepath.Base(main.Path), len(stats)-1)
}

func longestCommonPrefix(dirs []string) string {
	if len(dirs) == 0 {
		return ""
	}
	prefix := dirs[0]
	for _, d := range dirs[1:] {
		for !strings.HasPrefix(d+"/", prefix+"/") && prefix != "" {
			idx := strings.LastIndex(prefix, "/")
			if idx < 0 {
				prefix = ""
				break
			}
			prefix = prefix[:idx]
		}
		if prefix == "" {
			break
		}
	}
	return prefix
}

func countDiffLines(diff string) (added, removed int) {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return
}
