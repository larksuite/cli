// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newSearchUserTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("query", "", "")
	cmd.Flags().String("user-ids", "", "")
	cmd.Flags().Bool("is-resigned", false, "")
	cmd.Flags().Bool("has-chatted", false, "")
	cmd.Flags().Bool("exclude-outer-contact", false, "")
	cmd.Flags().Bool("has-enterprise-email", false, "")
	cmd.Flags().String("lang", "", "")
	cmd.Flags().String("page-size", "20", "")
	cmd.Flags().Bool("page-all", false, "")
	cmd.Flags().Int("page-limit", 20, "")
	return cmd
}

func searchUserDefaultConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "test", AppSecret: "test", Brand: core.BrandFeishu,
		UserOpenId: "ou_self",
	}
}

func TestPickName_ExplicitLang_Hit(t *testing.T) {
	meta := map[string]interface{}{
		"i18n_names": map[string]interface{}{"zh_cn": "张三", "en_us": "Zhangsan"},
	}
	got := pickName(meta, "en-US", core.BrandFeishu, "ou_x")
	if got != "Zhangsan" {
		t.Errorf("got %q, want Zhangsan", got)
	}
}

func TestPickName_ExplicitLang_MissFallsToBrand(t *testing.T) {
	meta := map[string]interface{}{
		"i18n_names": map[string]interface{}{"zh_cn": "张三"},
	}
	got := pickName(meta, "ja-JP", core.BrandFeishu, "ou_x")
	if got != "张三" {
		t.Errorf("got %q, want 张三 (brand fallback)", got)
	}
}

func TestPickName_BrandFeishu_PicksZh(t *testing.T) {
	meta := map[string]interface{}{
		"i18n_names": map[string]interface{}{"zh_cn": "张三", "en_us": "Zhangsan"},
	}
	got := pickName(meta, "", core.BrandFeishu, "ou_x")
	if got != "张三" {
		t.Errorf("got %q, want 张三", got)
	}
}

func TestPickName_BrandLark_PicksEn(t *testing.T) {
	meta := map[string]interface{}{
		"i18n_names": map[string]interface{}{"zh_cn": "张三", "en_us": "Zhangsan"},
	}
	got := pickName(meta, "", core.BrandLark, "ou_x")
	if got != "Zhangsan" {
		t.Errorf("got %q, want Zhangsan", got)
	}
}

func TestPickName_FixedLocaleList_HitJaJp(t *testing.T) {
	meta := map[string]interface{}{
		"i18n_names": map[string]interface{}{"ja_jp": "Yamada"},
	}
	got := pickName(meta, "", core.BrandFeishu, "ou_x")
	if got != "Yamada" {
		t.Errorf("got %q, want Yamada (fixed locale list fallback)", got)
	}
}

func TestPickName_DictOrderFallback(t *testing.T) {
	meta := map[string]interface{}{
		"i18n_names": map[string]interface{}{"xx_yy": "Foo", "aa_bb": "Bar"},
	}
	got := pickName(meta, "", core.BrandFeishu, "ou_x")
	if got != "Bar" {
		t.Errorf("got %q, want Bar (alphabetical tie-break, first non-empty is 'aa_bb')", got)
	}
}

func TestPickName_AllEmpty_FallsToOpenID(t *testing.T) {
	got := pickName(map[string]interface{}{}, "", core.BrandFeishu, "ou_x")
	if got != "ou_x" {
		t.Errorf("got %q, want ou_x", got)
	}
}

func TestPickName_Determinism(t *testing.T) {
	meta := map[string]interface{}{
		"i18n_names": map[string]interface{}{
			"xx_yy": "Foo", "aa_bb": "Bar", "mm_nn": "Baz",
		},
	}
	first := pickName(meta, "", core.BrandFeishu, "ou_x")
	for i := 0; i < 50; i++ {
		got := pickName(meta, "", core.BrandFeishu, "ou_x")
		if got != first {
			t.Fatalf("non-deterministic: iter %d got %q, expected %q (map iteration leaked)", i, got, first)
		}
	}
}

