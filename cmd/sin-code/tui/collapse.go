// SPDX-License-Identifier: MIT
// Purpose: Collapsible tool output — truncates long tool outputs to a
// configurable number of lines and appends a hint when collapsed. When
// expanded, the full output is returned unchanged. Used by renderToolCard
// to give tool calls (sin_bash, sin_read, etc.) a collapsible body in the
// chat view. Press Tab on a focused tool message to toggle.
package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// CollapsedToolLines is the number of output lines shown when a tool
// card is in its collapsed (default) state.
const CollapsedToolLines = 5

// CollapseOutput truncates output to maxLines lines and appends a hint
// styled with hintStyle. If expanded is true, the full output is
// returned unchanged. Outputs with maxLines or fewer lines are returned
// unchanged regardless of the expanded flag.
func CollapseOutput(output string, maxLines int, expanded bool, hintStyle lipgloss.Style) string {
	if expanded || maxLines <= 0 {
		return output
	}
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}
	visible := lines[:maxLines]
	remaining := len(lines) - maxLines
	hint := hintStyle.Render(fmt.Sprintf("[+%d more lines — press Tab to expand]", remaining))
	return strings.Join(visible, "\n") + "\n" + hint
}
