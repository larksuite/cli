#!/bin/sh
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -eu

repo="larksuite/cli"
install_dir="${LARK_CLI_INSTALL_DIR:-$HOME/.local/bin}"
version="${LARK_CLI_VERSION:-}"

command -v curl >/dev/null 2>&1 || {
  echo "curl is required to install lark-cli Extended" >&2
  exit 1
}

case "$(uname -s)" in
  Darwin) platform="darwin" ;;
  Linux) platform="linux" ;;
  *)
    echo "Unsupported operating system: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  riscv64) arch="riscv64" ;;
  *)
    echo "Unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

if [ -z "$version" ]; then
  latest_url="$(curl --fail --location --silent --show-error \
    --proto '=https' --proto-redir '=https' \
    --connect-timeout 10 --max-time 60 --max-redirs 5 \
    --output /dev/null --write-out '%{url_effective}' \
    "https://github.com/$repo/releases/latest")"
  version="${latest_url##*/}"
  version="${version#v}"
fi

case "$version" in
  ""|*[!0-9A-Za-z.+-]*)
    echo "Invalid lark-cli version: $version" >&2
    exit 1
    ;;
esac

archive="lark-cli-extended-$version-$platform-$arch.tar.gz"
base="https://github.com/$repo/releases/download/v$version"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/lark-cli-extended.XXXXXX")"
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

curl --fail --location --silent --show-error \
  --proto '=https' --proto-redir '=https' \
  --connect-timeout 10 --max-time 120 --max-redirs 5 \
  --output "$tmp_dir/checksums.txt" "$base/checksums.txt"
curl --fail --location --silent --show-error \
  --proto '=https' --proto-redir '=https' \
  --connect-timeout 10 --max-time 300 --max-redirs 5 \
  --output "$tmp_dir/$archive" "$base/$archive"

expected="$(awk -v asset="$archive" '$2 == asset || $2 == "*" asset { print $1 }' "$tmp_dir/checksums.txt")"
if [ -z "$expected" ]; then
  echo "Checksum entry not found for $archive" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp_dir/$archive" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$tmp_dir/$archive" | awk '{print $1}')"
fi
if [ "$actual" != "$expected" ]; then
  echo "Checksum verification failed for $archive" >&2
  exit 1
fi

tar -xzf "$tmp_dir/$archive" -C "$tmp_dir" lark-cli
metadata="$("$tmp_dir/lark-cli" version --json)"
printf '%s\n' "$metadata" | grep -Fq '"edition": "extended"' || {
  echo "Downloaded binary is not lark-cli Extended" >&2
  exit 1
}
printf '%s\n' "$metadata" | grep -Fq "\"version\": \"$version\"" || {
  echo "Downloaded binary is not lark-cli Extended $version" >&2
  exit 1
}

mkdir -p "$install_dir"
install -m 0755 "$tmp_dir/lark-cli" "$install_dir/lark-cli"
echo "lark-cli Extended $version installed at $install_dir/lark-cli"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) echo "Add $install_dir to PATH to run lark-cli." ;;
esac
