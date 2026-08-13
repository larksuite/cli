// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended && !windows

package externalcredential

func trustedCredentialProcessEnvironment() ([]string, error) {
	return []string{}, nil
}
