// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"context"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
)

// CleanupUnreferencedSigningKey removes a CLI-managed key after its configuration
// has been saved or removed. User-owned PEM files are retained.
func CleanupUnreferencedSigningKey(ctx context.Context, kc keychain.KeychainAccess, previous core.AppConfig, current *core.MultiAppConfig) error {
	if previous.AuthMethod != core.AuthMethodPrivateKeyJWTLocalKeyPair {
		return nil
	}
	ref := previous.KeyRef
	if ref == nil || ref.Source != core.KeySourceTEE || ref.ID == "" {
		return nil
	}
	if ref.Provider != keysigner.SoftwareSignerName && !keysigner.IsPlatformSignerName(ref.Provider) {
		return nil
	}
	if current != nil {
		for _, app := range current.Apps {
			bound := app.KeyRef
			if bound != nil && *bound == *ref {
				return nil
			}
		}
	}
	signer, err := ResolveSigner(ref.Provider, kc)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return deleteKeyContext(ctx, signer, ref.ID)
}
