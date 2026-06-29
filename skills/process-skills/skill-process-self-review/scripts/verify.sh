#!/usr/bin/env bash
#
# Ultra-CEO Verifikations-Lauf.
# Erkennt Build/Typecheck, Lint und Tests automatisch und führt sie real aus.
# Exit-Code != 0 => mindestens ein Schritt ist rot => BLOCKER.
#
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# Falls als Skill installiert: arbeite im aktuellen Projektverzeichnis.
PROJECT_DIR="${1:-$(pwd)}"
cd "$PROJECT_DIR" || { echo "Projektverzeichnis nicht erreichbar: $PROJECT_DIR"; exit 2; }

FAILURES=0
RAN_ANYTHING=0

hr()   { printf '%s\n' "------------------------------------------------------------"; }
say()  { printf '\n>>> %s\n' "$1"; }

# Führt einen Schritt aus, protokolliert Ergebnis, zählt Fehler.
run_step() {
  local label="$1"; shift
  RAN_ANYTHING=1
  say "$label: $*"
  if "$@"; then
    echo "[OK]   $label"
  else
    echo "[FAIL] $label  (Exit $?)"
    FAILURES=$((FAILURES + 1))
  fi
  hr
}

# Wählt den passenden JS-Paketmanager anhand der Lockfiles.
js_pm() {
  if [ -f pnpm-lock.yaml ]; then echo "pnpm"; return; fi
  if [ -f yarn.lock ];     then echo "yarn"; return; fi
  if [ -f bun.lockb ];     then echo "bun";  return; fi
  echo "npm"
}

# Prüft, ob ein npm-Script in package.json existiert.
has_npm_script() {
  [ -f package.json ] || return 1
  node -e "process.exit(((require('./package.json').scripts)||{})['$1']?0:1)" 2>/dev/null
}

run_npm_script() {
  local script="$1" pm; pm="$(js_pm)"
  case "$pm" in
    npm)  run_step "npm:$script"  npm  run "$script" --if-present ;;
    *)    run_step "$pm:$script"  "$pm" run "$script" ;;
  esac
}

say "ULTRA-CEO VERIFIKATION  |  $(date)"
say "Projekt: $PROJECT_DIR"
hr

# ---------- Git-Umfang ----------
if [ -d .git ] || git rev-parse --git-dir >/dev/null 2>&1; then
  say "Git-Status (geänderter Umfang)"
  git status --short
  hr
  say "Git-Diff (Statistik)"
  git --no-pager diff --stat
  hr
fi

# ---------- JavaScript / TypeScript ----------
if [ -f package.json ]; then
  has_npm_script typecheck && run_npm_script typecheck
  has_npm_script "type-check" && run_npm_script "type-check"
  if [ -f tsconfig.json ] && ! has_npm_script typecheck && ! has_npm_script "type-check"; then
    run_step "tsc --noEmit" npx --no-install tsc --noEmit
  fi
  has_npm_script build && run_npm_script build
  has_npm_script lint  && run_npm_script lint
  has_npm_script test  && run_npm_script test
fi

# ---------- Python ----------
if ls ./*.py >/dev/null 2>&1 || [ -f pyproject.toml ] || [ -f requirements.txt ]; then
  command -v ruff    >/dev/null 2>&1 && run_step "ruff"    ruff check .
  command -v mypy    >/dev/null 2>&1 && run_step "mypy"    mypy .
  command -v pytest  >/dev/null 2>&1 && run_step "pytest"  pytest -q
fi

# ---------- Go ----------
if [ -f go.mod ]; then
  run_step "go build" go build ./...
  run_step "go vet"   go vet ./...
  run_step "go test"  go test ./...
fi

# ---------- Rust ----------
if [ -f Cargo.toml ]; then
  run_step "cargo check" cargo check
  run_step "cargo clippy" cargo clippy -- -D warnings
  run_step "cargo test"  cargo test
fi

# ---------- Makefile-Fallback ----------
if [ "$RAN_ANYTHING" -eq 0 ] && [ -f Makefile ]; then
  grep -qE '^test:'  Makefile && run_step "make test"  make test
  grep -qE '^lint:'  Makefile && run_step "make lint"  make lint
  grep -qE '^build:' Makefile && run_step "make build" make build
fi

say "ZUSAMMENFASSUNG"
if [ "$RAN_ANYTHING" -eq 0 ]; then
  echo "WARNUNG: Keine Build-/Lint-/Test-Pipeline erkannt. Manuelle Verifikation zwingend."
  echo "STATUS: UNGEPRUEFT"
  exit 3
fi
if [ "$FAILURES" -gt 0 ]; then
  echo "FEHLGESCHLAGENE SCHRITTE: $FAILURES"
  echo "STATUS: ROT  (=> BLOCKER, Auslieferung verboten)"
  exit 1
fi
echo "Alle ausgeführten Schritte: OK"
echo "STATUS: GRUEN  (notwendiges Minimum erfüllt, Verhalten separat prüfen)"
exit 0
