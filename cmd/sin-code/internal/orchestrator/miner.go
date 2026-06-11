// SPDX-License-Identifier: MIT
// Purpose: Chain Miner — automatic distillation of successful tool
// sequences into new macro chains. The system manufactures its own
// tools from its own verified experience. New templates start in
// shadow mode and promote to active when live performance confirms
// historical evidence.
package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type ToolCall struct {
	Tool     string `json:"tool"`
	ArgShape string `json:"arg_shape"`
}

type SeqEpisode struct {
	EpisodeID int64
	Class     TaskClass
	Sequence  []ToolCall
	Passed    bool
}

type ChainTemplate struct {
	ID            int64
	Class         TaskClass
	Sequence      []ToolCall
	Support       int
	SuccessRate   float64
	Status        string
	LiveTrials    int
	LiveSuccesses int
}

func (t *ChainTemplate) Key() string {
	parts := make([]string, len(t.Sequence))
	for i, c := range t.Sequence {
		parts[i] = c.Tool + "(" + c.ArgShape + ")"
	}
	return strings.Join(parts, "->")
}

type Miner struct {
	db             *sql.DB
	MinSupport     int
	MinSuccessRate float64
	MinLength      int
	MaxLength      int
}

func NewMiner(db *sql.DB) (*Miner, error) {
	if db == nil {
		return &Miner{MinSupport: 5, MinSuccessRate: 0.8, MinLength: 3, MaxLength: 8}, nil
	}
	const schema = `
CREATE TABLE IF NOT EXISTS chain_templates (
	id INTEGER PRIMARY KEY,
	class TEXT NOT NULL,
	seq_key TEXT NOT NULL UNIQUE,
	sequence_json TEXT NOT NULL,
	support INTEGER NOT NULL,
	success_rate REAL NOT NULL,
	status TEXT NOT NULL DEFAULT 'shadow',
	live_trials INTEGER NOT NULL DEFAULT 0,
	live_successes INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS episode_sequences (
	episode_id INTEGER PRIMARY KEY,
	class TEXT NOT NULL,
	sequence_json TEXT NOT NULL,
	passed INTEGER NOT NULL
);`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("miner schema: %w", err)
	}
	return &Miner{db: db, MinSupport: 5, MinSuccessRate: 0.8, MinLength: 3, MaxLength: 8}, nil
}

