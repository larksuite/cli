// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/keychain"
)

type injectedTATTestKeychain struct {
	values   map[string]string
	getErr   error
	setErr   error
	getCalls []string
	setCalls []injectedTATSetCall
}

type injectedTATSetCall struct {
	service string
	account string
	value   string
}

func (k *injectedTATTestKeychain) Get(service, account string) (string, error) {
	k.getCalls = append(k.getCalls, service+"/"+account)
	if k.getErr != nil {
		return "", k.getErr
	}
	return k.values[service+"/"+account], nil
}

func (k *injectedTATTestKeychain) Set(service, account, value string) error {
	k.setCalls = append(k.setCalls, injectedTATSetCall{service: service, account: account, value: value})
	if k.setErr != nil {
		return k.setErr
	}
	if k.values == nil {
		k.values = make(map[string]string)
	}
	k.values[service+"/"+account] = value
	return nil
}

func (k *injectedTATTestKeychain) Remove(string, string) error { return nil }

func TestStoreInjectedTenantAccessTokenUsesTypedAccountKey(t *testing.T) {
	kc := &injectedTATTestKeychain{}
	if err := StoreInjectedTenantAccessToken(kc, "cli_test", "tenant-token"); err != nil {
		t.Fatalf("StoreInjectedTenantAccessToken() error = %v", err)
	}
	if err := StoreInjectedTenantAccessToken(kc, "cli_test", "replacement-token"); err != nil {
		t.Fatalf("replacement StoreInjectedTenantAccessToken() error = %v", err)
	}
	if len(kc.setCalls) != 2 {
		t.Fatalf("Set calls = %d, want 2", len(kc.setCalls))
	}
	got := kc.setCalls[1]
	if got.service != keychain.LarkCliService || got.account != "tat:cli_test" || got.value != "replacement-token" {
		t.Fatalf("replacement Set call = %#v, want service lark-cli, account tat:cli_test, replacement token", got)
	}
	if stored := kc.values[keychain.LarkCliService+"/tat:cli_test"]; stored != "replacement-token" {
		t.Fatalf("stored value = %q, want replacement-token", stored)
	}
}

func TestInjectedTenantTokenProviderCachesHitPerApp(t *testing.T) {
	kc := &injectedTATTestKeychain{values: map[string]string{
		keychain.LarkCliService + "/tat:cli_test": "tenant-token-v1",
	}}
	p := NewInjectedTenantTokenProvider(func() keychain.KeychainAccess { return kc })
	req := extcred.TokenSpec{Type: extcred.TokenTypeTAT, AppID: "cli_test"}

	first, err := p.ResolveToken(context.Background(), req)
	if err != nil {
		t.Fatalf("first ResolveToken() error = %v", err)
	}
	kc.values[keychain.LarkCliService+"/tat:cli_test"] = "tenant-token-v2"
	second, err := p.ResolveToken(context.Background(), req)
	if err != nil {
		t.Fatalf("second ResolveToken() error = %v", err)
	}
	if first == nil || second == nil || first.Value != "tenant-token-v1" || second.Value != "tenant-token-v1" {
		t.Fatalf("tokens = (%#v, %#v), want cached tenant-token-v1", first, second)
	}
	if len(kc.getCalls) != 1 {
		t.Fatalf("Get calls = %v, want one call", kc.getCalls)
	}
}

func TestInjectedTenantTokenProviderCachesMiss(t *testing.T) {
	kc := &injectedTATTestKeychain{values: map[string]string{}}
	p := NewInjectedTenantTokenProvider(func() keychain.KeychainAccess { return kc })
	req := extcred.TokenSpec{Type: extcred.TokenTypeTAT, AppID: "cli_missing"}

	for i := 0; i < 2; i++ {
		tok, err := p.ResolveToken(context.Background(), req)
		if err != nil || tok != nil {
			t.Fatalf("ResolveToken() = (%#v, %v), want nil, nil", tok, err)
		}
	}
	if len(kc.getCalls) != 1 {
		t.Fatalf("Get calls = %v, want one cached miss", kc.getCalls)
	}
}

