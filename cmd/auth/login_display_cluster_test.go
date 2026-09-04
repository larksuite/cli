// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

func TestEnsureRequestedScopesGranted(t *testing.T) {
	issue := ensureRequestedScopesGranted("im:message:send im:message:reply", "im:message:reply", getLoginMsg("en"), nil)
	if issue == nil {
		t.Fatal("expected missing scope issue")
	}
	if !strings.Contains(issue.Message, "im:message:send") {
		t.Fatalf("message %q missing requested scope", issue.Message)
	}
	for _, want := range []string{"Do not retry continuously", "scope being disabled", "lark-cli auth status"} {
		if !strings.Contains(issue.Hint, want) {
			t.Fatalf("hint %q missing %q", issue.Hint, want)
		}
	}
	if got := strings.Join(issue.Summary.Missing, " "); got != "im:message:send" {
		t.Fatalf("Missing = %q", got)
	}
}

func TestBuildLoginScopeSummary(t *testing.T) {
	summary := buildLoginScopeSummary("im:message:send im:message:reply im:message:send", "im:message:reply", "im:message:send im:message:reply im:chat:read")
	if got := strings.Join(summary.Requested, " "); got != "im:message:send im:message:reply" {
		t.Fatalf("Requested = %q", got)
	}
	if got := strings.Join(summary.NewlyGranted, " "); got != "im:message:send" {
		t.Fatalf("NewlyGranted = %q", got)
	}
	if got := strings.Join(summary.AlreadyGranted, " "); got != "im:message:reply" {
		t.Fatalf("AlreadyGranted = %q", got)
	}
	if len(summary.Missing) != 0 {
		t.Fatalf("Missing = %v, want empty", summary.Missing)
	}
	if got := strings.Join(summary.Granted, " "); got != "im:message:send im:message:reply im:chat:read" {
		t.Fatalf("Granted = %q", got)
	}
}

func TestBuildLoginScopeSummary_WithMissingScopes(t *testing.T) {
	summary := buildLoginScopeSummary("im:message:send im:message:reply", "im:message:reply", "im:message:reply")
	if got := strings.Join(summary.NewlyGranted, " "); got != "" {
		t.Fatalf("NewlyGranted = %q, want empty", got)
	}
	if got := strings.Join(summary.AlreadyGranted, " "); got != "im:message:reply" {
		t.Fatalf("AlreadyGranted = %q", got)
	}
	if got := strings.Join(summary.Missing, " "); got != "im:message:send" {
		t.Fatalf("Missing = %q", got)
	}
}

func TestWriteLoginSuccess_JSONIncludesScopeDiff(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	summary := &loginScopeSummary{
		Requested:      []string{"im:message:send", "im:message:reply"},
		NewlyGranted:   []string{"im:message:send"},
		AlreadyGranted: []string{"im:message:reply"},
		Granted:        []string{"im:message:send", "im:message:reply"},
		StatusMessage: "[用户跳过，可重试] 用户未勾选：calendar:calendar:update\n" +
			"应用身份\n[待审核，通过后自动生效] 以下权限正在等待管理员审核：im:message",
	}
	writeLoginSuccess(&LoginOptions{JSON: true}, getLoginMsg("en"), f, "ou_user", "tester", summary)

	var data map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if data["event"] != "authorization_complete" {
		t.Fatalf("event = %v", data["event"])
	}
	if data["scope"] != "im:message:send im:message:reply" {
		t.Fatalf("scope = %v", data["scope"])
	}
	if len(data["newly_granted"].([]interface{})) != 1 {
		t.Fatalf("newly_granted = %#v", data["newly_granted"])
	}
	if len(data["already_granted"].([]interface{})) != 1 {
		t.Fatalf("already_granted = %#v", data["already_granted"])
	}
	if _, ok := data["status_message"]; ok {
		t.Fatalf("status_message should not be exposed at the top level: %#v", data)
	}
}

func TestWriteLoginSuccess_JSONEmptySlicesNotNull(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	writeLoginSuccess(&LoginOptions{JSON: true}, getLoginMsg("en"), f, "ou_user", "tester", &loginScopeSummary{
		Granted: []string{"offline_access"},
	})

	var data map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	for _, k := range []string{"requested", "newly_granted", "already_granted", "missing", "granted"} {
		v, ok := data[k]
		if !ok {
			t.Fatalf("missing key %q in payload: %v", k, data)
		}
		if _, ok := v.([]interface{}); !ok {
			t.Fatalf("%s = %#v, want JSON array", k, v)
		}
	}
	if _, ok := data["status_message"]; ok {
		t.Fatalf("status_message should not be exposed at the top level: %#v", data)
	}
}

