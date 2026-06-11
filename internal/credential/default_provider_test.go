// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestDefaultTokenProvider_Dispatches(t *testing.T) {
	// Just verify the type implements DefaultTokenResolver
	var _ DefaultTokenResolver = &DefaultTokenProvider{}
}

func TestDefaultAccountProvider_Implements(t *testing.T) {
	var _ DefaultAccountResolver = &DefaultAccountProvider{}
}

// TestClassifyTATResponseCode_InvalidClient_MapsToInvalidClient pins that the
// unified Token Endpoint's OAuth2 invalid_client error surfaces as
// CategoryConfig/InvalidClient — the configured app_id/app_secret cannot mint a
// tenant access token, the same actionable failure the legacy 10003/10014 codes
// produced. The numeric code is intentionally not asserted: the v3 endpoint may
// return invalid_client with no Lark code (code defaults to 0).
func TestClassifyTATResponseCode_InvalidClient_MapsToInvalidClient(t *testing.T) {
	err := classifyTATResponseCode(0, "invalid_client", "client authentication failed", "feishu", "cli_app_x")
	if err == nil {
		t.Fatal("expected non-nil error for invalid_client")
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Category != errs.CategoryConfig {
		t.Errorf("Category = %q, want %q", cfgErr.Category, errs.CategoryConfig)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidClient {
		t.Errorf("Subtype = %q, want %q", cfgErr.Subtype, errs.SubtypeInvalidClient)
	}
	if cfgErr.Hint == "" {
		t.Error("Hint must be non-empty so the user gets a recovery action")
	}
}

// TestClassifyTATResponseCode_UnauthorizedClient_MapsToInvalidClient pins that
// unauthorized_client is treated as the same credential failure as
// invalid_client.
func TestClassifyTATResponseCode_UnauthorizedClient_MapsToInvalidClient(t *testing.T) {
	err := classifyTATResponseCode(0, "unauthorized_client", "client not authorized", "feishu", "cli_app_x")
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidClient {
		t.Errorf("Subtype = %q, want %q", cfgErr.Subtype, errs.SubtypeInvalidClient)
	}
}

// TestClassifyTATResponseCode_OtherErrorFallsThrough pins that OAuth errors
// outside the credential set fall through to the generic BuildAPIError fallback
// — still typed, but not a ConfigError. The mapping is narrow and intentional.
func TestClassifyTATResponseCode_OtherErrorFallsThrough(t *testing.T) {
	err := classifyTATResponseCode(20068, "invalid_scope", "unauthorized scope", "feishu", "cli_app_x")
	if err == nil {
		t.Fatal("expected non-nil error for invalid_scope")
	}
	var cfgErr *errs.ConfigError
	if errors.As(err, &cfgErr) {
		t.Fatalf("invalid_scope must not be classified as ConfigError, got %T", err)
	}
}
