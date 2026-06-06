# grill_me.py - Socratic Alignment Tool

## Purpose

Generates structured Product Requirements Documents (PRDs) through adversarial questioning,
preventing agents from starting to code before understanding edge cases, constraints, and system boundaries.

## What it does

1. Forces systematic questioning of the user's goals before coding
2. Captures 10 critical dimensions: problem definition, stakeholders, constraints, edge cases, migration paths, system boundaries, success metrics, integrations, rollback plans, and dependencies
3. Generates a standardized PRD.md with all answers documented
4. Enforces the Matt Pocock System-Design Paradigm alignment phase

## Dependencies

- `sys` - CLI arguments
- `argparse` - Command-line interface
- `json` - JSON serialization
- `dataclasses` - Data structures

## Usage

```bash
# Interactive mode
python3 -m sin_code_bundle.tools.pocock.grill_me "Implement API Gateway"

# Non-interactive (CI/CD)
python3 -m sin_code_bundle.tools.pocock.grill_me "API Gateway" --non-interactive \
  --answers '{"problem_definition": "Need auth", "stakeholders": "Backend team"}'

# Output JSON
python3 -m sin_code_bundle.tools.pocock.grill_me "API Gateway" --json
```

## Integration with Workflow

1. **Before coding**: Always run `grill_me` first
2. **Generates**: `PRD.md` in current directory
3. **Consumed by**: `dag_kanban.py` to extract task slices
4. **Enforced by**: `tdd_enforcer.py` to ensure tests exist before implementation

## Key Features

- **10 default questions** covering all critical dimensions
- **Custom question support** via custom_questions parameter
- **JSON export** for programmatic consumption
- **CI/CD ready** with non-interactive mode
- **Empty answer prevention** - forces precise answers

## Known Caveats

- Requires interactive input for full effectiveness
- Non-interactive mode requires all answers upfront
- PRD.md is overwritten if it exists

## Related Files

- `dag_kanban.py` - Consumes the generated PRD.md
- `tdd_enforcer.py` - Enforces RED phase after alignment
- `teammate-adapter.js` - Broadcasts tasks to swarm after alignment
