// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import "github.com/larksuite/cli/internal/vfs/localfileio"

// SafeOutputPath delegates to localfileio.SafeOutputPath.
func SafeOutputPath(path string) (string, error) {
	return localfileio.SafeOutputPath(path)
}
