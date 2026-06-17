// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
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
