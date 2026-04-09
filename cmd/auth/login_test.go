// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/zalando/go-keyring"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestSuggestDomain_PrefixMatch(t *testing.T) {
	known := map[string]bool{
		"calendar": true,
		"task":     true,
		"drive":    true,
		"im":       true,
	}

	// Input is prefix of known domain
	if s := suggestDomain("cal", known); s != "calendar" {
		t.Errorf("expected 'calendar', got %q", s)
	}

	// Known domain is prefix of input
	if s := suggestDomain("calendar_extra", known); s != "calendar" {
		t.Errorf("expected 'calendar', got %q", s)
	}
}

func TestSuggestDomain_NoMatch(t *testing.T) {
	known := map[string]bool{
		"calendar": true,
		"task":     true,
	}

	if s := suggestDomain("zzz", known); s != "" {
		t.Errorf("expected empty suggestion, got %q", s)
	}
}

func TestSuggestDomain_ExactMatch(t *testing.T) {
	known := map[string]bool{
		"calendar": true,
	}

	// Exact match: input is prefix of known AND known is prefix of input
	if s := suggestDomain("calendar", known); s != "calendar" {
		t.Errorf("expected 'calendar', got %q", s)
	}
}

func TestShortcutSupportsIdentity_DefaultUser(t *testing.T) {
	// Empty AuthTypes defaults to ["user"]
	sc := common.Shortcut{AuthTypes: nil}
	if !shortcutSupportsIdentity(sc, "user") {
		t.Error("expected default to support 'user'")
	}
	if shortcutSupportsIdentity(sc, "bot") {
		t.Error("expected default to NOT support 'bot'")
	}
}

func TestShortcutSupportsIdentity_ExplicitTypes(t *testing.T) {
	sc := common.Shortcut{AuthTypes: []string{"user", "bot"}}
	if !shortcutSupportsIdentity(sc, "user") {
		t.Error("expected to support 'user'")
	}
	if !shortcutSupportsIdentity(sc, "bot") {
		t.Error("expected to support 'bot'")
	}
	if shortcutSupportsIdentity(sc, "tenant") {
		t.Error("expected to NOT support 'tenant'")
	}
}

func TestShortcutSupportsIdentity_BotOnly(t *testing.T) {
	sc := common.Shortcut{AuthTypes: []string{"bot"}}
	if shortcutSupportsIdentity(sc, "user") {
		t.Error("expected bot-only to NOT support 'user'")
	}
	if !shortcutSupportsIdentity(sc, "bot") {
		t.Error("expected bot-only to support 'bot'")
	}
}

func TestCompleteDomain(t *testing.T) {
	projects := registry.ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	// Complete from empty prefix
	completions := completeDomain("")
	if len(completions) == 0 {
		t.Fatal("expected completions for empty prefix")
	}
	// All completions should match from_meta projects
	if len(completions) != len(projects) {
		t.Errorf("expected %d completions, got %d", len(projects), len(completions))
	}

	// Complete with partial prefix
	completions = completeDomain("cal")
	for _, c := range completions {
		if c != "calendar" && c[:3] != "cal" {
			t.Errorf("unexpected completion %q for prefix 'cal'", c)
		}
	}
}

func TestCompleteDomain_CommaSeparated(t *testing.T) {
	projects := registry.ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	// After a comma, should complete the next segment
	completions := completeDomain("calendar,")
	for _, c := range completions {
		if c[:9] != "calendar," {
			t.Errorf("expected 'calendar,' prefix, got %q", c)
		}
	}
}

func TestAllKnownDomains(t *testing.T) {
	domains := allKnownDomains()
	if len(domains) == 0 {
		t.Fatal("expected non-empty known domains")
	}

	// Should include from_meta projects
	for _, p := range registry.ListFromMetaProjects() {
		if !domains[p] {
			t.Errorf("expected from_meta project %q in known domains", p)
		}
	}
}

func TestSortedKnownDomains(t *testing.T) {
	sorted := sortedKnownDomains()
	if len(sorted) == 0 {
		t.Fatal("expected non-empty sorted domains")
	}

	if !sort.StringsAreSorted(sorted) {
		t.Error("expected sorted result")
	}

	// Should match allKnownDomains
	known := allKnownDomains()
	if len(sorted) != len(known) {
		t.Errorf("sorted (%d) and known (%d) length mismatch", len(sorted), len(known))
	}
}

func TestGetShortcutOnlyDomainNames_HaveDescriptions(t *testing.T) {
	for _, name := range getShortcutOnlyDomainNames() {
		zhDesc := registry.GetServiceDescription(name, "zh")
		enDesc := registry.GetServiceDescription(name, "en")
		if zhDesc == "" {
			t.Errorf("missing zh description for shortcut-only domain %q", name)
		}
		if enDesc == "" {
			t.Errorf("missing en description for shortcut-only domain %q", name)
		}
	}
}

