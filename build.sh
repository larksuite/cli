#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
set -euo pipefail
cd "$(dirname "$0")"
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
go build -ldflags "-s -w -X github.com/larksuite/cli/internal/build.Version=${VERSION} -X github.com/larksuite/cli/internal/build.Date=$(date +%Y-%m-%d)" -o lark-cli .
echo "OK: ./lark-cli (${VERSION})"
