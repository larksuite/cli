// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"context"
	"testing"
)

type stubProvider struct{ name string }

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Scan(_ context.Context, _ ScanRequest) (*Alert, error) {
	return nil, nil
}

func TestGetProvider_NilByDefault(t *testing.T) {
	mu.Lock()
	saved := provider
	provider = nil
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); provider = saved; mu.Unlock() })

	if got := GetProvider(); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestRegister_LastWriteWins(t *testing.T) {
	mu.Lock()
	saved := provider
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); provider = saved; mu.Unlock() })

	Register(&stubProvider{name: "a"})
	Register(&stubProvider{name: "b"})

	got := GetProvider()
	if got == nil || got.Name() != "b" {
		t.Fatalf("expected provider 'b', got %v", got)
	}
}
