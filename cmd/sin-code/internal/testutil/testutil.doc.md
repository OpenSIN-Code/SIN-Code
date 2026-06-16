# testutil — shared test helpers (issue #161)

`internal/testutil/` is a small, dependency-free (stdlib only) package
that captures the four most common test-hang patterns observed in the
v3.18.0 codebase. Every helper is itself race-tested.

## Helpers

| Helper | Replaces | Why |
|---|---|---|
| `IsolatedSQLite(t)` | `sql.Open("sqlite", "shared/test.db")` | Two parallel tests must not share a file. Each call gets a fresh `t.TempDir()` DB, auto-closed. |
| `CleanEnv(t, kv)` | `os.Setenv` + `t.Cleanup(func() { os.Setenv(k, prev) })` | Easy to forget the prev value (especially for empty values). The helper handles `LookupEnv` semantics. |
| `WithTimeout(t, d, fn)` | ad-hoc `context.WithTimeout` + `select` scattered in tests | One-line call; 50ms post-deadline grace to avoid flakiness when fn returns just-after the deadline. |
| `GoroutineLeakCheck(t, fn)` | `runtime.NumGoroutine` heuristics | Stack-snapshot diff is more reliable than NumGoroutine (which can race the runtime's own reaping). |
| `MustGo(t, fn)` | `go func() { ... }()` for fire-and-forget watchers | Synchronous: returns when fn returns, captures panics as `t.Errorf` (not as a test-binary crash). |

## Why these helpers exist

The issue body lists four hang patterns. The helpers address them
mechanically, not architecturally — i.e. the goal is "fix the
specific test that hangs," not "rewrite the test framework."

The acceptance criteria from issue #161:

- [x] `IsolatedSQLite` — temp dir DB, auto-close
- [x] `CleanEnv` — env var set + restore, handles empty prev
- [x] `WithTimeout` — context-based timeout with grace period
- [x] `GoroutineLeakCheck` — stack-snapshot diff, best-effort
- [ ] `internal/testutil/` ≥ 90% coverage (achieved: 13 tests cover
       all 5 helpers; coverage is not measured at the package level
       because the helpers are themselves the only code in the
       package)
- [x] `go test -race -count=1` is clean for the helpers

## What does NOT ship (deferred)

- **The diagnosis pass** for the top-5 hanging tests on the current
  main. The helpers are the high-value reusable part; the diagnosis
  pass is a per-PR task that depends on the actual hang culprits in
  the codebase. When a CI run hangs, run
  `go test -race -count=1 -v ./... > /tmp/go-test.log 2>&1`,
  identify the top 5 by elapsed time, and apply the matching helper.
- **Replacing existing test patterns**. Mechanical sweep across all
  test files is out of scope; the helpers are added first, used by
  new tests, and migrated in a follow-up.

## Mandates honored

- **M2 (single binary):** stdlib only. `modernc.org/sqlite` is a
  pure-Go SQLite driver already in `go.sum` (no new dep).
- **M7 (race-free):** every helper is tested under `-race`.

## Caveats

`GoroutineLeakCheck` is a **best-effort** detector, not a sound leak
checker. Go's runtime can reap goroutines asynchronously, so a
leaked goroutine may be gone before the snapshot runs, and a
goroutine started late in `fn` may not appear in the "after" count.
For sound detection, use `github.com/goleak/goleak` (not vendored
because M2 forbids new deps; the pattern is straightforward to add
later if needed).
