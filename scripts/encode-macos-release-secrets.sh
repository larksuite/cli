#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

# Fill in the two absolute paths before running this script.
P12_PATH="${P12_PATH:-/absolute/path/to/DeveloperIDApplication.p12}"
P8_PATH="${P8_PATH:-/absolute/path/to/AuthKey_XXXXXXXXXX.p8}"
OUTPUT_FILE="${1:-${TMPDIR:-/tmp}/macos-release-secrets.txt}"

for path in "$P12_PATH" "$P8_PATH"; do
  [[ -f "$path" && ! -L "$path" ]] || {
    echo "Set P12_PATH and P8_PATH to regular files before running this script." >&2
    exit 1
  }
done

grep -Fxq -- '-----BEGIN PRIVATE KEY-----' "$P8_PATH" || {
  echo "P8_PATH does not point to an Apple private-key P8 file." >&2
  exit 1
}

if [[ -e "$OUTPUT_FILE" ]]; then
  echo "Refusing to overwrite existing output: $OUTPUT_FILE" >&2
  exit 1
fi

read -r -s -p "P12 password: " P12_PASSWORD
printf '\n' >&2
[[ -n "$P12_PASSWORD" ]] || { echo "P12 password must not be empty." >&2; exit 1; }

umask 077
{
  printf 'MACOS_SIGN_P12=%s\n' "$(base64 < "$P12_PATH" | tr -d '\n')"
  printf 'MACOS_SIGN_PASSWORD=%s\n' "$P12_PASSWORD"
  printf 'MACOS_NOTARY_KEY=%s\n' "$(base64 < "$P8_PATH" | tr -d '\n')"
} > "$OUTPUT_FILE"

echo "Wrote $(basename "$OUTPUT_FILE") with mode 600. Copy each value after '=' into its GitHub Environment Secret." >&2