func TestWriteLoginSuccess_TextOutputScenarios(t *testing.T) {
	tests := []struct {
		name            string
		summary         *loginScopeSummary
		expectedPresent []string
		expectedAbsent  []string
	}{
		{
			name: "mixed newly granted and already granted",
			summary: &loginScopeSummary{
				Requested:      []string{"im:message:send", "im:message:reply"},
				NewlyGranted:   []string{"im:message:send"},
				AlreadyGranted: []string{"im:message:reply"},
				Granted:        []string{"im:message:send", "im:message:reply"},
			},
			expectedPresent: []string{
				"登录成功! 用户: tester (ou_user)",
				"登录成功! 用户: tester (ou_user)\n\n本次已成功授权：\n  im:message:send、im:message:reply",
			},
			expectedAbsent: []string{
				"以下是本次未授予的权限",
				"本次请求 scopes:",
				"本次新授予 scopes:",
				"lark-cli auth status",
			},
		},
		{
			name: "all already granted",
			summary: &loginScopeSummary{
				Requested:      []string{"im:message:send"},
				AlreadyGranted: []string{"im:message:send"},
				Granted:        []string{"im:message:send", "contact:user.base:readonly"},
			},
			expectedPresent: []string{
				"本次已成功授权：\n  im:message:send",
			},
			expectedAbsent: []string{
				"以下是本次未授予的权限",
				"本次请求 scopes:",
				"本次新授予 scopes:",
				"lark-cli auth status",
			},
		},
		{
			name: "missing scopes are shown",
			summary: &loginScopeSummary{
				Requested: []string{"im:message:send", "im:message:reply"},
				Missing:   []string{"im:message:send"},
				Granted:   []string{"im:message:reply"},
			},
			expectedPresent: []string{
				"本次已成功授权：\n  im:message:reply\n\n以下是本次未授予的权限：\n  im:message:send",
			},
			expectedAbsent: []string{
				"本次请求 scopes:",
				"本次新授予 scopes:",
				"lark-cli auth status",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, stderr, _ := cmdutil.TestFactory(t, nil)
			writeLoginSuccess(&LoginOptions{}, getLoginMsg("zh"), f, "ou_user", "tester", tt.summary)

			got := stderr.String()
			for _, want := range tt.expectedPresent {
				if !strings.Contains(got, want) {
					t.Fatalf("stderr missing %q, got:\n%s", want, got)
				}
			}
			for _, unwanted := range tt.expectedAbsent {
				if strings.Contains(got, unwanted) {
					t.Fatalf("stderr should not contain %q, got:\n%s", unwanted, got)
				}
			}
		})
	}
}

func TestWriteLoginSuccess_TextStatusMessageWithoutMissingScopesUsesDetailsHeading(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	statusMessage := "[用户跳过，可重试] 用户未勾选：calendar:calendar:update\n" +
		"应用身份\n[待审核，通过后自动生效] 以下权限正在等待管理员审核：im:message"

	writeLoginSuccess(&LoginOptions{}, getLoginMsg("zh"), f, "ou_user", "tester", &loginScopeSummary{
		Requested:     []string{"im:message:send"},
		NewlyGranted:  []string{"im:message:send"},
		Granted:       []string{"im:message:send"},
		StatusMessage: statusMessage,
	})

	got := stderr.String()
	wantBlock := "本次已成功授权：\n" +
		"  im:message:send\n\n" +
		"本次授权结果详情：\n" +
		"  [用户跳过，可重试] 用户未勾选：calendar:calendar:update\n" +
		"  应用身份\n" +
		"  [待审核，通过后自动生效] 以下权限正在等待管理员审核：im:message"
	if !strings.Contains(got, wantBlock) {
		t.Fatalf("stderr missing formatted authorization block %q, got:\n%s", wantBlock, got)
	}
	scopePos := strings.Index(got, "本次已成功授权：\n  im:message:send")
	detailsPos := strings.Index(got, "本次授权结果详情：")
	statusPos := strings.Index(got, "  [用户跳过，可重试]")
	if scopePos < 0 || detailsPos <= scopePos || statusPos <= detailsPos {
		t.Fatalf("status_message placement is wrong, got:\n%s", got)
	}
	if strings.Contains(got, "以下是本次未授予的权限：") {
		t.Fatalf("stderr should not label status_message as not granted when no scopes are missing, got:\n%s", got)
	}
	if strings.Contains(got, "可执行 `lark-cli auth status`") {
		t.Fatalf("stderr should not contain the hidden status hint, got:\n%s", got)
	}
}

