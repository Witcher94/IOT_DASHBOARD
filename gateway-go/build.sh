#!/bin/bash

# Build script for Mesh Gateway
# Compiles for multiple platforms

set -e

VERSION="1.0.0"
OUTPUT_DIR="./dist"
BINARY_NAME="mesh-gateway"

mkdir -p $OUTPUT_DIR

echo "Building Mesh Gateway v$VERSION..."

# Linux AMD64 (regular servers)
echo "  → Linux AMD64"
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${BINARY_NAME}-linux-amd64" .

# Linux ARM64 (Raspberry Pi 4, newer ARM)
echo "  → Linux ARM64 (Raspberry Pi 4)"
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${BINARY_NAME}-linux-arm64" .

# Linux ARM (Raspberry Pi 3, Zero W)
echo "  → Linux ARM (Raspberry Pi 3/Zero)"
GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${BINARY_NAME}-linux-arm" .

# macOS AMD64
echo "  → macOS AMD64"
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${BINARY_NAME}-darwin-amd64" .

# macOS ARM64 (M1/M2)
echo "  → macOS ARM64"
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${BINARY_NAME}-darwin-arm64" .

echo ""
echo "Build complete! Binaries in $OUTPUT_DIR:"
ls -lh $OUTPUT_DIR

echo ""
echo "To copy to Raspberry Pi:"
echo "  scp $OUTPUT_DIR/${BINARY_NAME}-linux-arm64 pi@<IP>:~/"
echo ""
echo "To run on Raspberry Pi:"
echo "  chmod +x mesh-gateway-linux-arm64"
echo "  ./mesh-gateway-linux-arm64 --port /dev/ttyUSB0 --backend https://chnu-iot.com --debug"

