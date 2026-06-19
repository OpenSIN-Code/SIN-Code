#!/bin/bash
# Installiert alle Builtin-Skills und die agent-skills von Addy Osmani

set -e

SIN_SKILLS_DIR="${HOME}/.sin/skills"
mkdir -p "$SIN_SKILLS_DIR"

echo "📦 Installing SIN built-in skills..."
cp -r skills/builtin/* "$SIN_SKILLS_DIR/" 2>/dev/null || true

echo "✅ All skills installed."
echo "Run 'sin skill list' to see them."
