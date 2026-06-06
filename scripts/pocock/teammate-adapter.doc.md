# teammate-adapter.js - Multi-Agent Swarm Communication Adapter

## Purpose

Standardizes peer-to-peer agent communication for Code-Swarm multi-agent orchestration.
Implements the Claude-Code-Swarm API specifications for decentralized team coordination.

## What it does

1. **spawnTeam** - Initializes team structures and shared directories
2. **write** - Sends direct P2P messages to specific agents
3. **broadcast** - Multicasts messages to all team members
4. **inbox** - Reads messages for a specific agent
5. **approvePlan** - Approves/rejects agent plans
6. **approveShutdown** - Authorizes agent shutdown
7. **requestJoin** - Handles agent join requests
8. **addTask** - Adds tasks to shared queue
9. **status** - Shows team status and metrics
10. **shutdownTeam** - Gracefully shuts down the team

## Directory Structure

```
~/.claude/
├── teams/
│   └── {team-name}/
│       ├── config.json          # Team configuration
│       ├── inboxes/
│       │   └── {agent-id}/
│       │       └── msg-{timestamp}.json
│       └── global/
│           └── broadcasts.json
└── tasks/
    └── {team-name}/
        └── tasks_queue.json
```

## Dependencies

- `fs/promises` - Async file operations
- `path` - Path joining
- `os` - Home directory detection

## Usage

```bash
# Initialize team
node teammate-adapter.js spawn "my-team" "API Gateway Team"

# Send P2P message
node teammate-adapter.js write "my-team" "agent-1" "Start implementation"

# Broadcast to all
node teammate-adapter.js broadcast "my-team" "team-lead" "All hands meeting"

# Read agent inbox
node teammate-adapter.js inbox "my-team" "agent-1"

# Show team status
node teammate-adapter.js status "my-team"

# Approve plan
node teammate-adapter.js approve "my-team" "agent-1" "plan-123"

# Add task
node teammate-adapter.js task "my-team" "Implement auth" "Add JWT" "agent-1"

# Join team
node teammate-adapter.js join "my-team" "agent-2" "backend" "api"

# Shutdown
node teammate-adapter.js shutdown "my-team"
```

## Message Format

```json
{
  "id": "msg-1234567890",
  "from": "team-lead",
  "timestamp": "2026-06-06T15:30:00.000Z",
  "body": "Message content",
  "type": "direct|broadcast|approval|shutdown|join_request",
  "read": false,
  "sequence": 1
}
```

## Task Format

```json
{
  "id": "task-1234567890",
  "name": "Implement auth",
  "description": "Add JWT",
  "assignee": "agent-1",
  "dependencies": [],
  "status": "pending",
  "created_at": "2026-06-06T15:30:00.000Z",
  "sequence": 1
}
```

## Integration with Workflow

1. **After alignment** - `grill_me.py` generates PRD
2. **Task distribution** - `dag_kanban.py` creates task order
3. **Team setup** - `teammate-adapter.js` spawn team
4. **Task assignment** - `addTask` assigns to agents
5. **Execution** - Agents communicate via P2P messages
6. **Approval** - Plans approved before implementation
7. **Cleanup** - `opencode-cleanup-hook.sh` removes team data

## Key Features

- **Decentralized** - No central broker, P2P communication
- **JSON-based** - Human-readable message storage
- **File-system backed** - Survives process restarts
- **Sequential** - Message ordering guaranteed by sequence numbers
- **Persistent** - Messages stored until explicitly read
- **Multi-agent** - Supports any number of agents
- **Extensible** - Easy to add new message types

## Known Caveats

- File-based (not suitable for high-frequency messaging)
- No built-in encryption (file system security relies on OS permissions)
- Messages stored in plaintext
- No automatic cleanup (use `opencode-cleanup-hook.sh`)
- No conflict resolution for concurrent writes
- Agent IDs must be unique within team

## Security Note

- Messages stored in `~/.claude/` directory
- File permissions rely on OS defaults
- Consider encryption for sensitive data
- Audit trail in message files

## Related Files

- `grill_me.py` - Generates PRD that drives task creation
- `dag_kanban.py` - Orchestrates task order
- `tdd_enforcer.py` - Enforces TDD per agent task
- `opencode-cleanup-hook.sh` - Cleans up stale team data

## Performance

- Suitable for < 100 messages/minute
- Each message is a separate file
- Inbox reading is O(n) where n = number of messages
- Status check is O(n*m) where n = agents, m = messages

## Exit Codes

- `0` - Success
- `1` - Error (missing args, file not found, etc.)