func TestWriteLoginSuccess_TextStatusMessageWithoutMissingScopesUsesEnglishDetailsHeading(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)

	writeLoginSuccess(&LoginOptions{}, getLoginMsg("en"), f, "ou_user", "tester", &loginScopeSummary{
		Requested:     []string{"im:message:send"},
		Granted:       []string{"im:message:send"},
		StatusMessage: "Authorization status details",
	})

	got := stderr.String()
	if !strings.Contains(got, "- Authorization details:\n  Authorization status details") {
		t.Fatalf("stderr missing neutral authorization details heading, got:\n%s", got)
	}
	if strings.Contains(got, "- Scopes not granted in this request:") {
		t.Fatalf("stderr should not label status_message as not granted when no scopes are missing, got:\n%s", got)
	}
}

func TestHandleLoginScopeIssue_NonJSONAlignsWithLoginSuccess(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	issue := &loginScopeIssue{
		Message: "授权结果异常: 以下请求 scopes 未被授予: im:message:send",
		Hint:    "以上结果是本次授权请求用户最终确认后的结果，请勿持续重试；Scopes 未授予的原因是多样的，如 scope 被禁用；具体原因已通过授权页提示用户。可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
		Summary: &loginScopeSummary{
			Requested: []string{"im:message:send", "im:message:reply"},
			Missing:   []string{"im:message:send"},
			Granted:   []string{"im:message:reply", "base:app:copy"},
			StatusMessage: "[用户跳过，可重试] 用户未勾选：calendar:calendar:update\n" +
				"应用身份\n[待审核，通过后自动生效] 以下权限正在等待管理员审核：im:message",
		},
	}
	err := handleLoginScopeIssue(&LoginOptions{}, getLoginMsg("zh"), f, issue, "ou_user", "tester")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if gotCode := output.ExitCodeOf(err); gotCode != output.ExitAuth {
		t.Fatalf("exit code = %d, want %d", gotCode, output.ExitAuth)
	}
	got := stderr.String()
	for _, want := range []string{
		"OK: 登录成功! 用户: tester (ou_user)",
		"OK: 登录成功! 用户: tester (ou_user)\n\n" +
			"本次已成功授权：\n" +
			"  im:message:reply\n\n" +
			"以下是本次未授予的权限：\n" +
			"  [用户跳过，可重试] 用户未勾选：calendar:calendar:update\n" +
			"  应用身份\n" +
			"  [待审核，通过后自动生效] 以下权限正在等待管理员审核：im:message",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q, got:\n%s", want, got)
		}
	}
	successPos := strings.Index(got, "OK: 登录成功! 用户: tester (ou_user)")
	scopePos := strings.Index(got, "本次已成功授权：\n  im:message:reply")
	missingPos := strings.Index(got, "以下是本次未授予的权限：")
	statusPos := strings.Index(got, "  [用户跳过，可重试]")
	if successPos < 0 || scopePos <= successPos || missingPos <= scopePos || statusPos <= missingPos {
		t.Fatalf("login result placement is wrong, got:\n%s", got)
	}
	for _, unwanted := range []string{
		issue.Message,
		"本次请求 scopes:",
		"本次新授予 scopes:",
		issue.Hint,
		"当前授权账号:",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stderr should not contain %q, got:\n%s", unwanted, got)
		}
	}
}