func TestRowFromItem_FullMapping(t *testing.T) {
	item := map[string]interface{}{
		"id": "ou_a",
		"meta_data": map[string]interface{}{
			"i18n_names":              map[string]interface{}{"zh_cn": "张三", "en_us": "Z"},
			"mail_address":            "z@example.com",
			"enterprise_mail_address": "z@corp.example.com",
			"is_registered":           true,
			"chat_id":                 "oc_abc",
			"is_cross_tenant":         false,
			"tenant_id":               "tenant_x",
			"description":             "Director / Marketing",
		},
		"display_info": "<h>张三</h>\nMarketing\n\n[Contacted 2 days ago]",
	}
	got := rowFromItem(item, "", core.BrandFeishu)

	checks := []struct {
		key  string
		want interface{}
	}{
		{"open_id", "ou_a"},
		{"name", "张三"},
		{"email", "z@example.com"},
		{"enterprise_email", "z@corp.example.com"},
		{"is_registered", true},
		{"chat_id", "oc_abc"},
		{"has_chatted", true},
		{"is_cross_tenant", false},
		{"tenant_id", "tenant_x"},
		{"description", "Director / Marketing"},
		{"display_info", "<h>张三</h>\nMarketing\n\n[Contacted 2 days ago]"},
	}
	for _, c := range checks {
		if got[c.key] != c.want {
			t.Errorf("key %q: got %v, want %v", c.key, got[c.key], c.want)
		}
	}
	i18n, ok := got["i18n_names"].(map[string]interface{})
	if !ok {
		t.Fatalf("i18n_names: expected map, got %T", got["i18n_names"])
	}
	if i18n["zh_cn"] != "张三" || i18n["en_us"] != "Z" {
		t.Errorf("i18n_names content: got %v", i18n)
	}
}

func TestRowFromItem_HasChattedFalseWhenChatIDEmpty(t *testing.T) {
	item := map[string]interface{}{
		"id":        "ou_a",
		"meta_data": map[string]interface{}{},
	}
	got := rowFromItem(item, "", core.BrandFeishu)
	if got["has_chatted"] != false {
		t.Errorf("has_chatted: got %v, want false", got["has_chatted"])
	}
	if got["chat_id"] != "" {
		t.Errorf("chat_id: got %q, want empty string", got["chat_id"])
	}
}

func TestRowFromItem_CrossTenantEmptyEmailNoPanic(t *testing.T) {
	item := map[string]interface{}{
		"id": "ou_outer",
		"meta_data": map[string]interface{}{
			"is_cross_tenant": true,
			"tenant_id":       "other_tenant",
		},
	}
	got := rowFromItem(item, "", core.BrandFeishu)
	if got["email"] != "" {
		t.Errorf("email: expected empty, got %q", got["email"])
	}
	if got["enterprise_email"] != "" {
		t.Errorf("enterprise_email: expected empty, got %q", got["enterprise_email"])
	}
}

func TestValidateSearchUser_AllEmpty_Errors(t *testing.T) {
	cmd := newSearchUserTestCommand()
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	err := validateSearchUser(rt)
	if err == nil || !strings.Contains(err.Error(), "specify at least one of") {
		t.Fatalf("expected AtLeastOne error, got %v", err)
	}
}

