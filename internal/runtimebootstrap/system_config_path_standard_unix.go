// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended && !darwin && !windows

package runtimebootstrap

func defaultStandardSystemConfigPath() string {
	return "/etc/lark-cli/external-credential.json"
}
