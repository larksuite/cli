// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keychain"
)

type memoryAATKeychain struct {
	mu    sync.Mutex
	items map[string]string
}

func newMemoryAATKeychain() *memoryAATKeychain {
	return &memoryAATKeychain{items: map[string]string{}}
}

func (k *memoryAATKeychain) key(service, account string) string {
	return service + "\x00" + account
}

func (k *memoryAATKeychain) Get(service, account string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.items[k.key(service, account)], nil
}

func (k *memoryAATKeychain) Set(service, account, value string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.items[k.key(service, account)] = value
	return nil
}

func (k *memoryAATKeychain) Remove(service, account string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.items, k.key(service, account))
	return nil
}

func testAAT(now time.Time, value string, grantedOffset, expiryOffset time.Duration) *StoredAAToken {
	return &StoredAAToken{
		AppID:       "cli_aat",
		UserOpenID:  "ou_agent",
		AccessToken: value,
		GrantedAt:   now.Add(grantedOffset).UnixMilli(),
		ExpiresAt:   now.Add(expiryOffset).UnixMilli(),
	}
}

func TestStoredAATSchema(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	kc := newMemoryAATKeychain()
	now := time.Unix(1_787_639_434, 0)
	token := testAAT(now, "aat-value", 0, 2*time.Hour)
	if err := SetStoredAAT(kc, token); err != nil {
		t.Fatal(err)
	}

	raw, err := kc.Get(keychain.LarkCliService, "cli_aat:aat")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"appId": "cli_aat", "userOpenId": "ou_agent", "accessToken": "aat-value",
		"grantedAt": float64(token.GrantedAt), "expiresAt": float64(token.ExpiresAt),
		"scope": "",
	}
	if len(got) != len(want) {
		t.Fatalf("stored fields = %#v", got)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("stored[%q] = %#v, want %#v", key, got[key], value)
		}
	}
}

func TestStoredAATState(t *testing.T) {
	now := time.Unix(1_787_639_434, 0)
	wrongPrincipal := testAAT(now, "cached", 0, time.Hour)
	wrongPrincipal.UserOpenID = "ou_old"
	tests := []struct {
		name  string
		token *StoredAAToken
		want  aatCacheState
	}{
		{name: "fresh", token: testAAT(now, "cached", -10*time.Minute, 50*time.Minute), want: aatCacheFresh},
		{name: "ten percent refresh window", token: testAAT(now, "cached", -18*time.Minute-30*time.Second, 90*time.Second), want: aatCacheNeedsRefresh},
		{name: "five minute refresh cap", token: testAAT(now, "cached", -115*time.Minute-10*time.Second, 4*time.Minute+50*time.Second), want: aatCacheNeedsRefresh},
		{name: "hard expiry", token: testAAT(now, "cached", -2*time.Hour, -time.Second), want: aatCacheExpired},
		{name: "principal mismatch", token: wrongPrincipal, want: aatCacheInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := storedAATState(tc.token, "cli_aat", "ou_agent", now); got != tc.want {
				t.Fatalf("storedAATState() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveStoredAATLifecycle(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	now := time.Unix(1_787_639_434, 0)

	t.Run("fresh cache", func(t *testing.T) {
		kc := newMemoryAATKeychain()
		if err := SetStoredAAT(kc, testAAT(now, "cached", -10*time.Minute, 50*time.Minute)); err != nil {
			t.Fatal(err)
		}
		calls := 0
		result, err := ResolveStoredAAT(context.Background(), kc, "cli_aat", "ou_agent", func() time.Time { return now },
			func(context.Context) (*StoredAAToken, error) {
				calls++
				return nil, errors.New("must not refresh")
			})
		if err != nil || result.Token.AccessToken != "cached" || calls != 0 {
			t.Fatalf("result = %#v, calls = %d, err = %v", result, calls, err)
		}
	})

	t.Run("expired cache", func(t *testing.T) {
		kc := newMemoryAATKeychain()
		if err := SetStoredAAT(kc, testAAT(now, "expired", -2*time.Hour, -time.Second)); err != nil {
			t.Fatal(err)
		}
		result, err := ResolveStoredAAT(context.Background(), kc, "cli_aat", "ou_agent", func() time.Time { return now },
			func(context.Context) (*StoredAAToken, error) { return testAAT(now, "renewed", 0, time.Hour), nil })
		stored, readErr := GetStoredAAT(kc, "cli_aat")
		if err != nil || readErr != nil || result.Token.AccessToken != "renewed" || stored.AccessToken != "renewed" {
			t.Fatalf("result = %#v, stored = %#v, err = %v, readErr = %v", result, stored, err, readErr)
		}
	})
}

func TestResolveStoredAATRefreshFailure(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	now := time.Unix(1_787_639_434, 0)
	refreshErr := errs.NewNetworkError(errs.SubtypeNetworkServer, "IAM unavailable").WithRetryable()
	tests := []struct {
		name         string
		cached       *StoredAAToken
		wantFallback bool
	}{
		{name: "before hard expiry", cached: testAAT(now, "still-valid", -119*time.Minute, time.Minute), wantFallback: true},
		{name: "after hard expiry", cached: testAAT(now, "expired", -2*time.Hour, -time.Second)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kc := newMemoryAATKeychain()
			if err := SetStoredAAT(kc, tc.cached); err != nil {
				t.Fatal(err)
			}
			result, err := ResolveStoredAAT(context.Background(), kc, "cli_aat", "ou_agent", func() time.Time { return now },
				func(context.Context) (*StoredAAToken, error) { return nil, refreshErr })
			if tc.wantFallback {
				if err != nil || result.Token.AccessToken != "still-valid" || !errors.Is(result.RefreshError, refreshErr) {
					t.Fatalf("result = %#v, err = %v; want old token and refresh warning", result, err)
				}
				return
			}
			if !errors.Is(err, refreshErr) {
				t.Fatalf("error = %v, want refresh failure", err)
			}
		})
	}
}

func TestResolveStoredAATConcurrentMissMintsOnce(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	kc := newMemoryAATKeychain()
	now := time.Unix(1_787_639_434, 0)
	var calls atomic.Int32
	refresh := func(context.Context) (*StoredAAToken, error) {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return testAAT(now, "one-generation", 0, time.Hour), nil
	}

	const workers = 4
	results := make(chan *AATCacheResult, workers)
	errsCh := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := ResolveStoredAAT(context.Background(), kc, "cli_aat", "ou_agent", func() time.Time { return now }, refresh)
			results <- result
			errsCh <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errsCh)
	for err := range errsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	for result := range results {
		if result == nil || result.Token.AccessToken != "one-generation" {
			t.Fatalf("result = %#v", result)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", calls.Load())
	}
}

func TestStoredAATLegacyRecordHasEmptyScope(t *testing.T) {
	kc := newMemoryAATKeychain()
	// A record written by a binary that predates the Scope field.
	legacy := `{"appId":"cli_a","userOpenId":"ou_x","accessToken":"tok",` +
		`"expiresAt":99999999999999,"grantedAt":1}`
	if err := kc.Set(keychain.LarkCliService, AATAccountKey("cli_a"), legacy); err != nil {
		t.Fatalf("seed: %v", err)
	}
	out, err := GetStoredAAT(kc, "cli_a")
	if err != nil {
		t.Fatalf("GetStoredAAT: %v", err)
	}
	if out.Scope != "" {
		t.Errorf("Scope = %q, want empty for a legacy record", out.Scope)
	}
}
