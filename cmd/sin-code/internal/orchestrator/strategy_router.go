// SPDX-License-Identifier: MIT
// Purpose: Adaptive Tool-Strategy Router (Thompson sampling).
// Per (task-class, strategy) Beta posterior learned from verified outcomes.
// No epsilon, no decay — exploration/exploitation in one mechanism.
package orchestrator

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
)

type Strategy string

const (
	StratASTEdit  Strategy = "ast-edit"
	StratHashline Strategy = "hashline-patch"
	StratRewrite  Strategy = "full-rewrite"
	StratShellGen Strategy = "shell-codegen"
)

type TaskClass string

const (
	ClassRename     TaskClass = "rename"
	ClassRefactor   TaskClass = "refactor"
	ClassBugfix     TaskClass = "bugfix"
	ClassGreenfield TaskClass = "greenfield"
	ClassConfig     TaskClass = "config"
	ClassUnknown    TaskClass = "unknown"
)

func ClassifyTask(task *Task) TaskClass {
	t := strings.ToLower(task.Title + " " + task.Description)
	switch {
	case strings.Contains(t, "rename") || strings.Contains(t, "umbenennen"):
		return ClassRename
	case strings.Contains(t, "refactor") || strings.Contains(t, "restructure"):
		return ClassRefactor
	case strings.Contains(t, "fix") || strings.Contains(t, "bug") || strings.Contains(t, "broken"):
		return ClassBugfix
	case strings.Contains(t, "create") || strings.Contains(t, "new ") || strings.Contains(t, "add "):
		return ClassGreenfield
	case strings.Contains(t, "config") || strings.Contains(t, "yaml") || strings.Contains(t, ".env"):
		return ClassConfig
	default:
		return ClassUnknown
	}
}

type arm struct {
	alpha float64
	beta  float64
}

type StrategyRouter struct {
	mu   sync.Mutex
	arms map[string]*arm
	db   *sql.DB
	rng  *rand.Rand
}

func NewStrategyRouter(db *sql.DB, seed int64) (*StrategyRouter, error) {
	r := &StrategyRouter{arms: map[string]*arm{}, db: db, rng: rand.New(rand.NewSource(seed))}
	if db != nil {
		const schema = `
CREATE TABLE IF NOT EXISTS router_arms (
	class TEXT NOT NULL, strategy TEXT NOT NULL,
	alpha REAL NOT NULL DEFAULT 1, beta REAL NOT NULL DEFAULT 1,
	PRIMARY KEY (class, strategy)
);`
		if _, err := db.Exec(schema); err != nil {
			return nil, fmt.Errorf("router schema: %w", err)
		}
		if err := r.load(); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *StrategyRouter) load() error {
	rows, err := r.db.Query(`SELECT class, strategy, alpha, beta FROM router_arms`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var class, strat string
		a := &arm{}
		if err := rows.Scan(&class, &strat, &a.alpha, &a.beta); err != nil {
			return err
		}
		r.arms[class+"|"+strat] = a
	}
	return rows.Err()
}

func (r *StrategyRouter) Pick(class TaskClass, candidates []Strategy) Strategy {
	if len(candidates) == 0 {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	best, bestDraw := candidates[0], -1.0
	for _, s := range candidates {
		a := r.arm(class, s)
		draw := sampleBeta(r.rng, a.alpha, a.beta)
		if draw > bestDraw {
			best, bestDraw = s, draw
		}
	}
	return best
}

func (r *StrategyRouter) Report(ctx context.Context, class TaskClass, s Strategy, success bool) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	a := r.arm(class, s)
	if success {
		a.alpha++
	} else {
		a.beta++
	}
	alpha, beta := a.alpha, a.beta
	r.mu.Unlock()

	if r.db == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO router_arms (class, strategy, alpha, beta) VALUES (?, ?, ?, ?)
		ON CONFLICT(class, strategy) DO UPDATE SET alpha=excluded.alpha, beta=excluded.beta`,
		string(class), string(s), alpha, beta)
	return err
}

func (r *StrategyRouter) Posterior(class TaskClass, s Strategy) (mean float64, n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.arm(class, s)
	return a.alpha / (a.alpha + a.beta), int(a.alpha + a.beta - 2)
}

func (r *StrategyRouter) arm(class TaskClass, s Strategy) *arm {
	key := string(class) + "|" + string(s)
	if a, ok := r.arms[key]; ok {
		return a
	}
	a := &arm{alpha: 1, beta: 1}
	r.arms[key] = a
	return a
}

func sampleBeta(rng *rand.Rand, a, b float64) float64 {
	x := sampleGamma(rng, a)
	y := sampleGamma(rng, b)
	if x+y == 0 {
		return 0.5
	}
	return x / (x + y)
}

func sampleGamma(rng *rand.Rand, shape float64) float64 {
	if shape < 1 {
		return sampleGamma(rng, shape+1) * math.Pow(rng.Float64(), 1/shape)
	}
	d := shape - 1.0/3.0
	c := 1.0 / math.Sqrt(9*d)
	for {
		x := rng.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1-0.0331*x*x*x*x || math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}
