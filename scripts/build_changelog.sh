#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Purpose: build CHANGELOG.md from CHANGELOG.d/ fragments (issue #222).
# Sorts fragments by PR number, preserves the static header and
# historical (versioned) sections, and writes a deterministic
# CHANGELOG.md. Use `--check` for pre-commit: exits 1 if the
# generated output would differ from the file on disk.
#
# Usage:
#   scripts/build_changelog.sh            # writes CHANGELOG.md
#   scripts/build_changelog.sh --check    # dry-run, exit 1 on diff
#   scripts/build_changelog.sh --diff     # print the diff
#
# The aggregator is the single source of truth for CHANGELOG.md
# content between the [Unreleased] section and the versioned
# sections below it.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

FRAG_DIR="CHANGELOG.d"
OUT="CHANGELOG.md"

CHECK=0
DIFF=0
for arg in "$@"; do
  case "$arg" in
    --check) CHECK=1 ;;
    --diff)  DIFF=1 ;;
    -h|--help)
      sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

# Static header: first 4 lines (title + intro).
HEADER=$(head -n 4 "$OUT")

# Historical sections: from the first "## [v" line to EOF.
# awk matches lines starting with "## [v" (a versioned section).
HISTORICAL=$(awk '/^## \[v/ {flag=1} flag' "$OUT")

# Fragments: every .md file in CHANGELOG.d/ except _template.md,
# sorted by PR number (the digits before the first '-' in the name).
UNRELEASED=""
shopt -s nullglob
for f in $(ls "$FRAG_DIR"/*.md 2>/dev/null | grep -v '_template\.md$' | sort -t- -k1 -n); do
  if [ -n "$UNRELEASED" ]; then
    UNRELEASED="$UNRELEASED"$'\n'
  fi
  UNRELEASED="$UNRELEASED$(cat "$f")"
done

# Reassemble: header + [Unreleased] block (with the title line) + history.
GENERATED=$(printf '%s\n\n## [Unreleased] - %s\n\n%s\n\n%s\n' \
  "$HEADER" \
  "$(date -u +%Y-%m-%d)" \
  "$UNRELEASED" \
  "$HISTORICAL")

# Trim exactly one trailing newline so CHANGELOG.md ends cleanly.
GENERATED="${GENERATED%$'\n'}"

if [ "$CHECK" = 1 ]; then
  if [ "$GENERATED" = "$(cat "$OUT")" ]; then
    echo "CHANGELOG.md is up to date with CHANGELOG.d/"
    exit 0
  else
    echo "CHANGELOG.md is out of sync with CHANGELOG.d/" >&2
    if [ "$DIFF" = 1 ]; then
      diff -u "$OUT" <(printf '%s' "$GENERATED") | head -40
    fi
    exit 1
  fi
fi

printf '%s\n' "$GENERATED" > "$OUT"
echo "CHANGELOG.md rebuilt from $FRAG_DIR/ ($(ls "$FRAG_DIR"/*.md 2>/dev/null | grep -vc '_template\.md$') fragments)"
