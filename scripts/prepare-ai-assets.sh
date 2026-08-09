#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
platform=$(uname -s)
architecture=$(uname -m)

if [ "$platform" != "Darwin" ] || [ "$architecture" != "arm64" ]; then
  echo "Built-in AI packaging currently supports macOS arm64 only." >&2
  exit 1
fi

asset_directory="$project_root/build/ai/darwin-arm64"
runtime_path="$asset_directory/llama-server"
model_path="$asset_directory/qwen2.5-coder-0.5b-q4km.gguf"
model_sha="20693aeb02c63304e263a72453b6ab89e1c700a87c6948cac523ac1e6f7cade0"
mkdir -p "$asset_directory"

if [ ! -x "$runtime_path" ]; then
  homebrew_runtime="/opt/homebrew/Cellar/ollama/0.31.1/libexec/lib/ollama/llama-server"
  if [ -x "$homebrew_runtime" ]; then
    cp "$homebrew_runtime" "$runtime_path"
  else
    temporary_directory=$(mktemp -d)
    trap 'rm -rf "$temporary_directory"' EXIT INT TERM
    archive="$temporary_directory/llama.tar.gz"
    curl -fL --retry 3 \
      "https://github.com/ggml-org/llama.cpp/releases/download/b9637/llama-b9637-bin-macos-arm64.tar.gz" \
      -o "$archive"
    echo "72a93f3e68c31de3e438d462669aad1fcdb423b995e9c41033cc7d27a9a3ac69  $archive" | shasum -a 256 -c -
    tar -xzf "$archive" -C "$temporary_directory"
    found_runtime=$(find "$temporary_directory" -type f -name llama-server -print -quit)
    if [ -z "$found_runtime" ]; then
      echo "llama-server was not found in the release archive." >&2
      exit 1
    fi
    cp "$found_runtime" "$runtime_path"
  fi
  chmod 755 "$runtime_path"
fi

if [ ! -f "$model_path" ]; then
  ollama_cache="$HOME/.ollama/models/blobs/sha256-$model_sha"
  if [ -f "$ollama_cache" ]; then
    cp "$ollama_cache" "$model_path"
  else
    curl -fL --retry 3 \
      "https://registry.ollama.ai/v2/library/qwen2.5-coder/blobs/sha256:$model_sha" \
      -o "$model_path"
  fi
fi

echo "$model_sha  $model_path" | shasum -a 256 -c -
echo "Built-in AI assets are ready in $asset_directory"
