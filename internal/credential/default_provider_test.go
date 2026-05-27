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

// TestClassifyTATResponseCode_10003_MapsToInvalidClient pins that the TAT
// endpoint's "invalid param" code surfaces as CategoryConfig/InvalidClient.
// Reason: a bad or non-existent app_id triggers 10003 on the TAT mint endpoint,
// which from the user's perspective is the same actionable failure as 10014
// ("app secret invalid") — both mean the configured credentials cannot mint a
// tenant access token. The global codemeta intentionally does not map 10003
// because in other Lark endpoints 10003 carries unrelated semantics (e.g. task
// API uses it for permission denied), so the override is local to this site.
func TestClassifyTATResponseCode_10003_MapsToInvalidClient(t *testing.T) {
	err := classifyTATResponseCode(10003, "invalid param", "feishu", "cli_app_x")
	if err == nil {
		t.Fatal("expected non-nil error for code=10003")
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
	if cfgErr.Code != 10003 {
		t.Errorf("Code = %d, want 10003", cfgErr.Code)
	}
	if cfgErr.Hint == "" {
		t.Error("Hint must be non-empty so the user gets a recovery action")
	}
}

// TestClassifyTATResponseCode_10014_RoutesViaCodeMeta pins that 10014 still
// goes through the global BuildAPIError path (codemeta entry) so the override
// for 10003 does not regress the existing mapping.
func TestClassifyTATResponseCode_10014_RoutesViaCodeMeta(t *testing.T) {
	err := classifyTATResponseCode(10014, "app secret invalid", "feishu", "cli_app_x")
	if err == nil {
		t.Fatal("expected non-nil error for code=10014")
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidClient {
		t.Errorf("Subtype = %q, want %q", cfgErr.Subtype, errs.SubtypeInvalidClient)
	}
	if cfgErr.Code != 10014 {
		t.Errorf("Code = %d, want 10014", cfgErr.Code)
	}
}

// TestClassifyTATResponseCode_UnknownCodeFallsThrough pins that codes outside
// the credential set fall through to the generic BuildAPIError fallback
// (CategoryAPI/SubtypeUnknown) — the override is narrow and intentional.
func TestClassifyTATResponseCode_UnknownCodeFallsThrough(t *testing.T) {
	err := classifyTATResponseCode(99999999, "some unknown failure", "feishu", "cli_app_x")
	if err == nil {
		t.Fatal("expected non-nil error for unmapped code")
	}
	var cfgErr *errs.ConfigError
	if errors.As(err, &cfgErr) {
		t.Fatalf("unmapped code must not be classified as ConfigError, got %T", err)
	}
}
