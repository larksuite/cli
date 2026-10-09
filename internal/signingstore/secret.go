// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signingstore

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/vfs"
)

// AcquireUnlockLock serializes initialization at the domain's resolved secret scope.
func AcquireUnlockLock(ctx context.Context, lockDirectory string) (*flock.Flock, error) {
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

// SecretPolicy retains domain-owned namespace, file detection and recovery errors.
// Retrieval and cancellation priority remain at the caller, under the unlock lock.
type SecretPolicy struct {
	Account     string
	Description string
	IsKeyFile   func(string) bool
	Missing     func(directory string) error
}

// ResolveSecret decodes existing protection data or initializes it only when the
// caller permits creation and no encrypted key record would be stranded.
// The caller holds the unlock lock and has already classified keychain Get.
func ResolveSecret(kc keychain.KeychainAccess, directory, encoded string, allowCreate bool, policy SecretPolicy) ([]byte, error) {
	if encoded != "" {
		secret, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			clear(secret)
			return nil, fmt.Errorf("%w: decode %s: %w", keysigner.ErrCorrupt, policy.Description, err)
		}
		if len(secret) != 32 {
			clear(secret)
			return nil, fmt.Errorf("%w: invalid %s", keysigner.ErrCorrupt, policy.Description)
		}
		return secret, nil
	}
	if !allowCreate {
		return nil, policy.Missing(directory)
	}
	// The key file writer creates the directory after the first successful unlock.
	entries, err := vfs.ReadDir(directory)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		// Losing the secret must not silently replace it while encrypted keys exist.
		if policy.IsKeyFile(entry.Name()) {
			return nil, policy.Missing(directory)
		}
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		clear(secret)
		return nil, err
	}
	if err := kc.Set(keychain.LarkCliService, policy.Account, base64.StdEncoding.EncodeToString(secret)); err != nil {
		clear(secret)
		return nil, err
	}
	return secret, nil
}
