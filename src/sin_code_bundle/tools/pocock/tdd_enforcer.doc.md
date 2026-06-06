# tdd_enforcer.py - TDD Gatekeeper

## Purpose

Enforces the strict Test-Driven Development (TDD) Red-Green-Refactor cycle by locking
source files until a failing test is registered and executed.

## What it does

1. **RED Phase**: Runs tests before code changes - must fail (proves test exists and is meaningful)
2. **GREEN Phase**: After implementing code, tests must pass (unlocks file for refactoring)
3. **REFACTOR Phase**: With passing tests, code can be refactored
4. **Lock Management**: Creates/locks files to prevent unauthorized editing

## Dependencies

- `subprocess` - Test execution
- `os` - File operations
- `json` - State persistence
- `argparse` - CLI interface

## Usage

```bash
# Check if editing is allowed (enforces TDD cycle)
python3 -m sin_code_bundle.tools.pocock.tdd_enforcer "pytest tests/" "src/api.py"

# Check lock status only
python3 -m sin_code_bundle.tools.pocock.tdd_enforcer "pytest tests/" "src/api.py" --check

# Reset TDD state for a file
python3 -m sin_code_bundle.tools.pocock.tdd_enforcer "pytest tests/" "src/api.py" --reset

# Output JSON
python3 -m sin_code_bundle.tools.pocock.tdd_enforcer "pytest tests/" "src/api.py" --json
```

## State Machine

```
UNKNOWN -> RED (tests fail) -> GREEN (tests pass) -> REFACTOR
  ^                                       |
  |_______________________________________|
  (After refactoring, cycle repeats)
```

## Lock Files

- Location: `.tdd-locks/` in current directory
- Format: `.lock.{safe_filename}`
- Contains: phase, timestamp, status
- State file: `.tdd-locks/tdd-state.json`

## Integration with Workflow

1. **After alignment**: Once PRD is ready, write tests first
2. **Enforces**: Must have failing test before touching implementation
3. **Prevents**: Unilateral code changes without tests
4. **Tracks**: TDD phase per file in `.tdd-locks/`

## Key Features

- **Per-file tracking** - Each file has its own TDD state
- **Automatic lock management** - Locks/unlocks based on test results
- **JSON state persistence** - Survives process restarts
- **CI/CD integration** - Non-interactive mode available
- **Test output capture** - Shows why tests fail

## Known Caveats

- Requires test command to return non-zero on failure
- Lock directory must be in `.gitignore`
- Timeout of 60 seconds for test execution
- Only works for single file at a time

## Related Files

- `grill_me.py` - Must run before TDD starts
- `dag_kanban.py` - Orchestrates which files to work on
- `opencode-cleanup-hook.sh` - Cleans stale lock files
