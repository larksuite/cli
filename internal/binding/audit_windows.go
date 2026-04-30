// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package binding

// checkOwnerUID is a no-op on Windows where Unix UID semantics don't apply.
func checkOwnerUID(path, label string) error {
	return nil
}
