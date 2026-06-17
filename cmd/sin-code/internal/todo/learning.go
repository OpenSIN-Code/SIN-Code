// SPDX-License-Identifier: MIT
// Purpose: TodoLearning records completion data and predicts task duration
// from keyword patterns in todo titles (issue #327). All shared state is
// mutex-guarded (M7).
package todo

import (
	"strings"
	"sync"
	"time"
)

// TodoPattern is a learned keyword pattern from completed todos.
type TodoPattern struct {
	Keyword     string        `json:"keyword"`
	AvgDuration time.Duration `json:"avg_duration"`
	SuccessRate float64       `json:"success_rate"`
	Frequency   int           `json:"frequency"`
}

// TodoLearning learns patterns from completed todos.
type TodoLearning struct {
	mu       sync.RWMutex
	patterns map[string]*patternAccum
}

type patternAccum struct {
	totalDuration time.Duration
	count         int
	successCount  int
}

// NewTodoLearning creates a new learning instance.
func NewTodoLearning() *TodoLearning {
	return &TodoLearning{patterns: make(map[string]*patternAccum)}
}

// RecordCompletion records completion data for a todo, extracting keywords
// from the title and accumulating duration/success per keyword.
func (l *TodoLearning) RecordCompletion(todo *Todo, duration time.Duration, success bool) {
	if l == nil || todo == nil {
		return
	}
	keywords := extractKeywords(todo.Title)
	if len(keywords) == 0 {
		keywords = []string{"_general_"}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, kw := range keywords {
		acc, ok := l.patterns[kw]
		if !ok {
			acc = &patternAccum{}
			l.patterns[kw] = acc
		}
		acc.totalDuration += duration
		acc.count++
		if success {
			acc.successCount++
		}
	}
}

// PredictDuration predicts how long a similar todo will take based on keyword
// patterns in the title. Returns a frequency-weighted average of matching
// patterns; returns 0 when no keywords match.
func (l *TodoLearning) PredictDuration(title string) time.Duration {
	if l == nil {
		return 0
	}
	keywords := extractKeywords(title)
	if len(keywords) == 0 {
		return 0
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	var weightedSum time.Duration
	var totalFreq int
	for _, kw := range keywords {
		acc, ok := l.patterns[kw]
		if !ok || acc.count == 0 {
			continue
		}
		avg := acc.totalDuration / time.Duration(acc.count)
		weightedSum += avg * time.Duration(acc.count)
		totalFreq += acc.count
	}
	if totalFreq == 0 {
		return 0
	}
	return weightedSum / time.Duration(totalFreq)
}

// Patterns returns all learned patterns sorted by frequency (descending).
func (l *TodoLearning) Patterns() []TodoPattern {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]TodoPattern, 0, len(l.patterns))
	for kw, acc := range l.patterns {
		p := TodoPattern{
			Keyword:   kw,
			Frequency: acc.count,
		}
		if acc.count > 0 {
			p.AvgDuration = acc.totalDuration / time.Duration(acc.count)
			p.SuccessRate = float64(acc.successCount) / float64(acc.count)
		}
		out = append(out, p)
	}
	sortPatternsByFreq(out)
	return out
}

func sortPatternsByFreq(ps []TodoPattern) {
	for i := 1; i < len(ps); i++ {
		for j := i; j > 0 && ps[j].Frequency > ps[j-1].Frequency; j-- {
			ps[j], ps[j-1] = ps[j-1], ps[j]
		}
	}
}

var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "to": true,
	"for": true, "and": true, "or": true, "of": true, "in": true,
	"on": true, "at": true, "by": true, "with": true, "this": true,
	"that": true, "it": true, "from": true, "into": true, "be": true,
}

// extractKeywords splits a title into lowercase significant words, filtering
// out stop words and tokens shorter than 3 characters.
func extractKeywords(title string) []string {
	words := strings.Fields(strings.ToLower(title))
	var out []string
	seen := map[string]bool{}
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?()[]{}\"'-_/")
		if len(w) < 3 {
			continue
		}
		if stopWords[w] {
			continue
		}
		if seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}
