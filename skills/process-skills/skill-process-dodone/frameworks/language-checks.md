# DoD Frameworks — Language-Specific Check Commands

## Go

### Build
```bash
go build ./...
```

### Lint/Vet
```bash
go vet ./...
# Optional: staticcheck ./... (if installed)
# Optional: gosec ./... (if installed)
```

### Test
```bash
go test ./... -v -count=1 -race
```

### Coverage
```bash
go test ./... -cover -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -1 | awk '{print $3}'
```

### Unused Imports
```bash
go vet ./...  # catches unused imports
# Or: goimports -l .
```

### Forbidden Patterns
```bash
grep -rn 'TODO\|FIXME\|panic(' --include='*.go' .
grep -rn '_ = err' --include='*.go' .
grep -rn '// .*Logik\|// .*implement' --include='*.go' .
```

## Python

### Build (syntax check)
```bash
python -m py_compile $(find . -name '*.py' -not -path './venv/*')
```

### Lint
```bash
ruff check .
# Or: pylint --disable=all --enable=E .
```

### Test
```bash
pytest -v --tb=short
```

### Coverage
```bash
pytest --cov=. --cov-report=term-missing
```

### Forbidden Patterns
```bash
grep -rn 'TODO\|FIXME\|pass  *#\|NotImplemented' --include='*.py' .
grep -rn 'except.*:\s*pass' --include='*.py' .
grep -rn 'raise NotImplementedError' --include='*.py' .
```

## JavaScript/TypeScript

### Build
```bash
npm run build  # or: tsc --noEmit
```

### Lint
```bash
npx eslint .
# Or: npx tsc --noEmit --strict
```

### Test
```bash
npm test -- --verbose
# Or: jest --verbose
# Or: vitest run --reporter=verbose
```

### Forbidden Patterns
```bash
grep -rn 'TODO\|FIXME\|not implemented' --include='*.ts' --include='*.js' --include='*.tsx' .
grep -rn 'catch.*{\s*}' --include='*.ts' --include='*.js' .
grep -rn 'throw new Error("not implemented")' --include='*.ts' --include='*.js' .
```

## Rust

### Build
```bash
cargo build
```

### Lint
```bash
cargo clippy -- -D warnings
```

### Test
```bash
cargo test -- --nocapture
```

### Forbidden Patterns
```bash
grep -rn 'todo!\|unimplemented!\|panic!' --include='*.rs' .
grep -rn 'unwrap()' --include='*.rs' .  # warn, not fail
```

## Auto-Detection

The check script auto-detects the language from project files:

| File exists | Language | Test command |
|---|---|---|
| `go.mod` | Go | `go test ./... -v -count=1` |
| `pyproject.toml` / `setup.py` | Python | `pytest -v` |
| `package.json` | Node | `npm test -- --verbose` |
| `Cargo.toml` | Rust | `cargo test` |
| `Makefile` | Make | `make test` |

If no language is detected, only the static pattern checks (Säule 1, 5) run.
