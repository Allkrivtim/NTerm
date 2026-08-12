#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
app_path="$project_root/build/bin/NTerm.app"
source_directory="$project_root/build/ai/darwin-arm64"
resource_directory="$app_path/Contents/Resources/ai"
runtime_manifest="$project_root/scripts/llama-b9637-darwin-arm64.sha256"
model_sha="20693aeb02c63304e263a72453b6ab89e1c700a87c6948cac523ac1e6f7cade0"
codesign_identity=${NTERM_CODESIGN_IDENTITY:--}

if [ ! -d "$app_path" ]; then
  echo "NTerm.app was not found at $app_path" >&2
  exit 1
fi

(cd "$source_directory" && shasum -a 256 -c "$runtime_manifest")
echo "$model_sha  $source_directory/qwen2.5-coder-0.5b-q4km.gguf" | shasum -a 256 -c -

rm -rf "$resource_directory" "$app_path/Contents/Resources/licenses"
mkdir -p "$resource_directory"
while read -r checksum filename; do
  cp "$source_directory/$filename" "$resource_directory/$filename"
done < "$runtime_manifest"
cp -P \
  "$source_directory/libllama-common.0.dylib" \
  "$source_directory/libllama.0.dylib" \
  "$source_directory/libmtmd.0.dylib" \
  "$source_directory/libggml.0.dylib" \
  "$source_directory/libggml-base.0.dylib" \
  "$source_directory/libggml-blas.0.dylib" \
  "$source_directory/libggml-cpu.0.dylib" \
  "$source_directory/libggml-metal.0.dylib" \
  "$source_directory/libggml-rpc.0.dylib" \
  "$resource_directory/"
cp "$source_directory/qwen2.5-coder-0.5b-q4km.gguf" "$resource_directory/qwen2.5-coder-0.5b-q4km.gguf"
chmod 755 "$resource_directory/llama-server"

cp "$project_root/THIRD_PARTY_NOTICES.md" "$app_path/Contents/Resources/THIRD_PARTY_NOTICES.md"
mkdir -p "$app_path/Contents/Resources/licenses"
cp "$project_root/licenses/"* "$app_path/Contents/Resources/licenses/"
# Wails signs before post-build hooks, so seal the final bundle after every
# runtime, model and notice has reached its final location. A Developer ID can
# be supplied for distribution; local builds remain ad-hoc signed.
if [ "$codesign_identity" = "-" ]; then
  find "$resource_directory" -type f \( -name '*.dylib' -o -name 'llama-server' \) -exec codesign --force --sign - {} \;
  codesign --force --sign - "$app_path"
else
  find "$resource_directory" -type f \( -name '*.dylib' -o -name 'llama-server' \) \
    -exec codesign --force --timestamp --options runtime --sign "$codesign_identity" {} \;
  codesign --force --timestamp --options runtime --sign "$codesign_identity" "$app_path"
fi
echo "Packaged built-in AI runtime and model into $app_path"
