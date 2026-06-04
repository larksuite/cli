// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package ratelimit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

func testRule() Rule {
	return Rule{
		Method:        "GET",
		CanonicalPath: "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/:message_id",
		Window:        2 * time.Second,
		Limit:         2,
		Scope:         ScopeApp,
	}
}

func testRequest() Request {
	return Request{
		Brand:  core.BrandFeishu,
		AppID:  "app-1",
		Method: "GET",
		Path:   "/open-apis/mail/v1/user_mailboxes/me/messages/msg_1",
	}
}

func TestLimiterAllowsUntilLimitThenReturnsRateLimit(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := NewLimiterForDir(t.TempDir(), []Rule{testRule()}, func() time.Time { return now })
	ctx := context.Background()

	if err := limiter.Allow(ctx, testRequest()); err != nil {
		t.Fatalf("first check err = %v", err)
	}
	if err := limiter.Allow(ctx, testRequest()); err != nil {
		t.Fatalf("second check err = %v", err)
	}
	err := limiter.Allow(ctx, testRequest())
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "rate_limit" || exitErr.Detail.Code != output.LarkErrRateLimit {
		t.Fatalf("unexpected detail: %#v", exitErr.Detail)
	}
	detail, ok := exitErr.Detail.Detail.(map[string]any)
	if !ok {
		t.Fatalf("expected detail map, got %T", exitErr.Detail.Detail)
	}
	if got := detail["retry_after_ms"]; got != int64(2000) {
		t.Fatalf("retry_after_ms = %v, want 2000", got)
	}
	if got := detail["source"]; got != "local_ratelimit" {
		t.Fatalf("source = %v, want local_ratelimit", got)
	}
}

func TestLimiterPrunesWindowAndAllowsAgain(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := NewLimiterForDir(t.TempDir(), []Rule{testRule()}, func() time.Time { return now })
	ctx := context.Background()

	if err := limiter.Allow(ctx, testRequest()); err != nil {
		t.Fatal(err)
	}
	if err := limiter.Allow(ctx, testRequest()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2*time.Second + time.Millisecond)
	if err := limiter.Allow(ctx, testRequest()); err != nil {
		t.Fatalf("check after window err = %v", err)
	}
}

