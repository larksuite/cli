// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/signingstore"
	"github.com/larksuite/cli/internal/vfs"
)

const softwareUnlockAccount = "dpop:software:unlock:v1"

func signerStorageDir() (string, error) { return signingstore.StorageDir() }

func unlockSecretLockDirectory(directory string) (string, error) {
	return signingstore.UnlockLockDirectory(directory, keysigner.SoftwareSignerName, "DPoP unlock lock directory")
}

// softwareSigner supplies DPoP's platform storage policy to software signing.
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
	directory, err := signerDirectory(s.Name())
	if err != nil {
		return nil, err
	}
	return keysigner.NewSoftwareSigner(directory, func(ctx context.Context) ([]byte, error) {
		return s.unlock(ctx, directory, allowCreate)
	})
}

func acquireUnlockSecretLock(ctx context.Context, directory string) (*flock.Flock, error) {
	lockDirectory, err := unlockSecretLockDirectory(directory)
	if err != nil {
		return nil, err
	}
	return signingstore.AcquireUnlockLock(ctx, lockDirectory)
}

func (s softwareSigner) unlock(ctx context.Context, directory string, allowCreate bool) (secret []byte, err error) {
	// Serialize initialization wherever the keychain shares this account.
	lock, err := acquireUnlockSecretLock(ctx, directory)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	encoded, err := s.keychain.Get(keychain.LarkCliService, softwareUnlockAccount)
	if ctx.Err() != nil {
		return nil, errors.Join(ctx.Err(), err)
	}
	// This account may protect keys in other directories (Windows HKCU).
	// An empty local directory cannot justify replacing an unreadable secret.
	if err != nil && !errors.Is(err, keychain.ErrNotFound) {
		return nil, err
	}
	return signingstore.ResolveSecret(s.keychain, directory, encoded, allowCreate, signingstore.SecretPolicy{
		Account:     softwareUnlockAccount,
		Description: "DPoP software unlock secret",
		IsKeyFile:   isSoftwareKeyFile,
		Missing:     missingSoftwareUnlockSecret,
	})
}

// resetUnrecoverableKeys removes software keys only after re-checking that the
// shared unlock secret is absent. Callers must limit this to explicit
// re-authorization because every removed key invalidates its existing binding.
func (s softwareSigner) resetUnrecoverableKeys(ctx context.Context) (removed bool, err error) {
	directory, err := signerDirectory(s.Name())
	if err != nil {
		return false, err
	}
	lock, err := acquireUnlockSecretLock(ctx, directory)
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	encoded, err := s.keychain.Get(keychain.LarkCliService, softwareUnlockAccount)
	if err != nil && !errors.Is(err, keychain.ErrNotFound) {
		return false, err
	}
	if encoded != "" {
		return false, nil
	}
	entries, err := vfs.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !isSoftwareKeyFile(entry.Name()) {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		info, err := vfs.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return removed, err
		}
		if !info.Mode().IsRegular() {
			return removed, fmt.Errorf("%w: software key path %q is not a regular file", keysigner.ErrCorrupt, path)
		}
		if err := vfs.Remove(path); err != nil {
			return removed, err
		}
		removed = true
	}
	return removed, nil
}

func isSoftwareKeyFile(name string) bool {
	digest := strings.TrimSuffix(name, ".json")
	if len(digest) != sha256.Size*2 || name == digest || digest != strings.ToLower(digest) {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func missingSoftwareUnlockSecret(directory string) error {
	hint := recovery.Join("", recovery.Command(
		recovery.TargetAuthLogin,
		"run `lark-cli auth login` to replace the unrecoverable local DPoP key and re-authorize",
	)).WithFallback(
		"replace the unrecoverable local DPoP key through this distribution's supported authorization flow",
	)
	return recovery.Attach(errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
		"DPoP software unlock secret is missing; encrypted key directory: %q", directory).
		WithCause(keysigner.ErrUnlockRequired), hint)
}
