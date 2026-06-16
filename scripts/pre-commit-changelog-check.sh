#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Purpose: enforce CHANGELOG.md stays in sync with CHANGELOG.d/
# (issue #222). If the operator edited a fragment without running
# the aggregator, the commit is rejected.
#
# Install:
#   ln -sf ../../scripts/pre-commit-changelog-check.sh \
#          .git/hooks/pre-commit
#
# Skip:
#   git commit --no-verify
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
exec bash "$REPO_ROOT/scripts/build_changelog.sh" --check
