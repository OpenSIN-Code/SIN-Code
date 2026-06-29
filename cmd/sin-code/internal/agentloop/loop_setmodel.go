// SPDX-License-Identifier: MIT
// Purpose: mid-session model switching. SetModel updates the LLM model
// in-place without losing conversation context (mandate M7: thread-safe).
package agentloop

import (
	"context"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// SetModel switches the LLM model mid-session. It rebuilds the Completion
// closure via CompletionBuilder (if set) so the new model is used on the
// next Run call. The session, messages, and all loop state are preserved —
// only the model name and the completion function change. Thread-safe (M7).
func (l *Loop) SetModel(model string) {
	l.modelMu.Lock()
	defer l.modelMu.Unlock()
	l.Model = model
	if l.CompletionBuilder != nil {
		l.Completion = l.CompletionBuilder(model)
	}
}

// GetModel returns the current LLM model name (thread-safe, M7).
func (l *Loop) GetModel() string {
	l.modelMu.RLock()
	defer l.modelMu.RUnlock()
	return l.Model
}

// getCompletion returns the current Completion function under the read
// lock. This ensures Run never observes a partially-updated function
// pointer when SetModel is called from another goroutine.
func (l *Loop) getCompletion() func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
	l.modelMu.RLock()
	defer l.modelMu.RUnlock()
	return l.Completion
}