func TestValidateSearchUser_QueryOnly_OK(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("query", "hello")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	if err := validateSearchUser(rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSearchUser_FilterOnly_OK(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("has-chatted", "true")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	if err := validateSearchUser(rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSearchUser_QueryTooLong_Errors(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("query", strings.Repeat("a", 65))
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	err := validateSearchUser(rt)
	if err == nil || !strings.Contains(err.Error(), "64") {
		t.Fatalf("expected length error mentioning 64, got %v", err)
	}
}

func TestValidateSearchUser_Query64Runes_OK(t *testing.T) {
	cmd := newSearchUserTestCommand()
	q := strings.Repeat("中", 32) + strings.Repeat("a", 32) // 64 rune, >64 bytes
	if utf8.RuneCountInString(q) != 64 {
		t.Fatalf("test string is %d runes, expected 64", utf8.RuneCountInString(q))
	}
	_ = cmd.Flags().Set("query", q)
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	if err := validateSearchUser(rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSearchUser_UserIDsOver100_Errors(t *testing.T) {
	cmd := newSearchUserTestCommand()
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = fmt.Sprintf("ou_%05d", i)
	}
	_ = cmd.Flags().Set("user-ids", strings.Join(ids, ","))
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	err := validateSearchUser(rt)
	if err == nil || !strings.Contains(err.Error(), "100") {
		t.Fatalf("expected 100-cap error, got %v", err)
	}
}

func TestValidateSearchUser_UserIDsBadPrefix_Errors(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("user-ids", "foo")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	err := validateSearchUser(rt)
	if err == nil || !strings.Contains(err.Error(), "ou_") {
		t.Fatalf("expected ou_ prefix error, got %v", err)
	}
}

func TestValidateSearchUser_MeWithoutLogin_Errors(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("user-ids", "me")
	cfg := searchUserDefaultConfig()
	cfg.UserOpenId = ""
	rt := common.TestNewRuntimeContext(cmd, cfg)
	err := validateSearchUser(rt)
	if err == nil || !strings.Contains(err.Error(), "me") {
		t.Fatalf("expected 'me without login' error, got %v", err)
	}
}

func TestBuildBody_QueryOnly(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("query", "hello")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	body, err := buildSearchUserBody(rt)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if body["query"] != "hello" {
		t.Errorf("query: got %v", body["query"])
	}
	if _, ok := body["filter"]; ok {
		t.Errorf("filter: should not exist when no filter set, got %v", body["filter"])
	}
}

func TestBuildBody_BoolTriState_NotSet_Omitted(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("query", "x")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	body, _ := buildSearchUserBody(rt)
	if _, ok := body["filter"]; ok {
		t.Errorf("filter: should be omitted when no bool changed, got %v", body["filter"])
	}
}

func TestBuildBody_BoolTriState_True(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("query", "x")
	_ = cmd.Flags().Set("has-chatted", "true")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	body, _ := buildSearchUserBody(rt)
	filter, ok := body["filter"].(map[string]interface{})
	if !ok {
		t.Fatalf("filter: expected map, got %v", body["filter"])
	}
	if filter["has_contact"] != true {
		t.Errorf("filter.has_contact: got %v, want true", filter["has_contact"])
	}
}

func TestBuildBody_BoolTriState_False(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("query", "x")
	_ = cmd.Flags().Set("has-chatted", "false")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	body, _ := buildSearchUserBody(rt)
	filter, ok := body["filter"].(map[string]interface{})
	if !ok {
		t.Fatalf("filter: expected map, got %v", body["filter"])
	}
	if filter["has_contact"] != false {
		t.Errorf("filter.has_contact: got %v, want false (distinct from omitted)", filter["has_contact"])
	}
}

func TestBuildBody_UserIDsResolveAndDedup(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("user-ids", "me,ou_a,me,ou_a")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	body, _ := buildSearchUserBody(rt)
	filter, ok := body["filter"].(map[string]interface{})
	if !ok {
		t.Fatalf("filter: expected map, got %v", body["filter"])
	}
	ids, _ := filter["user_ids"].([]string)
	if len(ids) != 2 || ids[0] != "ou_self" || ids[1] != "ou_a" {
		t.Errorf("user_ids: got %v, want [ou_self ou_a]", ids)
	}
}

func TestBuildParams_PageSize(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("query", "x")
	_ = cmd.Flags().Set("page-size", "25")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	params := buildSearchUserParams(rt)
	if params["page_size"] != 25 {
		t.Errorf("page_size: got %v, want 25", params["page_size"])
	}
	if _, ok := params["page_token"]; ok {
		t.Errorf("page_token: should never appear in initial params (managed by the pagination loop)")
	}
}

func TestBuildParams_DefaultPageSize(t *testing.T) {
	cmd := newSearchUserTestCommand()
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	params := buildSearchUserParams(rt)
	if params["page_size"] != 20 {
		t.Errorf("page_size: got %v, want 20 (default)", params["page_size"])
	}
}

func TestPaginationConfig_Defaults(t *testing.T) {
	cmd := newSearchUserTestCommand()
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	auto, limit := searchUserPaginationConfig(rt)
	if auto {
		t.Errorf("autoPaginate: want false when no flags set, got true")
	}
	if limit != defaultSearchUserPageLimit {
		t.Errorf("pageLimit: want %d default, got %d", defaultSearchUserPageLimit, limit)
	}
}

func TestPaginationConfig_PageAllAloneUsesMax(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("page-all", "true")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	auto, limit := searchUserPaginationConfig(rt)
	if !auto {
		t.Errorf("autoPaginate: want true, got false")
	}
	if limit != maxSearchUserPageLimit {
		t.Errorf("pageLimit: --page-all alone should use max %d, got %d", maxSearchUserPageLimit, limit)
	}
}

func TestPaginationConfig_PageLimitAloneImpliesAuto(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("page-limit", "5")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	auto, limit := searchUserPaginationConfig(rt)
	if !auto {
		t.Errorf("autoPaginate: --page-limit alone should imply auto, got false")
	}
	if limit != 5 {
		t.Errorf("pageLimit: got %d, want 5", limit)
	}
}

func TestPaginationConfig_PageLimitCapsAtMax(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("page-limit", "999")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	_, limit := searchUserPaginationConfig(rt)
	if limit != maxSearchUserPageLimit {
		t.Errorf("pageLimit: should cap at %d, got %d", maxSearchUserPageLimit, limit)
	}
}

// mountAndRun mounts the shortcut under a parent cobra command and runs it
// with the given args. Mirrors the pattern used in other shortcut packages
// (e.g. minutes_download_test.go).
func mountAndRun(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "contact"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func searchUserStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/contact/v3/users/search",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id": "ou_a",
						"meta_data": map[string]interface{}{
							"i18n_names":      map[string]interface{}{"zh_cn": "张三"},
							"mail_address":    "z@x.com",
							"is_registered":   true,
							"chat_id":         "oc_abc",
							"is_cross_tenant": false,
							"tenant_id":       "tenant_1",
							"description":     "Director",
						},
						"display_info": "<h>张三</h>\nMarketing",
					},
				},
				"has_more":   false,
				"page_token": "",
			},
		},
	}
}

