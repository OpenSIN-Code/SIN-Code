#!/usr/bin/env bash
# SIN-Code one-line installer (issue #170): downloads the single static
# sin-code binary from the latest GitHub release. Settle into a writable
# bin dir ($SIN_CODE_BIN_DIR or $HOME/.local/bin) and delegate to
# `sin-code install --auto` so the Go entrypoint handles SHA256 verify
# and atomic placement. Mirrors install.ps1 for Windows.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/OpenSIN-Code/SIN-Code/main/install.sh | bash
#   SIN_CODE_BIN_DIR=~/my-tools bash install.sh --release=v3.17.0
set -euo pipefail
OS=$(uname -s | tr 'A-Z' 'a-z'); case "$OS" in darwin|linux) ;; *) echo "unsupported: $OS" >&2; exit 1 ;; esac
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
BIN_DIR="${SIN_CODE_BIN_DIR:-$HOME/.local/bin}"
URL="https://github.com/OpenSIN-Code/SIN-Code/releases/latest/download/sin-code-${OS}-${ARCH}.tar.gz"
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
echo "[install.sh] downloading $URL"
curl -fsSL --retry 3 -o "$TMP/sc.tgz" "$URL"
mkdir -p "$BIN_DIR"
tar -xzf "$TMP/sc.tgz" -O sin-code > "$BIN_DIR/sin-code" 2>/dev/null
[ -s "$BIN_DIR/sin-code" ] || { echo "[install.sh] ERROR: sin-code binary not found in archive" >&2; exit 1; }
chmod 0755 "$BIN_DIR/sin-code"
echo "[install.sh] installed: $BIN_DIR/sin-code"
case ":$PATH:" in *":$BIN_DIR:"*) ;; *) echo "[install.sh] Add to PATH:  export PATH=\"$BIN_DIR:\$PATH\"" ;; esac
exec "$BIN_DIR/sin-code" install --auto "$@"
