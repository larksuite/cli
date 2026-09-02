// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keychain"
)

type tenantTokenStoreKeychain struct {
	values      map[string]string
	getErr      error
	setErr      error
	removeErr   error
	getCalls    []string
	setCalls    []string
	removeCalls []string
}

func (k *tenantTokenStoreKeychain) Get(service, account string) (string, error) {
	k.getCalls = append(k.getCalls, service+"/"+account)
	if k.getErr != nil {
		return "", k.getErr
	}
	return k.values[service+"/"+account], nil
}

func (k *tenantTokenStoreKeychain) Set(service, account, value string) error {
	k.setCalls = append(k.setCalls, service+"/"+account)
	if k.setErr != nil {
		return k.setErr
	}
	if k.values == nil {
		k.values = make(map[string]string)
	}
	k.values[service+"/"+account] = value
	return nil
}

func (k *tenantTokenStoreKeychain) Remove(service, account string) error {
	k.removeCalls = append(k.removeCalls, service+"/"+account)
	if k.removeErr != nil {
		return k.removeErr
	}
	delete(k.values, service+"/"+account)
	return nil
}

func TestTenantTokenStoreSetGetRemove(t *testing.T) {
	kc := &tenantTokenStoreKeychain{}
	store := NewTenantTokenStore(func() keychain.KeychainAccess { return kc })
	appID := "cli_Test/opaque"

	if err := store.Set(appID, "token-v1"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Set(appID, "token-v2"); err != nil {
		t.Fatalf("replacement Set() error = %v", err)
	}
	if len(kc.setCalls) != 2 || kc.setCalls[0] != kc.setCalls[1] {
		t.Fatalf("Set calls = %v, want one stable account key", kc.setCalls)
	}
	account := strings.TrimPrefix(kc.setCalls[0], keychain.LarkCliService+"/")
	if !strings.HasPrefix(account, tenantAccessTokenAccountPrefix) || strings.Contains(account, appID) {
		t.Fatalf("account key = %q, want versioned opaque key", account)
	}
	if got, found, err := store.Get(appID); err != nil || !found || got != "token-v2" {
		t.Fatalf("Get() = (%q, %v, %v), want token-v2, true, nil", got, found, err)
	}
	if err := store.Remove(appID); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if got, found, err := store.Get(appID); err != nil || found || got != "" {
		t.Fatalf("Get() after Remove = (%q, %v, %v), want empty, false, nil", got, found, err)
	}
}

func TestTenantTokenStoreKeysDoNotAlias(t *testing.T) {
	left := tenantAccessTokenAccountKey("cli/a")
	right := tenantAccessTokenAccountKey("cli_a")
	caseVariant := tenantAccessTokenAccountKey("CLI/A")
	if left == right || left == caseVariant || right == caseVariant {
		t.Fatalf("account keys alias: left=%q right=%q case=%q", left, right, caseVariant)
	}
}

func TestTenantTokenStorePreservesStorageErrors(t *testing.T) {
	sentinel := errors.New("keychain unavailable")
	kc := &tenantTokenStoreKeychain{getErr: sentinel, setErr: sentinel, removeErr: sentinel}
	store := NewTenantTokenStore(func() keychain.KeychainAccess { return kc })

	for name, run := range map[string]func() error{
		"set":    func() error { return store.Set("cli_test", "token") },
		"get":    func() error { _, _, err := store.Get("cli_test"); return err },
		"remove": func() error { return store.Remove("cli_test") },
	} {
		t.Run(name, func(t *testing.T) {
			err := run()
			var internalErr *errs.InternalError
			if !errors.As(err, &internalErr) || internalErr.Subtype != errs.SubtypeStorage || !errors.Is(err, sentinel) {
				t.Fatalf("error = %T %v, want internal/storage preserving cause", err, err)
			}
		})
	}
}

func TestTenantTokenStorePassesTypedKeychainErrorThrough(t *testing.T) {
	typed := errs.NewAPIError(errs.SubtypeUnknown, "keychain locked")
	kc := &tenantTokenStoreKeychain{setErr: typed}
	store := NewTenantTokenStore(func() keychain.KeychainAccess { return kc })
	if err := store.Set("cli_test", "token"); err != typed {
		t.Fatalf("Set() error = %T %v, want original typed error", err, err)
	}
}

var _ keychain.KeychainAccess = (*tenantTokenStoreKeychain)(nil)
