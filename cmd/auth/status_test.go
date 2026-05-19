// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

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
			"data": map[string]interface{}{
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
