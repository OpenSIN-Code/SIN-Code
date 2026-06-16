// SPDX-License-Identifier: MIT
// Purpose: render the active-instinct block for the system prompt.
// This is where the learning loop *closes* — the model sees the result
// of its own past behavior and can adjust future behavior.
// Docs: inject.doc.md
package instinct

import (
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/style"
)

// RenderSystemBlock produces a compact, deterministic block of active
// instincts to prepend/append to the agent's system prompt. Returns ""
// when none active.
//
// Ordering is strongest-first; confidence is surfaced so the model can
// weigh soft guidance vs. strong habits.
func RenderSystemBlock(active []*Instinct, max int) string {
	if len(active) == 0 {
		return ""
	}
	SortByConfidence(active)
	if max > 0 && len(active) > max {
		active = active[:max]
	}
	var b strings.Builder
	b.WriteString("# Learned instincts\n")
	b.WriteString("Apply these learned behaviors when relevant. Higher confidence = stronger habit.\n\n")
	for _, i := range active {
		strength := "consider"
		switch {
		case i.Confidence >= 0.80:
			strength = "strongly prefer"
		case i.Confidence >= 0.65:
			strength = "prefer"
		}
		b.WriteString("- (")
		b.WriteString(ftoaShort(i.Confidence))
		b.WriteString(", ")
		b.WriteString(i.Domain)
		b.WriteString(") ")
		b.WriteString(strings.TrimSpace(i.Trigger))
		b.WriteString(" — ")
		b.WriteString(strength)
		b.WriteString(": ")
		b.WriteString(strings.TrimSpace(i.Action))
		b.WriteString("\n")
	}
	return b.String()
}

// RenderSystemBlockWithVerbosity (issue #167) is RenderSystemBlock plus
// a verbosity ruleset appended after the instinct list.
//
// The mode parameter is canonical: empty, "default", or "verbose"
// produces exactly the RenderSystemBlock output (instincts alone).
// Any non-default mode appends the matching style block via
// `style.RenderRules` (which also embeds skillBody verbatim when
// non-empty). Order is stable: instincts first, then style. The two
// segments are separated by exactly one blank line; the skill body's
// own trailing newlines are preserved so downstream Markdown linters
// do not need to re-add them.
//
// Output is byte-stable for any fixed (active, max, mode, skillBody)
// input; this is the prerequisite for the system-prompt hash metric.
func RenderSystemBlockWithVerbosity(active []*Instinct, max int, mode string, skillBody string) string {
	raw := RenderSystemBlock(active, max)
	styleBlock := style.RenderRules(style.ParseMode(mode), skillBody)
	if raw == "" {
		return styleBlock
	}
	if styleBlock == "" {
		return raw
	}
	return strings.TrimRight(raw, "\n") + "\n\n" + styleBlock
}

// SystemBlockForProject is the convenience entry point used by the
// agent loop.
func (m *Manager) SystemBlockForProject(max int) (string, error) {
	active, err := m.Active()
	if err != nil {
		return "", err
	}
	return RenderSystemBlock(active, max), nil
}

// SystemBlockForProjectWithStyle is the verbosity-aware convenience
// entry point. Pass the user-configured style level (e.g. from
// `llm.style` or the chat --style flag) and any skill body the caller
// has pulled from disk. Returns the composed system-prompt fragment.
func (m *Manager) SystemBlockForProjectWithStyle(max int, mode, skillBody string) (string, error) {
	active, err := m.Active()
	if err != nil {
		return "", err
	}
	return RenderSystemBlockWithVerbosity(active, max, mode, skillBody), nil
}

func ftoaShort(f float64) string {
	whole := int(f)
	frac := int((f-float64(whole))*100 + 0.5)
	if frac < 0 {
		frac = -frac
	}
	fs := itoa(frac)
	if len(fs) < 2 {
		fs = "0" + fs
	}
	return itoa(whole) + "." + fs
}
