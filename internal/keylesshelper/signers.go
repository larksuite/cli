// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"path/filepath"
	"runtime"

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
		if name == "" && runtime.GOOS == "darwin" {
			name = keysigner.MacOSKeychainSignerName
		}
		return keysigner.ResolvePlatformSigner(name, signerDirectory)
	}
	if kc == nil {
		kc = keychain.Default()
	}
	return newSoftwareSigner(kc), nil
}

// Private-key JWT retains its dedicated macOS Keychain backend. DPoP's
// platform preference (including Secure Enclave) is intentionally independent.
func registrationPlatformSigners() []keysigner.Signer {
	signers := keysigner.NewPlatformSigners(signerDirectory)
	if runtime.GOOS != "darwin" {
		return signers
	}
	var allowed []keysigner.Signer
	for _, signer := range signers {
		if signer.Name() == keysigner.MacOSKeychainSignerName {
			allowed = append(allowed, signer)
		}
	}
	return allowed
}

func signerDirectory(backend string) (string, error) {
	directory, err := signerStorageDir()
	if err != nil {
		return "", err
	}
	return validate.SafeEnvDirPath(filepath.Join(directory, "keysigner", backend), "keyless signer directory")
}
