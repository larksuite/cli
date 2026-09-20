// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"path/filepath"

	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/validate"
)

// RegistrationSigners orders new bindings from native L1/L2 backends to L3
// software storage. Construction does not access keys or the credential store.
func RegistrationSigners(kc keychain.KeychainAccess) []keysigner.Signer {
	return NewKeyStore(kc).signers
}

// ResolveSigner restores the exact built-in backend recorded in a profile.
// The legacy empty provider retains its native-backend selection.
func ResolveSigner(name string, kc keychain.KeychainAccess) (keysigner.Signer, error) {
	if name != keysigner.SoftwareSignerName {
		return keysigner.ResolvePlatformSigner(name, signerDirectory)
	}
	if kc == nil {
		kc = keychain.Default()
	}
	return softwareSigner{keychain: kc}, nil
}

func signerDirectory(backend string) (string, error) {
	directory, err := signerStorageDir()
	if err != nil {
		return "", err
	}
	return validate.SafeEnvDirPath(filepath.Join(directory, "keysigner", backend), "keyless signer directory")
}
