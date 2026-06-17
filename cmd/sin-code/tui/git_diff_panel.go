// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type GitRunner func(args ...string) (string, error)

var defaultGitRunner GitRunner = func(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type FileDiffStat struct {
	Path    string
	Status  string
	Added   int
	Removed int
}

type GitDiffPanel struct {
	mu           sync.Mutex
	runner       GitRunner
	renderer     *DiffRenderer
	rawDiff      string
	fileStats    []FileDiffStat
	totalAdded   int
	totalRemoved int
	totalFiles   int
	scroll       int
	loaded       bool
	staged       bool
	fileName     string
	fileMode     bool
}

func NewGitDiffPanel() *GitDiffPanel {
	return &GitDiffPanel{
		runner:   defaultGitRunner,
		renderer: NewDiffRenderer(NewStyles(Themes[0])),
	}
}

func (p *GitDiffPanel) SetRunner(r GitRunner) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r != nil {
		p.runner = r
	}
}

func (p *GitDiffPanel) LoadDiff(staged bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	args := []string{"diff"}
	if staged {
		args = append(args, "--cached")
	}

	raw, err := p.runner(args...)
	if err != nil {
		return err
	}

	p.rawDiff = raw
	p.staged = staged
	p.fileMode = false
	p.fileName = ""
	p.scroll = 0
	p.loaded = true
	p.fileStats = parseFileStats(raw)
	p.totalAdded, p.totalRemoved, p.totalFiles = aggregateStats(p.fileStats)
	return nil
}

func (p *GitDiffPanel) LoadFileDiff(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	raw, err := p.runner("diff", "--", path)
	if err != nil {
		return err
	}

	p.rawDiff = raw
	p.fileMode = true
	p.fileName = path
	p.scroll = 0
	p.loaded = true
	p.fileStats = parseFileStats(raw)
	p.totalAdded, p.totalRemoved, p.totalFiles = aggregateStats(p.fileStats)
	return nil
}

func (p *GitDiffPanel) ScrollUp(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scroll -= n
	if p.scroll < 0 {
		p.scroll = 0
	}
}

func (p *GitDiffPanel) ScrollDown(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scroll += n
}

func (p *GitDiffPanel) Render(styles Styles, width, height int) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.loaded {
		return styles.Muted.Render("  No diff loaded.")
	}

	if p.rawDiff == "" {
		label := "No changes"
		if p.fileMode {
			label = fmt.Sprintf("No changes in %s", p.fileName)
		}
		return styles.Muted.Render("  " + label)
	}

	if width < 20 {
		width = 20
	}

	var b strings.Builder

	mode := "unstaged"
	if p.staged {
		mode = "staged"
	}
	if p.fileMode {
		mode = p.fileName
	}

	b.WriteString(styles.ContentHdr.Render(fmt.Sprintf("git diff — %s", mode)))
	b.WriteString("\n")

	summary := fmt.Sprintf("%d %s changed, +%d -%d",
		p.totalFiles, pluralFiles(p.totalFiles), p.totalAdded, p.totalRemoved)
	b.WriteString(styles.AccentText.Render(summary))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n")

	for _, fs := range p.fileStats {
		header := fmt.Sprintf("  %s: %s (+%d -%d)", fs.Status, fs.Path, fs.Added, fs.Removed)
		b.WriteString(styles.Bold.Render(header))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	lines := p.renderer.ParseDiff(p.rawDiff)
	diffOutput := p.renderer.Render(lines, styles, width)

	allLines := strings.Split(diffOutput, "\n")
	if p.scroll > 0 && p.scroll < len(allLines) {
		allLines = allLines[p.scroll:]
	} else if p.scroll >= len(allLines) {
		allLines = nil
	}

	headerHeight := 4 + len(p.fileStats)
	maxLines := height - headerHeight
	if maxLines < 5 {
		maxLines = 5
	}
	if len(allLines) > maxLines {
		allLines = allLines[:maxLines]
	}

	b.WriteString(strings.Join(allLines, "\n"))

	if p.scroll > 0 || (p.scroll == 0 && len(allLines) < len(strings.Split(diffOutput, "\n"))) {
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render(fmt.Sprintf("  ↑↓ scroll · offset %d", p.scroll)))
	}

	return b.String()
}

func (p *GitDiffPanel) RawDiff() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rawDiff
}

func (p *GitDiffPanel) TotalAdded() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.totalAdded
}

func (p *GitDiffPanel) TotalRemoved() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.totalRemoved
}

func (p *GitDiffPanel) TotalFiles() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.totalFiles
}

func (p *GitDiffPanel) Loaded() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loaded
}

func (p *GitDiffPanel) ScrollOffset() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.scroll
}

func (p *GitDiffPanel) FileStats() []FileDiffStat {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]FileDiffStat, len(p.fileStats))
	copy(cp, p.fileStats)
	return cp
}

func parseFileStats(diffText string) []FileDiffStat {
	if diffText == "" {
		return nil
	}

	sections := strings.Split(diffText, "diff --git ")
	var stats []FileDiffStat

	for _, section := range sections {
		if section == "" {
			continue
		}
		section = "diff --git " + section

		stat := FileDiffStat{Status: "modified"}

		lines := strings.Split(section, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "+++ b/") {
				stat.Path = strings.TrimPrefix(line, "+++ b/")
			} else if strings.HasPrefix(line, "+++ /dev/null") {
				stat.Status = "deleted"
			} else if strings.Contains(line, "new file mode") {
				stat.Status = "added"
			} else if strings.Contains(line, "deleted file mode") {
				stat.Status = "deleted"
			} else if strings.HasPrefix(line, "rename to ") {
				stat.Status = "renamed"
			} else if strings.HasPrefix(line, "copy to ") {
				stat.Status = "copied"
			}

			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				stat.Added++
			}
			if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				stat.Removed++
			}
		}

		if stat.Path == "" {
			for _, line := range lines {
				if strings.HasPrefix(line, "--- a/") {
					stat.Path = strings.TrimPrefix(line, "--- a/")
					break
				}
			}
		}

		if stat.Path != "" {
			stats = append(stats, stat)
		}
	}

	return stats
}

func aggregateStats(stats []FileDiffStat) (added, removed, files int) {
	for _, s := range stats {
		added += s.Added
		removed += s.Removed
		files++
	}
	return
}
