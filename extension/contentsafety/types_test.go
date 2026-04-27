// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"context"
	"io"
	"testing"
)

func TestAlertFields(t *testing.T) {
	a := &Alert{
		Provider:     "regex",
		MatchedRules: []string{"rule_a", "rule_b"},
	}
	if a.Provider != "regex" {
		t.Errorf("Provider = %q, want %q", a.Provider, "regex")
	}
	if len(a.MatchedRules) != 2 {
		t.Errorf("MatchedRules length = %d, want 2", len(a.MatchedRules))
	}
}

type stubProvider struct{}

func (s *stubProvider) Name() string { return "stub" }
func (s *stubProvider) Scan(_ context.Context, _ ScanRequest) (*Alert, error) {
	return &Alert{Provider: "stub", MatchedRules: []string{"test"}}, nil
}

func TestProviderInterface(t *testing.T) {
	var p Provider = &stubProvider{}
	if p.Name() != "stub" {
		t.Errorf("Name() = %q, want %q", p.Name(), "stub")
	}
	alert, err := p.Scan(context.Background(), ScanRequest{Path: "test", Data: nil, ErrOut: io.Discard})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if alert.Provider != "stub" {
		t.Errorf("alert.Provider = %q, want %q", alert.Provider, "stub")
	}
}

func TestRegistryLastWriteWins(t *testing.T) {
	mu.Lock()
	old := provider
	provider = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		provider = old
		mu.Unlock()
	}()

	if GetProvider() != nil {
		t.Fatal("expected nil provider initially")
	}
	p1 := &stubProvider{}
	Register(p1)
	if GetProvider() != p1 {
		t.Fatal("expected p1 after first Register")
	}
	p2 := &stubProvider{}
	Register(p2)
	if GetProvider() != p2 {
		t.Fatal("expected p2 after second Register (last-write-wins)")
	}
}
