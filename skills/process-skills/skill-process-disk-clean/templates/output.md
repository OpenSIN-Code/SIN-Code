# Output Template — skill-process-disk-clean

```markdown
## Disk Cleanup Report

**Before:** <capacity>% used, <free> GB free
**After:**  <capacity>% used, <free> GB free
**Reclaimed:** <n> GB

### Deleted (safe)
| Target | Size |
|--------|------|
| ~/Library/Caches/Yarn/ | <n> GB |
| ... | ... |

### opencode.db
- Before: <n> GB
- After:  <n> GB  (VACUUM)
- Sessions lost: none

### Skipped (ask-class, not confirmed)
| Target | Size | Reason |
|--------|------|--------|
| ~/.config/sin-solver/authd/state-backups/ | <n> MB | awaiting confirmation |

### Next step
Run `VACUUM` after stopping opencode to reclaim the remaining <n> GB.
```
