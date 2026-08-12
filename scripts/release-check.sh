#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$project_root"

export GOTOOLCHAIN=go1.26.5
go test -race -count=1 ./...
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
node --check frontend/dist/app.js
sh -n scripts/build-macos.sh scripts/prepare-ai-assets.sh scripts/package-ai-assets.sh
./scripts/build-macos.sh
codesign --verify --deep --strict --verbose=2 build/bin/NTerm.app
build/bin/NTerm.app/Contents/Resources/ai/llama-server --version

echo "Release checks passed for build/bin/NTerm.app"
