// SPDX-License-Identifier: MIT
// Purpose: Causal Blame — bisect the edit log to find the first edit that
// flipped a check green→red. O(log n) verification runs via binary search.
// Precondition: the full log fails the check (caller verified).
package orchestrator

import (
	"context"
	"fmt"
	"os/exec"
)

type EditRecord struct {
	Seq     int
	SHA     string
	Path    string
	Summary string
}

type EditLog struct {
	TaskID  string
	Workdir string
	Base    string
	Edits   []EditRecord
}

type BlameResult struct {
	Culprit    *EditRecord
	Check      Check
	Bisections int
	PriorGreen int
}

func (b *BlameResult) Diagnosis() string {
	if b.Culprit == nil {
		return fmt.Sprintf("check %q was already failing before this run (pre-existing); do not blame current edits", b.Check.Name)
	}
	return fmt.Sprintf(
		"CULPRIT: edit #%d (%s) broke check %q.\nEdit summary: %s\n"+
			"Edits 1..%d are verified green. Fix or replace ONLY edit #%d.",
		b.Culprit.Seq, b.Culprit.Path, b.Check.Name,
		b.Culprit.Summary, b.PriorGreen, b.Culprit.Seq)
}

type Blamer struct {
	Verifier *Verifier
}

func (bl *Blamer) Blame(ctx context.Context, log *EditLog, failing Check) (*BlameResult, error) {
	if len(log.Edits) == 0 {
		return nil, fmt.Errorf("blame: empty edit log")
	}

	res := &BlameResult{Check: failing}

	if log.Base != "" {
		ok, err := bl.checkAt(ctx, log, log.Base, failing)
		if err != nil {
			return nil, err
		}
		res.Bisections++
		if !ok {
			return res, nil
		}
	}

	lo, hi := 1, len(log.Edits)
	for lo < hi {
		mid := (lo + hi) / 2
		ok, err := bl.checkAt(ctx, log, log.Edits[mid-1].SHA, failing)
		if err != nil {
			return nil, err
		}
		res.Bisections++
		if ok {
			lo = mid + 1
		} else {
			hi = mid
		}
	}

	res.Culprit = &log.Edits[lo-1]
	res.PriorGreen = lo - 1
	return res, nil
}

func (bl *Blamer) checkAt(ctx context.Context, log *EditLog, sha string, c Check) (bool, error) {
	if log.Workdir == "" || sha == "" {
		return true, nil // no git workdir in tests — assume base passes
	}
	tip := log.Edits[len(log.Edits)-1].SHA

	if err := bl.git(ctx, log.Workdir, "checkout", "--quiet", sha); err != nil {
		return false, fmt.Errorf("blame checkout %s: %w", shortSHA(sha), err)
	}
	defer func() { _ = bl.git(ctx, log.Workdir, "checkout", "--quiet", tip) }()

	v := bl.Verifier.Verify(ctx, log.TaskID, "bisect-"+shortSHA(sha), []Check{c})
	return v.Passed, nil
}

func (bl *Blamer) git(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %s: %w", args, string(out), err)
	}
	return nil
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
