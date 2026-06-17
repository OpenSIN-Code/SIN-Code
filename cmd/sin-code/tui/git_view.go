// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type GitStatus struct {
	Branch    string
	Modified  int
	Staged    int
	Untracked int
	Ahead     int
	Behind    int
	Clean     bool
}

type GitSubMode int

const (
	GitSubNone GitSubMode = iota
	GitSubDiff
	GitSubCommit
	GitSubLog
	GitSubPR
)

type GitViewState struct {
	DiffPanel  *GitDiffPanel
	CommitFlow *GitCommitFlow
	LogView    *GitLogView
	PRFlow     *PRCreateFlow
	SubMode    GitSubMode
	MenuOpen   bool
}

func NewGitViewState() *GitViewState {
	return &GitViewState{
		DiffPanel:  NewGitDiffPanel(),
		CommitFlow: NewGitCommitFlow(),
		LogView:    NewGitLogView(),
		PRFlow:     NewPRCreateFlow(),
		SubMode:    GitSubNone,
		MenuOpen:   false,
	}
}

func (gs *GitViewState) OpenMenu() {
	gs.MenuOpen = true
	gs.SubMode = GitSubNone
}

func (gs *GitViewState) CloseMenu() {
	gs.MenuOpen = false
	gs.SubMode = GitSubNone
}

func (gs *GitViewState) HandleMenuKey(key string) bool {
	if !gs.MenuOpen {
		return false
	}
	switch key {
	case "d":
		gs.MenuOpen = false
		gs.SubMode = GitSubDiff
		_ = gs.DiffPanel.LoadDiff(false)
		return true
	case "c":
		gs.MenuOpen = false
		gs.SubMode = GitSubCommit
		_ = gs.CommitFlow.Start()
		return true
	case "l":
		gs.MenuOpen = false
		gs.SubMode = GitSubLog
		_ = gs.LogView.Load(20)
		return true
	case "p":
		gs.MenuOpen = false
		gs.SubMode = GitSubPR
		_ = gs.PRFlow.Start()
		return true
	case "esc", "g":
		gs.CloseMenu()
		return true
	}
	return false
}

func (gs *GitViewState) HandleSubModeKey(key string) bool {
	switch gs.SubMode {
	case GitSubDiff:
		switch key {
		case "up", "k":
			gs.DiffPanel.ScrollUp(5)
			return true
		case "down", "j":
			gs.DiffPanel.ScrollDown(5)
			return true
		case "esc":
			gs.SubMode = GitSubNone
			return true
		}
	case GitSubCommit:
		switch key {
		case "enter":
			_ = gs.CommitFlow.Execute(gs.CommitFlow.Message())
			gs.SubMode = GitSubNone
			return true
		case "esc":
			gs.CommitFlow.Cancel()
			gs.SubMode = GitSubNone
			return true
		}
	case GitSubLog:
		switch key {
		case "up", "k":
			gs.LogView.MoveUp()
			return true
		case "down", "j":
			gs.LogView.MoveDown()
			return true
		case "esc":
			gs.SubMode = GitSubNone
			return true
		}
	case GitSubPR:
		switch key {
		case "enter":
			_ = gs.PRFlow.Execute()
			gs.SubMode = GitSubNone
			return true
		case "esc":
			gs.PRFlow.Cancel()
			gs.SubMode = GitSubNone
			return true
		}
	}
	return false
}

func (gs *GitViewState) HandleKey(key string) bool {
	if gs.MenuOpen {
		return gs.HandleMenuKey(key)
	}
	if gs.SubMode != GitSubNone {
		return gs.HandleSubModeKey(key)
	}
	if key == "g" {
		gs.OpenMenu()
		return true
	}
	return false
}

func (gs *GitViewState) IsActive() bool {
	return gs.MenuOpen || gs.SubMode != GitSubNone
}

