#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$project_root"

go run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 build
