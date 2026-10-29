// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"testing"
	"time"
)

func TestTokenStatusUsesRefreshAheadWindow(t *testing.T) {
	now := time.Now().UnixMilli()
	token := &StoredUAToken{
		ExpiresAt:        now + (61 * time.Minute).Milliseconds(),
		RefreshExpiresAt: now + (24 * time.Hour).Milliseconds(),
	}
	if got := TokenStatus(token); got != "valid" {
		t.Fatalf("TokenStatus outside refresh window = %q, want valid", got)
	}

	token.ExpiresAt = now + (59 * time.Minute).Milliseconds()
	if got := TokenStatus(token); got != "needs_refresh" {
		t.Fatalf("TokenStatus inside refresh window = %q, want needs_refresh", got)
	}
}