func TestSearchUser_Integration_PrettyRendersAllColumns(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	reg.Register(searchUserStub())

	err := mountAndRun(t, ContactSearchUser, []string{"+search-user", "--query", "张三", "--format", "pretty", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := stdout.String()
	// internal/output sorts table columns alphabetically (house behavior shared
	// with all PrintTable callers), so we assert presence of each pretty column
	// rather than a specific layout.
	for _, col := range []string{"display_info", "name", "open_id", "email", "is_registered", "has_chatted"} {
		if !strings.Contains(out, col) {
			t.Errorf("pretty output missing column %q; got=%q", col, out)
		}
	}
	// The hit-highlight payload should make it into the rendered cell.
	if !strings.Contains(out, "<h>张三</h>") {
		t.Errorf("expected display_info hit-highlight in output, got=%q", out)
	}
}

func TestSearchUser_Integration_JSONFullFields(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	reg.Register(searchUserStub())

	err := mountAndRun(t, ContactSearchUser, []string{"+search-user", "--query", "张三", "--format", "json", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\noutput=%s", err, stdout.String())
	}
	// CLI wraps all structured output in an {ok, identity, data, meta} envelope.
	data, ok := got["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("envelope.data: expected object, got %v\nraw=%s", got["data"], stdout.String())
	}
	users, _ := data["users"].([]interface{})
	if len(users) != 1 {
		t.Fatalf("users: expected 1, got %d (output=%s)", len(users), stdout.String())
	}
	u, _ := users[0].(map[string]interface{})
	for _, k := range []string{
		"name", "open_id", "i18n_names", "email", "is_registered", "chat_id",
		"has_chatted", "is_cross_tenant", "tenant_id", "description", "display_info",
	} {
		if _, ok := u[k]; !ok {
			t.Errorf("missing JSON key %q in user object", k)
		}
	}
}

func TestSearchUser_Integration_NDJSONHasNoPaginationHint(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/contact/v3/users/search",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": "ou_a"}},
				"has_more":   true,
				"page_token": "tok_next",
			},
		},
	})

	err := mountAndRun(t, ContactSearchUser, []string{"+search-user", "--query", "x", "--format", "ndjson", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.Contains(stdout.String(), "more available") {
		t.Errorf("ndjson stdout must not contain the pagination hint (would corrupt the stream); got=%q", stdout.String())
	}
	if strings.Contains(stderr.String(), "more available") {
		t.Errorf("ndjson stderr must not contain the pagination hint either (non-human format opts out entirely); got=%q", stderr.String())
	}
}

func TestSearchUser_Integration_PrettyHintGoesToStderr(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/contact/v3/users/search",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": "ou_a"}},
				"has_more":   true,
				"page_token": "tok_next",
			},
		},
	})

	err := mountAndRun(t, ContactSearchUser, []string{"+search-user", "--query", "x", "--format", "pretty", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.Contains(stdout.String(), "more available") {
		t.Errorf("pretty stdout must not carry the hint (informational text belongs on stderr); got=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "more available") || !strings.Contains(stderr.String(), "--page-all") {
		t.Errorf("pretty stderr should guide the user toward --page-all when has_more=true; got=%q", stderr.String())
	}
}

func TestSearchUser_Integration_EmptyResult(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/contact/v3/users/search",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"items": []interface{}{}, "has_more": false, "page_token": ""},
		},
	})

	err := mountAndRun(t, ContactSearchUser, []string{"+search-user", "--query", "nope", "--format", "pretty", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(stdout.String(), "No users found.") {
		t.Errorf("expected 'No users found.' in output, got %q", stdout.String())
	}
}

func TestSearchUser_Integration_EmptyResult_JSONArray(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/contact/v3/users/search",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"items": []interface{}{}, "has_more": false, "page_token": ""},
		},
	})

	err := mountAndRun(t, ContactSearchUser, []string{"+search-user", "--query", "nope", "--format", "json", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\noutput=%s", err, stdout.String())
	}
	data, ok := got["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("envelope.data: expected object, got %v\nraw=%s", got["data"], stdout.String())
	}
	// Empty result must serialize as [] (not null) so jq consumers can iterate
	// without special-casing: `jq '.data.users[]'` / `.data.users | length`.
	usersRaw, exists := data["users"]
	if !exists {
		t.Fatalf("data.users key missing\nraw=%s", stdout.String())
	}
	if usersRaw == nil {
		t.Fatalf("data.users serialized as null; expected [] for empty result\nraw=%s", stdout.String())
	}
	users, ok := usersRaw.([]interface{})
	if !ok {
		t.Fatalf("data.users: expected []interface{}, got %T\nraw=%s", usersRaw, stdout.String())
	}
	if len(users) != 0 {
		t.Errorf("data.users: expected empty array, got %d entries", len(users))
	}
}

