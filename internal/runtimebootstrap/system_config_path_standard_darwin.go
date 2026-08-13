// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended && darwin

package runtimebootstrap

func defaultStandardSystemConfigPath() string {
	return "/Library/Application Support/lark-cli/external-credential.json"
}
