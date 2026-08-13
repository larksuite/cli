#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

dist_dir="${1:-dist}"
version="${2:-}"

if [[ ! "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-beta\.(0|[1-9][0-9]*))?$ ]]; then
  echo "release asset verification requires an X.Y.Z or X.Y.Z-beta.N version" >&2
  exit 1
fi
if [[ ! -d "$dist_dir" ]]; then
  echo "release asset directory does not exist: $dist_dir" >&2
  exit 1
fi

platforms=(
  darwin-amd64.tar.gz
  darwin-arm64.tar.gz
  linux-amd64.tar.gz
  linux-arm64.tar.gz
  linux-riscv64.tar.gz
  windows-amd64.zip
  windows-arm64.zip
)
archives=()
for platform in "${platforms[@]}"; do
  archives+=(
    "lark-cli-${version}-${platform}"
    "lark-cli-extended-${version}-${platform}"
  )
done
checksummed_files=(
  "${archives[@]}"
  install-extended.sh
  install-extended.ps1
)

for required in checksums.txt install-extended.sh install-extended.ps1; do
  if [[ ! -s "$dist_dir/$required" ]]; then
    echo "required release asset is missing or empty: $required" >&2
    exit 1
  fi
done

expected_archives="$(printf '%s\n' "${archives[@]}" | LC_ALL=C sort)"
shopt -s nullglob
archive_paths=("$dist_dir"/lark-cli-*.tar.gz "$dist_dir"/lark-cli-*.zip)
shopt -u nullglob
actual_archives="$(
  for archive_path in "${archive_paths[@]}"; do
    basename "$archive_path"
  done | LC_ALL=C sort
)"
if [[ "$actual_archives" != "$expected_archives" ]]; then
  echo "release archive set does not match the supported platform matrix" >&2
  diff -u <(printf '%s\n' "$expected_archives") <(printf '%s\n' "$actual_archives") >&2 || true
  exit 1
fi

checksum_for() {
  local asset="$1"
  awk -v asset="$asset" '
    {
      name = $2
      sub(/^\*/, "", name)
      if (name == asset) {
        count++
        checksum = tolower($1)
      }
    }
    END {
      if (count != 1 || length(checksum) != 64 || checksum !~ /^[0-9a-f]+$/) {
        exit 1
      }
      print checksum
    }
  ' "$dist_dir/checksums.txt"
}

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print tolower($1)}'
    return
  fi
  shasum -a 256 "$file" | awk '{print tolower($1)}'
}

for asset in "${checksummed_files[@]}"; do
  if [[ ! -s "$dist_dir/$asset" ]]; then
    echo "checksummed release asset is missing or empty: $asset" >&2
    exit 1
  fi
  if ! expected="$(checksum_for "$asset")"; then
    echo "checksums.txt must contain exactly one valid SHA-256 for $asset" >&2
    exit 1
  fi
  actual="$(sha256_file "$dist_dir/$asset")"
  if [[ "$actual" != "$expected" ]]; then
    echo "release asset checksum mismatch: $asset" >&2
    exit 1
  fi
done

expected_checksum_names="$(printf '%s\n' "${checksummed_files[@]}" | LC_ALL=C sort)"
actual_checksum_names="$(
  awk '
    NF == 2 {
      name = $2
      sub(/^\*/, "", name)
      print name
    }
  ' "$dist_dir/checksums.txt" | LC_ALL=C sort
)"
if [[ "$actual_checksum_names" != "$expected_checksum_names" ]]; then
  echo "checksums.txt contains an unexpected release asset set" >&2
  diff -u <(printf '%s\n' "$expected_checksum_names") <(printf '%s\n' "$actual_checksum_names") >&2 || true
  exit 1
fi

expected_unix_members=$'CHANGELOG.md\nLICENSE\nREADME.md\nlark-cli'
expected_windows_members=$'CHANGELOG.md\nLICENSE\nREADME.md\nlark-cli.exe'
for archive in "${archives[@]}"; do
  if [[ "$archive" == *.zip ]]; then
    members="$(unzip -Z1 "$dist_dir/$archive" | LC_ALL=C sort)"
    expected_members="$expected_windows_members"
  else
    members="$(tar -tzf "$dist_dir/$archive" | LC_ALL=C sort)"
    expected_members="$expected_unix_members"
  fi
  if [[ "$members" != "$expected_members" ]]; then
    echo "release archive has unexpected members: $archive" >&2
    diff -u <(printf '%s\n' "$expected_members") <(printf '%s\n' "$members") >&2 || true
    exit 1
  fi
done

echo "Verified ${#archives[@]} release archives and ${#checksummed_files[@]} checksummed assets."
