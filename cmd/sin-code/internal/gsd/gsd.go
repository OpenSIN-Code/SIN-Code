// SPDX-License-Identifier: MIT
// Package gsd provides project lifecycle management: project init,
// phase CRUD, plan creation, and wave-based execution state. It manages
// markdown state files (PROJECT.md, ROADMAP.md, STATE.md) in a .gsd/
// directory at the project root.
package gsd

import "time"

type Project struct {
	Name        string
	Description string
	TechStack   []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Phase struct {
	ID        string
	Title     string
	Priority  string
	Status    string
	CreatedAt time.Time
}

type Plan struct {
	PhaseID   string
	Tasks     []Task
	CreatedAt time.Time
	Status    string
}

type Task struct {
	ID          string
	Description string
	Dependencies []string
	Validation  string
	Effort      string
	Status      string
}

type State struct {
	CurrentPhase string
	History      []string
	UpdatedAt    time.Time
}

const (
	StatusPlanning   = "planning"
	StatusInProgress = "in-progress"
	StatusCompleted  = "completed"
	StatusPhaseBlocked = "blocked"
)

const (
	TaskStatusTodo       = "todo"
	TaskStatusInProgress = "in-progress"
	TaskStatusDone       = "done"
	TaskStatusBlocked    = "blocked"
)

const (
	PriorityP0 = "P0"
	PriorityP1 = "P1"
	PriorityP2 = "P2"
	PriorityP3 = "P3"
)

const (
	EffortSmall  = "S"
	EffortMedium = "M"
	EffortLarge  = "L"
)
