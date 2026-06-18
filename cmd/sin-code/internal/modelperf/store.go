// SPDX-License-Identifier: MIT
// Purpose: Model Performance Registry (issue #395) — persistent
// per-model-per-category performance database that drives benchmark-based
// model selection for SIN Fusion.
//
// The store records benchmark results from eval datasets run across all
// fusion providers, and provides recommendation queries that return the
// best-performing models for a given task category.
//
// SQLite-based, CGo-free (modernc.org/sqlite, M2). Race-free (M7):
// SetMaxOpenConns(1) serializes all writes.
package modelperf

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// PerfRecord is one benchmark result for a model on a category.
type PerfRecord struct {
	Model        string  `json:"model"`
	Category     string  `json:"category"`
	Dataset      string  `json:"dataset"`
	PassRate     float64 `json:"pass_rate"`
	AvgLatencyMs int64   `json:"avg_latency_ms"`
	AvgCostUSD   float64 `json:"avg_cost_usd"`
	AvgTokens    int     `json:"avg_tokens"`
	SampleCount  int     `json:"sample_count"`
	RecordedAt   string  `json:"recorded_at"`
}

// Recommendation is a model recommendation for a task category.
type Recommendation struct {
	Model    string  `json:"model"`
	Score    float64 `json:"score"`
	PassRate float64 `json:"pass_rate"`
	Samples  int     `json:"samples"`
	Reason   string  `json:"reason"`
}

// Store is the SQLite-backed model performance registry.
type Store struct {
	db *sql.DB
}

// DefaultPath returns the default modelperf.db location.
func DefaultPath() string {
	if h := os.Getenv("SIN_CODE_HOME"); h != "" {
		return filepath.Join(h, "modelperf.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "modelperf.db"
	}
	return filepath.Join(home, ".local", "share", "sin-code", "modelperf.db")
}

// Open opens or creates the modelperf store. Parent dirs are created.
func Open(path string) (*Store, error) {
	if path == "" {
		path = DefaultPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the store.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS model_perf (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model TEXT NOT NULL,
    category TEXT NOT NULL,
    dataset TEXT NOT NULL,
    pass_rate REAL NOT NULL,
    avg_latency_ms INTEGER DEFAULT 0,
    avg_cost_usd REAL DEFAULT 0,
    avg_tokens INTEGER DEFAULT 0,
    sample_count INTEGER DEFAULT 1,
    recorded_at TEXT NOT NULL,
    UNIQUE(model, category, dataset)
);`
	_, err := s.db.Exec(schema)
	return err
}

// Upsert records or updates a benchmark result. On conflict of
// (model, category, dataset), the existing row is updated with the
// new metrics and sample_count is incremented.
func (s *Store) Upsert(ctx context.Context, r PerfRecord) error {
	if r.Model == "" || r.Category == "" || r.Dataset == "" {
		return fmt.Errorf("modelperf: model, category, dataset are required")
	}
	if r.RecordedAt == "" {
		r.RecordedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO model_perf (model, category, dataset, pass_rate, avg_latency_ms, avg_cost_usd, avg_tokens, sample_count, recorded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(model, category, dataset) DO UPDATE SET
    pass_rate = excluded.pass_rate,
    avg_latency_ms = excluded.avg_latency_ms,
    avg_cost_usd = excluded.avg_cost_usd,
    avg_tokens = excluded.avg_tokens,
    sample_count = model_perf.sample_count + 1,
    recorded_at = excluded.recorded_at;`,
		r.Model, r.Category, r.Dataset, r.PassRate, r.AvgLatencyMs, r.AvgCostUSD, r.AvgTokens, r.SampleCount, r.RecordedAt)
	return err
}

// Recommend returns the top-N models for a category, sorted by a
// blended score of pass_rate (weight 0.8) and cost-efficiency (weight 0.2).
// Models with fewer than minSamples runs are excluded (cold-start protection).
func (s *Store) Recommend(ctx context.Context, category string, n int, minSamples int) ([]Recommendation, error) {
	if n <= 0 {
		n = 3
	}
	if minSamples < 0 {
		minSamples = 0
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT model, AVG(pass_rate) as avg_pass, AVG(avg_cost_usd) as avg_cost, SUM(sample_count) as total_samples
FROM model_perf
WHERE category = ? AND sample_count >= ?
GROUP BY model
ORDER BY avg_pass DESC`, category, minSamples)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []Recommendation
	for rows.Next() {
		var model string
		var avgPass, avgCost float64
		var totalSamples int
		if err := rows.Scan(&model, &avgPass, &avgCost, &totalSamples); err != nil {
			return nil, err
		}
		// Score: 80% pass_rate + 20% cost-efficiency (lower cost = higher score)
		costScore := 1.0
		if avgCost > 0 {
			costScore = 1.0 / (1.0 + avgCost)
		}
		score := 0.8*avgPass + 0.2*costScore
		recs = append(recs, Recommendation{
			Model:    model,
			Score:    score,
			PassRate: avgPass,
			Samples:  totalSamples,
			Reason:   fmt.Sprintf("pass_rate=%.1f%%, samples=%d, avg_cost=$%.4f", avgPass*100, totalSamples, avgCost),
		})
	}
	if recs == nil {
		return nil, nil
	}
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].Score > recs[j].Score })
	if len(recs) > n {
		recs = recs[:n]
	}
	return recs, nil
}

// Ranking returns all records grouped by category, sorted by pass_rate desc.
func (s *Store) Ranking(ctx context.Context) ([]PerfRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT model, category, dataset, pass_rate, avg_latency_ms, avg_cost_usd, avg_tokens, sample_count, recorded_at
FROM model_perf
ORDER BY category, pass_rate DESC, model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []PerfRecord
	for rows.Next() {
		var r PerfRecord
		if err := rows.Scan(&r.Model, &r.Category, &r.Dataset, &r.PassRate, &r.AvgLatencyMs, &r.AvgCostUSD, &r.AvgTokens, &r.SampleCount, &r.RecordedAt); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, nil
}

// Categories returns all distinct categories in the store.
func (s *Store) Categories(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT category FROM model_perf ORDER BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, nil
}

// DetectCategory infers a task category from a prompt or dataset name.
// This is a simple heuristic — the benchmark runner can also set it explicitly.
func DetectCategory(input string) string {
	lower := strings.ToLower(input)
	categories := []struct {
		keywords []string
		category string
	}{
		{[]string{"test", "spec", "fuzz", "mutation"}, "test-generation"},
		{[]string{"debug", "fix", "bug", "trace", "error"}, "debugging"},
		{[]string{"plan", "design", "architect", "rfc", "spec"}, "planning"},
		{[]string{"refactor", "rename", "extract", "restructure"}, "refactoring"},
		{[]string{"review", "audit", "lint", "quality", "ceo"}, "review"},
		{[]string{"doc", "readme", "changelog", "comment"}, "documentation"},
		{[]string{"security", "vuln", "sbom", "secret"}, "security"},
	}
	for _, cat := range categories {
		for _, kw := range cat.keywords {
			if strings.Contains(lower, kw) {
				return cat.category
			}
		}
	}
	return "code-generation"
}
