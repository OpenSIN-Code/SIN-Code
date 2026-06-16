# evalharness/store.go — on-disk format

```
<base>/
├── sets/<name>.json
└── runs/<run-id>.json
```

`<base>` resolution: `SIN_EVAL_DIR` → `$XDG_DATA_HOME/sin-code-eval` →
`~/.local/share/sin-code-eval`.

## Why JSON (and not JSONL for runs)

A `Run` is a single document; per-case results live inside it. JSONL
is the right shape for *streams* (audit logs, exchanges). A run is
*one* thing.

## Why no rotation

Eval runs are rare (CI + manual). Storage cost is negligible.
Rotation is a `find $base/runs -mtime +90 -delete` away if it
becomes a problem.

## Related files

- `types.go` — `Run` / `EvalSet` types
- `cli.go` — uses `LoadSet` / `SaveRun` / `LoadRun`
