#!/usr/bin/env bash
# Purpose: Verify the ceo-audit skill is correctly installed and all dependencies are present
# Docs: SKILL.md
#
# Read-only: does NOT change the filesystem. Safe to run in CI as a
# pre-flight check before audit.sh.
#
# Checks:
#   1. All .sh scripts under scripts/ are executable
#   2. All required Python modules import
#   3. All 7 core SIN-Code tools are on PATH (or in a venv)
#   4. audit.sh --help runs and produces output
#   5. The template workflow file is present
#
# Output:
#   Green [OK] / Red [FAIL] lines per check. Exit 0 only if all OK.
#   `--quiet` mode: silent on OK, only print failures
#
# Exit codes:
#   0   all checks pass
#   1   one or more checks failed
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKILL_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

QUIET=0
for arg in "$@"; do
  case "$arg" in
    --quiet|-q) QUIET=1 ;;
    -h|--help)
      sed -n '2,25p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
  esac
done

# ── Color helpers ──────────────────────────────────────────────────────
if [[ -t 1 ]] && [[ -z "${NO_COLOR:-}" ]]; then
  C_RESET=$'\033[0m'; C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'
  C_YELLOW=$'\033[1;33m'; C_BLUE=$'\033[0;34m'; C_BOLD=$'\033[1m'
else
  C_RESET=""; C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_BOLD=""
fi

ok()    { [[ $QUIET -eq 0 ]] && printf "%s[OK]%s    %s\n"   "$C_GREEN"  "$C_RESET" "$*"; return 0; }
warn()  { printf "%s[WARN]%s  %s\n" "$C_YELLOW" "$C_RESET" "$*"; }
fail()  { printf "%s[FAIL]%s  %s\n" "$C_RED"   "$C_RESET" "$*"; FAIL=1; }
info()  { [[ $QUIET -eq 0 ]] && printf "%s[INFO]%s  %s\n" "$C_BLUE"   "$C_RESET" "$*"; }
heading(){ [[ $QUIET -eq 0 ]] && printf "\n%s%s== %s ==%s\n" "$C_BOLD" "$C_BLUE" "$*" "$C_RESET"; }

FAIL=0

heading "ceo-audit installation validation"
info "Skill root: $SKILL_ROOT"

# ── Check 1: executables ──────────────────────────────────────────────
heading "Check 1/5: Script permissions"
EXPECTED_SCRIPTS=(
  audit.sh
  install-skill.sh
  validate-install.sh
  benchmark.sh
  axis_security.sh
  axis_performance.sh
  axis_quality.sh
  axis_testing.sh
  axis_deps.sh
  axis_docs.sh
  axis_architecture.sh
  axis_compliance.sh
)
# (post_audit_pr.py is a Python module invoked with `python3`, not a
# shell entry point — it intentionally stays non-executable, matching
# the convention used by score.py and report.py.)
for s in "${EXPECTED_SCRIPTS[@]}"; do
  p="$SKILL_ROOT/scripts/$s"
  if [[ ! -f "$p" ]]; then
    fail "missing: scripts/$s"
  elif [[ ! -x "$p" ]]; then
    fail "not executable: scripts/$s (chmod +x)"
  else
    ok "executable: scripts/$s"
  fi
done

# ── Check 2: Python deps ──────────────────────────────────────────────
heading "Check 2/5: Python dependencies"
PY_MODS=(jinja2 yaml)
for m in "${PY_MODS[@]}"; do
  if python3 -c "import $m" 2>/dev/null; then
    ok "python: $m"
  else
    fail "python: $m MISSING (pip install $m)"
  fi
done

# Optional but recommended
for m in pytest cryptography requests; do
  if python3 -c "import $m" 2>/dev/null; then
    ok "python (optional): $m"
  else
    warn "python (optional): $m not installed"
  fi
done

# ── Check 3: SIN-Code tools on PATH ───────────────────────────────────
heading "Check 3/5: SIN-Code toolchain"
CORE_TOOLS=(discover map grasp scout execute harvest orchestrate)
for tool in "${CORE_TOOLS[@]}"; do
  if command -v "$tool" >/dev/null 2>&1; then
    ok "$tool: $(command -v "$tool")"
  else
    fail "$tool: NOT on PATH (install SIN-Code)"
  fi
done

# Optional but recommended
OPT_TOOLS=(bandit mypy ruff gosec govulncheck pip-audit jscpd)
for tool in "${OPT_TOOLS[@]}"; do
  if command -v "$tool" >/dev/null 2>&1; then
    ok "$tool: $(command -v "$tool")"
  else
    warn "$tool: not installed (some gates will skip)"
  fi
done

# ── Check 4: audit.sh --help ──────────────────────────────────────────
heading "Check 4/5: audit.sh --help"
if [[ ! -x "$SKILL_ROOT/scripts/audit.sh" ]]; then
  fail "audit.sh not executable — cannot test --help"
else
  if HELP_OUT="$(bash "$SKILL_ROOT/scripts/audit.sh" --help 2>&1)"; then
    if [[ -n "$HELP_OUT" ]] && echo "$HELP_OUT" | grep -q "CEO Audit"; then
      ok "audit.sh --help works"
    else
      fail "audit.sh --help output is empty or missing 'CEO Audit' header"
    fi
  else
    fail "audit.sh --help exited non-zero"
  fi
fi

# ── Check 5: Templates + tests present ─────────────────────────────────
heading "Check 5/5: Required files"
for f in \
  SKILL.md \
  README.md \
  CHANGELOG.md \
  templates/ceo-audit.yml \
  templates/report.md \
  templates/sarif.json \
  tests/test_github_app.py \
  lib/owasp_asvs.py \
  lib/cwe.py \
  lib/sin_tools.py \
  lib/add_finding.py \
  lib/github_app.py \
  scripts/score.py \
  scripts/report.py; do
  if [[ -f "$SKILL_ROOT/$f" ]]; then
    ok "present: $f"
  else
    fail "missing: $f"
  fi
done

# ── Optional: pytest discovery ────────────────────────────────────────
if command -v pytest >/dev/null 2>&1 || python3 -m pytest --version >/dev/null 2>&1; then
  heading "Bonus: pytest discovery"
  if python3 -m pytest "$SKILL_ROOT/tests" --collect-only -q 2>/dev/null | tail -3; then
    ok "pytest can discover tests"
  else
    warn "pytest discovery failed (some test files broken?)"
  fi
fi

# ── Summary ────────────────────────────────────────────────────────────
heading "Validation summary"
if [[ $FAIL -eq 0 ]]; then
  printf "%s[OK]%s    All checks passed — ceo-audit is ready to run\n" "$C_GREEN" "$C_RESET"
  echo ""
  echo "Try:"
  echo "  bash $SKILL_ROOT/scripts/audit.sh . --profile=SECURITY"
  exit 0
else
  printf "%s[FAIL]%s  One or more checks failed. See above.\n" "$C_RED" "$C_RESET"
  echo ""
  echo "Fix suggestions:"
  echo "  - Missing tool? Run: bash $SKILL_ROOT/scripts/install-skill.sh"
  echo "  - Missing Python module? pip install <module>"
  echo "  - Non-executable script? chmod +x $SKILL_ROOT/scripts/<name>.sh"
  exit 1
fi
