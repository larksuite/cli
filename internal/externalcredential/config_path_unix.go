// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !darwin && !windows

package externalcredential

func defaultSystemConfigPath() string {
	return "/etc/lark-cli/external-credential.json"
}
