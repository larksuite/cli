// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"context"
	"errors"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
)

// CleanupUnreferencedSigningKey removes a CLI-managed key after its configuration
// has been saved or removed. User-owned files and external signer keys are retained.
func CleanupUnreferencedSigningKey(ctx context.Context, kc keychain.KeychainAccess, previous core.AppConfig, current *core.MultiAppConfig) error {
	if !core.IsPrivateKeyJWTLocalKeyPairAuthMethod(previous.AuthMethod) {
		return nil
	}
	ref := previous.KeyRef
	if ref == nil || ref.Source != core.SecretSourceTEE || ref.ID == "" ||
		(ref.Provider != "" && ref.Provider != keysigner.SoftwareSignerName && !keysigner.IsPlatformSignerName(ref.Provider)) {
		return nil
	}
	signer, err := ResolveSigner(ref.Provider, kc)
	if err != nil {
		return err
	}
	if current != nil {
		for _, app := range current.Apps {
			bound := app.KeyRef
			if bound == nil || bound.Source != core.SecretSourceTEE || bound.ID != ref.ID {
				continue
			}
			if bound.Provider == ref.Provider || bound.Provider == signer.Name() {
				return nil
			}
			if bound.Provider == "" {
				legacy, err := ResolveSigner("", kc)
				if err != nil {
					return err
				}
				if legacy.Name() == signer.Name() {
					return nil
				}
			}
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	store := NewKeyStoreWithSigners(kc, signer)
	key, err := store.LoadContext(ctx, signer.Name(), ref.ID)
	if errors.Is(err, ErrKeyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return store.DeleteKeyContext(ctx, key)
}
