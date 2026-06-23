// SPDX-License-Identifier: MIT
// Purpose: instinct domain model — small learned behaviors with confidence
// 0.3–0.9, project-scoped or global. Schema/format inspired by the
// "continuous-learning-v2" pattern (affaan-m/ecc), reimplemented natively.
// Docs: types.doc.md
package instinct

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Scope determines where an instinct applies.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

// Status tracks an instinct's lifecycle.
type Status string

const (
	StatusPending  Status = "pending"
	StatusActive   Status = "active"
	StatusEvolved  Status = "evolved"
	StatusArchived Status = "archived"
)

// Confidence bounds. ECC's 0.3–0.9 weighting, but bound at 0.3 to keep
// the active set narrow.
const (
	MinConfidence       = 0.30
	MaxConfidence       = 0.90
	ActivationThreshold = 0.60
	EvolveThreshold     = 0.70
)

// Instinct is a single small learned behavior. Serialized as Markdown
// with YAML frontmatter (ECC-compatible).
type Instinct struct {
	ID             string    `yaml:"id"`
	Trigger        string    `yaml:"trigger"`
	Confidence     float64   `yaml:"confidence"`
	Domain         string    `yaml:"domain"`
	Source         string    `yaml:"source"`
	Scope          Scope     `yaml:"scope"`
	ProjectID      string    `yaml:"project_id,omitempty"`
	ProjectName    string    `yaml:"project_name,omitempty"`
	SourceRepo     string    `yaml:"source_repo,omitempty"`
	Status         Status    `yaml:"status,omitempty"`
	Observations   int       `yaml:"observations,omitempty"`
	Contradictions int       `yaml:"contradictions,omitempty"`
	SeenInProjects []string  `yaml:"seen_in_projects,omitempty"`
	CreatedAt      time.Time `yaml:"created_at,omitempty"`
	UpdatedAt      time.Time `yaml:"updated_at,omitempty"`

	Action   string   `yaml:"-"`
	Evidence []string `yaml:"-"`
}

// NewInstinct builds an instinct with a deterministic ID.
func NewInstinct(trigger, domain, action, source string, scope Scope) *Instinct {
	now := time.Now().UTC()
	i := &Instinct{
		Trigger:      trigger,
		Domain:       domain,
		Action:       action,
		Source:       source,
		Scope:        scope,
		Confidence:   MinConfidence,
		Status:       StatusPending,
		Observations: 1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	i.ID = i.computeID()
	return i
}

// computeID yields a stable slug used as the filename and dedup key.
// Slug is derived from the trigger; the suffix is a short hash of
// (trigger|domain) so re-titled triggers stay distinct.
func (i *Instinct) computeID() string {
	slug := slugify(i.Trigger)
	if slug == "" {
		slug = slugify(i.Action)
	}
	h := sha256.Sum256([]byte(strings.ToLower(i.Trigger + "|" + i.Domain)))
	suffix := hex.EncodeToString(h[:])[:6]
	if slug == "" {
		return "instinct-" + suffix
	}
	if len(slug) > 48 {
		slug = slug[:48]
	}
	return fmt.Sprintf("%s-%s", slug, suffix)
}

// SignatureKey identifies "the same instinct" across projects.
func (i *Instinct) SignatureKey() string {
	return strings.ToLower(strings.TrimSpace(i.Trigger)) + "::" + strings.ToLower(strings.TrimSpace(i.Domain))
}

// Reinforce nudges confidence up on a confirming observation.
func (i *Instinct) Reinforce() {
	i.Observations++
	step := currentTuning().reinforceStep
	gap := MaxConfidence - i.Confidence
	i.Confidence = round2(i.Confidence + gap*step)
	i.touch()
}

// Contradict nudges confidence down on a conflicting observation.
func (i *Instinct) Contradict() {
	i.Contradictions++
	step := currentTuning().contradictStep
	gap := i.Confidence - MinConfidence
	i.Confidence = round2(i.Confidence - gap*step)
	i.touch()
}

// Decay applies time-based forgetting; call periodically (e.g. on prune).
func (i *Instinct) Decay(days float64) {
	if days <= 0 {
		return
	}
	// Lose ~5% of the gap-to-floor per 30 idle days.
	factor := math.Pow(0.95, days/30.0)
	gap := i.Confidence - MinConfidence
	i.Confidence = round2(MinConfidence + gap*factor)
	i.touch()
}

func (i *Instinct) touch() {
	if i.Confidence > MaxConfidence {
		i.Confidence = MaxConfidence
	}
	if i.Confidence < MinConfidence {
		i.Confidence = MinConfidence
	}
	i.UpdatedAt = time.Now().UTC()
	i.recomputeStatus()
}

func (i *Instinct) recomputeStatus() {
	if i.Status == StatusEvolved || i.Status == StatusArchived {
		return
	}
	if i.Confidence >= currentTuning().activation {
		i.Status = StatusActive
	} else {
		i.Status = StatusPending
	}
}

// IsActive reports whether this instinct should influence behavior.
func (i *Instinct) IsActive() bool { return i.Status == StatusActive }

// EligibleForEvolution reports readiness to graduate.
func (i *Instinct) EligibleForEvolution() bool {
	return i.Confidence >= currentTuning().evolve && i.Observations >= 3 && i.Status == StatusActive
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '/':
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// SortByConfidence orders instincts strongest-first (stable on ID).
func SortByConfidence(list []*Instinct) {
	sort.SliceStable(list, func(a, b int) bool {
		if list[a].Confidence != list[b].Confidence {
			return list[a].Confidence > list[b].Confidence
		}
		return list[a].ID < list[b].ID
	})
}
