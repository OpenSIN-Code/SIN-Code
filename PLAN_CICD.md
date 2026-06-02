# PLAN: CI/CD mit GitHub Actions

**Ziel:** Automatisches Testen, Bauen und Release-Erstellung für alle 7 SIN-Code Tools.

**Status:** 🟡 WICHTIG
**Aufwand:** ~2 Stunden

---

## Pipeline-Struktur

Für jedes Tool-Repo:

```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main]
    tags: ['v*']
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      
      - name: Build
        run: go build -o bin/discover cmd/discover/main.go
      
      - name: Test
        run: go test -v -cover ./...
      
      - name: Coverage
        run: go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
      
      - name: Lint
        run: go vet ./... && gofmt -l .
      
      - name: Install
        run: |
          mkdir -p ~/.local/bin
          cp bin/discover ~/.local/bin/
  
  release:
    needs: test
    runs-on: macos-latest
    if: startsWith(github.ref, 'refs/tags/v')
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      
      - name: Build for all platforms
        run: |
          GOOS=darwin GOARCH=amd64 go build -o bin/discover-darwin-amd64 cmd/discover/main.go
          GOOS=darwin GOARCH=arm64 go build -o bin/discover-darwin-arm64 cmd/discover/main.go
          GOOS=linux GOARCH=amd64 go build -o bin/discover-linux-amd64 cmd/discover/main.go
      
      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: bin/*
          generate_release_notes: true
```

---

## Per-Tool CI

Für jedes der 7 Tools erstelle `.github/workflows/ci.yml`:

1. SIN-Code-Discover-Tool
2. SIN-Code-Execute-Tool
3. SIN-Code-Map-Tool
4. SIN-Code-Grasp-Tool
5. SIN-Code-Scout-Tool
6. SIN-Code-Harvest-Tool
7. SIN-Code-Orchestrate-Tool

---

## Bundle CI (Python)

Für SIN-Code-Bundle:

```yaml
# .github/workflows/ci.yml
name: Bundle CI

on:
  push:
    branches: [main]
    tags: ['v*']

jobs:
  test:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.12'
      
      - name: Install dependencies
        run: pip install -e .
      
      - name: Run tests
        run: pytest tests/
      
      - name: Lint
        run: ruff check src/
```

---

## Coverage-Threshold

Jedes Repo sollte mindestens 70% Coverage haben.

```yaml
      - name: Coverage Check
        run: |
          coverage=$(go test -cover ./... | grep -oP '\d+\.\d+(?=%\s*$)' | head -1)
          if (( $(echo "$coverage < 70" | bc -l) )); then
            echo "Coverage $coverage% is below 70%"
            exit 1
          fi
```

---

## Auto-Update Bundle

Wenn ein Tool-Repo released wird, soll das Bundle automatisch die neue Version erkennen:

```yaml
# SIN-Code-Bundle/.github/workflows/update-tools.yml
name: Update Tools

on:
  repository_dispatch:
    types: [tool-released]

jobs:
  update:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Detect latest tool versions
        run: |
          python src/sin_code_bundle/cli.py sin-code update
```

---

## Files zu erstellen

Pro Tool-Repo:
- `.github/workflows/ci.yml` — Build & Test Pipeline
- `.github/workflows/release.yml` — Release Pipeline

Für SIN-Code-Bundle:
- `.github/workflows/ci.yml` — Python Tests
- `.github/workflows/update-tools.yml` — Auto-Update

---

## Geschätzte Zeit

- Pro Tool: 15 Minuten
- Total: ~2 Stunden
