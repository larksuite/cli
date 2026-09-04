// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestAuthLoginRun_DeviceCodeUsesCachedRequestedScopes(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	t.Setenv("HOME", t.TempDir())
	const statusMessage = "[待审核，通过后用户需重新授权] 以下权限正在等待管理员审核：offline_access"

	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{Name: "default", AppId: "cli_test"},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  0,
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathOAuthTokenV2,
		Body: map[string]interface{}{
			"access_token":             "user-access-token",
			"refresh_token":            "refresh-token",
			"expires_in":               7200,
			"refresh_token_expires_in": 604800,
			"scope":                    "im:message:send offline_access",
			"status_message":           statusMessage,
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    larkauth.PathUserInfoV1,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"open_id": "ou_user",
				"name":    "tester",
			},
		},
	})

	err := authLoginRun(&LoginOptions{
		Factory: f,
		Ctx:     context.Background(),
		Scope:   "im:message:send",
		NoWait:  true,
	}, builtinResolver())
	if err != nil {
		t.Fatalf("no-wait authLoginRun() error = %v", err)
	}
	if got, err := loadLoginRequestedScope("device-code"); err != nil || got != "im:message:send" {
		t.Fatalf("loadLoginRequestedScope() = (%q, %v), want requested scope", got, err)
	}

	stdout.Reset()
	stderr.Reset()

	err = authLoginRun(&LoginOptions{
		Factory:    f,
		Ctx:        context.Background(),
		DeviceCode: "device-code",
	}, builtinResolver())
	if err != nil {
		t.Fatalf("device-code authLoginRun() error = %v", err)
	}
	got := stderr.String()
	for _, want := range []string{
		"OK: 登录成功! 用户: tester (ou_user)",
		"本次已成功授权：\n  im:message:send\n\n本次授权结果详情：\n  " + statusMessage,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q, got:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"本次请求 scopes:", "本次新授予 scopes:", "lark-cli auth status"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stderr should not contain %q, got:\n%s", unwanted, got)
		}
	}
	if got, err := loadLoginRequestedScope("device-code"); err != nil || got != "" {
		t.Fatalf("loadLoginRequestedScope() after cleanup = (%q, %v), want empty", got, err)
	}
}

func TestWriteLoginSuccess_TextOutputEnglishUsesCompactScopeSummary(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)

	writeLoginSuccess(&LoginOptions{}, getLoginMsg("en"), f, "ou_user", "tester", &loginScopeSummary{
		Requested:    []string{"im:message:send"},
		NewlyGranted: []string{"im:message:send"},
		Granted:      []string{"im:message:send"},
	})

	got := stderr.String()
	for _, want := range []string{
		"Authorization successful! User: tester (ou_user)",
		"Authorization successful! User: tester (ou_user)\n\n" +
			"- Successfully authorized in this request:\n" +
			"  im:message:send",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q, got:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"Scopes not granted in this request", "Requested scopes:", "Newly granted scopes:", "lark-cli auth status"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stderr should not contain %q, got:\n%s", unwanted, got)
		}
	}
}