func TestLimiterChecksMultipleRulesForSameKey(t *testing.T) {
	now := time.Unix(100, 0)
	shortRule := testRule()
	shortRule.Window = time.Second
	shortRule.Limit = 2
	longRule := testRule()
	longRule.Window = 10 * time.Second
	longRule.Limit = 3
	limiter := NewLimiterForDir(t.TempDir(), []Rule{shortRule, longRule}, func() time.Time { return now })
	ctx := context.Background()

	if err := limiter.Allow(ctx, testRequest()); err != nil {
		t.Fatal(err)
	}
	if err := limiter.Allow(ctx, testRequest()); err != nil {
		t.Fatal(err)
	}
	if err := limiter.Allow(ctx, testRequest()); !IsLocalRateLimit(err) {
		t.Fatalf("third check err = %v, want local rate_limit", err)
	}

	now = now.Add(time.Second + time.Millisecond)
	if err := limiter.Allow(ctx, testRequest()); err != nil {
		t.Fatalf("check after short window err = %v", err)
	}
	err := limiter.Allow(ctx, testRequest())
	if !IsLocalRateLimit(err) {
		t.Fatalf("check after long window limit err = %v, want local rate_limit", err)
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	detail, ok := exitErr.Detail.Detail.(map[string]any)
	if !ok {
		t.Fatalf("expected detail map, got %T", exitErr.Detail.Detail)
	}
	if got := detail["retry_after_ms"]; got != int64(8999) {
		t.Fatalf("retry_after_ms = %v, want 8999", got)
	}
}

func TestLimiterKeyIsIsolatedByBrandAppMethodAndCanonicalPath(t *testing.T) {
	now := time.Unix(100, 0)
	rules := []Rule{
		{
			Method:        "GET",
			CanonicalPath: "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/:message_id",
			Window:        2 * time.Second,
			Limit:         1,
			Scope:         ScopeApp,
		},
		{
			Method:        "POST",
			CanonicalPath: "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/search",
			Window:        2 * time.Second,
			Limit:         1,
			Scope:         ScopeApp,
		},
	}
	limiter := NewLimiterForDir(t.TempDir(), rules, func() time.Time { return now })
	ctx := context.Background()
	base := testRequest()
	if err := limiter.Allow(ctx, base); err != nil {
		t.Fatal(err)
	}

	cases := []Request{
		{Brand: core.BrandLark, AppID: base.AppID, Method: base.Method, Path: base.Path},
		{Brand: base.Brand, AppID: "app-2", Method: base.Method, Path: base.Path},
		{Brand: base.Brand, AppID: base.AppID, Method: "POST", Path: "/open-apis/mail/v1/user_mailboxes/me/search"},
	}
	for _, req := range cases {
		if err := limiter.Allow(ctx, req); err != nil {
			t.Fatalf("isolated request %#v err = %v", req, err)
		}
	}
	if err := limiter.Allow(ctx, base); err == nil {
		t.Fatal("expected original key to remain limited")
	}
}

func TestLimiterNoopsForNonMailAndUnconfiguredMail(t *testing.T) {
	limiter := NewLimiterForDir(t.TempDir(), []Rule{testRule()}, time.Now)
	ctx := context.Background()
	requests := []Request{
		{Method: "GET", Path: "/open-apis/contact/v3/users/u1"},
		{Method: "GET", Path: "/open-apis/mail/v1/user_mailboxes/me/settings"},
	}
	for _, req := range requests {
		if err := limiter.Allow(ctx, req); err != nil {
			t.Fatalf("request %#v err = %v", req, err)
		}
	}
}

func TestLimiterSkipsMissingAppID(t *testing.T) {
	now := time.Unix(100, 0)
	rule := testRule()
	rule.Limit = 1
	limiter := NewLimiterForDir(t.TempDir(), []Rule{rule}, func() time.Time { return now })
	req := testRequest()
	req.AppID = ""

	for i := 0; i < 2; i++ {
		if err := limiter.Allow(context.Background(), req); err != nil {
			t.Fatalf("allow %d err = %v", i+1, err)
		}
	}
}

func TestLimiterSupportsConfiguredNonMailRules(t *testing.T) {
	rule := Rule{
		Method:        "GET",
		CanonicalPath: "/open-apis/contact/v3/users/:user_id",
		Window:        time.Second,
		Limit:         1,
		Scope:         ScopeApp,
	}
	limiter := NewLimiterForDir(t.TempDir(), []Rule{rule}, time.Now)
	req := Request{
		Brand:  core.BrandFeishu,
		AppID:  "app-1",
		Method: "GET",
		Path:   "/open-apis/contact/v3/users/u1",
	}
	if err := limiter.Allow(context.Background(), req); err != nil {
		t.Fatalf("first allow err = %v", err)
	}
	if err := limiter.Allow(context.Background(), req); !IsLocalRateLimit(err) {
		t.Fatalf("second allow err = %v, want local rate_limit", err)
	}
}

func TestLimiterSkipsUnsupportedScope(t *testing.T) {
	rule := testRule()
	rule.Scope = Scope("user")
	limiter := NewLimiterForDir(t.TempDir(), []Rule{rule}, time.Now)
	for i := 0; i < 2; i++ {
		if err := limiter.Allow(context.Background(), testRequest()); err != nil {
			t.Fatalf("allow %d err = %v", i+1, err)
		}
	}
}

func TestLimiterSkipsInvalidRuleParams(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Rule)
	}{
		{
			name: "zero limit",
			mutate: func(rule *Rule) {
				rule.Limit = 0
			},
		},
		{
			name: "negative limit",
			mutate: func(rule *Rule) {
				rule.Limit = -1
			},
		},
		{
			name: "zero window",
			mutate: func(rule *Rule) {
				rule.Window = 0
			},
		},
		{
			name: "negative window",
			mutate: func(rule *Rule) {
				rule.Window = -time.Second
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rule := testRule()
			tt.mutate(&rule)
			limiter := NewLimiterForDir(t.TempDir(), []Rule{rule}, time.Now)
			for i := 0; i < 2; i++ {
				if err := limiter.Allow(context.Background(), testRequest()); err != nil {
					t.Fatalf("allow %d err = %v", i+1, err)
				}
			}
		})
	}
}

func TestLimiterUsesValidRulesWhenMixedWithInvalidRules(t *testing.T) {
	now := time.Unix(100, 0)
	invalid := testRule()
	invalid.Scope = Scope("user")
	valid := testRule()
	valid.Limit = 1
	limiter := NewLimiterForDir(t.TempDir(), []Rule{invalid, valid}, func() time.Time { return now })

	if err := limiter.Allow(context.Background(), testRequest()); err != nil {
		t.Fatalf("first allow err = %v", err)
	}
	if err := limiter.Allow(context.Background(), testRequest()); !IsLocalRateLimit(err) {
		t.Fatalf("second allow err = %v, want local rate_limit", err)
	}
}

func TestLimiterConcurrentSameKeyAllowsOnlyLimit(t *testing.T) {
	now := time.Unix(100, 0)
	rule := testRule()
	rule.Limit = 3
	limiter := NewLimiterForDir(t.TempDir(), []Rule{rule}, func() time.Time { return now })

	const workers = 20
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- limiter.Allow(context.Background(), testRequest())
		}()
	}
	wg.Wait()
	close(errs)

	allowed := 0
	limited := 0
	for err := range errs {
		switch {
		case err == nil:
			allowed++
		case IsLocalRateLimit(err):
			limited++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if allowed != rule.Limit {
		t.Fatalf("allowed = %d, want %d", allowed, rule.Limit)
	}
	if limited != workers-rule.Limit {
		t.Fatalf("limited = %d, want %d", limited, workers-rule.Limit)
	}
}
