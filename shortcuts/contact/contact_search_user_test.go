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
	cmd.Flags().Bool("left-organization", false, "")
	cmd.Flags().Bool("has-chatted", false, "")
	cmd.Flags().Bool("exclude-external-users", false, "")
	cmd.Flags().Bool("has-enterprise-email", false, "")
	cmd.Flags().String("lang", "", "")
	cmd.Flags().Int("page-size", 20, "")
	return cmd
}

func searchUserDefaultConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "test", AppSecret: "test", Brand: core.BrandFeishu,
		UserOpenId: "ou_self",
	}
}

func TestPickName_ExplicitLang_Hit(t *testing.T) {
	i18n := map[string]string{"zh_cn": "张三", "en_us": "Zhangsan"}
	got := pickName(i18n, "en-US", core.BrandFeishu, "ou_x")
	if got != "Zhangsan" {
		t.Errorf("got %q, want Zhangsan", got)
	}
}

func TestPickName_ExplicitLang_MissFallsToBrand(t *testing.T) {
	i18n := map[string]string{"zh_cn": "张三"}
	got := pickName(i18n, "ja-JP", core.BrandFeishu, "ou_x")
	if got != "张三" {
		t.Errorf("got %q, want 张三 (brand fallback)", got)
	}
}

func TestPickName_BrandFeishu_PicksZh(t *testing.T) {
	i18n := map[string]string{"zh_cn": "张三", "en_us": "Zhangsan"}
	got := pickName(i18n, "", core.BrandFeishu, "ou_x")
	if got != "张三" {
		t.Errorf("got %q, want 张三", got)
	}
}

func TestPickName_BrandLark_PicksEn(t *testing.T) {
	i18n := map[string]string{"zh_cn": "张三", "en_us": "Zhangsan"}
	got := pickName(i18n, "", core.BrandLark, "ou_x")
	if got != "Zhangsan" {
		t.Errorf("got %q, want Zhangsan", got)
	}
}

func TestPickName_FixedLocaleList_HitJaJp(t *testing.T) {
	i18n := map[string]string{"ja_jp": "Yamada"}
	got := pickName(i18n, "", core.BrandFeishu, "ou_x")
	if got != "Yamada" {
		t.Errorf("got %q, want Yamada (fixed locale list fallback)", got)
	}
}

func TestPickName_DictOrderFallback(t *testing.T) {
	i18n := map[string]string{"xx_yy": "Foo", "aa_bb": "Bar"}
	got := pickName(i18n, "", core.BrandFeishu, "ou_x")
	if got != "Bar" {
		t.Errorf("got %q, want Bar (alphabetical tie-break, first non-empty is 'aa_bb')", got)
	}
}

func TestPickName_AllEmpty_FallsToOpenID(t *testing.T) {
	got := pickName(map[string]string{}, "", core.BrandFeishu, "ou_x")
	if got != "ou_x" {
		t.Errorf("got %q, want ou_x", got)
	}
}

func TestPickName_Determinism(t *testing.T) {
	i18n := map[string]string{"xx_yy": "Foo", "aa_bb": "Bar", "mm_nn": "Baz"}
	first := pickName(i18n, "", core.BrandFeishu, "ou_x")
	for i := 0; i < 50; i++ {
		got := pickName(i18n, "", core.BrandFeishu, "ou_x")
		if got != first {
			t.Fatalf("non-deterministic: iter %d got %q, expected %q (map iteration leaked)", i, got, first)
		}
	}
}

func TestParseDisplayInfo_FullShape(t *testing.T) {
	raw := "<h>李海峰</h>\nLark Office Engineering-Intelligence-Search\n\n[Contacted 2 days ago]"
	segments, dept, recency := parseDisplayInfo(raw)
	if len(segments) != 1 || segments[0] != "李海峰" {
		t.Errorf("segments: got %v, want [李海峰]", segments)
	}
	if dept != "Lark Office Engineering-Intelligence-Search" {
		t.Errorf("department: got %q", dept)
	}
	if recency != "Contacted 2 days ago" {
		t.Errorf("chat_recency_hint: got %q", recency)
	}
}

func TestParseDisplayInfo_NoRecency(t *testing.T) {
	raw := "<h>张三</h>\nMarketing\n"
	segments, dept, recency := parseDisplayInfo(raw)
	if len(segments) != 1 || segments[0] != "张三" {
		t.Errorf("segments: got %v", segments)
	}
	if dept != "Marketing" {
		t.Errorf("department: got %q, want Marketing", dept)
	}
	if recency != "" {
		t.Errorf("chat_recency_hint: got %q, want empty", recency)
	}
}

