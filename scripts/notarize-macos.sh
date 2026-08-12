#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
app_path="$project_root/build/bin/NTerm.app"
archive_path="$project_root/build/bin/NTerm-notarization.zip"

if [ -z "${NTERM_NOTARY_PROFILE:-}" ]; then
  echo "Set NTERM_NOTARY_PROFILE to an xcrun notarytool Keychain profile." >&2
  exit 1
fi
if [ ! -d "$app_path" ]; then
  echo "Build NTerm.app before notarizing." >&2
  exit 1
fi

codesign --verify --deep --strict --verbose=2 "$app_path"
rm -f "$archive_path"
ditto -c -k --keepParent "$app_path" "$archive_path"
xcrun notarytool submit "$archive_path" --keychain-profile "$NTERM_NOTARY_PROFILE" --wait
xcrun stapler staple "$app_path"
xcrun stapler validate "$app_path"
echo "Notarized $app_path"
