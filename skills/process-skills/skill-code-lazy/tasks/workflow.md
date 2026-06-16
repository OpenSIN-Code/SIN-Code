# Tasks: Workflow

Docs: ../SKILL.md

Each lazy-ship sample task is **pre-gated by `verify.pass`**. The
gate is a precondition, not a step in the task itself.

## Pre-flight (every task)

- [ ] Confirm `verify.result == pass` in the active session state.
- [ ] Confirm intensity level (`lite` | `full` | `ultra`).
- [ ] Confirm subject of laziness: file / function / PR.
- [ ] Confirm no `// TODO: write tests` markers remain in the diff.

## Task 1 — Replace hand-rolled HTTP poll with `http.Get`

Sample: a 40-line polling loop with backoff and a custom URL builder
collapses to a single `http.Get` call inside a `for { … }`.

Verify gate (precondition):

```bash
go test ./... -race -count=1 -run TestPollOnce
curl -fsS http://localhost:8080/healthz
```

Lazy version:

```go
// sin-debt: no exponential backoff, upgrade: add backoff when 429 is observed
for {
    resp, err := http.Get("http://upstream/healthz")
    if err == nil && resp.StatusCode == 200 {
        return
    }
    time.Sleep(time.Second)
}
```

## Task 2 — Delete an unused interface

Sample: a `type Reader interface{ Read(p []byte) (int, error) }`
exists in a file with exactly one implementation. Delete the
interface, change the call site to the concrete type, remove the
file if it now contains nothing else.

Verify gate: `grep -r "Reader" --include="*.go"` returns 0 hits
outside the implementation file.

Lazy version:

- Delete `interfaces.go`.
- Replace `r Reader` with `r *FileReader` in the one call site.
- Remove the now-empty `interfaces_test.go`.

## Task 3 — Collapse a config struct to a flag set

Sample: a 30-line `Config` struct + `LoadConfig()` is replaced by
`flag.StringVar` calls in `main()`.

Verify gate:

```bash
./tool --addr=:9090 --token=xyz 2>&1 | head -1
```

Lazy version:

```go
// sin-debt: flags only, no config file, upgrade: add file when >5 flags appear
addr := flag.String("addr", ":8080", "listen address")
token := flag.String("token", "", "auth token")
flag.Parse()
```

## Task 4 — Replace a logger abstraction with `log/slog`

Sample: a project-local `Logger` interface with three implementations
collapses to `slog`.

Verify gate:

```bash
go test ./... -race -count=1 -run TestLoggerOutput
```

Lazy version:

```go
// sin-debt: text handler, no OTLP, upgrade: switch handler when trace-exporter=otlp
slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
```

## Task 5 — One-line JSON unmarshal

Sample: a custom streaming JSON decoder is replaced by
`json.Unmarshal`.

Verify gate:

```bash
echo '{"x":1}' | ./tool --decode
```

Lazy version:

```go
// sin-debt: in-memory only, no streaming, upgrade: switch when payload >10MB
var v struct{ X int }
_ = json.Unmarshal(payload, &v)
```

## Post-flight (every task)

- [ ] Run `go test ./... -race -count=1` and confirm `ok`.
- [ ] Run `python3 scripts/validate_skill.py --all-bundled --strict`
      and confirm 0 failures.
- [ ] Confirm each `sin-debt:` marker is paired with an `upgrade:`
      clause.
- [ ] Record the lazy-ship in `instinct.Learner.BeforeTurn` log so
      reviewers can audit intensity transitions.
- [ ] If intensity reached `ultra`, document the deletion in
      `CHANGELOG.md` Unreleased-deletion-section.