func TestParseDisplayInfo_EmptyDept(t *testing.T) {
	raw := "<h>李海峰</h>\n\n"
	segments, dept, recency := parseDisplayInfo(raw)
	if len(segments) != 1 || segments[0] != "李海峰" {
		t.Errorf("segments: got %v", segments)
	}
	if dept != "" {
		t.Errorf("department: got %q, want empty", dept)
	}
	if recency != "" {
		t.Errorf("chat_recency_hint: got %q, want empty", recency)
	}
}

func TestParseDisplayInfo_MultipleHighlights(t *testing.T) {
	raw := "<h>ali</h>ce <h>wang</h>\nEng\n"
	segments, _, _ := parseDisplayInfo(raw)
	if len(segments) != 2 || segments[0] != "ali" || segments[1] != "wang" {
		t.Errorf("segments: got %v, want [ali wang]", segments)
	}
}

func TestParseDisplayInfo_Empty(t *testing.T) {
	segments, dept, recency := parseDisplayInfo("")
	if segments == nil {
		t.Errorf("segments: got nil, want empty (non-nil) slice")
	}
	if len(segments) != 0 {
		t.Errorf("segments: got %v, want empty", segments)
	}
	if dept != "" || recency != "" {
		t.Errorf("dept/recency: got %q / %q, want empty", dept, recency)
	}
}

func TestRowFromItem_FullMapping(t *testing.T) {
	item := &searchUserAPIItem{
		ID:          "ou_a",
		DisplayInfo: "<h>张三</h>\nMarketing\n\n[Contacted 2 days ago]",
		MetaData: searchUserAPIMeta{
			I18nNames:             map[string]string{"zh_cn": "张三", "en_us": "Z"},
			MailAddress:           "z@example.com",
			EnterpriseMailAddress: "z@corp.example.com",
			IsRegistered:          true,
			ChatID:                "oc_abc",
			IsCrossTenant:         false,
			Description:           "Coffee fanatic ☕",
		},
	}
	got := rowFromItem(item, "", core.BrandFeishu)

	if got.OpenID != "ou_a" {
		t.Errorf("OpenID: got %q, want ou_a", got.OpenID)
	}
	if got.LocalizedName != "张三" {
		t.Errorf("LocalizedName: got %q, want 张三", got.LocalizedName)
	}
	if got.Email != "z@example.com" {
		t.Errorf("Email: got %q", got.Email)
	}
	if got.EnterpriseEmail != "z@corp.example.com" {
		t.Errorf("EnterpriseEmail: got %q", got.EnterpriseEmail)
	}
	if !got.IsActivated {
		t.Errorf("IsActivated: got false, want true")
	}
	if got.P2PChatID != "oc_abc" {
		t.Errorf("P2PChatID: got %q", got.P2PChatID)
	}
	if !got.HasChatted {
		t.Errorf("HasChatted: got false, want true")
	}
	if got.IsCrossTenant {
		t.Errorf("IsCrossTenant: got true, want false")
	}
	if got.Department != "Marketing" {
		t.Errorf("Department: got %q", got.Department)
	}
	if got.Signature != "Coffee fanatic ☕" {
		t.Errorf("Signature: got %q (must come from meta.description)", got.Signature)
	}
	if got.ChatRecencyHint != "Contacted 2 days ago" {
		t.Errorf("ChatRecencyHint: got %q", got.ChatRecencyHint)
	}
	if len(got.MatchSegments) != 1 || got.MatchSegments[0] != "张三" {
		t.Errorf("MatchSegments: got %v", got.MatchSegments)
	}
}

func TestRowFromItem_HasChattedFalseWhenChatIDEmpty(t *testing.T) {
	item := &searchUserAPIItem{ID: "ou_a"}
	got := rowFromItem(item, "", core.BrandFeishu)
	if got.HasChatted {
		t.Errorf("HasChatted: got true, want false")
	}
	if got.P2PChatID != "" {
		t.Errorf("P2PChatID: got %q, want empty", got.P2PChatID)
	}
}

