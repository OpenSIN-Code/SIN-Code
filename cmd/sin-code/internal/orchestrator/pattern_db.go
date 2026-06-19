// SPDX-License-Identifier: MIT
// Purpose: Pattern Learning database — stores completed plan task-sequences
// and aggregates them into reusable patterns (issue #288). When a new prompt
// arrives, the PatternMatcher finds similar past sequences and returns
// probability-weighted task predictions that feed into the DeepPlanner.
package orchestrator

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// TaskSequenceRecord is one completed plan's task sequence stored in the DB.
type TaskSequenceRecord struct {
	ID         int64
	PromptHash string
	PromptText string
	TaskSeq    []TaskType
	Success    bool
	DurationMs int64
	TokensUsed int
	Cost       float64
	CreatedAt  time.Time
}

// TaskPattern is an aggregated pattern for a given prompt hash.
type TaskPattern struct {
	TaskType      TaskType `json:"task_type"`
	Position      int      `json:"position"`
	Probability   float64  `json:"probability"`
	AvgDurationMs int64    `json:"avg_duration_ms"`
	AvgTokens     int      `json:"avg_tokens"`
	Frequency     int      `json:"frequency"`
}

// PatternPrediction is the result of matching a new prompt against the
// pattern DB. It contains the predicted task types with probabilities.
type PatternPrediction struct {
	PromptHash  string        `json:"prompt_hash"`
	MatchCount  int           `json:"match_count"`
	Patterns    []TaskPattern `json:"patterns"`
	SuccessRate float64       `json:"success_rate"`
}

// PatternDB stores and retrieves task sequence patterns from SQLite.
type PatternDB struct {
	db *sql.DB
}

// NewPatternDB creates a PatternDB. If db is nil, all methods are no-ops.
func NewPatternDB(db *sql.DB) (*PatternDB, error) {
	if db == nil {
		return &PatternDB{}, nil
	}
	const schema = `
CREATE TABLE IF NOT EXISTS task_sequences (
    id INTEGER PRIMARY KEY,
    prompt_hash TEXT NOT NULL,
    prompt_text TEXT NOT NULL,
    task_sequence TEXT NOT NULL,
    success INTEGER NOT NULL,
    duration_ms INTEGER,
    tokens_used INTEGER,
    cost REAL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_task_sequences_hash ON task_sequences(prompt_hash);
CREATE INDEX IF NOT EXISTS idx_task_sequences_success ON task_sequences(success);
CREATE TABLE IF NOT EXISTS task_patterns (
    prompt_hash TEXT NOT NULL,
    task_type TEXT NOT NULL,
    position INTEGER NOT NULL,
    probability REAL NOT NULL,
    avg_duration_ms INTEGER,
    avg_tokens INTEGER,
    frequency INTEGER,
    PRIMARY KEY (prompt_hash, task_type, position)
);`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("pattern db schema: %w", err)
	}
	return &PatternDB{db: db}, nil
}

// HashPrompt normalizes a prompt and returns its sha256 hash (hex, 16 chars).
func HashPrompt(prompt string) string {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	normalized = strings.Join(strings.Fields(normalized), " ")
	h := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", h[:8])
}

