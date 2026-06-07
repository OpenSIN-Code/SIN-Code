package tui

import (
	"fmt"
	"strings"
	"time"
)

func RenderEFMView(stacks []EFMStack, styles Styles, width, height int, spinner Spinner) string {
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("⚡ EFM — Ephemeral Full-Stack Mocking"))
	b.WriteString(" ")
	b.WriteString(spinner.View(styles.Spinner))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n\n")

	if len(stacks) == 0 {
		b.WriteString(styles.Muted.Render("  No active stacks."))
		b.WriteString("\n\n")
		b.WriteString(styles.Muted.Render("  Press n to spin up a new ephemeral stack."))
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render("  Press r to refresh from the EFM backend."))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString(styles.AccentText.Render(fmt.Sprintf("  %-22s  %-12s  %-32s  %s", "Name", "Status", "URL", "TTL")))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", max(width-6, 10))))
	b.WriteString("\n")

	for _, st := range stacks {
		ttl := fmt.Sprintf("%ds", st.TTL)
		if st.TTL == 0 {
			ttl = "—"
		}
		status := st.Status
		line := fmt.Sprintf("  %-22s  %-12s  %-32s  %s", truncate(st.Name, 22), status, truncate(st.URL, 32), ttl)
		switch status {
		case "running", "up":
			b.WriteString(styles.StatusOK.Render(line))
		case "starting":
			b.WriteString(styles.StatusWarn.Render(line))
		case "down", "error":
			b.WriteString(styles.StatusErr.Render(line))
		default:
			b.WriteString(styles.Content.Render(line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(fmt.Sprintf("  %d stack(s)  ·  refreshed %s", len(stacks), time.Now().Format("15:04:05"))))
	b.WriteString("\n")

	return b.String()
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