func TestRowFromItem_CrossTenantEmptyEmailNoPanic(t *testing.T) {
	item := &searchUserAPIItem{
		ID: "ou_outer",
		MetaData: searchUserAPIMeta{
			IsCrossTenant: true,
		},
	}
	got := rowFromItem(item, "", core.BrandFeishu)
	if got.Email != "" {
		t.Errorf("Email: expected empty, got %q", got.Email)
	}
	if got.EnterpriseEmail != "" {
		t.Errorf("EnterpriseEmail: expected empty, got %q", got.EnterpriseEmail)
	}
}

func TestValidateSearchUser_AllEmpty_Errors(t *testing.T) {
	cmd := newSearchUserTestCommand()
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	err := validateSearchUser(rt)
	if err == nil || !strings.Contains(err.Error(), "specify at least one of") {
		t.Fatalf("expected AtLeastOne error, got %v", err)
	}
	// Error message must list the new flag names so agents see the right hints.
	for _, want := range []string{"--query", "--user-ids", "--has-chatted", "--exclude-external-users", "--left-organization"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message missing %q; got %v", want, err)
		}
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
	_ = cmd.Flags().Set("query", strings.Repeat("a", 51))
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	err := validateSearchUser(rt)
	if err == nil || !strings.Contains(err.Error(), "50") {
		t.Fatalf("expected length error mentioning 50, got %v", err)
	}
}

