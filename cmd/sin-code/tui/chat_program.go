// SPDX-License-Identifier: MIT
// Purpose: thin bridge so *tea.Program (which has Send(msg tea.Msg)) can be
// stored in the Model.Program field typed as the local teaProgramIface
// interface. Imported in main.go via tui.ProgramFromTeaProgram().
package tui

import tea "github.com/charmbracelet/bubbletea"

// ProgramFromTeaProgram wraps a *tea.Program so it satisfies
// teaProgramIface. Returns nil if p is nil.
func ProgramFromTeaProgram(p *tea.Program) teaProgramIface {
	if p == nil {
		return nil
	}
	return teaProgramWrapper{p}
}

type teaProgramWrapper struct {
	p *tea.Program
}

func (w teaProgramWrapper) Send(msg any) {
	w.p.Send(msg)
}
