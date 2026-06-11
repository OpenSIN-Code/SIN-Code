# orchestrator/contract.go

Intent Contract — machine-enforceable mandate compiled from a task.
Enforced twice: pre-flight gate on every proposed edit (instant) and
post-hoc diff-scope check in the Verifier (authoritative).

## Public surface

- `Contract{TaskID, AllowedGlobs, FrozenGlobs, ForbiddenPatterns, MaxFilesChanged, MaxLinesChanged, RequiredInvariants}`
- `ForbiddenPattern{Name, Pattern, OnlyNewCode}`
- `Violation{Kind, Path, Line, Detail}`
- `DefaultForbidden()` — secrets, private keys, `t.Skip()`, debug leftovers
- `CheckEdit(path, addedLines) []Violation` — pre-flight gate
- `CheckDiffStats(filesChanged, linesChanged) []Violation` — blast radius
- `AsChecks() []Check` — convert to verifier checks (post-hoc)
- `CompileContract(task) *Contract` — derive a contract from a task

## Defaults

- `MaxFilesChanged = 25`
- `MaxLinesChanged = 2000`
- Lockfiles (`go.sum`, `package-lock.json`, …) and `.github/workflows/*`
  are auto-frozen unless the task title/description mentions
  `lockfile`/`dependency`/`ci`/`workflow`/`pipeline`.

## Forbidden patterns (only checked on new code)

| Name              | Example match                                  |
|-------------------|------------------------------------------------|
| hardcoded-secret  | `apiKey := "sk_live_..."`                      |
| private-key-block | `-----BEGIN RSA PRIVATE KEY-----`              |
| disabled-test     | `t.Skip()`, `it.skip()`, `@pytest.mark.skip`   |
| debug-leftover    | `console.log("[v0]")`, `fmt.Println("DEBUG")`  |

## Globs

- `internal/tui/*` matches `internal/tui/x.go` (filepath.Match).
- `internal/tui/*` also matches `internal/tui/widgets/y.go` (prefix form).
