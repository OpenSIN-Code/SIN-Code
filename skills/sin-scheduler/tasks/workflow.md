# Tasks: Workflow

Docs: ../SKILL.md

## Pre-flight

- [ ] Capture job name, command, and schedule.

## Execution

- [ ] Task 1: Define job.
  - Acceptance: Name, command, schedule type, expression clear.
  - Verify: Input complete.
- [ ] Task 2: Schedule job.
  - Acceptance: `schedule_job` returns job ID.
  - Verify: Job listed.
- [ ] Task 3: Verify listing.
  - Acceptance: Job appears in `schedule_list`.
  - Verify: Enabled and scheduled correctly.
- [ ] Task 4: Run or wait.
  - Acceptance: Job executes or is triggered with `schedule_run_now`.
  - Verify: Execution recorded.
- [ ] Task 5: Review logs.
  - Acceptance: `schedule_logs` shows output and exit code.
  - Verify: No unexpected errors.
- [ ] Task 6: Cancel if needed.
  - Acceptance: `schedule_cancel` removes job.
  - Verify: Job no longer listed.

## Post-flight

- [ ] Summarize scheduled jobs and their status.
- [ ] Note any failures or warnings.
