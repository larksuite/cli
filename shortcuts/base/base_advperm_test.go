// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// ---------------------------------------------------------------------------
// Validate tests
// ---------------------------------------------------------------------------

func TestBaseAdvpermEnableValidate(t *testing.T) {
	ctx := context.Background()

	t.Run("blank base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": ""}, nil, nil)
		if err := BaseAdvpermEnable.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--base-token must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("whitespace base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "   "}, nil, nil)
		if err := BaseAdvpermEnable.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--base-token must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("valid", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x"}, nil, nil)
		if err := BaseAdvpermEnable.Validate(ctx, rt); err != nil {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestBaseAdvpermDisableValidate(t *testing.T) {
	ctx := context.Background()

	t.Run("blank base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": ""}, nil, nil)
		if err := BaseAdvpermDisable.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--base-token must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("whitespace base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "  "}, nil, nil)
		if err := BaseAdvpermDisable.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--base-token must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("valid", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x"}, nil, nil)
		if err := BaseAdvpermDisable.Validate(ctx, rt); err != nil {
			t.Fatalf("err=%v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// DryRun tests
// ---------------------------------------------------------------------------

func TestBaseAdvpermEnableDryRun(t *testing.T) {
	rt := newBaseTestRuntime(map[string]string{"base-token": "app_x"}, nil, nil)
	dr := BaseAdvpermEnable.DryRun(context.Background(), rt)
	if dr == nil {
		t.Fatal("DryRun returned nil")
	}
}

func TestBaseAdvpermDisableDryRun(t *testing.T) {
	rt := newBaseTestRuntime(map[string]string{"base-token": "app_x"}, nil, nil)
	dr := BaseAdvpermDisable.DryRun(context.Background(), rt)
	if dr == nil {
		t.Fatal("DryRun returned nil")
	}
}

// ---------------------------------------------------------------------------
// Metadata tests
// ---------------------------------------------------------------------------

func TestBaseAdvpermMetadata(t *testing.T) {
	t.Run("enable", func(t *testing.T) {
		s := BaseAdvpermEnable
		if s.Command != "+advperm-enable" {
			t.Fatalf("command=%q", s.Command)
		}
		if s.Risk != "write" {
			t.Fatalf("risk=%q", s.Risk)
		}
		if s.Service != "base" {
			t.Fatalf("service=%q", s.Service)
		}
		if len(s.Scopes) != 1 || s.Scopes[0] != "base:app:update" {
			t.Fatalf("scopes=%v", s.Scopes)
		}
	})

	t.Run("disable", func(t *testing.T) {
		s := BaseAdvpermDisable
		if s.Command != "+advperm-disable" {
			t.Fatalf("command=%q", s.Command)
		}
		if s.Risk != "high-risk-write" {
			t.Fatalf("risk=%q", s.Risk)
		}
		if s.Service != "base" {
			t.Fatalf("service=%q", s.Service)
		}
		if len(s.Scopes) != 1 || s.Scopes[0] != "base:app:update" {
			t.Fatalf("scopes=%v", s.Scopes)
		}
	})
}

// ---------------------------------------------------------------------------
// Execute tests (happy path)
// ---------------------------------------------------------------------------

func TestBaseAdvpermEnableExecute(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/advperm/enable",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": nil,
		},
	})
	args := []string{"+advperm-enable", "--base-token", "app_x"}
	if err := runShortcut(t, BaseAdvpermEnable, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "success") {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseAdvpermDisableExecute(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/advperm/enable",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": nil,
		},
	})
	args := []string{"+advperm-disable", "--base-token", "app_x", "--yes"}
	if err := runShortcut(t, BaseAdvpermDisable, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "success") {
		t.Fatalf("stdout=%s", got)
	}
}

// ---------------------------------------------------------------------------
// Execute error paths
// ---------------------------------------------------------------------------

func TestBaseAdvpermEnableExecuteTransportError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/advperm/enable",
		Status: 500,
		Body:   "internal server error",
	})
	args := []string{"+advperm-enable", "--base-token", "app_x"}
	if err := runShortcut(t, BaseAdvpermEnable, args, factory, stdout); err == nil {
		t.Fatal("expected error")
	}
}

func TestBaseAdvpermEnableExecuteAPIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/advperm/enable",
		Body: map[string]interface{}{
			"code": 190001,
			"msg":  "bad request",
		},
	})
	args := []string{"+advperm-enable", "--base-token", "app_x"}
	if err := runShortcut(t, BaseAdvpermEnable, args, factory, stdout); err == nil || !strings.Contains(err.Error(), "190001") {
		t.Fatalf("err=%v", err)
	}
}

func TestBaseAdvpermDisableExecuteTransportError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/advperm/enable",
		Status: 500,
		Body:   "internal server error",
	})
	args := []string{"+advperm-disable", "--base-token", "app_x", "--yes"}
	if err := runShortcut(t, BaseAdvpermDisable, args, factory, stdout); err == nil {
		t.Fatal("expected error")
	}
}

func TestBaseAdvpermDisableExecuteAPIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/advperm/enable",
		Body: map[string]interface{}{
			"code": 190002,
			"msg":  "permission denied",
		},
	})
	args := []string{"+advperm-disable", "--base-token", "app_x", "--yes"}
	if err := runShortcut(t, BaseAdvpermDisable, args, factory, stdout); err == nil || !strings.Contains(err.Error(), "190002") {
		t.Fatalf("err=%v", err)
	}
}
