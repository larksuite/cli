// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import "github.com/larksuite/cli/internal/vfs/localfileio"

// SafeOutputPath delegates to localfileio.SafeOutputPath.
func SafeOutputPath(path string) (string, error) {
	return localfileio.SafeOutputPath(path)
}

// SafeInputPath delegates to localfileio.SafeInputPath.
func SafeInputPath(path string) (string, error) {
	return localfileio.SafeInputPath(path)
}

// SafeLocalFlagPath delegates to localfileio.SafeLocalFlagPath.
func SafeLocalFlagPath(flagName, value string) (string, error) {
	return localfileio.SafeLocalFlagPath(flagName, value)
}
