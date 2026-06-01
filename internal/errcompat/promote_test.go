// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errcompat_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errcompat"
)

func TestPromoteConfigError_TypeAuth_PromotesToAuthenticationError(t *testing.T) {
	cfg := &core.ConfigError{
		Type:    "auth",
		Code:    3,
		Message: "not logged in",
		Hint:    "run: lark-cli auth login",
	}
	got := errcompat.PromoteConfigError(cfg)

	var authErr *errs.AuthenticationError
	if !errors.As(got, &authErr) {
		t.Fatalf("expected *errs.AuthenticationError, got %T", got)
	}
	if authErr.Subtype != errs.SubtypeTokenMissing {
		t.Errorf("subtype = %v, want %v", authErr.Subtype, errs.SubtypeTokenMissing)
	}
	// Cause chain must preserve original *core.ConfigError for errors.As compat.
	var cfgPreserved *core.ConfigError
	if !errors.As(got, &cfgPreserved) {
		t.Error("Unwrap chain lost *core.ConfigError — breaks cmd/auth/list.go consumer")
	}
}

func TestPromoteConfigError_TypeConfig_PromotesToConfigError(t *testing.T) {
	cases := []struct {
		name        string
		msg         string
		wantSubtype errs.Subtype
	}{
		{"not_configured", "not configured", errs.SubtypeNotConfigured},
		{"invalid_config_parse", "failed to parse config", errs.SubtypeInvalidConfig},
		{"invalid_config_keyword", "invalid config file", errs.SubtypeInvalidConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &core.ConfigError{Type: "config", Code: 3, Message: tc.msg}
			got := errcompat.PromoteConfigError(cfg)

			var ce *errs.ConfigError
			if !errors.As(got, &ce) {
				t.Fatalf("expected *errs.ConfigError, got %T", got)
			}
			if ce.Subtype != tc.wantSubtype {
				t.Errorf("subtype = %v, want %v", ce.Subtype, tc.wantSubtype)
			}
		})
	}
}

func TestPromoteConfigError_TypeDynamic_PromotesToConfigError(t *testing.T) {
	for _, wsName := range []string{"openclaw", "hermes", "bind"} {
		t.Run(wsName, func(t *testing.T) {
			cfg := &core.ConfigError{Type: wsName, Code: 3, Message: "not configured"}
			got := errcompat.PromoteConfigError(cfg)

			var ce *errs.ConfigError
			if !errors.As(got, &ce) {
				t.Fatalf("expected *errs.ConfigError, got %T", got)
			}
			if ce.Subtype != errs.SubtypeNotConfigured {
				t.Errorf("subtype = %v, want %v", ce.Subtype, errs.SubtypeNotConfigured)
			}
		})
	}
}

func TestPromoteConfigError_Nil_ReturnsNil(t *testing.T) {
	if got := errcompat.PromoteConfigError(nil); got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
}

func TestPromoteConfigError_PreservesMessageHint(t *testing.T) {
	cfg := &core.ConfigError{
		Type:    "auth",
		Message: "session expired (user: u_xxx)",
		Hint:    "re-authenticate",
	}
	got := errcompat.PromoteConfigError(cfg)
	if !strings.Contains(got.Error(), "session expired") {
		t.Errorf("message lost in promotion: %v", got)
	}
	var authErr *errs.AuthenticationError
	if !errors.As(got, &authErr) {
		t.Fatalf("expected *errs.AuthenticationError, got %T", got)
	}
	if authErr.Hint != "re-authenticate" {
		t.Errorf("hint = %q, want preserved", authErr.Hint)
	}
}
