// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"os"
	"path/filepath"

	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/validate"
)

func signerStorageDir() (string, error) {
	// Private keys use the shared encrypted-file signer. Only its unlock
	// secret is stored by the Windows keychain backend (DPAPI + HKCU).
	if directory := os.Getenv("LARKSUITE_CLI_DATA_DIR"); directory != "" {
		directory, err := validate.SafeEnvDirPath(directory, "LARKSUITE_CLI_DATA_DIR")
		if err != nil {
			return "", err
		}
		return filepath.Join(directory, keychain.LarkCliService), nil
	}
	directory, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, keychain.LarkCliService), nil
}

func softwareUnlockLockDir(_ string) (string, error) {
	// HKCU shares one secret even when callers override the private-key directory.
	directory, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return validate.SafeEnvDirPath(filepath.Join(directory, keychain.LarkCliService, "keysigner", keysigner.SoftwareSignerName), "DPoP unlock lock directory")
}
