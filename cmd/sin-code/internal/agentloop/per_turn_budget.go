// SPDX-License-Identifier: MIT
// Purpose: backward-compatible methods on the PerTurnBudget type
// declared in budget.go (issue #375). The AddThinking/AddTokens/
// ThinkingExceeded/TokenExceeded/Stats API predates the Charge/
// PreFlight API and is retained for test compatibility.
package agentloop

// AddThinking accumulates n thinking tokens against the budget.
func (p *PerTurnBudget) AddThinking(n int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.thinkingUsed += n
	p.thinkingAllTime += n
	p.mu.Unlock()
}

// AddTokens accumulates n output tokens against the budget.
func (p *PerTurnBudget) AddTokens(n int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.tokensUsed += n
	p.tokensAllTime += n
	p.mu.Unlock()
}

// ThinkingExceeded reports whether the thinking-token budget has been
// surpassed. A limit of 0 (unlimited) never exceeds.
func (p *PerTurnBudget) ThinkingExceeded() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.thinkingBudget <= 0 {
		return false
	}
	return p.thinkingUsed > p.thinkingBudget
}

// TokenExceeded reports whether the output-token budget has been
// surpassed. A limit of 0 (unlimited) never exceeds.
func (p *PerTurnBudget) TokenExceeded() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tokenBudget <= 0 {
		return false
	}
	return p.tokensUsed > p.tokenBudget
}

// Stats returns the accumulated thinking-token and output-token usage.
func (p *PerTurnBudget) Stats() (thinking, tokens int) {
	if p == nil {
		return 0, 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.thinkingUsed, p.tokensUsed
}
