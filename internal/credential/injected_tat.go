// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"sync"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/keychain"
)

const injectedTATKeyPrefix = "tat:"

func injectedTATAccountKey(appID string) string {
	return injectedTATKeyPrefix + appID
}

// IsSafeInjectedTenantAppID reports whether appID survives every platform
// keychain backend without filename/registry normalization. Lowercase-only
// input also avoids collisions on case-insensitive filesystems and registries.
func IsSafeInjectedTenantAppID(appID string) bool {
	if appID == "" {
		return false
	}
	for _, r := range appID {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// StoreInjectedTenantAccessToken persists a caller-supplied TAT under the
// app-scoped key used by NewInjectedTenantTokenProvider.
func StoreInjectedTenantAccessToken(kc keychain.KeychainAccess, appID, token string) error {
	if !IsSafeInjectedTenantAppID(appID) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"injected tenant token app ID must use only lowercase letters, digits, '.', '_', or '-'").
			WithParam("app_id")
	}
	if kc == nil {
		return errs.NewInternalError(errs.SubtypeStorage, "injected tenant token storage is unavailable")
	}
	if err := kc.Set(keychain.LarkCliService, injectedTATAccountKey(appID), token); err != nil {
		return injectedTATStorageError("store", appID, err)
	}
	return nil
}

type injectedTATCacheEntry struct {
	value string
	found bool
	err   error
}

// InjectedTenantTokenProvider resolves app-scoped TATs from the injected
// KeychainAccess. Cache state is owned by one Factory/provider assembly, not by
// the process-global extension registry.
type InjectedTenantTokenProvider struct {
	keychain func() keychain.KeychainAccess

	mu    sync.Mutex
	cache map[string]injectedTATCacheEntry
}

// NewInjectedTenantTokenProvider creates a token-only extension provider. The
// keychain closure is evaluated on first lookup so Factory.Keychain remains
// replaceable after Factory construction in tests and embeddings.
func NewInjectedTenantTokenProvider(kc func() keychain.KeychainAccess) *InjectedTenantTokenProvider {
	if kc == nil {
		kc = keychain.Default
	}
	return &InjectedTenantTokenProvider{
		keychain: kc,
		cache:    make(map[string]injectedTATCacheEntry),
	}
}

func (p *InjectedTenantTokenProvider) Name() string { return "injected-tat" }

func (p *InjectedTenantTokenProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return nil, nil
}

func (p *InjectedTenantTokenProvider) ResolveToken(_ context.Context, req extcred.TokenSpec) (*extcred.Token, error) {
	if req.Type != extcred.TokenTypeTAT || req.AppID == "" {
		return nil, nil
	}
	if !IsSafeInjectedTenantAppID(req.AppID) {
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"injected tenant token app ID %q contains unsupported characters", req.AppID)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if cached, ok := p.cache[req.AppID]; ok {
		return injectedTATToken(cached, req.AppID)
	}

	kc := p.keychain()
	if kc == nil {
		entry := injectedTATCacheEntry{err: errs.NewInternalError(errs.SubtypeStorage, "injected tenant token storage is unavailable")}
		p.cache[req.AppID] = entry
		return nil, entry.err
	}
	value, err := kc.Get(keychain.LarkCliService, injectedTATAccountKey(req.AppID))
	entry := injectedTATCacheEntry{value: value, found: value != ""}
	if err != nil {
		entry.err = injectedTATStorageError("read", req.AppID, err)
	}
	p.cache[req.AppID] = entry
	return injectedTATToken(entry, req.AppID)
}

func injectedTATToken(entry injectedTATCacheEntry, appID string) (*extcred.Token, error) {
	if entry.err != nil {
		return nil, entry.err
	}
	if !entry.found {
		return nil, nil
	}
	return &extcred.Token{
		Value:  entry.value,
		Source: "keychain:" + injectedTATAccountKey(appID),
	}, nil
}

func injectedTATStorageError(op, appID string, cause error) error {
	storageErr := errs.NewInternalError(errs.SubtypeStorage,
		"failed to %s injected tenant token for app %s: %v", op, appID, cause).
		WithCause(cause)
	if problem, ok := errs.ProblemOf(cause); ok && problem.Hint != "" {
		storageErr.WithHint("%s", problem.Hint)
	}
	return storageErr
}

var _ extcred.Provider = (*InjectedTenantTokenProvider)(nil)
