# instinct/project.go — stable project identity

A `Project` is the unit of scoping for an instinct. Two checkouts of the
same repo must produce the same `Project.ID` so the agent doesn't
"learn from scratch" on every CI run.

## Identity resolution

1. `git config --get remote.origin.url` → strip credentials, lowercase, drop `.git`
2. SHA-256 of the normalized remote → first 12 hex chars
3. Fallback: SHA-256 of the toplevel absolute path

This means:

| Checked-out state | Resulting `Project.ID` |
|---|---|
| `https://github.com/Org/Repo` | stable, one ID |
| `git@github.com:Org/Repo.git` | same ID as above |
| `https://user:token@github.com/Org/Repo` | same ID (creds stripped) |
| No git, `/Users/me/work/foo` | stable for that path |

## Why not use the path directly

Path would change between local dev and CI (`/home/runner/work/Repo` vs
`~/code/Repo`). Normalizing the remote is the only stable handle that
survives cloning.

## Related files

- `store.go` — uses `Project.ID` for the projects sub-directory
- `promote.go` — uses `Project.ID` for cross-project promotion counting