func TestValidateSearchUser_Query50Chars_OK(t *testing.T) {
	cmd := newSearchUserTestCommand()
	q := strings.Repeat("中", 25) + strings.Repeat("a", 25) // 50 runes, >50 bytes
	if utf8.RuneCountInString(q) != 50 {
		t.Fatalf("test string is %d runes, expected 50", utf8.RuneCountInString(q))
	}
	_ = cmd.Flags().Set("query", q)
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	if err := validateSearchUser(rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Regression: prior versions accepted "--user-ids ',,,'" because SplitCSV
// drops empty segments and validation only capped the upper bound, sending
// an empty body to the API.
func TestValidateSearchUser_UserIDsAllSeparators_Errors(t *testing.T) {
	for _, raw := range []string{",,,", " , , ", ","} {
		cmd := newSearchUserTestCommand()
		_ = cmd.Flags().Set("user-ids", raw)
		rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
		err := validateSearchUser(rt)
		if err == nil || !strings.Contains(err.Error(), "--user-ids") {
			t.Fatalf("raw=%q: expected --user-ids error, got %v", raw, err)
		}
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

// =false on any bool filter must fail validation. Agents passing =false almost
// always mean "do not filter", but the API treats it as "must NOT match";
// silent acceptance produces wrong results. Hard reject up front.
func TestValidateSearchUser_BoolFalse_Rejected(t *testing.T) {
	cases := []string{"has-chatted", "has-enterprise-email", "exclude-external-users", "left-organization"}
	for _, flag := range cases {
		t.Run(flag, func(t *testing.T) {
			cmd := newSearchUserTestCommand()
			_ = cmd.Flags().Set("query", "x")
			_ = cmd.Flags().Set(flag, "false")
			rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
			err := validateSearchUser(rt)
			if err == nil || !strings.Contains(err.Error(), "--"+flag) {
				t.Fatalf("expected rejection mentioning --%s, got %v", flag, err)
			}
			if !strings.Contains(err.Error(), "=false is rejected") {
				t.Errorf("error should explain why =false is rejected; got %v", err)
			}
		})
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
	if body.Query != "hello" {
		t.Errorf("Query: got %q, want hello", body.Query)
	}
	if body.Filter != nil {
		t.Errorf("Filter: should be nil when no filter set, got %+v", body.Filter)
	}
}

func TestBuildBody_BoolNotSet_Omitted(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("query", "x")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	body, _ := buildSearchUserBody(rt)
	if body.Filter != nil {
		t.Errorf("Filter: should be nil when no bool changed, got %+v", body.Filter)
	}
}

func TestBuildBody_BoolTrue_MapsToAPI(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("query", "x")
	_ = cmd.Flags().Set("has-chatted", "true")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	body, _ := buildSearchUserBody(rt)
	if body.Filter == nil {
		t.Fatalf("Filter: expected non-nil")
	}
	// Flag --has-chatted must map to API field has_contact (the rename
	// rationale lives in searchUserBoolFilters).
	if !body.Filter.HasContact {
		t.Errorf("Filter.HasContact: got false, want true")
	}
}

func TestBuildBody_BoolFalse_NotInBody(t *testing.T) {
	// Validate rejects =false up front, but buildSearchUserBody must also
	// defensively skip it (in case a code path bypasses Validate).
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("query", "x")
	_ = cmd.Flags().Set("has-chatted", "false")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	body, _ := buildSearchUserBody(rt)
	if body.Filter != nil && body.Filter.HasContact {
		t.Errorf("Filter.HasContact: must stay false when flag is =false")
	}
}

func TestBuildBody_RenamedFlagsMapToAPI(t *testing.T) {
	// Each renamed CLI flag must still hit its original API field. Using a
	// getter closure keeps the assertion compile-time typed against the
	// searchUserAPIFilter struct.
	cases := []struct {
		flag string
		get  func(*searchUserAPIFilter) bool
	}{
		{"has-chatted", func(f *searchUserAPIFilter) bool { return f.HasContact }},
		{"exclude-external-users", func(f *searchUserAPIFilter) bool { return f.ExcludeOuterContact }},
		{"left-organization", func(f *searchUserAPIFilter) bool { return f.IsResigned }},
		{"has-enterprise-email", func(f *searchUserAPIFilter) bool { return f.HasEnterpriseEmail }},
	}
	for _, c := range cases {
		t.Run(c.flag, func(t *testing.T) {
			cmd := newSearchUserTestCommand()
			_ = cmd.Flags().Set("query", "x")
			_ = cmd.Flags().Set(c.flag, "true")
			rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
			body, _ := buildSearchUserBody(rt)
			if body.Filter == nil || !c.get(body.Filter) {
				t.Errorf("--%s did not set the corresponding filter field; body.Filter=%+v", c.flag, body.Filter)
			}
		})
	}
}

func TestBuildBody_UserIDsResolveAndDedup(t *testing.T) {
	cmd := newSearchUserTestCommand()
	_ = cmd.Flags().Set("user-ids", "me,ou_a,me,ou_a")
	rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
	body, _ := buildSearchUserBody(rt)
	if body.Filter == nil {
		t.Fatalf("Filter: expected non-nil")
	}
	ids := body.Filter.UserIDs
	if len(ids) != 2 || ids[0] != "ou_self" || ids[1] != "ou_a" {
		t.Errorf("UserIDs: got %v, want [ou_self ou_a]", ids)
	}
}

func TestValidateSearchUser_PageSizeOutOfRange_Errors(t *testing.T) {
	for _, n := range []int{0, 31} {
		cmd := newSearchUserTestCommand()
		_ = cmd.Flags().Set("query", "x")
		_ = cmd.Flags().Set("page-size", fmt.Sprintf("%d", n))
		rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
		err := validateSearchUser(rt)
		if err == nil || !strings.Contains(err.Error(), "page-size") {
			t.Errorf("page-size=%d: expected range error, got %v", n, err)
		}
	}
}

func TestValidateSearchUser_PageSizeBoundaries_OK(t *testing.T) {
	for _, n := range []int{1, 30} {
		cmd := newSearchUserTestCommand()
		_ = cmd.Flags().Set("query", "x")
		_ = cmd.Flags().Set("page-size", fmt.Sprintf("%d", n))
		rt := common.TestNewRuntimeContext(cmd, searchUserDefaultConfig())
		if err := validateSearchUser(rt); err != nil {
			t.Errorf("page-size=%d: unexpected error %v", n, err)
		}
	}
}

// mountAndRun mounts the shortcut under a parent cobra command and runs it
// with the given args. Mirrors the pattern used in other shortcut packages.
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
							"description":     "Coffee fanatic ☕",
						},
						"display_info": "<h>张三</h>\nMarketing\n\n[Contacted 2 days ago]",
					},
				},
				"has_more":   false,
				"page_token": "",
			},
		},
	}
}

