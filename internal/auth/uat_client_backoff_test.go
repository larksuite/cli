// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRefreshRateLimitDelay(t *testing.T) {
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name, header string
		want         time.Duration
	}{
		{"seconds", "7", 7 * time.Second},
		{"whitespace", " 2 ", 2 * time.Second},
		{"date", now.Add(12 * time.Second).Format(http.TimeFormat), 12 * time.Second},
		{"seconds capped", "18446744073709551615", 30 * time.Second},
		{"date capped", now.Add(time.Hour).Format(http.TimeFormat), 30 * time.Second},
		{"missing", "", time.Second},
		{"invalid", "later", time.Second},
		{"negative", "-1", time.Second},
		{"fractional", "1.5", time.Second},
		{"overflow", "18446744073709551616", time.Second},
		{"zero", "0", time.Second},
		{"past", now.Add(-time.Hour).Format(http.TimeFormat), time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := refreshRateLimitDelay(tt.header, now); got != tt.want {
				t.Fatalf("delay = %v, want %v", got, tt.want)
			}
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{tt.header}}, Body: io.NopCloser(strings.NewReader("not JSON")), Request: req}, nil
			})}
			// Seconds and fallback cases also verify the HTTP classification carries the delay.
			result := refreshOnce(context.Background(), client, "https://example.test/token", newRefreshTestOptions(newRefreshTestToken()), newRefreshTestToken(), nil, false)
			if result.action != refreshRetryAndPreserve {
				t.Fatalf("action = %v", result.action)
			}
			if _, err := http.ParseTime(tt.header); err != nil && result.retryDelay != tt.want {
				t.Fatalf("HTTP delay = %v, want %v", result.retryDelay, tt.want)
			}
		})
	}
}

func TestWaitRefreshRetry(t *testing.T) {
	if err := waitRefreshRetry(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- waitRefreshRetry(ctx, time.Hour) }()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v", err)
	}
}

func TestRefreshRateLimitWaitCancellationPreservesToken(t *testing.T) {
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	if err := SetStoredToken(stored); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls > 1 {
			t.Fatal("retried after cancellation")
		}
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"30"}}, Body: io.NopCloser(strings.NewReader("limited")), Request: req}, nil
	})}
	waits := 0
	_, err := doRefreshTokenWithRetryWait(ctx, client, newRefreshTestOptions(stored), stored, func(waitCtx context.Context, delay time.Duration) error {
		waits++
		if delay != 30*time.Second {
			t.Fatalf("delay = %v", delay)
		}
		cancel()
		return waitRefreshRetry(waitCtx, delay)
	})
	if waits != 1 {
		t.Fatalf("waits = %d", waits)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	got, err := GetStoredToken(stored.AppId, stored.UserOpenId)
	if err != nil || got == nil || got.RefreshToken != stored.RefreshToken {
		t.Fatalf("stored token = %#v, error = %v", got, err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestRefreshRateLimitWaitBeforeRetry(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(map[bool]string{false: "repeated 429", true: "rotated token"}[success], func(t *testing.T) {
			setupStoredTokenTest(t)
			stored := newRefreshTestToken()
			if err := SetStoredToken(stored); err != nil {
				t.Fatal(err)
			}
			calls, waits := 0, 0
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls > 2 || (calls == 2 && waits != 1) {
					t.Fatalf("calls=%d waits=%d", calls, waits)
				}
				if success && calls == 2 {
					return refreshHTTPResponse(req, `{"access_token":"access-new","refresh_token":"refresh-new","expires_in":7200,"refresh_token_expires_in":604800}`), nil
				}
				return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"4"}}, Body: io.NopCloser(strings.NewReader("limited")), Request: req}, nil
			})}
			got, err := doRefreshTokenWithRetryWait(context.Background(), client, newRefreshTestOptions(stored), stored, func(_ context.Context, delay time.Duration) error {
				waits++
				if calls != 1 || delay != 4*time.Second {
					t.Fatalf("calls=%d delay=%v", calls, delay)
				}
				return nil
			})
			if calls != 2 || waits != 1 {
				t.Fatalf("calls=%d waits=%d", calls, waits)
			}
			persisted, storageErr := GetStoredToken(stored.AppId, stored.UserOpenId)
			if storageErr != nil || persisted == nil {
				t.Fatalf("persisted=%#v error=%v", persisted, storageErr)
			}
			if success {
				if err != nil || got == nil || persisted.RefreshToken != "refresh-new" {
					t.Fatalf("result=%#v error=%v persisted=%#v", got, err, persisted)
				}
			} else {
				if err == nil || got != nil || persisted.RefreshToken != stored.RefreshToken {
					t.Fatalf("result=%#v error=%v persisted=%#v", got, err, persisted)
				}
			}
		})
	}
}
