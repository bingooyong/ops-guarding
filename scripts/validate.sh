#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}"

mkdir -p "$GOCACHE" "$GOMODCACHE" "$ROOT_DIR/bin"

echo "validation: go test ./..."
go test ./...

echo "validation: go build -o bin/guarding ./cmd"
go build -o "$ROOT_DIR/bin/guarding" ./cmd

echo "validation: PASS"