func TestCollectScopesForDomains(t *testing.T) {
	projects := registry.ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	scopes := collectScopesForDomains([]string{"calendar"}, "user")
	if len(scopes) == 0 {
		t.Fatal("expected non-empty scopes for calendar domain")
	}

	// Should be sorted
	if !sort.StringsAreSorted(scopes) {
		t.Error("expected sorted result")
	}

	// Should include at least the API scopes
	apiScopes := registry.CollectScopesForProjects([]string{"calendar"}, "user")
	for _, s := range apiScopes {
		found := false
		for _, cs := range scopes {
			if cs == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("API scope %q missing from collectScopesForDomains result", s)
		}
	}
}

func TestCollectScopesForDomains_NonexistentDomain(t *testing.T) {
	scopes := collectScopesForDomains([]string{"nonexistent_domain_xyz"}, "user")
	if len(scopes) != 0 {
		t.Errorf("expected empty scopes for nonexistent domain, got %d", len(scopes))
	}
}

func TestGetDomainMetadata_IncludesFromMeta(t *testing.T) {
	domains := getDomainMetadata("zh")
	nameSet := make(map[string]bool)
	for _, dm := range domains {
		nameSet[dm.Name] = true
	}

	// from_meta projects must be present
	for _, p := range registry.ListFromMetaProjects() {
		if !nameSet[p] {
			t.Errorf("from_meta project %q missing from getDomainMetadata", p)
		}
	}
}

func TestGetDomainMetadata_IncludesShortcutOnlyDomains(t *testing.T) {
	domains := getDomainMetadata("zh")
	nameSet := make(map[string]bool)
	for _, dm := range domains {
		nameSet[dm.Name] = true
	}

	for _, name := range getShortcutOnlyDomainNames() {
		if !nameSet[name] {
			t.Errorf("shortcut-only domain %q missing from getDomainMetadata", name)
		}
	}
}

func TestGetDomainMetadata_Sorted(t *testing.T) {
	domains := getDomainMetadata("zh")
	for i := 1; i < len(domains); i++ {
		if domains[i].Name < domains[i-1].Name {
			t.Errorf("not sorted: %q before %q", domains[i-1].Name, domains[i].Name)
		}
	}
}

func TestGetDomainMetadata_HasTitleAndDescription(t *testing.T) {
	domains := getDomainMetadata("zh")
	for _, dm := range domains {
		if dm.Title == "" {
			t.Errorf("domain %q has empty Title", dm.Name)
		}
	}
}

func TestAuthLoginRun_NonTerminal_NoFlags_RejectsWithHint(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	// TestFactory has IsTerminal=false by default
	opts := &LoginOptions{Factory: f, Ctx: context.Background()}
	err := authLoginRun(opts)
	if err == nil {
		t.Fatal("expected error for non-terminal without flags")
	}
	// Should mention specifying scopes
	msg := err.Error()
	if !strings.Contains(msg, "scopes") {
		t.Errorf("expected error to mention scopes, got: %s", msg)
	}
	// Stderr should contain background hint
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "background") {
		t.Errorf("expected stderr to mention background, got: %s", stderrStr)
	}
}

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

func TestWriteLoginSuccess_JSONIncludesScopeDiff(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	writeLoginSuccess(&LoginOptions{JSON: true}, getLoginMsg("en"), f, "ou_user", "tester", &loginScopeSummary{
		Requested:      []string{"im:message:send", "im:message:reply"},
		NewlyGranted:   []string{"im:message:send"},
		AlreadyGranted: []string{"im:message:reply"},
		Granted:        []string{"im:message:send", "im:message:reply"},
	})

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
}

func TestHandleLoginScopeIssue_NonJSONAlignsWithLoginSuccess(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	err := handleLoginScopeIssue(&LoginOptions{}, getLoginMsg("zh"), f, &loginScopeIssue{
		Message: "授权完成，但以下请求 scopes 未被授予: im:message:send",
		Hint:    "以上结果是本次授权请求用户最终确认后的结果，请勿持续重试；Scopes 未授予的原因是多样的，如 scope 被禁用；具体原因已通过授权页提示用户。可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
		Summary: &loginScopeSummary{
			Requested: []string{"im:message:send"},
			Missing:   []string{"im:message:send"},
			Granted:   []string{"base:app:copy"},
		},
	}, "ou_user", "tester")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	got := stderr.String()
	for _, want := range []string{
		"OK: 登录成功! 用户: tester (ou_user)",
		"授权完成，但以下请求 scopes 未被授予: im:message:send",
		"本次请求 scopes: im:message:send",
		"本次新授予 scopes: （空）",
		"本次未授予 scopes: im:message:send",
		"以上结果是本次授权请求用户最终确认后的结果，请勿持续重试",
		"scope 被禁用",
		"lark-cli auth status",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "最终已授权 scopes:") {
		t.Fatalf("stderr should not contain final granted scopes, got:\n%s", got)
	}
	if strings.Contains(got, "ERROR:") {
		t.Fatalf("stderr should not contain error prefix, got:\n%s", got)
	}
}

