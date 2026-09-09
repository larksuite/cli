#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Reports whether <package>@<version> exists on <registry>: prints `exists` or
# `missing` and exits 0. Any other outcome — auth, network, an unexpected
# error — exits 1 with npm's message, because "could not tell" must never be
# read as "not published yet".
#
# Usage: .codebase/release/registry-version.sh <package> <version> <registry>
set -euo pipefail

PACKAGE="${1:?usage: $0 <package> <version> <registry>}"
VERSION="${2:?usage: $0 <package> <version> <registry>}"
REGISTRY="${3:?usage: $0 <package> <version> <registry>}"

cd "$(git rev-parse --show-toplevel)"

stderr_file="$(mktemp)"
trap 'rm -f -- "$stderr_file"' EXIT
if npm view "${PACKAGE}@${VERSION}" version --json --registry "$REGISTRY" >/dev/null 2>"$stderr_file"; then
  echo exists
  exit 0
fi
if node - "$stderr_file" <<'NODE'
const fs = require("node:fs");
const { isNpmVersionMissing } = require("./scripts/release-publish-policy");
process.exit(isNpmVersionMissing(fs.readFileSync(process.argv[2], "utf8")) ? 0 : 1);
NODE
then
  echo missing
  exit 0
fi
cat "$stderr_file" >&2
echo "Could not determine whether ${PACKAGE}@${VERSION} exists on ${REGISTRY}." >&2
exit 1
