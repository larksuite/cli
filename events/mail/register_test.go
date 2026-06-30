// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import "testing"

func TestMailScopesForEventFormatWithoutFiltersSkipsMessageScopes(t *testing.T) {
	got := mailScopesForParams(map[string]string{"msg_format": "event"})

	if containsScope(got, "mail:user_mailbox.message:readonly") {
		t.Fatalf("event format without metadata filters should not require message read scope: %#v", got)
	}
	if !containsScope(got, "mail:event") || !containsScope(got, "mail:user_mailbox.event.mail_address:read") {
		t.Fatalf("event format missing event scopes: %#v", got)
	}
}

func TestMailScopesForEventFormatWithFiltersRequiresMessageScopes(t *testing.T) {
	got := mailScopesForParams(map[string]string{
		"msg_format": "event",
		"label_ids":  `["FLAGGED"]`,
	})

	for _, want := range mailMessageReadScopes {
		if !containsScope(got, want) {
			t.Fatalf("filtered event format missing message scope %q: %#v", want, got)
		}
	}
}

func containsScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}
	return false
}
