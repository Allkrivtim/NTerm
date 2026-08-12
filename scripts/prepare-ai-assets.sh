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
runtime_manifest="$project_root/scripts/llama-b9637-darwin-arm64.sha256"
model_sha="20693aeb02c63304e263a72453b6ab89e1c700a87c6948cac523ac1e6f7cade0"
mkdir -p "$asset_directory"

runtime_is_ready() {
  [ -x "$runtime_path" ] || return 1
  (cd "$asset_directory" && shasum -a 256 -c "$runtime_manifest" >/dev/null 2>&1) || return 1
  [ "$(readlink "$asset_directory/libllama-common.0.dylib")" = "libllama-common.0.0.9637.dylib" ] || return 1
  [ "$(readlink "$asset_directory/libllama.0.dylib")" = "libllama.0.0.9637.dylib" ] || return 1
  [ "$(readlink "$asset_directory/libmtmd.0.dylib")" = "libmtmd.0.0.9637.dylib" ] || return 1
  [ "$(readlink "$asset_directory/libggml.0.dylib")" = "libggml.0.15.1.dylib" ] || return 1
  [ "$(readlink "$asset_directory/libggml-base.0.dylib")" = "libggml-base.0.15.1.dylib" ] || return 1
  [ "$(readlink "$asset_directory/libggml-blas.0.dylib")" = "libggml-blas.0.15.1.dylib" ] || return 1
  [ "$(readlink "$asset_directory/libggml-cpu.0.dylib")" = "libggml-cpu.0.15.1.dylib" ] || return 1
  [ "$(readlink "$asset_directory/libggml-metal.0.dylib")" = "libggml-metal.0.15.1.dylib" ] || return 1
  [ "$(readlink "$asset_directory/libggml-rpc.0.dylib")" = "libggml-rpc.0.15.1.dylib" ] || return 1
  "$runtime_path" --version >/dev/null 2>&1
}

if ! runtime_is_ready; then
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
  runtime_source=$(dirname "$found_runtime")
  cp "$found_runtime" "$runtime_path"
  cp -P "$runtime_source"/libggml*.dylib "$asset_directory/"
  cp -P "$runtime_source"/libllama-server-impl.dylib "$asset_directory/"
  cp -P "$runtime_source"/libllama-common.0.dylib "$runtime_source"/libllama-common.0.0.9637.dylib "$asset_directory/"
  cp -P "$runtime_source"/libllama.0.dylib "$runtime_source"/libllama.0.0.9637.dylib "$asset_directory/"
  cp -P "$runtime_source"/libmtmd.0.dylib "$runtime_source"/libmtmd.0.0.9637.dylib "$asset_directory/"
  chmod 755 "$runtime_path"
  runtime_is_ready
fi

if ! echo "$model_sha  $model_path" | shasum -a 256 -c - >/dev/null 2>&1; then
  ollama_cache="$HOME/.ollama/models/blobs/sha256-$model_sha"
  if [ -f "$ollama_cache" ]; then
    cp "$ollama_cache" "$model_path"
  else
    curl -fL --retry 3 \
      "https://registry.ollama.ai/v2/library/qwen2.5-coder/blobs/sha256:$model_sha" \
      -o "$model_path"
  fi
fi

(cd "$asset_directory" && shasum -a 256 -c "$runtime_manifest")
echo "$model_sha  $model_path" | shasum -a 256 -c -
echo "Built-in AI assets are ready in $asset_directory"
