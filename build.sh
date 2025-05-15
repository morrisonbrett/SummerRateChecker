#!/bin/bash

# SummerRateChecker Build Script for Go

echo "🏖️ Building SummerRateChecker (Go version)..."

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go not found. Please install Go first:"
    echo "   Visit: https://golang.org/dl/"
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | cut -d ' ' -f 3)
echo "✅ Found Go: $GO_VERSION"

# Initialize module if go.mod doesn't exist
if [ ! -f "go.mod" ]; then
    echo "📦 Initializing Go module..."
    go mod init github.com/morrisonbrett/SummerRateChecker
fi

# Clean and download dependencies
echo "📥 Downloading dependencies..."
go clean -modcache
go mod tidy

# Build the project
echo "🔨 Building..."
go build -o bin/SummerRateChecker.exe ./main.go

if [ $? -eq 0 ]; then
    echo "✅ Build successful!"
    echo "📍 Binary location: bin/SummerRateChecker.exe"
    echo ""
    echo "Next steps:"
    echo "1. Copy config.toml.example to config.toml and fill in your details"
    echo "   OR set environment variables (see .env file)"
    echo "2. Set up your Discord bot and webhook"
    echo "3. Run: ./bin/SummerRateChecker.exe"
else
    echo "❌ Build failed!"
    exit 1
fi