func TestHandleLoginScopeIssue_JSONAlignsWithLoginSuccess(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := handleLoginScopeIssue(&LoginOptions{JSON: true}, getLoginMsg("en"), f, &loginScopeIssue{
		Message: "authorization completed, but these requested scopes were not granted: im:message:send",
		Hint:    "Granted scopes: base:app:copy. Check app scopes.",
		Summary: &loginScopeSummary{
			Requested: []string{"im:message:send"},
			Missing:   []string{"im:message:send"},
			Granted:   []string{"base:app:copy"},
		},
	}, "ou_user", "tester")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
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
				"本次请求 scopes: im:message:send im:message:reply",
				"本次新授予 scopes: im:message:send",
				"本次未授予 scopes: （空）",
				"可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
			},
			expectedAbsent: []string{
				"最终已授权 scopes:",
				"已有 scopes:",
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
				"本次请求 scopes: im:message:send",
				"本次新授予 scopes: （空）",
				"本次未授予 scopes: （空）",
				"可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
			},
			expectedAbsent: []string{
				"最终已授权 scopes:",
				"已有 scopes:",
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
				"本次请求 scopes: im:message:send im:message:reply",
				"本次新授予 scopes: （空）",
				"本次未授予 scopes: im:message:send",
			},
			expectedAbsent: []string{
				"已有 scopes:",
				"最终已授权 scopes:",
				"可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
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

func TestAuthLoginRun_MissingRequestedScopeAlignsWithLoginSuccess(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	t.Setenv("HOME", t.TempDir())

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
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	got := stderr.String()
	for _, want := range []string{
		"OK: 登录成功! 用户: tester (ou_user)",
		"授权完成，但以下请求 scopes 未被授予: im:message:send",
		"本次请求 scopes: im:message:send",
		"本次未授予 scopes: im:message:send",
		"以上结果是本次授权请求用户最终确认后的结果，请勿持续重试",
		"scope 被禁用",
		"lark-cli auth status",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "最终已授权 scopes:") {
		t.Fatalf("stderr should not contain final granted scopes, got:\n%s", got)
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

func TestAuthLoginRun_DeviceCodeUsesCachedRequestedScopes(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	t.Setenv("HOME", t.TempDir())

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
	})
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
	})
	if err != nil {
		t.Fatalf("device-code authLoginRun() error = %v", err)
	}
	got := stderr.String()
	for _, want := range []string{
		"OK: 登录成功! 用户: tester (ou_user)",
		"本次请求 scopes: im:message:send",
		"本次新授予 scopes: im:message:send",
		"可执行 `lark-cli auth status` 查看账号当前已授予的全部 scopes；",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "最终已授权 scopes:") {
		t.Fatalf("stderr should not contain final granted scopes, got:\n%s", got)
	}
	if got, err := loadLoginRequestedScope("device-code"); err != nil || got != "" {
		t.Fatalf("loadLoginRequestedScope() after cleanup = (%q, %v), want empty", got, err)
	}
}

func TestWriteLoginSuccess_TextOutputEnglishIncludesStatusHintWhenNoMissingScopes(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)

	writeLoginSuccess(&LoginOptions{}, getLoginMsg("en"), f, "ou_user", "tester", &loginScopeSummary{
		Requested:    []string{"im:message:send"},
		NewlyGranted: []string{"im:message:send"},
		Granted:      []string{"im:message:send"},
	})

	got := stderr.String()
	for _, want := range []string{
		"Login successful! User: tester (ou_user)",
		"Requested scopes: im:message:send",
		"Newly granted scopes: im:message:send",
		"Not granted scopes: (none)",
		"Run `lark-cli auth status` to inspect all scopes currently granted to the account.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q, got:\n%s", want, got)
		}
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
	})
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

func TestAuthLoginRun_JSONWriteFailure_NoWaitReturnsWriterError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})
	f.IOStreams.Out = failWriter{}

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  5,
		},
	})

	err := authLoginRun(&LoginOptions{
		Factory: f,
		Ctx:     context.Background(),
		Scope:   "im:message:send",
		NoWait:  true,
		JSON:    true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to write JSON output") {
		t.Fatalf("error = %v, want JSON write failure", err)
	}
}

func TestAuthLoginRun_JSONWriteFailure_DeviceAuthorizationReturnsWriterError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})
	f.IOStreams.Out = failWriter{}

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  5,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := authLoginRun(&LoginOptions{
		Factory: f,
		Ctx:     ctx,
		Scope:   "im:message:send",
		JSON:    true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to write JSON output") {
		t.Fatalf("error = %v, want JSON write failure", err)
	}
}

func TestGetDomainMetadata_ExcludesEvent(t *testing.T) {
	domains := getDomainMetadata("zh")
	for _, dm := range domains {
		if dm.Name == "event" {
			t.Error("event should not appear in interactive domain list")
		}
	}
}
