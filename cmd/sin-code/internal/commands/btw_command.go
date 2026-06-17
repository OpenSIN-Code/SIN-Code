// SPDX-License-Identifier: MIT
// Purpose: /btw built-in slash command (issue #276). Lets the user ask a
// side question that is answered in a temporary one-shot LLM call WITHOUT
// touching the main conversation history. The main agent loop resumes
// unaffected after the answer is displayed. Modeled on Claude Code's
// hidden /btw command from the leaked source (src/commands/btw.ts).
package commands

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// DefaultBtwSystemPrompt is the system prompt used for the ephemeral
// sub-agent when the caller does not provide one. It instructs the model
// to answer briefly and not to invoke tools.
const DefaultBtwSystemPrompt = "You are a helpful assistant answering a quick side question. Answer concisely and directly. Do not invoke tools."

// DefaultBtwMaxLen is the default maximum question length (in runes) before
// the question is truncated. Keeps a runaway /btw from blowing the context
// budget of the ephemeral call.
const DefaultBtwMaxLen = 8000

// DefaultBtwMaxPerSession caps the number of /btw invocations per session
// to limit context cost (issue #276 requirement: max 3 /btw per session).
const DefaultBtwMaxPerSession = 3

// BTWCommand implements /btw <question> — a side question answered in an
// ephemeral LLM call whose result is displayed to the user but NOT added
// to the session history. The main conversation context is preserved.
type BTWCommand struct {
	llm          SideLLM
	systemPrompt string
	maxLen       int
	maxPerSess   int
	count        atomic.Int64
}

// NewBTWCommand constructs a /btw command. If llm is nil, Execute returns
// ErrNoLLM at call time (construction always succeeds so the command can
// still be registered and surfaced in /help). systemPrompt defaults to
// DefaultBtwSystemPrompt when empty.
func NewBTWCommand(llm SideLLM, systemPrompt string) *BTWCommand {
	if systemPrompt == "" {
		systemPrompt = DefaultBtwSystemPrompt
	}
	return &BTWCommand{
		llm:          llm,
		systemPrompt: systemPrompt,
		maxLen:       DefaultBtwMaxLen,
		maxPerSess:   DefaultBtwMaxPerSession,
	}
}

// Name returns "btw".
func (c *BTWCommand) Name() string { return "btw" }

// Description returns the one-line help text.
func (c *BTWCommand) Description() string {
	return "Ask a side question without breaking context"
}

// Execute answers the side question via a one-shot LLM call. The session
// history is never modified — the answer is returned as a string for the
// caller to display. Returns a usage hint when args is empty, ErrNoLLM
// when no LLM client was wired, and an error when the per-session cap is
// exceeded or the context is cancelled.
func (c *BTWCommand) Execute(ctx context.Context, args string, sess *session.Session) (string, error) {
	q := strings.TrimSpace(args)
	if q == "" {
		return "usage: /btw <question> — ask a side question without breaking the main context", nil
	}
	if c.llm == nil {
		return "", ErrNoLLM{CommandName: c.Name()}
	}
	if c.maxPerSess > 0 {
		n := c.count.Add(1)
		if n > int64(c.maxPerSess) {
			return "", fmt.Errorf("/btw: per-session limit (%d) reached", c.maxPerSess)
		}
	}
	if c.maxLen > 0 {
		runes := []rune(q)
		if len(runes) > c.maxLen {
			q = string(runes[:c.maxLen])
		}
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	out, err := c.llm.Complete(ctx, c.systemPrompt, q)
	if err != nil {
		return "", fmt.Errorf("/btw: %w", err)
	}
	return "BTW: " + strings.TrimSpace(out), nil
}
