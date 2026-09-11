// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"context"
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
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

func (s softwareSigner) unlock(ctx context.Context, directory string, allowCreate bool) (secret []byte, err error) {
	// Serialize initialization wherever the keychain shares this account.
	lockDirectory, err := softwareUnlockLockDir(directory)
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
		return nil, keysigner.ErrUnlockRequired
	}
	entries, err := vfs.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		// Losing the secret must not silently replace it while encrypted keys exist.
		if strings.HasSuffix(entry.Name(), ".json") {
			return nil, keysigner.ErrUnlockRequired
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
