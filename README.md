# SIN-Code Bundle

Unified SOTA Agent-Engineering Stack. Orchestrates all 5 tools as a single MCP server + CLI.

## Install
```bash
# Install all 5 subsystems first
for repo in SIN-Code-Semantic-Codebase-Knowledge-Graphs \
            SIN-Code-Intent-Based-Diffing \
            SIN-Code-Proof-of-Correctness \
            SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration \
            SIN-Code-Architectural-Debt-Watchdogs; do
    cd $repo && pip install -e . && cd ..
done

# Then install the bundle
cd SIN-Code-Bundle && pip install -e .
```

## Usage

### Bootstrap a project
```bash
sin bootstrap .
```

### Review a change
```bash
sin review old_file.py new_file.py
```

### Verify correctness
```bash
sin verify my_module.py my_function
```

### Spin up mocks
```bash
sin mock my-test --api stripe --api github
```

### Check debt
```bash
sin debt .
```

### Start MCP server
```bash
sin serve
```

## MCP Integration
```yaml
mcpServers:
  sin-code:
    command: sin
    args: [serve]
```

## Architecture
- `sin bootstrap`: Initializes SCKG, ADW baseline, and cost tracking
- `sin review`: Combines IBD diffing with SCKG impact analysis
- `sin verify`: Proof-of-correctness via POC
- `sin mock`: Ephemeral mocking via EFSM
- `sin debt`: Architectural debt scan via ADW
- `sin serve`: Unified MCP server exposing all tools
