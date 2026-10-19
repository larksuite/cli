package auth

import (
	"testing"
	"time"
)

func TestExpiredIdentityTokenDoesNotInvalidateAPIAccess(t *testing.T) {
	now := time.Now().UnixMilli()
	token := &StoredUAToken{IDToken: "metadata-only", IDTokenExpiresAt: now - 1000, AccessToken: "access", ExpiresAt: now + 3600000, RefreshToken: "refresh", RefreshExpiresAt: now + 7200000}
	if got := TokenStatus(token); got != "valid" {
		t.Fatalf("expired identity metadata blocked API access: %s", got)
	}
	token.ExpiresAt = now - 1
	if got := TokenStatus(token); got != "needs_refresh" {
		t.Fatalf("refresh must remain usable: %s", got)
	}
}
