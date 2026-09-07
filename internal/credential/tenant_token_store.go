// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keychain"
)

const tenantAccessTokenAccountPrefix = "tat:v1:"

// TenantTokenStore owns persisted tenant access tokens. The account key hashes
// the opaque app ID so platform-specific filename/registry normalization never
// narrows or aliases the caller's identifier.
type TenantTokenStore struct {
	keychain func() keychain.KeychainAccess
}

// NewTenantTokenStore returns a store backed by the invocation's KeychainAccess.
func NewTenantTokenStore(kc func() keychain.KeychainAccess) *TenantTokenStore {
	if kc == nil {
		kc = keychain.Default
	}
	return &TenantTokenStore{keychain: kc}
}

func tenantAccessTokenAccountKey(appID string) string {
	sum := sha256.Sum256([]byte(appID))
	return tenantAccessTokenAccountPrefix + hex.EncodeToString(sum[:])
}

// Set stores a TAT for appID, replacing the previous value.
func (s *TenantTokenStore) Set(appID, token string) error {
	if appID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "app ID is required").WithParam("app_id")
	}
	if token == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "tenant access token is empty").WithParam("token")
	}
	kc := s.keychain()
	if kc == nil {
		return errs.NewInternalError(errs.SubtypeStorage, "tenant access token storage is unavailable")
	}
	if err := kc.Set(keychain.LarkCliService, tenantAccessTokenAccountKey(appID), token); err != nil {
		return tenantTokenStorageError("store", appID, err)
	}
	return nil
}

// Get returns the stored TAT and whether it exists.
func (s *TenantTokenStore) Get(appID string) (string, bool, error) {
	if appID == "" {
		return "", false, errs.NewConfigError(errs.SubtypeInvalidConfig, "tenant access token app ID is missing")
	}
	kc := s.keychain()
	if kc == nil {
		return "", false, errs.NewInternalError(errs.SubtypeStorage, "tenant access token storage is unavailable")
	}
	value, err := kc.Get(keychain.LarkCliService, tenantAccessTokenAccountKey(appID))
	if err != nil {
		return "", false, tenantTokenStorageError("read", appID, err)
	}
	if value == "" {
		return "", false, nil
	}
	return value, true, nil
}

// Remove removes the stored TAT. Removing an absent value is successful.
func (s *TenantTokenStore) Remove(appID string) error {
	if appID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "app ID is required").WithParam("app_id")
	}
	kc := s.keychain()
	if kc == nil {
		return errs.NewInternalError(errs.SubtypeStorage, "tenant access token storage is unavailable")
	}
	if err := kc.Remove(keychain.LarkCliService, tenantAccessTokenAccountKey(appID)); err != nil {
		return tenantTokenStorageError("remove", appID, err)
	}
	return nil
}

func tenantTokenStorageError(op, appID string, cause error) error {
	if errs.IsTyped(cause) {
		return cause
	}
	storageErr := errs.NewInternalError(errs.SubtypeStorage,
		"failed to %s tenant access token for app %s: %v", op, appID, cause).
		WithCause(cause)
	if problem, ok := errs.ProblemOf(cause); ok && problem.Hint != "" {
		storageErr.WithHint("%s", problem.Hint)
	}
	return storageErr
}