// RecordSequence stores a completed plan's task sequence in the DB.
func (p *PatternDB) RecordSequence(ctx context.Context, plan *Plan) error {
	if p == nil || p.db == nil || plan == nil {
		return nil
	}

	taskSeq := make([]TaskType, 0, len(plan.Tasks))
	for _, t := range plan.Tasks {
		taskSeq = append(taskSeq, t.Type)
	}

	seqJSON, err := json.Marshal(taskSeq)
	if err != nil {
		return fmt.Errorf("marshal task seq: %w", err)
	}

	promptHash := HashPrompt(plan.Prompt)
	duration := plan.Completed.Sub(plan.Started).Milliseconds()
	if duration < 0 {
		duration = 0
	}

	_, err = p.db.ExecContext(ctx,
		`INSERT INTO task_sequences (prompt_hash, prompt_text, task_sequence, success, duration_ms, tokens_used, cost, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		promptHash, plan.Prompt, string(seqJSON),
		boolToInt(plan.Success), duration, plan.TokensUsed, plan.TotalCost,
		plan.Completed.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert task sequence: %w", err)
	}

	return p.recomputePatterns(ctx, promptHash)
}

// recomputePatterns aggregates all sequences for a prompt_hash into
// the task_patterns table.
func (p *PatternDB) recomputePatterns(ctx context.Context, promptHash string) error {
	rows, err := p.db.QueryContext(ctx,
		`SELECT task_sequence, success, duration_ms, tokens_used FROM task_sequences WHERE prompt_hash = ?`,
		promptHash)
	if err != nil {
		return fmt.Errorf("query sequences: %w", err)
	}
	defer rows.Close()

	type seqInfo struct {
		tasks    []TaskType
		success  bool
		duration int64
		tokens   int
	}
	var sequences []seqInfo

	for rows.Next() {
		var seqStr string
		var successInt int
		var dur int64
		var tokens int
		if err := rows.Scan(&seqStr, &successInt, &dur, &tokens); err != nil {
			return fmt.Errorf("scan sequence: %w", err)
		}
		var tasks []TaskType
		if err := json.Unmarshal([]byte(seqStr), &tasks); err != nil {
			continue
		}
		sequences = append(sequences, seqInfo{
			tasks:    tasks,
			success:  successInt == 1,
			duration: dur,
			tokens:   tokens,
		})
	}
	rows.Close()

	if len(sequences) == 0 {
		return nil
	}

	// Clear old patterns for this hash.
	_, err = p.db.ExecContext(ctx, `DELETE FROM task_patterns WHERE prompt_hash = ?`, promptHash)
	if err != nil {
		return fmt.Errorf("clear patterns: %w", err)
	}

	// Aggregate: for each (task_type, position), count frequency and
	// compute average stats.
	type key struct {
		taskType TaskType
		pos      int
	}
	type agg struct {
		count    int
		duration int64
		tokens   int
	}

	aggregates := map[key]*agg{}
	successCount := 0
	for _, seq := range sequences {
		if seq.success {
			successCount++
		}
		for i, tt := range seq.tasks {
			k := key{taskType: tt, pos: i}
			if a, ok := aggregates[k]; ok {
				a.count++
				a.duration += seq.duration
				a.tokens += seq.tokens
			} else {
				aggregates[k] = &agg{
					count:    1,
					duration: seq.duration,
					tokens:   seq.tokens,
				}
			}
		}
	}

	totalSeqs := len(sequences)
	for k, a := range aggregates {
		prob := float64(a.count) / float64(totalSeqs)
		avgDur := int64(0)
		avgTokens := 0
		if a.count > 0 {
			avgDur = a.duration / int64(a.count)
			avgTokens = a.tokens / a.count
		}
		_, err = p.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO task_patterns (prompt_hash, task_type, position, probability, avg_duration_ms, avg_tokens, frequency)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			promptHash, string(k.taskType), k.pos, prob, avgDur, avgTokens, a.count)
		if err != nil {
			return fmt.Errorf("insert pattern: %w", err)
		}
	}

	return nil
}

// MatchPrompt finds patterns for a given prompt. It checks the exact hash
// first, then falls back to prefix matching for fuzzy similarity.
func (p *PatternDB) MatchPrompt(ctx context.Context, prompt string) (*PatternPrediction, error) {
	if p == nil || p.db == nil {
		return &PatternPrediction{Patterns: nil}, nil
	}

	promptHash := HashPrompt(prompt)

	// Try exact hash match first.
	pred, err := p.getPatterns(ctx, promptHash)
	if err != nil {
		return nil, err
	}
	if pred != nil && pred.MatchCount > 0 {
		return pred, nil
	}

	// Fuzzy: try prefix matches (first 4 hex chars = first 2 bytes).
	prefix := promptHash[:4]
	rows, err := p.db.QueryContext(ctx,
		`SELECT DISTINCT prompt_hash FROM task_patterns WHERE prompt_hash LIKE ?`,
		prefix+"%")
	if err != nil {
		return nil, fmt.Errorf("query pattern hashes: %w", err)
	}

	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			continue
		}
		if h != promptHash {
			hashes = append(hashes, h)
		}
	}
	rows.Close()

	if len(hashes) == 0 {
		return &PatternPrediction{PromptHash: promptHash}, nil
	}

	// Merge patterns from all fuzzy-matched hashes.
	allPatterns := map[TaskType]*TaskPattern{}
	totalMatches := 0
	for _, h := range hashes {
		pred, err := p.getPatterns(ctx, h)
		if err != nil || pred == nil {
			continue
		}
		totalMatches += pred.MatchCount
		for _, pat := range pred.Patterns {
			if existing, ok := allPatterns[pat.TaskType]; ok {
				existing.Probability = (existing.Probability + pat.Probability) / 2
				existing.Frequency += pat.Frequency
			} else {
				cp := pat
				allPatterns[pat.TaskType] = &cp
			}
		}
	}

	result := &PatternPrediction{
		PromptHash:  promptHash,
		MatchCount:  totalMatches,
		SuccessRate: 0,
	}
	for _, pat := range allPatterns {
		result.Patterns = append(result.Patterns, *pat)
	}
	sort.Slice(result.Patterns, func(i, j int) bool {
		return result.Patterns[i].Position < result.Patterns[j].Position
	})

	return result, nil
}

func (p *PatternDB) getPatterns(ctx context.Context, promptHash string) (*PatternPrediction, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT task_type, position, probability, avg_duration_ms, avg_tokens, frequency
		 FROM task_patterns WHERE prompt_hash = ? ORDER BY position ASC`,
		promptHash)
	if err != nil {
		return nil, fmt.Errorf("query patterns: %w", err)
	}
	defer rows.Close()

	var patterns []TaskPattern
	for rows.Next() {
		var pat TaskPattern
		var taskTypeStr string
		if err := rows.Scan(&taskTypeStr, &pat.Position, &pat.Probability,
			&pat.AvgDurationMs, &pat.AvgTokens, &pat.Frequency); err != nil {
			continue
		}
		pat.TaskType = TaskType(taskTypeStr)
		patterns = append(patterns, pat)
	}
	rows.Close()

	// Count sequences and success rate.
	var matchCount int
	var successCount int
	err = p.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN success=1 THEN 1 ELSE 0 END), 0) FROM task_sequences WHERE prompt_hash = ?`,
		promptHash).Scan(&matchCount, &successCount)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("query sequence count: %w", err)
	}

	successRate := 0.0
	if matchCount > 0 {
		successRate = float64(successCount) / float64(matchCount)
	}

	return &PatternPrediction{
		PromptHash:  promptHash,
		MatchCount:  matchCount,
		Patterns:    patterns,
		SuccessRate: successRate,
	}, nil
}

// PatternStats returns global statistics about the pattern DB.
type PatternStats struct {
	TotalSequences int     `json:"total_sequences"`
	TotalPatterns  int     `json:"total_patterns"`
	UniquePrompts  int     `json:"unique_prompts"`
	OverallSuccess float64 `json:"overall_success"`
}

func (p *PatternDB) Stats(ctx context.Context) (*PatternStats, error) {
	if p == nil || p.db == nil {
		return &PatternStats{}, nil
	}
	var stats PatternStats
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(AVG(CASE WHEN success=1 THEN 1.0 ELSE 0.0 END), 0) FROM task_sequences`).
		Scan(&stats.TotalSequences, &stats.OverallSuccess)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	err = p.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT prompt_hash) FROM task_sequences`).
		Scan(&stats.UniquePrompts)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	err = p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_patterns`).
		Scan(&stats.TotalPatterns)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return &stats, nil
}