func TestSearchUser_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, searchUserDefaultConfig())

	err := mountAndRun(t, ContactSearchUser, []string{"+search-user", "--query", "x", "--has-chatted=true", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"POST", "/contact/v3/users/search", "has_contact"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run missing %q in output: %q", want, out)
		}
	}
}

func TestSearchUser_Integration_RequestShape(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	stub := searchUserStub()
	reg.Register(stub)

	err := mountAndRun(t, ContactSearchUser, []string{"+search-user", "--query", "x", "--has-chatted=true", "--user-ids", "me", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal request body: %v\nraw=%s", err, string(stub.CapturedBody))
	}
	if body["query"] != "x" {
		t.Errorf("body.query: got %v, want x", body["query"])
	}
	filter, _ := body["filter"].(map[string]interface{})
	if filter == nil {
		t.Fatalf("body.filter: expected object, got %v", body["filter"])
	}
	if filter["has_contact"] != true {
		t.Errorf("filter.has_contact: got %v, want true", filter["has_contact"])
	}
	uids, _ := filter["user_ids"].([]interface{})
	if len(uids) != 1 || uids[0] != "ou_self" {
		t.Errorf("filter.user_ids: got %v, want [ou_self]", filter["user_ids"])
	}
	// Unset bool filters must not appear in the body.
	for _, k := range []string{"is_resigned", "exclude_outer_contact", "has_enterprise_email"} {
		if _, ok := filter[k]; ok {
			t.Errorf("filter.%s: should be omitted (not Changed), got %v", k, filter[k])
		}
	}
}

func TestSearchUser_Integration_AutoPaginateAggregates(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	// First page: has_more=true, token=p2
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/contact/v3/users/search",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": "ou_a"}},
				"has_more":   true,
				"page_token": "p2",
			},
		},
	})
	// Second page: has_more=false, ends the loop
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/contact/v3/users/search",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": "ou_b"}},
				"has_more":   false,
				"page_token": "",
			},
		},
	})

	err := mountAndRun(t, ContactSearchUser, []string{"+search-user", "--query", "x", "--page-all", "--format", "json", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	reg.Verify(t) // ensures both stubs were consumed

	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\noutput=%s", err, stdout.String())
	}
	data, _ := got["data"].(map[string]interface{})
	users, _ := data["users"].([]interface{})
	if len(users) != 2 {
		t.Fatalf("users: want 2 aggregated across pages, got %d (output=%s)", len(users), stdout.String())
	}
	if data["has_more"] != false {
		t.Errorf("has_more: want false when last page ended, got %v", data["has_more"])
	}
}

