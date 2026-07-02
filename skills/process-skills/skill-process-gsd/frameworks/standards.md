# GSD Frameworks — Standards

## Integration with plan v2

GSD phases map to plan v2 phases. For each GSD phase:
1. Create a plan using `plan v2` (full, --lite, or --from-spec)
2. Save the plan to `.gsd/plans/phase-N.md`
3. Execute via `sin-code gsd execute N`

## Integration with delegate-subagents

Each wave in a phase can be executed in parallel:
1. `sin-code gsd execute <phase-id>` — get wave structure
2. Launch all tasks in a wave via `delegate-subagents`
3. Mark each task done: `sin-code gsd execute <phase-id> --task <id> --status done`

## Integration with dodone + self-review

Before marking a phase as completed:
1. Run `self-review` (CEO-grade review of all changes)
2. Run `dodone check` (deterministic 11-pillar machine gate)
3. Only then: `sin-code gsd phase edit <phase-id> --status completed`
