// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin

package keychain

import (
	"context"
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestResolveMasterKey_OnlyCreatesOnNotFound(t *testing.T) {
	expected := []byte("12345678901234567890123456789012")
	setCalled := false

	_, err := resolveMasterKey(
		func() (string, error) { return "", context.DeadlineExceeded },
		func(string) error {
			setCalled = true
			return nil
		},
		func() ([]byte, error) { return expected, nil },
	)
	if err == nil {
		t.Fatal("expected resolveMasterKey to return unavailable error")
	}
	if setCalled {
		t.Fatal("expected non-ErrNotFound failure to avoid rotating master key")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected original cause in error chain, got %v", err)
	}
}

func TestResolveMasterKey_CreatesOnNotFound(t *testing.T) {
	setCalled := false

	key, err := resolveMasterKey(
		func() (string, error) { return "", keyring.ErrNotFound },
		func(encoded string) error {
			setCalled = true
			return nil
		},
		func() ([]byte, error) { return []byte("12345678901234567890123456789012"), nil },
	)
	if err != nil {
		t.Fatalf("resolveMasterKey: %v", err)
	}
	if !setCalled {
		t.Fatal("expected keyring.Set path on ErrNotFound")
	}
	if len(key) != masterKeyBytes {
		t.Fatalf("master key len = %d, want %d", len(key), masterKeyBytes)
	}
}
