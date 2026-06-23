// SPDX-License-Identifier: MIT
// Purpose: Calibrated Confidence — honest agent self-assessment.
// Agent declares P(passes); we score against reality (Brier) and learn
// a per-agent calibration curve. Auto-merge uses CALIBRATED, not raw.
package orchestrator

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

type ConfidenceClaim struct {
	AgentName string
	TaskClass TaskClass
	Declared  float64
	Passed    bool
}

type Calibrator struct {
	db       *sql.DB
	binCount int
}

func NewCalibrator(db *sql.DB) (*Calibrator, error) {
	c := &Calibrator{db: db, binCount: 10}
	if db == nil {
		return c, nil
	}
	const schema = `
CREATE TABLE IF NOT EXISTS confidence_claims (
	id INTEGER PRIMARY KEY,
	agent TEXT NOT NULL,
	class TEXT NOT NULL,
	declared REAL NOT NULL,
	passed INTEGER NOT NULL,
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_claims_agent ON confidence_claims(agent);`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("calibrator schema: %w", err)
	}
	return c, nil
}

func (c *Calibrator) Record(ctx context.Context, claim ConfidenceClaim) error {
	if c == nil || c.db == nil {
		return nil
	}
	_, err := c.db.ExecContext(ctx,
		`INSERT INTO confidence_claims (agent, class, declared, passed) VALUES (?, ?, ?, ?)`,
		claim.AgentName, string(claim.TaskClass), claim.Declared, boolToInt(claim.Passed))
	return err
}

func (c *Calibrator) Calibrate(ctx context.Context, agent string, declared float64) (float64, error) {
	if c == nil || c.db == nil {
		return declared, nil
	}
	rows, err := c.db.QueryContext(ctx,
		`SELECT declared, passed FROM confidence_claims WHERE agent = ?`, agent)
	if err != nil {
		return declared, err
	}
	defer rows.Close()

	type obs struct {
		d float64
		p bool
	}
	var all []obs
	for rows.Next() {
		var o obs
		var pi int
		if err := rows.Scan(&o.d, &pi); err != nil {
			return declared, err
		}
		o.p = pi == 1
		all = append(all, o)
	}
	if err := rows.Err(); err != nil {
		return declared, err
	}

	if len(all) < 10 {
		return 0.5 + (declared-0.5)*0.5, nil
	}

	passes := 0
	for _, o := range all {
		if o.p {
			passes++
		}
	}
	global := float64(passes) / float64(len(all))

	width := 1.0 / float64(c.binCount)
	var localN, localPass int
	for _, o := range all {
		if o.d >= declared-width && o.d <= declared+width {
			localN++
			if o.p {
				localPass++
			}
		}
	}
	if localN == 0 {
		return global, nil
	}
	local := float64(localPass) / float64(localN)

	const k = 10.0
	return (float64(localN)*local + k*global) / (float64(localN) + k), nil
}

func (c *Calibrator) BrierScore(ctx context.Context, agent string) (float64, int, error) {
	if c == nil || c.db == nil {
		return 0, 0, nil
	}
	rows, err := c.db.QueryContext(ctx,
		`SELECT declared, passed FROM confidence_claims WHERE agent = ?`, agent)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	var sum float64
	var n int
	for rows.Next() {
		var d float64
		var pi int
		if err := rows.Scan(&d, &pi); err != nil {
			return 0, 0, err
		}
		outcome := float64(pi)
		sum += (d - outcome) * (d - outcome)
		n++
	}
	if n == 0 {
		return 0, 0, rows.Err()
	}
	return sum / float64(n), n, rows.Err()
}

type MergePolicy struct {
	AutoMergeThreshold float64
	ReviewThreshold    float64
}

func DefaultMergePolicy() MergePolicy {
	return MergePolicy{AutoMergeThreshold: 0.85, ReviewThreshold: 0.6}
}

type Decision string

const (
	DecisionAutoMerge   Decision = "auto-merge"
	DecisionGreenReview Decision = "green-needs-review"
	DecisionBlock       Decision = "block"
)

func (p MergePolicy) Decide(verified bool, calibrated float64) Decision {
	switch {
	case !verified:
		return DecisionBlock
	case calibrated >= p.AutoMergeThreshold:
		return DecisionAutoMerge
	case calibrated >= p.ReviewThreshold:
		return DecisionGreenReview
	default:
		return DecisionGreenReview
	}
}

var _ = sort.Float64s
