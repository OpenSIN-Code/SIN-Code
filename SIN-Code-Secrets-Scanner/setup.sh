#!/usr/bin/env bash
set -e

echo "🔐 Setting up SIN-Code Secrets Scanner..."

if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.22+ first."
    exit 1
fi

go version

echo "📦 Installing dependencies..."
go mod tidy

echo "🏗️ Building sin-secrets CLI..."
go build -o sin-secrets ./cmd/sin-secrets

echo "🧪 Running tests..."
go test -v ./...

echo "✅ Setup complete! Binary: ./sin-secrets"
echo ""
echo "Usage:"
echo "  ./sin-secrets scan ./my-project"
echo "  ./sin-secrets list-rules"
echo "  ./sin-secrets scan ./my-project --output json --severity high"
