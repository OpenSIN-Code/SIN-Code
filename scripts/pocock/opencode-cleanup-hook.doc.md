# opencode-cleanup-hook.sh - Automated Post-Flight Cleanup

## Purpose

Implements the self-healer skill's cleanup standards fully autonomously. Frees the system
(local macOS terminal or cloud runner) from temporary data garbage after successful task runs,
while explicitly protecting critical sessions, authentications, and configurations.

## What it does

1. **Log-Buffer cleanup** - Removes huge tool-output buffers from `~/.local/share/opencode/tool-output/`
2. **Debug screenshot removal** - Deletes old screenshots from `/tmp/`
3. **NPM cache clearing** - Removes unused Node.js npm cache from `~/.npm/_cacache/`
4. **Runner log cleanup** - Deletes old runner logs from `~/.local/share/opencode/log/`
5. **Temporary file cleanup** - Removes old temporary files (older than 1 hour)
6. **Stale lock removal** - Removes old TDD and DAG lock files (older than 24 hours)
7. **Audit trail** - Documents cleanup in `~/.local/share/sin-solver/repair-docs.md`

## Protected Areas (NEVER touched)

- Agent sessions and memory directories
- Auth keys and API tokens
- Configuration files (`opencode.json`, `config.json`)
- Git repositories and commit history
- Files containing "session", "auth", "token", "secret" in their name
- `.pcpm/` directory (local brain)
- `sin-brain/` directory

## Dependencies

- `bash` - Shell interpreter
- `find` - File finding and age filtering
- `du` - Disk usage calculation
- `stat` - File metadata (macOS/Linux compatible)

## Usage

```bash
# Manual execution
./opencode-cleanup-hook.sh

# Post-flight in scripts
your_task_command && ./opencode-cleanup-hook.sh

# Cron job (hourly cleanup)
0 * * * * /path/to/opencode-cleanup-hook.sh

# In OpenCode task completion
# Add to your task end script or alias
alias task-done='./opencode-cleanup-hook.sh'
```

## Output

```
🧹 OpenSIN Post-Flight Cleanup Hook
══════════════════════════════════════════════════════════════
Start: 2026-06-06 15:30:00

📦 Log-Buffer
ℹ️  Aktuelle Größe: 2.4G
✅ Log-Buffer bereinigt

📦 Debug-Screenshots
✅ 12 Screenshots entfernt

📦 NPM Cache
ℹ️  Cache-Größe: 150M
✅ NPM Cache geleert

📦 Runner-Logs
✅ 45 Runner-Logs entfernt

📦 Temporäre Dateien
✅ 23 temporäre Dateien entfernt

📦 Stale Lock-Files
✅ 3 stale Lock-Files entfernt

🛡️  Schutzzonen-Status
ℹ️  Folgende Bereiche wurden NICHT verändert:
   • Agenten-Sessions und Memory-Verzeichnisse
   • Auth-Keys und API-Token
   • Konfigurationsdateien
   • Git-Repository und Commit-History
   • Alle Dateien mit 'session', 'auth', 'token', 'secret' im Namen

📦 Audit-Trail
✅ Audit-Trail aktualisiert

══════════════════════════════════════════════════════════════
✅ Systembereinigung erfolgreich abgeschlossen
Ende: 2026-06-06 15:30:05
══════════════════════════════════════════════════════════════
```

## Target Directories

- `~/.local/share/opencode/tool-output/` - Tool execution logs
- `~/.npm/_cacache/` - NPM cache
- `~/.local/share/opencode/log/` - Runner logs
- `/tmp/` - Temporary files and screenshots
- `~/.cache/` - User cache
- `./.tdd-locks/` - TDD lock files
- `./.dag-locks/` - DAG lock files

## Age Thresholds

- Temporary files: > 1 hour old
- Lock files: > 24 hours old
- Screenshots: all (immediate removal)
- Logs: all in directory
- NPM cache: all (entire cache)

## Integration with Workflow

1. **Post-flight** - Run after every successful task completion
2. **Automatic** - Can be triggered by task completion hooks
3. **Safe** - Protects critical files and configurations
4. **Audited** - Documents all cleanup operations
5. **Cross-platform** - Works on macOS and Linux

## Key Features

- **Protected zones** - Never touches sensitive data
- **Content scanning** - Detects sensitive files by content
- **Age-based filtering** - Only removes old files
- **Cross-platform** - macOS and Linux compatible
- **Audit trail** - Documents all operations
- **Non-destructive** - Safe to run repeatedly
- **Pattern matching** - Protects files by name patterns

## Known Caveats

- Requires bash (not sh)
- Some operations may require sudo for system-wide cleanup
- File age detection uses mtime (not atime)
- Protected pattern matching is case-sensitive

## Security Note

- Double-checks file content before deletion
- Never deletes files containing "session", "auth", "token", "secret"
- Audit trail helps track what was cleaned
- Protected patterns can be extended in script

## Related Files

- `tdd_enforcer.py` - Creates lock files (cleaned by this hook)
- `dag_kanban.py` - Creates DAG locks (cleaned by this hook)
- `opencode-safe-start.sh` - Sets up environment (run before this)
- `opencode-zod-patch.js` - Runtime patch (run before this)

## Exit Codes

- `0` - Success (or nothing to clean)
- `1` - Critical error (very rare)