func TestSearchUser_Integration_PrettyRendersExpectedColumns(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	reg.Register(searchUserStub())

	err := mountAndRun(t, ContactSearchUser, []string{"+search-user", "--query", "张三", "--format", "pretty", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := stdout.String()
	for _, col := range []string{"localized_name", "department", "open_id", "enterprise_email", "has_chatted", "chat_recency_hint"} {
		if !strings.Contains(out, col) {
			t.Errorf("pretty output missing column %q; got=%q", col, out)
		}
	}
	// department is the disambiguation field — must reach the rendered cell.
	if !strings.Contains(out, "Marketing") {
		t.Errorf("expected department 'Marketing' in pretty output, got=%q", out)
	}
	// Legacy column must be gone.
	if strings.Contains(out, "display_info ") || strings.Contains(out, "| display_info") {
		t.Errorf("legacy 'display_info' column must not appear; got=%q", out)
	}
}

func TestSearchUser_Integration_JSONStructuredFields(t *testing.T) {
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
	data, ok := got["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("envelope.data: expected object, got %v\nraw=%s", got["data"], stdout.String())
	}
	users, _ := data["users"].([]interface{})
	if len(users) != 1 {
		t.Fatalf("users: expected 1, got %d (output=%s)", len(users), stdout.String())
	}
	u, _ := users[0].(map[string]interface{})

	// New schema keys must all be present.
	for _, k := range []string{
		"open_id", "localized_name", "email", "enterprise_email",
		"is_activated", "is_cross_tenant",
		"p2p_chat_id", "has_chatted",
		"department", "signature", "chat_recency_hint", "match_segments",
	} {
		if _, ok := u[k]; !ok {
			t.Errorf("missing JSON key %q in user object", k)
		}
	}
	// Legacy keys must be gone.
	for _, k := range []string{"name", "chat_id", "is_registered", "description", "display_info", "display_info_raw", "match_rank", "tenant_id", "i18n_names"} {
		if _, ok := u[k]; ok {
			t.Errorf("legacy JSON key %q must not appear", k)
		}
	}

	// Spot-check a few values that prove structured parsing works end-to-end.
	if u["department"] != "Marketing" {
		t.Errorf("department: got %v, want Marketing", u["department"])
	}
	if u["chat_recency_hint"] != "Contacted 2 days ago" {
		t.Errorf("chat_recency_hint: got %v", u["chat_recency_hint"])
	}
	// Signature must come through from raw meta.description, under the
	// agent-friendly key "signature" (not "description").
	if u["signature"] != "Coffee fanatic ☕" {
		t.Errorf("signature: got %v, want %q", u["signature"], "Coffee fanatic ☕")
	}
}

func TestSearchUser_Integration_NDJSONHasNoRefineHint(t *testing.T) {
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
	if strings.Contains(stdout.String(), "refine") {
		t.Errorf("ndjson stdout must not contain the refine hint (would corrupt the stream); got=%q", stdout.String())
	}
	if strings.Contains(stderr.String(), "refine") {
		t.Errorf("ndjson stderr must not contain the refine hint either (non-human format opts out entirely); got=%q", stderr.String())
	}
}

func TestSearchUser_Integration_PrettyRefineHintGoesToStderr(t *testing.T) {
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
	if strings.Contains(stdout.String(), "refine") {
		t.Errorf("pretty stdout must not carry the hint (informational text belongs on stderr); got=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "refine") {
		t.Errorf("pretty stderr should suggest refining the query when has_more=true; got=%q", stderr.String())
	}
	// The hint must explicitly NOT recommend pagination — by design there
	// is no auto-pagination and agents must refine instead.
	if strings.Contains(stderr.String(), "--page-all") || strings.Contains(stderr.String(), "auto-paginate") {
		t.Errorf("hint must not mention pagination flags; got=%q", stderr.String())
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

// Guards against the int/string flag mismatch: the stub URL match requires
// page_size=25 to appear in the query string, which only happens if --page-size
// actually reaches DoAPI's QueryParams. A regression that silently defaults
// to 20 would fail the stub match.
func TestSearchUser_Integration_PageSizeFlowsToQuery(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/contact/v3/users/search?page_size=25",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"items": []interface{}{}, "has_more": false, "page_token": ""},
		},
	})

	err := mountAndRun(t, ContactSearchUser, []string{"+search-user", "--query", "x", "--page-size", "25", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v (likely page-size did not reach the request)", err)
	}
	reg.Verify(t)
}

// Verifies that with the auto-pagination flags removed, --page-all / --page-limit
// are no longer accepted. cobra must reject the unknown flag at parse time —
// no stub is registered because the command should never reach the API.
func TestSearchUser_Integration_NoAutoPaginationFlags(t *testing.T) {
	for _, removed := range []string{"--page-all", "--page-limit"} {
		t.Run(removed, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, searchUserDefaultConfig())
			args := []string{"+search-user", "--query", "x", removed}
			if removed == "--page-limit" {
				args = append(args, "5")
			}
			args = append(args, "--as", "user")
			err := mountAndRun(t, ContactSearchUser, args, f, stdout)
			if err == nil {
				t.Errorf("%s should be rejected (unknown flag), but command succeeded", removed)
			}
		})
	}
}
