// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/vfs"
)

const softwareUnlockAccount = "dpop:software:unlock:v1"

// softwareSigner supplies DPoP's platform storage policy to software signing.
type softwareSigner struct {
	keychain keychain.KeychainAccess
}

func (softwareSigner) Name() string                           { return keysigner.SoftwareSignerName }
func (softwareSigner) SecurityLevel() keysigner.SecurityLevel { return keysigner.SecurityLevelL3 }

func (s softwareSigner) EnsureKey(ctx context.Context, ref keysigner.KeyRef) (crypto.PublicKey, error) {
	signer, err := s.open(true)
	if err != nil {
		return nil, err
	}
	return signer.EnsureKey(ctx, ref)
}

func (s softwareSigner) PublicKey(ctx context.Context, ref keysigner.KeyRef) (crypto.PublicKey, error) {
	signer, err := s.open(false)
	if err != nil {
		return nil, err
	}
	return signer.PublicKey(ctx, ref)
}

func (s softwareSigner) Sign(ctx context.Context, ref keysigner.KeyRef, input []byte) ([]byte, string, error) {
	signer, err := s.open(false)
	if err != nil {
		return nil, "", err
	}
	return signer.Sign(ctx, ref, input)
}

func (s softwareSigner) DeleteKey(ctx context.Context, ref keysigner.KeyRef) error {
	signer, err := s.open(false)
	if err != nil {
		return err
	}
	return signer.DeleteKey(ctx, ref)
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
	if err := vfs.MkdirAll(lockDirectory, 0700); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(lockDirectory, "unlock.lock"))
	lockCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	locked, err := lock.TryLockContext(lockCtx, 10*time.Millisecond)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, lockCtx.Err()
	}
	return lock, nil
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
	if err != nil && !errors.Is(err, keychain.ErrNotFound) {
		return nil, err
	}
	if encoded != "" {
		secret, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			clear(secret)
			return nil, fmt.Errorf("%w: decode DPoP software unlock secret: %w", keysigner.ErrCorrupt, err)
		}
		if len(secret) != 32 {
			clear(secret)
			return nil, fmt.Errorf("%w: invalid DPoP software unlock secret", keysigner.ErrCorrupt)
		}
		return secret, nil
	}
	if !allowCreate {
		return nil, missingSoftwareUnlockSecret(directory)
	}
	// The key file writer creates the directory after the first successful unlock.
	entries, err := vfs.ReadDir(directory)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		// Losing the secret must not silently replace it while encrypted keys exist.
		if isSoftwareKeyFile(entry.Name()) {
			return nil, missingSoftwareUnlockSecret(directory)
		}
	}
	secret = make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		clear(secret)
		return nil, err
	}
	if err := s.keychain.Set(keychain.LarkCliService, softwareUnlockAccount, base64.StdEncoding.EncodeToString(secret)); err != nil {
		clear(secret)
		return nil, err
	}
	return secret, nil
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
