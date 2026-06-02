// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import "github.com/larksuite/cli/internal/vfs/localfileio"

// SafeOutputPath validates a download/export target path.
// Delegates to localfileio.SafeOutputPath.
func SafeOutputPath(path string) (string, error) {
	return localfileio.SafeOutputPath(path)
}

// SafeInputPath validates an upload/read source path.
// Delegates to localfileio.SafeInputPath.
func SafeInputPath(path string) (string, error) {
	return localfileio.SafeInputPath(path)
}

// SafeEnvDirPath validates an environment-provided application directory path.
// Delegates to localfileio.SafeEnvDirPath.
func SafeEnvDirPath(path, envName string) (string, error) {
	return localfileio.SafeEnvDirPath(path, envName)
}

// SafeLocalFlagPath validates a flag value as a local file path.
// Delegates to localfileio.SafeLocalFlagPath.
func SafeLocalFlagPath(flagName, value string) (string, error) {
	return localfileio.SafeLocalFlagPath(flagName, value)
}

// SafeAbsoluteInputPath validates an absolute path for safety (control
// characters, symlink resolution) without restricting to the working directory.
func SafeAbsoluteInputPath(path string) error {
	_, err := localfileio.SafeAbsoluteInputPath(path)
	return err
}
