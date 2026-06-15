// SPDX-License-Identifier: MIT
// Purpose: render the active-instinct block for the system prompt.
// This is where the learning loop *closes* — the model sees the result
// of its own past behavior and can adjust future behavior.
// Docs: inject.doc.md
package instinct

import "strings"

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

// SystemBlockForProject is the convenience entry point used by the
// agent loop.
func (m *Manager) SystemBlockForProject(max int) (string, error) {
	active, err := m.Active()
	if err != nil {
		return "", err
	}
	return RenderSystemBlock(active, max), nil
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
