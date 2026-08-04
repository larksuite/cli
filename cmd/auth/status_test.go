// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	envprovider "github.com/larksuite/cli/extension/credential/env"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestAuthStatusHelpDistinguishesFromWhoami(t *testing.T) {
	cmd := NewCmdAuthStatus(nil, nil)
	for _, want := range []string{
		"OAuth user login",
		"auth status --json --verify",
		"not profile/app selection diagnostics",
		"lark-cli whoami",
	} {
		if !strings.Contains(cmd.Long, want) {
			t.Errorf("auth status --help Long missing %q; got:\n%s", want, cmd.Long)
		}
	}
}

func TestAuthStatusRun_SplitsBotAndUserIdentity(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "secret", Brand: core.BrandFeishu,
	})

	if err := authStatusRun(&StatusOptions{Factory: f}); err != nil {
		t.Fatalf("authStatusRun() error = %v", err)
	}

	var got statusOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.Identity != "bot" {
		t.Fatalf("identity = %q, want bot", got.Identity)
	}
	if got.Identities.Bot.Status != "ready" || !got.Identities.Bot.Available {
		t.Fatalf("bot = %#v, want ready and available", got.Identities.Bot)
	}
	if got.Identities.User.Status != "missing" || got.Identities.User.Available {
		t.Fatalf("user = %#v, want missing and unavailable", got.Identities.User)
	}
}

func TestAuthStatusRun_VerifyReportsBotIdentity(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/bot/v3/info",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"bot": map[string]interface{}{
				"open_id":  "ou_bot",
				"app_name": "diagnostic bot",
			},
		},
	})

	if err := authStatusRun(&StatusOptions{Factory: f, Verify: true}); err != nil {
		t.Fatalf("authStatusRun() error = %v", err)
	}

	var got statusOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.Identity != "bot" {
		t.Fatalf("identity = %q, want bot", got.Identity)
	}
	if got.Verified == nil || !*got.Verified {
		t.Fatalf("verified = %v, want true", got.Verified)
	}
	if got.Identities.Bot.Verified == nil || !*got.Identities.Bot.Verified {
		t.Fatalf("bot verified = %v, want true", got.Identities.Bot.Verified)
	}
	if got.Identities.Bot.OpenID != "ou_bot" {
		t.Fatalf("bot open id = %q, want ou_bot", got.Identities.Bot.OpenID)
	}
	if got.Identities.User.Status != "missing" {
		t.Fatalf("user status = %q, want missing", got.Identities.User.Status)
	}
}

type fixedStatusAccountResolver struct {
	account *credential.Account
}

func (r *fixedStatusAccountResolver) ResolveAccount(context.Context) (*credential.Account, error) {
	return r.account, nil
}

func TestAuthStatus_AllowsMatchingAppIDOnlySelectedProfile(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliAppID, "cli_a")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		CurrentApp: "tenant_a",
		Apps: []core.AppConfig{{
			Name:      "tenant_a",
			AppId:     "cli_a",
			AppSecret: core.PlainSecret("test-secret"),
			Brand:     core.BrandFeishu,
		}},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	config := &core.CliConfig{ProfileName: "tenant_a", AppID: "cli_a", AppSecret: "test-secret", Brand: core.BrandFeishu}
	f, stdout, _, _ := cmdutil.TestFactory(t, config)
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{&envprovider.Provider{}},
		&fixedStatusAccountResolver{account: credential.AccountFromCliConfig(config)},
		nil,
		nil,
	).WithProfileFromFlag("tenant_a")

	cmd := NewCmdAuth(f)
	cmd.SetArgs([]string{"status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth status should use the selected built-in profile: %v", err)
	}
	if strings.Contains(stdout.String(), "credentials are provided externally") {
		t.Fatalf("matching APP_ID-only env was misclassified as external:\n%s", stdout.String())
	}
}

type statusOutput struct {
	Identity   string `json:"identity"`
	Verified   *bool  `json:"verified"`
	Identities struct {
		Bot  statusIdentity `json:"bot"`
		User statusIdentity `json:"user"`
	} `json:"identities"`
}

type statusIdentity struct {
	Status    string `json:"status"`
	Available bool   `json:"available"`
	Verified  *bool  `json:"verified"`
	OpenID    string `json:"openId"`
}
