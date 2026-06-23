// SPDX-License-Identifier: MIT
// Purpose: PRP data model — Product Requirement Prompt. A persistent,
// reviewable plan-of-record for a change. Port of ECC's PRP workflow
// in a clean-room Go reimplementation.
// Docs: types.doc.md
package prp

import "time"

// Phase is a stage in the PRP lifecycle.
type Phase string

const (
	PhaseDraft       Phase = "draft"       // created, not yet planned
	PhasePlanned     Phase = "planned"     // tasks decomposed
	PhaseImplementing Phase = "implementing"
	PhaseVerifying   Phase = "verifying"
	PhaseReady       Phase = "ready"       // verified, ready for PR
	PhaseShipped     Phase = "shipped"     // PR opened/merged
)

// TaskState tracks a single task within a PRP.
type TaskState string

const (
	TaskTodo    TaskState = "todo"
	TaskDoing   TaskState = "doing"
	TaskDone    TaskState = "done"
	TaskBlocked TaskState = "blocked"
)

// Task is one unit of implementation work.
type Task struct {
	ID    string    `yaml:"id" json:"id"`
	Title string    `yaml:"title" json:"title"`
	State TaskState `yaml:"state" json:"state"`
	Notes string    `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// PRP is a Product Requirement Prompt: the plan-of-record for a change.
type PRP struct {
	// Frontmatter
	ID        string    `yaml:"id"`
	Title     string    `yaml:"title"`
	Phase     Phase     `yaml:"phase"`
	Goal      string    `yaml:"goal"`
	Branch    string    `yaml:"branch,omitempty"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
	Tasks     []Task    `yaml:"tasks"`

	// Body sections (markdown)
	Context    string `yaml:"-"` // ## Context
	Plan       string `yaml:"-"` // ## Plan
	Acceptance string `yaml:"-"` // ## Acceptance Criteria
}

// Progress returns done/total task counts.
func (p *PRP) Progress() (done, total int) {
	total = len(p.Tasks)
	for _, t := range p.Tasks {
		if t.State == TaskDone {
			done++
		}
	}
	return done, total
}

// NextTask returns the first todo/doing task, or nil when all are
// done.
func (p *PRP) NextTask() *Task {
	for i := range p.Tasks {
		if p.Tasks[i].State == TaskTodo || p.Tasks[i].State == TaskDoing {
			return &p.Tasks[i]
		}
	}
	return nil
}

// AllDone reports whether every task is done.
func (p *PRP) AllDone() bool {
	d, t := p.Progress()
	return t > 0 && d == t
}
