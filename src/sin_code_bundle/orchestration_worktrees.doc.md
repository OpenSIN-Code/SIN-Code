# Worktree Orchestration — Isolated Git Worktrees for Parallel Agents

## Purpose

Git worktree orchestration for parallel agent tasks — gives each task
its own branch and working copy, so multiple agents can run in parallel
without stomping on each other. After the task finishes, the worktree
can optionally be merged back into `main` and removed.

This is a thin wrapper around `git worktree add` / `git worktree remove`
with an in-memory registry of active worktrees.

## Public API

### `SINWorktreeOrchestrator`

- `__init__(repo_root=None)` — defaults to `Path.cwd()`. Initializes an
  empty `active_worktrees: list[Path]`.
- `is_git_repo() -> bool` — `True` iff `<repo_root>/.git` exists.
- `create_worktree(branch_name=None) -> dict`
  - Returns `{"error": "Not a git repository..."}` if not in a git repo.
  - If `branch_name` is `None`, generates `sin-task-<8 hex chars>` via
    `uuid.uuid4().hex[:8]`.
  - Worktree path:
    `<repo_root.parent>/.sin-worktrees-<repo_root.name>/<branch>` —
    sibling directory of the repo, so it doesn't pollute the working
    tree.
  - Runs `git worktree add <path> -b <branch>` (capture_output,
    check=True).
  - On success: appends the path to `active_worktrees`, returns
    `{"success", "worktree_path", "branch", "message"}`.
  - On `CalledProcessError`: returns
    `{"error": "Git worktree creation failed: <stderr>"}`.
- `execute_in_worktree(worktree_path, task_func, *args, **kwargs) -> dict`
  - `os.chdir(worktree_path)`, calls `task_func(*args, **kwargs)`,
    restores the original cwd in `finally`. Returns
    `{"success": True, "result": ...}` or
    `{"success": False, "error": "..."}`.
  - Note: only changes `os.getcwd()`, not the env-var `PWD`.
- `cleanup_worktree(worktree_path, merge_back=False) -> dict`
  - If `worktree_path` is not in `self.active_worktrees`, returns
    `{"error": "Worktree not managed by this orchestrator"}`.
  - If `merge_back=True`:
    1. `git checkout main` in the repo root.
    2. `git merge --no-ff <branch> -m "Auto-merge from SIN worktree: <branch>"`.
    3. If merge fails (returncode != 0), returns
       `{"error": "Merge conflict: <stderr>"}` **without** removing the
       worktree (caller is expected to resolve manually).
  - Then `git worktree remove <path> --force`, removes the path from
    the active list, and `shutil.rmtree(path)` if it still exists.
  - Returns `{"success": True, "message": "Worktree cleaned up
    successfully"}` or `{"error": "Worktree cleanup failed: <stderr>"}`.

## Failure modes

| Failure                                         | Result                                                       |
|-------------------------------------------------|--------------------------------------------------------------|
| Not a git repo                                  | `{"error": "Not a git repository. Worktree isolation requires git."}` |
| `git worktree add` fails (e.g. branch exists)   | `{"error": "Git worktree creation failed: <stderr>"}`        |
| `merge_back=True` + merge conflict              | Returns `{"error": "Merge conflict: <stderr>"}`; **worktree is preserved** so the agent can resolve manually. Caller must re-run `cleanup_worktree(merge_back=False)` after fixing. |
| `git worktree remove` fails (e.g. dirty worktree) | `{"error": "Worktree cleanup failed: <stderr>"}`            |
| Worktree path not in `active_worktrees`         | `{"error": "Worktree not managed by this orchestrator"}`     |

## Dependencies

- **Stdlib only** for the orchestrator: `os`, `shutil`, `subprocess`,
  `uuid`, `pathlib`.
- Requires `git` on `PATH` (no graceful fallback — if git is missing,
  every `subprocess.run` raises `FileNotFoundError` and the user sees
  the raw exception via the `CalledProcessError` path).

## Usage example

```python
from pathlib import Path
from sin_code_bundle.orchestration_worktrees import SINWorktreeOrchestrator

orch = SINWorktreeOrchestrator(repo_root=Path("/repo"))
result = orch.create_worktree(branch_name="sin-task-feature-x")
# {"success": True, "worktree_path": ".../.sin-worktrees-.../sin-task-feature-x", ...}

orch.execute_in_worktree(
    result["worktree_path"],
    lambda: do_some_edits(),
)

# Merge back into main and clean up
orch.cleanup_worktree(result["worktree_path"], merge_back=True)
```

Also exposed via MCP: `sin_create_worktree(branch_name="")` and
`sin_cleanup_worktree(worktree_path, merge_back=False)`
(see `mcp_server.py`).

## Known caveats

- `execute_in_worktree` uses `os.chdir` — this is **process-global**.
  Concurrent calls from the same process will race. In the typical
  `sin_code_orchestration` usage, one task runs at a time, so this is
  acceptable.
- Merge conflicts leave the worktree in place; subsequent
  `cleanup_worktree(merge_back=True)` calls will keep failing. Resolve
  manually then call with `merge_back=False`.
- The worktree directory lives **outside** the repo (`<parent>/.sin-worktrees-<name>/<branch>`).
  If the repo is on a different filesystem than its parent (mount
  boundary), `git worktree` may fail.
- No rebase support — only `--no-ff` merge to `main`.
