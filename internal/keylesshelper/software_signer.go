// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/signingstore"
)

const softwareUnlockAccount = "keyless:software:unlock:v1"

func signerStorageDir() (string, error) { return signingstore.StorageDir() }

func unlockSecretLockDirectory(directory string) (string, error) {
	return signingstore.UnlockLockDirectory(directory, filepath.Join("keyless", keysigner.SoftwareSignerName), "software unlock lock directory")
}

// softwareSigner manages the software backend's protection secret in the CLI credential store.
type softwareSigner struct {
	signingstore.SoftwareSigner
	keychain keychain.KeychainAccess
}

func newSoftwareSigner(kc keychain.KeychainAccess) softwareSigner {
	signer := softwareSigner{keychain: kc}
	signer.SoftwareSigner = signingstore.SoftwareSigner{Open: signer.open}
	return signer
}

func (s softwareSigner) open(allowCreate bool) (keysigner.Signer, error) {
	// Retain the existing PKJWT namespace: DPoP software keys use a different
	// unlock account. Sharing their directory would mix independently encrypted
	// key records and strand existing profiles.
	directory, err := signerDirectory(filepath.Join("keyless", s.Name()))
	if err != nil {
		return nil, err
	}
	return keysigner.NewSoftwareSigner(directory, func(ctx context.Context) ([]byte, error) {
		return s.unlock(ctx, directory, allowCreate)
	})
}

func (s softwareSigner) unlock(ctx context.Context, directory string, allowCreate bool) (secret []byte, err error) {
	// Serialize initialization wherever the keychain shares this account.
	lockDirectory, err := unlockSecretLockDirectory(directory)
	if err != nil {
		return nil, err
	}
	lock, err := signingstore.AcquireUnlockLock(ctx, lockDirectory)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	encoded, err := s.keychain.Get(keychain.LarkCliService, softwareUnlockAccount)
	if err != nil && !errors.Is(err, keychain.ErrNotFound) {
		return nil, err
	}
	return signingstore.ResolveSecret(s.keychain, directory, encoded, allowCreate, signingstore.SecretPolicy{
		Account:     softwareUnlockAccount,
		Description: "software unlock secret",
		IsKeyFile:   func(name string) bool { return strings.HasSuffix(name, ".json") },
		Missing:     func(string) error { return keysigner.ErrUnlockRequired },
	})
}
