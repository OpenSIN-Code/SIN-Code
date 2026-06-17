// SPDX-License-Identifier: MIT
// Purpose: configuration for the quality gate pipeline.
// Docs: runner.doc.md
package testgate

import (
	"context"
	"time"
)

// StepKind identifies a pipeline step.
type StepKind string

const (
	StepBuild       StepKind = "build"
	StepVet         StepKind = "vet"
	StepTest        StepKind = "test"
	StepStaticcheck StepKind = "staticcheck"
	StepGosec       StepKind = "gosec"
	StepGovulncheck StepKind = "govulncheck"
)

// AllSteps is the default pipeline order.
var AllSteps = []StepKind{
	StepBuild,
	StepVet,
	StepTest,
	StepStaticcheck,
	StepGosec,
	StepGovulncheck,
}

// Config controls the quality gate pipeline.
type Config struct {
	// Workdir is the directory in which commands run.
	Workdir string

	// Timeout caps the whole pipeline.
	Timeout time.Duration

	// CoverageThreshold is the minimum coverage percent required (0 = disabled).
	CoverageThreshold float64

	// Steps lists which steps to run. Empty means AllSteps.
	Steps []StepKind

	// Race enables -race in the test step.
	Race bool

	// JsonOut returns the report as JSON (used by the chat tool).
	JsonOut bool

	// CommandRunner is swappable for tests.
	CommandRunner func(ctx context.Context, name string, args []string, dir string, timeout time.Duration) (string, error)

	// LookPath is swappable for tests.
	LookPath func(string) (string, error)
}

func (c *Config) effectiveSteps() []StepKind {
	if len(c.Steps) == 0 {
		return AllSteps
	}
	return c.Steps
}

func (c *Config) effectiveTimeout() time.Duration {
	if c.Timeout <= 0 {
		return 5 * time.Minute
	}
	return c.Timeout
}
