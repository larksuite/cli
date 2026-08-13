// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin

package externalcredential

func defaultSystemConfigPath() string {
	return "/Library/Application Support/lark-cli/external-credential.json"
}
