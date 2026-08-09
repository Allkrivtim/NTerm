#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
app_path="$project_root/build/bin/NTerm.app"
source_directory="$project_root/build/ai/darwin-arm64"
resource_directory="$app_path/Contents/Resources/ai"

if [ ! -d "$app_path" ]; then
  echo "NTerm.app was not found at $app_path" >&2
  exit 1
fi

mkdir -p "$resource_directory"
cp "$source_directory/llama-server" "$resource_directory/llama-server"
cp "$source_directory/qwen2.5-coder-0.5b-q4km.gguf" "$resource_directory/qwen2.5-coder-0.5b-q4km.gguf"
chmod 755 "$resource_directory/llama-server"

# Wails signs before post-build hooks, so seal the bundle after adding resources.
codesign --force --deep --sign - "$app_path"
echo "Packaged built-in AI runtime and model into $app_path"
