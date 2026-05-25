// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !darwin

package keychain

// extraHint is a no-op on non-darwin platforms. The keychain-downgrade
// command is macOS-only, so there is no extra suggestion to surface.
func extraHint(err error) string { return "" }