func RenderGitMenu(styles Styles, width int) string {
	items := []struct {
		key  string
		desc string
	}{
		{"d", "git diff"},
		{"c", "git commit"},
		{"l", "git log"},
		{"p", "create PR"},
	}

	popupWidth := 28
	if width < popupWidth {
		popupWidth = width - 4
	}

	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" Git Actions"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", max(popupWidth-6, 10))))
	b.WriteString("\n")

	for _, item := range items {
		line := fmt.Sprintf("  %s  %s", styles.FooterKey.Render(item.key), item.desc)
		b.WriteString(styles.PopupItem.Render(padRight(line, popupWidth-4)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  esc close"))

	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c(styles.Theme.Accent)).
		Foreground(c(styles.Theme.Text)).
		Background(c(styles.Theme.Background)).
		Padding(1, 2).
		Width(popupWidth)

	return popupStyle.Render(b.String())
}

func (gs *GitViewState) Render(styles Styles, width, height int) string {
	if gs.MenuOpen {
		return RenderGitMenu(styles, width)
	}

	switch gs.SubMode {
	case GitSubDiff:
		return gs.DiffPanel.Render(styles, width, height)
	case GitSubCommit:
		return gs.CommitFlow.Render(styles, width, height)
	case GitSubLog:
		return gs.LogView.Render(styles, width, height)
	case GitSubPR:
		return gs.PRFlow.Render(styles, width, height)
	}

	return ""
}

type GitRefreshMsg struct {
	Status GitStatus
}

func GetGitStatus() GitStatus {
	st := GitStatus{Clean: true}

	branch, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return GitStatus{Clean: true}
	}
	st.Branch = strings.TrimSpace(branch)
	if st.Branch == "" {
		return GitStatus{Clean: true}
	}

	porcelain, err := gitOutput("status", "--porcelain")
	if err == nil {
		for _, line := range strings.Split(porcelain, "\n") {
			if len(line) < 2 {
				continue
			}
			if strings.HasPrefix(line, "??") {
				st.Untracked++
				continue
			}
			if line[0] != ' ' && line[0] != '?' {
				st.Staged++
			}
			if line[1] != ' ' && line[1] != '?' {
				st.Modified++
			}
		}
		if st.Modified > 0 || st.Staged > 0 || st.Untracked > 0 {
			st.Clean = false
		}
	}

	st.Ahead = gitCount("rev-list", "--count", "@{u}..HEAD")
	st.Behind = gitCount("rev-list", "--count", "HEAD..@{u}")

	return st
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func gitCount(args ...string) int {
	out, err := gitOutput(args...)
	if err != nil {
		return 0
	}
	var n int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &n)
	return n
}

func pluralFiles(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

func gitRow(label, value string) string {
	return fmt.Sprintf("  %-10s %s\n", label+":", value)
}

func RenderGitStatus(status GitStatus, styles Styles, width int) string {
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("git"))
	b.WriteString(" ")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-6, 0))))
	b.WriteString("\n")

	b.WriteString(gitRow("Branch", styles.FooterVal.Render(status.Branch)))
	b.WriteString(gitRow("Modified", styles.FooterVal.Render(pluralFiles(status.Modified))))
	b.WriteString(gitRow("Staged", styles.FooterVal.Render(pluralFiles(status.Staged))))
	b.WriteString(gitRow("Untracked", styles.FooterVal.Render(pluralFiles(status.Untracked))))

	cleanStr := styles.StatusErr.Render("no")
	if status.Clean {
		cleanStr = styles.StatusOK.Render("yes")
	}
	b.WriteString(gitRow("Clean", cleanStr))

	b.WriteString(gitRow("Ahead/Behind", styles.FooterVal.Render(fmt.Sprintf("↑%d ↓%d", status.Ahead, status.Behind))))

	return b.String()
}

func GitBranchShort(status GitStatus) string {
	if status.Branch == "" {
		return ""
	}
	s := status.Branch
	if status.Ahead > 0 {
		s += fmt.Sprintf(" ↑%d", status.Ahead)
	}
	if status.Behind > 0 {
		s += fmt.Sprintf(" ↓%d", status.Behind)
	}
	return s
}

func RefreshGitCmd() tea.Cmd {
	return func() tea.Msg {
		return GitRefreshMsg{Status: GetGitStatus()}
	}
}