func TestHandleLoginScopeIssue_JSONAlignsWithLoginSuccess(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	issue := &loginScopeIssue{
		Message: "authorization result is abnormal: these requested scopes were not granted: im:message:send",
		Hint:    "Granted scopes: base:app:copy. Check app scopes.",
		Summary: &loginScopeSummary{
			Requested: []string{"im:message:send"},
			Missing:   []string{"im:message:send"},
			Granted:   []string{"base:app:copy"},
			StatusMessage: "[用户跳过，可重试] 用户未勾选：calendar:calendar:update\n" +
				"应用身份\n[待审核，通过后自动生效] 以下权限正在等待管理员审核：im:message",
		},
	}
	err := handleLoginScopeIssue(&LoginOptions{JSON: true}, getLoginMsg("en"), f, issue, "ou_user", "tester")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if gotCode := output.ExitCodeOf(err); gotCode != output.ExitAuth {
		t.Fatalf("exit code = %d, want %d", gotCode, output.ExitAuth)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if data["event"] != "authorization_complete" {
		t.Fatalf("event = %v", data["event"])
	}
	if data["user_open_id"] != "ou_user" {
		t.Fatalf("user_open_id = %v", data["user_open_id"])
	}
	warning, ok := data["warning"].(map[string]interface{})
	if !ok {
		t.Fatalf("warning = %#v", data["warning"])
	}
	if warning["type"] != "missing_scope" {
		t.Fatalf("warning.type = %v", warning["type"])
	}
	if _, ok := warning["message"]; ok {
		t.Fatalf("warning.message should not be exposed: %#v", warning)
	}
	if warning["hint"] != issue.Summary.StatusMessage {
		t.Fatalf("warning.hint = %v, want %q", warning["hint"], issue.Summary.StatusMessage)
	}
	if _, ok := data["status_message"]; ok {
		t.Fatalf("status_message should not be exposed at the top level: %#v", data)
	}
}

func TestAuthLoginRun_MissingRequestedScopeAlignsWithLoginSuccess(t *testing.T) {
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

	f, _, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
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
			"scope":                    "offline_access",
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
	}, builtinResolver())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if gotCode := output.ExitCodeOf(err); gotCode != output.ExitAuth {
		t.Fatalf("exit code = %d, want %d", gotCode, output.ExitAuth)
	}
	got := stderr.String()
	for _, want := range []string{
		"OK: 登录成功! 用户: tester (ou_user)",
		"本次已成功授权：\n  （空）\n\n以下是本次未授予的权限：\n  " + statusMessage,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q, got:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		"授权结果异常:",
		"本次请求 scopes:",
		"本次新授予 scopes:",
		"以上结果是本次授权请求用户最终确认后的结果",
		"lark-cli auth status",
		"当前授权账号:",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stderr should not contain %q, got:\n%s", unwanted, got)
		}
	}
	if strings.Contains(got, "ERROR:") {
		t.Fatalf("stderr should not contain error prefix, got:\n%s", got)
	}
	stored := larkauth.GetStoredToken("cli_test", "ou_user")
	if stored == nil {
		t.Fatal("expected token to be stored when authorization succeeds with missing scopes")
	}
	if stored.Scope != "offline_access" {
		t.Fatalf("stored scope = %q", stored.Scope)
	}
	cfg, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if len(cfg.Apps) != 1 || len(cfg.Apps[0].Users) != 1 {
		t.Fatalf("unexpected users in config: %#v", cfg.Apps)
	}
	if cfg.Apps[0].Users[0].UserOpenId != "ou_user" {
		t.Fatalf("stored user open id = %q", cfg.Apps[0].Users[0].UserOpenId)
	}
	if cfg.Apps[0].Users[0].UserName != "tester" {
		t.Fatalf("stored user name = %q", cfg.Apps[0].Users[0].UserName)
	}
}

func TestAuthLoginRun_DeviceCodeTokenNilCleansScopeCache(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)

	if err := saveLoginRequestedScope("device-code", "im:message:send"); err != nil {
		t.Fatalf("saveLoginRequestedScope() error = %v", err)
	}

	original := pollDeviceToken
	t.Cleanup(func() { pollDeviceToken = original })
	pollDeviceToken = func(ctx context.Context, httpClient *http.Client, appId, appSecret string, brand core.LarkBrand, deviceCode string, interval, expiresIn int, errOut io.Writer) *larkauth.DeviceFlowResult {
		return &larkauth.DeviceFlowResult{OK: true, Token: nil}
	}

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})

	err := authLoginRun(&LoginOptions{
		Factory:    f,
		Ctx:        context.Background(),
		DeviceCode: "device-code",
	}, builtinResolver())
	if err == nil {
		t.Fatal("expected error for nil token")
	}
	if !strings.Contains(err.Error(), "authorization succeeded but no token returned") {
		t.Fatalf("error = %v, want nil token error", err)
	}
	if got, err := loadLoginRequestedScope("device-code"); err != nil || got != "" {
		t.Fatalf("loadLoginRequestedScope() after nil token = (%q, %v), want empty", got, err)
	}
}

func TestAuthorizationCompletePayload_EmptyStatusMessageFallsBackToIssueMessage(t *testing.T) {
	issue := &loginScopeIssue{
		Message: "authorization result is abnormal: these requested scopes were not granted: im:message:send",
		Summary: &loginScopeSummary{
			Missing: []string{"im:message:send"},
		},
	}

	payload := authorizationCompletePayload("ou_user", "tester", issue.Summary, issue)
	warning, ok := payload["warning"].(map[string]interface{})
	if !ok {
		t.Fatalf("warning = %#v", payload["warning"])
	}
	if warning["hint"] != issue.Message {
		t.Fatalf("warning.hint = %v, want %q", warning["hint"], issue.Message)
	}
	if _, ok := warning["message"]; ok {
		t.Fatalf("warning.message should not be exposed: %#v", warning)
	}
}
