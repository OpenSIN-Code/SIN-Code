// SPDX-License-Identifier: MIT
package agentloop

import (
	"fmt"
	"strings"
	"sync"
)

type ContextMeter struct {
	mu        sync.RWMutex
	used      int
	maxTokens int
}

func NewContextMeter(maxTokens int) *ContextMeter {
	if maxTokens <= 0 {
		maxTokens = 128000
	}
	return &ContextMeter{maxTokens: maxTokens}
}

func (m *ContextMeter) Update(used int) {
	m.mu.Lock()
	m.used = used
	m.mu.Unlock()
}

func (m *ContextMeter) Usage() (used, max int, pct float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	used = m.used
	max = m.maxTokens
	if max > 0 {
		pct = float64(used) / float64(max) * 100
	}
	return
}

func (m *ContextMeter) ShouldWarn() bool {
	_, _, pct := m.Usage()
	return pct >= 80
}

func (m *ContextMeter) ShouldCompact() bool {
	_, _, pct := m.Usage()
	return pct >= 90
}

func (m *ContextMeter) String() string {
	used, max, pct := m.Usage()
	filled := int(pct / 10)
	if filled > 10 {
		filled = 10
	}
	if filled < 0 {
		filled = 0
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 10-filled)
	return fmt.Sprintf("Context: %s %.0f%% (%dk/%dk)", bar, pct, used/1000, max/1000)
}

func (m *ContextMeter) SetMax(max int) {
	m.mu.Lock()
	m.maxTokens = max
	m.mu.Unlock()
}