// ListPatterns returns all stored patterns grouped by prompt hash.
type PatternEntry struct {
	PromptHash  string        `json:"prompt_hash"`
	PromptText  string        `json:"prompt_text"`
	MatchCount  int           `json:"match_count"`
	SuccessRate float64       `json:"success_rate"`
	Patterns    []TaskPattern `json:"patterns"`
}

func (p *PatternDB) ListPatterns(ctx context.Context, limit int) ([]*PatternEntry, error) {
	if p == nil || p.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := p.db.QueryContext(ctx,
		`SELECT prompt_hash, prompt_text, COUNT(*) as cnt,
		        COALESCE(SUM(CASE WHEN success=1 THEN 1 ELSE 0 END), 0) as successes
		 FROM task_sequences GROUP BY prompt_hash ORDER BY cnt DESC LIMIT ?`,
		limit)
	if err != nil {
		return nil, err
	}

	// Collect all rows first, then close the rows cursor before making
	// further queries. This avoids connection-pool exhaustion when
	// SetMaxOpenConns(1) is used (common in tests with :memory: SQLite).
	type rawEntry struct {
		Hash      string
		Text      string
		Count     int
		Successes int
	}
	var raws []rawEntry
	for rows.Next() {
		var r rawEntry
		if err := rows.Scan(&r.Hash, &r.Text, &r.Count, &r.Successes); err != nil {
			continue
		}
		raws = append(raws, r)
	}
	rows.Close()

	var entries []*PatternEntry
	for _, r := range raws {
		e := &PatternEntry{
			PromptHash:  r.Hash,
			PromptText:  r.Text,
			MatchCount:  r.Count,
			SuccessRate: 0,
		}
		if r.Count > 0 {
			e.SuccessRate = float64(r.Successes) / float64(r.Count)
		}
		pred, err := p.getPatterns(ctx, r.Hash)
		if err == nil && pred != nil {
			e.Patterns = pred.Patterns
		}
		entries = append(entries, e)
	}
	return entries, nil
}
