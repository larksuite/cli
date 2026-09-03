// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/zalando/go-keyring"
)

func TestAuthStatusRun_SplitsBotAndUserIdentity(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "secret", Brand: core.BrandFeishu,
	})

	if err := authStatusRun(&StatusOptions{Factory: f}, nil); err != nil {
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

	if err := authStatusRun(&StatusOptions{Factory: f, Verify: true}, nil); err != nil {
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

func TestAuthStatusRun_ReportsCorruptStoredToken(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_DATA_DIR", t.TempDir())

	config := &core.CliConfig{
		AppID:      "cli_status_corrupt",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_status_corrupt",
		UserName:   "Corrupt Token Fixture",
	}

	const secret = "sensitive-status-token"
	malformed := `{"accessToken":"` + secret + `",`
	account := config.AppID + ":" + config.UserOpenId
	if err := keychain.Set(keychain.LarkCliService, account, malformed); err != nil {
		t.Fatalf("keychain.Set() error = %v", err)
	}

	f, stdout, stderr, _ := cmdutil.TestFactory(t, config)
	if err := authStatusRun(&StatusOptions{Factory: f, Verify: true}, nil); err != nil {
		t.Fatalf("authStatusRun() corrupt-token error = %v", err)
	}
	var got statusOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() corrupt-token error = %v\nstdout=%s", err, stdout.String())
	}
	if got.Identities.User.Status != "error" || got.Identities.User.Available {
		t.Fatalf("corrupt user = %#v, want error and unavailable", got.Identities.User)
	}
	problem := got.Identities.User.Error
	if problem == nil || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeStorage {
		t.Fatalf("corrupt user error = %#v, want internal/storage", problem)
	}
	if !strings.Contains(problem.Message, "failed to decode stored token") {
		t.Fatalf("corrupt user error message = %q, want decode failure", problem.Message)
	}
	if !strings.Contains(got.Note, "failed to decode stored token") {
		t.Fatalf("note = %q, want diagnostic storage error", got.Note)
	}
	if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
		t.Fatalf("auth status leaked credential content\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want status result entirely on stdout", stderr.String())
	}
}

type statusOutput struct {
	Identity   string `json:"identity"`
	Verified   *bool  `json:"verified"`
	Note       string `json:"note"`
	Identities struct {
		Bot  statusIdentity `json:"bot"`
		User statusIdentity `json:"user"`
	} `json:"identities"`
}

type statusIdentity struct {
	Status    string        `json:"status"`
	Available bool          `json:"available"`
	Verified  *bool         `json:"verified"`
	OpenID    string        `json:"openId"`
	Error     *errs.Problem `json:"error"`
}
