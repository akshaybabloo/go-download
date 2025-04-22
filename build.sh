#!/bin/bash

# Build script for go-download CLI tool

# Exit on error
set -e

# Create output directory
mkdir -p build

echo "Building go-download CLI tool..."

# Build for Linux
echo "Building for Linux (amd64)..."
GOOS=linux GOARCH=amd64 go build -o build/gdl-linux-amd64 ./cli/gdl

# Build for macOS
echo "Building for macOS (amd64)..."
GOOS=darwin GOARCH=amd64 go build -o build/gdl-darwin-amd64 ./cli/gdl

# Build for macOS ARM (Apple Silicon)
echo "Building for macOS (arm64)..."
GOOS=darwin GOARCH=arm64 go build -o build/gdl-darwin-arm64 ./cli/gdl

# Build for Windows
echo "Building for Windows (amd64)..."
GOOS=windows GOARCH=amd64 go build -o build/gdl-windows-amd64.exe ./cli/gdl

echo "Build complete! Binaries are available in the 'build' directory."
echo ""
echo "Linux:       build/gdl-linux-amd64"
echo "macOS Intel: build/gdl-darwin-amd64"
echo "macOS ARM:   build/gdl-darwin-arm64"
echo "Windows:     build/gdl-windows-amd64.exe"
