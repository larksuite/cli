// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package ratelimit

import (
	"strings"
	"testing"
	"time"
)

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		want      string
		wantMatch bool
	}{
		{
			name:      "non mail path is ignored",
			method:    "GET",
			path:      "/open-apis/contact/v3/users/u1",
			wantMatch: false,
		},
		{
			name:      "unconfigured mail path is ignored",
			method:    "GET",
			path:      "/open-apis/mail/v1/user_mailboxes/me/settings",
			wantMatch: false,
		},
		{
			name:      "full URL query and fragment canonicalize",
			method:    "get",
			path:      "https://open.feishu.cn/open-apis/mail/v1/user_mailboxes/me/messages/msg_1?format=full#body",
			want:      "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/:message_id",
			wantMatch: true,
		},
		{
			name:      "relative path query and fragment canonicalize",
			method:    "GET",
			path:      "/open-apis/mail/v1/user_mailboxes/me/messages/msg_1?format=metadata#body",
			want:      "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/:message_id",
			wantMatch: true,
		},
		{
			name:      "concrete batch get canonicalizes",
			method:    "POST",
			path:      "/open-apis/mail/v1/user_mailboxes/me/messages/batch_get",
			want:      "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/batch_get",
			wantMatch: true,
		},
		{
			name:      "SDK template path matches exactly",
			method:    "GET",
			path:      "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/:message_id",
			want:      "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/:message_id",
			wantMatch: true,
		},
		{
			name:      "method mismatch is ignored",
			method:    "POST",
			path:      "/open-apis/mail/v1/user_mailboxes/me/messages/msg_1",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rule, ok := Canonicalize(tt.method, tt.path)
			if ok != tt.wantMatch {
				t.Fatalf("match = %v, want %v", ok, tt.wantMatch)
			}
			if !ok {
				return
			}
			if got != tt.want {
				t.Fatalf("canonical = %q, want %q", got, tt.want)
			}
			if rule == nil {
				t.Fatal("expected rule")
			}
		})
	}
}

func TestBuiltinRulesUseConfirmedThreshold(t *testing.T) {
	type threshold struct {
		window time.Duration
		limit  int
	}
	want := map[string][]threshold{
		"GET /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/:message_id": {
			{window: time.Minute, limit: 100},
		},
		"POST /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/batch_get": {
			{window: time.Second, limit: 10},
		},
		"GET /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages": {
			{window: time.Second, limit: 10},
		},
		"POST /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/search": {
			{window: time.Minute, limit: 1000},
			{window: time.Second, limit: 50},
		},
	}
	seen := make(map[string][]threshold)
	for _, rule := range builtinRules {
		key := rule.Method + " " + rule.CanonicalPath
		if _, ok := want[key]; !ok {
			t.Fatalf("unexpected builtin rule %s", key)
		}
		if rule.Window <= 0 {
			t.Fatalf("%s window must be positive", key)
		}
		if rule.Limit <= 0 {
			t.Fatalf("%s limit must be positive", key)
		}
		if rule.Scope != ScopeApp {
			t.Fatalf("%s scope = %q, want %q", key, rule.Scope, ScopeApp)
		}
		if rule.Method == "" {
			t.Fatalf("%s method must not be empty", key)
		}
		if !strings.HasPrefix(rule.CanonicalPath, "/open-apis/mail/") {
			t.Fatalf("%s canonical path must be under /open-apis/mail/", key)
		}
		seen[key] = append(seen[key], threshold{window: rule.Window, limit: rule.Limit})
	}
	if len(builtinRules) != 5 {
		t.Fatalf("builtinRules len = %d, want 5", len(builtinRules))
	}
	for key, thresholds := range want {
		if len(seen[key]) != len(thresholds) {
			t.Fatalf("missing builtin rule %s", key)
		}
		for _, threshold := range thresholds {
			found := false
			for _, got := range seen[key] {
				if got == threshold {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("%s missing threshold window=%s limit=%d; got %#v", key, threshold.window, threshold.limit, seen[key])
			}
		}
	}
}
