// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

func TestDiagnoseRequestedScopes_UnknownAndDisabled(t *testing.T) {
	known := map[string]bool{
		"base:app:create":      true,
		"base:field:create":    true,
		"base:view:write_only": true,
	}
	enabled := map[string]bool{
		"base:field:create": true,
	}

	diag := diagnoseRequestedScopes(
		"base:app:create base:view:create base:view:write base:field:create offline_access",
		known,
		enabled,
	)

	if len(diag.NotEnabled) != 1 || diag.NotEnabled[0] != "base:app:create" {
		t.Fatalf("unexpected disabled scopes: %#v", diag.NotEnabled)
	}
	if len(diag.Unknown) != 2 || diag.Unknown[0] != "base:view:create" || diag.Unknown[1] != "base:view:write" {
		t.Fatalf("unexpected unknown scopes: %#v", diag.Unknown)
	}
	if got := diag.Suggestions["base:view:create"]; len(got) == 0 || got[0] != "base:view:write_only" {
		t.Fatalf("expected suggestion for base:view:create, got %#v", got)
	}
	if got := diag.Suggestions["base:view:write"]; len(got) == 0 || got[0] != "base:view:write_only" {
		t.Fatalf("expected suggestion for base:view:write, got %#v", got)
	}
}

func TestExplainScopeRequestError_FormatsDetailedValidation(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})

	err := explainScopeRequestError(
		context.Background(),
		f,
		&core.CliConfig{AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu},
		"base:view:create base:view:write",
		errors.New("Device authorization failed: The provided scope list contains invalid or malformed scopes. Please ensure all scopes are valid."),
	)
	if err == nil {
		t.Fatal("expected validation error")
	}
	exitErr, ok := err.(*output.ExitError)
	if !ok || exitErr.Code != output.ExitAuth {
		t.Fatalf("expected auth exit error, got %#v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown scope: base:view:create") {
		t.Fatalf("expected unknown scope detail, got: %s", msg)
	}
	if !strings.Contains(msg, "base:view:write_only") {
		t.Fatalf("expected suggestion in message, got: %s", msg)
	}
	if !strings.Contains(msg, "lark-cli auth scopes") {
		t.Fatalf("expected auth scopes hint, got: %s", msg)
	}
}

func TestExplainScopeRequestError_IgnoresOtherErrors(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})

	err := explainScopeRequestError(
		context.Background(),
		f,
		&core.CliConfig{AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu},
		"base:view:create",
		errors.New("Device authorization failed: temporary network timeout"),
	)
	if err != nil {
		t.Fatalf("expected nil for unrelated errors, got %v", err)
	}
}

func TestExplainScopeRequestError_NotEnabledScope(t *testing.T) {
	orig := loadAppInfo
	loadAppInfo = func(_ context.Context, _ *cmdutil.Factory, _ string) (*appInfo, error) {
		return &appInfo{UserScopes: []string{"base:field:create"}}, nil
	}
	defer func() { loadAppInfo = orig }()

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})

	err := explainScopeRequestError(
		context.Background(),
		f,
		&core.CliConfig{AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu},
		"base:app:create",
		errors.New("Device authorization failed: invalid scope"),
	)
	if err == nil {
		t.Fatal("expected auth error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "scope not enabled for current app: base:app:create") {
		t.Fatalf("expected not-enabled detail, got: %s", msg)
	}
	if !strings.Contains(msg, "lark-cli auth scopes") {
		t.Fatalf("expected auth scopes hint, got: %s", msg)
	}
}

func TestExplainScopeRequestError_AppScopeLookupFailureStillGuidesUser(t *testing.T) {
	orig := loadAppInfo
	loadAppInfo = func(_ context.Context, _ *cmdutil.Factory, _ string) (*appInfo, error) {
		return nil, errors.New("lookup failed")
	}
	defer func() { loadAppInfo = orig }()

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})

	err := explainScopeRequestError(
		context.Background(),
		f,
		&core.CliConfig{AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu},
		"base:app:create",
		errors.New("Device authorization failed: malformed scope list"),
	)
	if err == nil {
		t.Fatal("expected auth error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "could not be fully diagnosed") {
		t.Fatalf("expected fallback guidance, got: %s", msg)
	}
	if !strings.Contains(msg, "lookup failed") {
		t.Fatalf("expected app scope lookup error in message, got: %s", msg)
	}
}