func TestSearchUser_Integration_PageLimitTruncates(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	// Two pages; both return has_more=true — limit 2 should stop after the
	// second and emit a truncation warning.
	for i := 0; i < 2; i++ {
		reg.Register(&httpmock.Stub{
			Method: "POST", URL: "/open-apis/contact/v3/users/search",
			Body: map[string]interface{}{
				"code": 0, "msg": "ok",
				"data": map[string]interface{}{
					"items":      []interface{}{map[string]interface{}{"id": fmt.Sprintf("ou_%d", i)}},
					"has_more":   true,
					"page_token": fmt.Sprintf("p%d", i+1),
				},
			},
		})
	}

	err := mountAndRun(t, ContactSearchUser, []string{"+search-user", "--query", "x", "--page-limit", "2", "--format", "pretty", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(stderr.String(), "stopped after fetching 2 page(s)") {
		t.Errorf("expected truncation warning on stderr, got=%q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--page-limit") {
		t.Errorf("warning should mention --page-limit hint, got=%q", stderr.String())
	}
}

func TestValidateSearchUser_PageLimitOutOfRange_Errors(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("query", "x")
	_ = cmd.Flags().Set("page-limit", "100")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	err := validateSearchUser(rt)
	if err == nil || !strings.Contains(err.Error(), "40") {
		t.Fatalf("expected --page-limit range error mentioning 40, got %v", err)
	}
}
