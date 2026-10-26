// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/i18n"
)

func noRefreshSummary(refreshToken string) *loginScopeSummary {
	token := &larkauth.StoredUAToken{
		AccessToken:  "u-access",
		RefreshToken: refreshToken,
		ExpiresAt:    time.Date(2026, 10, 26, 18, 47, 49, 0, time.UTC).UnixMilli(),
	}
	return &loginScopeSummary{
		Requested: []string{"contact:user.base:readonly", "offline_access"},
		Granted:   []string{"contact:user.base:readonly", "offline_access"},
		Refresh:   newLoginRefreshInfo(core.BrandFeishu, "cli_no_rt", token),
	}
}

func TestWriteLoginSuccess_JSONReportsMissingRefreshToken(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	writeLoginSuccess(&LoginOptions{JSON: true}, getLoginMsg("en"), f, "ou_user", "tester", noRefreshSummary(""))

	var data map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if data["refresh_token_issued"] != false {
		t.Fatalf("refresh_token_issued = %v, want false", data["refresh_token_issued"])
	}
	wantExpiry := time.Date(2026, 10, 26, 18, 47, 49, 0, time.UTC).Local().Format(time.RFC3339)
	if data["expires_at"] != wantExpiry {
		t.Fatalf("expires_at = %v", data["expires_at"])
	}
	warning, ok := data["no_refresh_token"].(map[string]interface{})
	if !ok {
		t.Fatalf("no_refresh_token = %#v, want object", data["no_refresh_token"])
	}
	if warning["settings_url"] != "https://open.feishu.cn/app/cli_no_rt/safe" {
		t.Fatalf("settings_url = %v", warning["settings_url"])
	}
	for _, field := range []string{"message", "cause", "fix"} {
		if s, _ := warning[field].(string); s == "" {
			t.Fatalf("no_refresh_token.%s = %q, want non-empty", field, s)
		}
	}
	if !strings.Contains(warning["cause"].(string), `"Refresh user_access_token"`) {
		t.Fatalf("cause = %q, want the refresh setting named", warning["cause"])
	}
}

func TestWriteLoginSuccess_JSONWithRefreshTokenHasNoWarning(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	writeLoginSuccess(&LoginOptions{JSON: true}, getLoginMsg("en"), f, "ou_user", "tester", noRefreshSummary("ur-refresh"))

	var data map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if data["refresh_token_issued"] != true {
		t.Fatalf("refresh_token_issued = %v, want true", data["refresh_token_issued"])
	}
	if _, ok := data["no_refresh_token"]; ok {
		t.Fatalf("no_refresh_token present for a renewable login: %#v", data["no_refresh_token"])
	}
}

func TestWriteLoginSuccess_TextWarnsWhenNoRefreshToken(t *testing.T) {
	for _, tc := range []struct {
		lang  string
		wants []string
	}{
		{"zh", []string{"WARN: 本次登录没有拿到 refresh_token", "「刷新 user_access_token」", "发布新版本", "https://open.feishu.cn/app/cli_no_rt/safe"}},
		{"en", []string{"WARN: This login received no refresh_token", `"Refresh user_access_token"`, "publish a new app version", "https://open.feishu.cn/app/cli_no_rt/safe"}},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			f, _, stderr, _ := cmdutil.TestFactory(t, nil)
			writeLoginSuccess(&LoginOptions{}, getLoginMsg(i18n.Lang(tc.lang)), f, "ou_user", "tester", noRefreshSummary(""))
			for _, want := range tc.wants {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
				}
			}
		})
	}
}

func TestWriteLoginSuccess_TextSilentWhenRefreshTokenIssued(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	writeLoginSuccess(&LoginOptions{}, getLoginMsg("en"), f, "ou_user", "tester", noRefreshSummary("ur-refresh"))
	if strings.Contains(stderr.String(), "refresh_token") {
		t.Fatalf("unexpected refresh warning:\n%s", stderr.String())
	}
}

func TestHandleLoginScopeIssue_TextStillWarnsWhenNoRefreshToken(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	summary := noRefreshSummary("")
	summary.Requested = append(summary.Requested, "okr:okr.content:readonly")
	summary.Missing = []string{"okr:okr.content:readonly"}
	issue := &loginScopeIssue{Message: "missing scopes", Summary: summary}

	_ = handleLoginScopeIssue(&LoginOptions{}, getLoginMsg("en"), f, issue, "ou_user", "tester")
	if !strings.Contains(stderr.String(), "WARN: This login received no refresh_token") {
		t.Fatalf("stderr missing refresh warning:\n%s", stderr.String())
	}
}
