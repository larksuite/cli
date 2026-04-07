package credential

import (
	"context"
	"testing"
)

type stubProvider struct{ name string }

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) ResolveAccount(ctx context.Context) (*Account, error) {
	return &Account{AppID: s.name}, nil
}
func (s *stubProvider) ResolveToken(ctx context.Context, req TokenSpec) (*Token, error) {
	return &Token{Value: "tok-" + s.name, Source: s.name}, nil
}

func TestRegisterAndProviders(t *testing.T) {
	mu.Lock()
	old := providers
	providers = nil
	mu.Unlock()
	defer func() { mu.Lock(); providers = old; mu.Unlock() }()

	Register(&stubProvider{name: "a"})
	Register(&stubProvider{name: "b"})

	got := Providers()
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Name() != "a" || got[1].Name() != "b" {
		t.Errorf("unexpected order: %s, %s", got[0].Name(), got[1].Name())
	}
}

func TestProviders_ReturnsSnapshot(t *testing.T) {
	mu.Lock()
	old := providers
	providers = nil
	mu.Unlock()
	defer func() { mu.Lock(); providers = old; mu.Unlock() }()

	Register(&stubProvider{name: "x"})
	snap := Providers()
	Register(&stubProvider{name: "y"})

	if len(snap) != 1 {
		t.Fatalf("snapshot should not be affected, got %d", len(snap))
	}
}
