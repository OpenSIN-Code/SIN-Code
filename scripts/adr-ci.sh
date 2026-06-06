#!/bin/bash
# ADR CI check — generates ADRs and fails on critical issues
set -e
ROOT="${1:-.}"
GRAPH="/tmp/sin-adr-ci-graph.json"
sckg index "$ROOT" --output "$GRAPH" 2>/dev/null
sckg adr "$GRAPH" --output /tmp/sin-adr-ci-adrs 2>/dev/null
# Check for CRITICAL ADRs
if grep -r "CRITICAL" /tmp/sin-adr-ci-adrs/ 2>/dev/null; then
    echo "FAIL: Critical ADRs found"
    exit 1
fi
echo "OK: No critical ADRs"
exit 0
