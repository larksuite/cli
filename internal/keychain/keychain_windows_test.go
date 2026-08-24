// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package keychain

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestRegistryGetDistinguishesMissingFromCorruptValue(t *testing.T) {
	service := "lark-cli-test-" + strings.ReplaceAll(t.Name(), "/", "-")
	account := "tat:cli_test"
	keyPath := registryPathForService(service)
	_ = registry.DeleteKey(registry.CURRENT_USER, keyPath)
	t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, keyPath) })

	value, found, err := registryGet(service, account)
	if err != nil || found || value != "" {
		t.Fatalf("missing registryGet() = (%q, %v, %v), want empty, false, nil", value, found, err)
	}

	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("CreateKey() error = %v", err)
	}
	if err := k.SetStringValue(valueNameForAccount(account), "not-base64!"); err != nil {
		_ = k.Close()
		t.Fatalf("SetStringValue() error = %v", err)
	}
	if err := k.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, found, err := registryGet(service, account); err == nil || found {
		t.Fatalf("corrupt registryGet() = (found=%v, err=%v), want false with error", found, err)
	}
	if _, err := platformGet(service, account); err == nil {
		t.Fatal("platformGet() error = nil for corrupt registry value")
	}
}
