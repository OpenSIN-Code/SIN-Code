#!/usr/bin/env bash
set -e

echo "🔍 Setting up SIN-Code SAST Tool..."

if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.22+ first."
    exit 1
fi

go version

echo "📦 Installing dependencies..."
go mod tidy

echo "🏗️ Building sin-sast CLI..."
go build -o sin-sast ./cmd/sin-sast

echo "🧪 Running tests..."
go test -v ./...

echo "✅ Setup complete! Binary: ./sin-sast"
echo ""
echo "Usage:"
echo "  ./sin-sast scan ./my-project"
echo "  ./sin-sast list-rules"