func TestInjectedTenantTokenProviderCachesTypedStorageError(t *testing.T) {
	underlying := errors.New("keychain unavailable")
	kc := &injectedTATTestKeychain{getErr: underlying}
	p := NewInjectedTenantTokenProvider(func() keychain.KeychainAccess { return kc })
	req := extcred.TokenSpec{Type: extcred.TokenTypeTAT, AppID: "cli_test"}

	for i := 0; i < 2; i++ {
		_, err := p.ResolveToken(context.Background(), req)
		if err == nil {
			t.Fatal("ResolveToken() error = nil, want typed storage error")
		}
		var internalErr *errs.InternalError
		if !errors.As(err, &internalErr) || internalErr.Subtype != errs.SubtypeStorage {
			t.Fatalf("error = %T %v, want internal/storage", err, err)
		}
		if !errors.Is(err, underlying) {
			t.Fatalf("error = %v, want underlying cause", err)
		}
	}
	if len(kc.getCalls) != 1 {
		t.Fatalf("Get calls = %v, want one cached failure", kc.getCalls)
	}
}

func TestInjectedTenantTokenProviderSkipsUnsupportedOrEmptyRequests(t *testing.T) {
	kc := &injectedTATTestKeychain{}
	p := NewInjectedTenantTokenProvider(func() keychain.KeychainAccess { return kc })

	for _, req := range []extcred.TokenSpec{
		{Type: extcred.TokenTypeUAT, AppID: "cli_test"},
		{Type: extcred.TokenTypeTAT, AppID: ""},
	} {
		tok, err := p.ResolveToken(context.Background(), req)
		if err != nil || tok != nil {
			t.Fatalf("ResolveToken(%+v) = (%#v, %v), want nil, nil", req, tok, err)
		}
	}
	if len(kc.getCalls) != 0 {
		t.Fatalf("Get calls = %v, want none", kc.getCalls)
	}
}

func TestInjectedTenantTokenStorageRejectsUnsafeAppIDWithoutAccess(t *testing.T) {
	kc := &injectedTATTestKeychain{}
	err := StoreInjectedTenantAccessToken(kc, "cli/test", "tenant-token")
	if err == nil {
		t.Fatal("StoreInjectedTenantAccessToken() error = nil, want unsafe app ID rejection")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != "app_id" {
		t.Fatalf("store error = %T %v, want validation/invalid_argument for app_id", err, err)
	}
	if len(kc.setCalls) != 0 {
		t.Fatalf("Set calls = %v, want none", kc.setCalls)
	}

	p := NewInjectedTenantTokenProvider(func() keychain.KeychainAccess { return kc })
	_, err = p.ResolveToken(context.Background(), extcred.TokenSpec{Type: extcred.TokenTypeTAT, AppID: "cli/test"})
	if err == nil {
		t.Fatal("ResolveToken() error = nil, want unsafe app ID rejection")
	}
	var configErr *errs.ConfigError
	if !errors.As(err, &configErr) || configErr.Subtype != errs.SubtypeInvalidConfig {
		t.Fatalf("resolve error = %T %v, want config/invalid_config", err, err)
	}
	if len(kc.getCalls) != 0 {
		t.Fatalf("Get calls = %v, want none", kc.getCalls)
	}
}

func TestInjectedTenantTokenStorageRejectsUppercaseAppIDWithoutAccess(t *testing.T) {
	kc := &injectedTATTestKeychain{}
	err := StoreInjectedTenantAccessToken(kc, "cli_Test", "tenant-token")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != "app_id" {
		t.Fatalf("store error = %T %v, want validation/invalid_argument for app_id", err, err)
	}
	p := NewInjectedTenantTokenProvider(func() keychain.KeychainAccess { return kc })
	_, err = p.ResolveToken(context.Background(), extcred.TokenSpec{Type: extcred.TokenTypeTAT, AppID: "cli_Test"})
	var configErr *errs.ConfigError
	if !errors.As(err, &configErr) || configErr.Subtype != errs.SubtypeInvalidConfig {
		t.Fatalf("resolve error = %T %v, want config/invalid_config", err, err)
	}
	if len(kc.setCalls) != 0 || len(kc.getCalls) != 0 {
		t.Fatalf("keychain calls = set:%v get:%v, want none", kc.setCalls, kc.getCalls)
	}
}