func (m *Miner) RecordSequence(ctx context.Context, ep SeqEpisode) error {
	if m == nil || m.db == nil {
		return nil
	}
	seq, err := json.Marshal(ep.Sequence)
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO episode_sequences (episode_id, class, sequence_json, passed)
		 VALUES (?, ?, ?, ?)`,
		ep.EpisodeID, string(ep.Class), string(seq), boolToInt(ep.Passed))
	return err
}

func (m *Miner) Mine(ctx context.Context, class TaskClass) ([]*ChainTemplate, error) {
	if m == nil || m.db == nil {
		return nil, nil
	}
	episodes, err := m.loadSequences(ctx, class)
	if err != nil {
		return nil, err
	}
	if len(episodes) < m.MinSupport {
		return nil, nil
	}

	type stat struct{ support, passes int }
	counts := map[string]*stat{}
	patterns := map[string][]ToolCall{}

	for _, ep := range episodes {
		seen := map[string]bool{}
		n := len(ep.Sequence)
		for length := m.MinLength; length <= m.MaxLength && length <= n; length++ {
			for start := 0; start+length <= n; start++ {
				sub := ep.Sequence[start : start+length]
				key := seqKey(sub)
				if seen[key] {
					continue
				}
				seen[key] = true
				if counts[key] == nil {
					counts[key] = &stat{}
					patterns[key] = append([]ToolCall{}, sub...)
				}
				counts[key].support++
				if ep.Passed {
					counts[key].passes++
				}
			}
		}
	}

	var qualified []*ChainTemplate
	for key, st := range counts {
		rate := float64(st.passes) / float64(st.support)
		if st.support >= m.MinSupport && rate >= m.MinSuccessRate {
			qualified = append(qualified, &ChainTemplate{
				Class: class, Sequence: patterns[key],
				Support: st.support, SuccessRate: rate, Status: "shadow",
			})
		}
	}
	sort.Slice(qualified, func(i, j int) bool {
		return len(qualified[i].Sequence) > len(qualified[j].Sequence)
	})
	qualified = dropContained(qualified)

	var fresh []*ChainTemplate
	for _, t := range qualified {
		seq, _ := json.Marshal(t.Sequence)
		res, err := m.db.ExecContext(ctx, `
			INSERT INTO chain_templates (class, seq_key, sequence_json, support, success_rate)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(seq_key) DO UPDATE SET support=excluded.support, success_rate=excluded.success_rate`,
			string(t.Class), t.Key(), string(seq), t.Support, t.SuccessRate)
		if err != nil {
			return nil, err
		}
		if id, _ := res.LastInsertId(); id > 0 {
			t.ID = id
			fresh = append(fresh, t)
		}
	}
	return fresh, nil
}

func (m *Miner) ReportLive(ctx context.Context, templateID int64, success bool) error {
	if m == nil || m.db == nil {
		return nil
	}
	inc := 0
	if success {
		inc = 1
	}
	_, err := m.db.ExecContext(ctx, `
		UPDATE chain_templates
		SET live_trials = live_trials + 1, live_successes = live_successes + ?,
		    status = CASE
		      WHEN live_trials + 1 >= 10 AND (live_successes + ?) * 1.0 / (live_trials + 1) >= success_rate - 0.1
		        THEN 'active'
		      WHEN live_trials + 1 >= 10 AND (live_successes + ?) * 1.0 / (live_trials + 1) < 0.5
		        THEN 'retired'
		      ELSE status END
		WHERE id = ?`, inc, inc, inc, templateID)
	return err
}

func (m *Miner) SuggestionsFor(ctx context.Context, class TaskClass) (string, error) {
	if m == nil || m.db == nil {
		return "", nil
	}
	rows, err := m.db.QueryContext(ctx, `
		SELECT sequence_json, support, success_rate, status FROM chain_templates
		WHERE class = ? AND status != 'retired'
		ORDER BY status = 'active' DESC, success_rate DESC LIMIT 3`, string(class))
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var b strings.Builder
	for rows.Next() {
		var seqJSON, status string
		var support int
		var rate float64
		if err := rows.Scan(&seqJSON, &support, &rate, &status); err != nil {
			return "", err
		}
		var seq []ToolCall
		_ = json.Unmarshal([]byte(seqJSON), &seq)
		names := make([]string, len(seq))
		for i, c := range seq {
			names[i] = c.Tool
		}
		fmt.Fprintf(&b, "- [%s, %.0f%% verified over %d episodes] %s\n",
			status, rate*100, support, strings.Join(names, " -> "))
	}
	if b.Len() == 0 {
		return "", rows.Err()
	}
	return "## Proven tool sequences for this task class\n" + b.String(), rows.Err()
}

func (m *Miner) loadSequences(ctx context.Context, class TaskClass) ([]SeqEpisode, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT episode_id, sequence_json, passed FROM episode_sequences WHERE class = ?`,
		string(class))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SeqEpisode
	for rows.Next() {
		var ep SeqEpisode
		var seqJSON string
		var pi int
		if err := rows.Scan(&ep.EpisodeID, &seqJSON, &pi); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(seqJSON), &ep.Sequence)
		ep.Passed = pi == 1
		ep.Class = class
		out = append(out, ep)
	}
	return out, rows.Err()
}

func seqKey(seq []ToolCall) string {
	parts := make([]string, len(seq))
	for i, c := range seq {
		parts[i] = c.Tool + "(" + c.ArgShape + ")"
	}
	return strings.Join(parts, "->")
}

func dropContained(ts []*ChainTemplate) []*ChainTemplate {
	var out []*ChainTemplate
	for i, t := range ts {
		contained := false
		for j := 0; j < i; j++ {
			if strings.Contains(ts[j].Key(), t.Key()) {
				contained = true
				break
			}
		}
		if !contained {
			out = append(out, t)
		}
	}
	return out
}
