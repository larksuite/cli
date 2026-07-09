// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// ── resolveDriveMemberAddTarget unit tests ──────────────────────────────────

func TestResolveDriveMemberAddTarget_URLAndBareToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		explicit string
		wantTok  string
		wantType string
	}{
		{"docx URL", "https://example.feishu.cn/docx/doxTok?from=share", "", "doxTok", "docx"},
		{"folder URL", "https://example.feishu.cn/drive/folder/fldTok", "", "fldTok", "folder"},
		{"wiki URL", "https://example.feishu.cn/wiki/wikTok", "", "wikTok", "wiki"},
		{"mindnotes URL", "https://example.feishu.cn/mindnotes/mndTok", "", "mndTok", "mindnote"},
		{"larkoffice URL", "https://tenant.larkoffice.com/docx/doxTok", "", "doxTok", "docx"},
		{"explicit type overrides URL", "https://example.feishu.cn/docx/doxTok", "wiki", "doxTok", "wiki"},
		{"bare token with explicit docx type", "N83ZduEnHooFswxnVWGcazlLnFf", "docx", "N83ZduEnHooFswxnVWGcazlLnFf", "docx"},
		{"bare token with explicit folder type", "fldToken123", "folder", "fldToken123", "folder"},
	}
	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			token, resourceType, err := resolveDriveMemberAddTarget(tt.raw, tt.explicit)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if token != tt.wantTok || resourceType != tt.wantType {
				t.Fatalf("got token=%q type=%q, want %q/%q", token, resourceType, tt.wantTok, tt.wantType)
			}
		})
	}
}

func TestResolveDriveMemberAddTarget_RejectsBareTokenWithoutType(t *testing.T) {
	t.Parallel()

	_, _, err := resolveDriveMemberAddTarget("N83ZduEnHooFswxnVWGcazlLnFf", "")
	if err == nil || !strings.Contains(err.Error(), "--type is required when --token is a bare token") {
		t.Fatalf("expected bare token type-required error, got: %v", err)
	}
}

func TestResolveDriveMemberAddTarget_AcceptsAnyHost(t *testing.T) {
	t.Parallel()

	token, resourceType, err := resolveDriveMemberAddTarget("https://google.com/docx/doxTok", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "doxTok" {
		t.Fatalf("expected token 'doxTok', got %q", token)
	}
	if resourceType != "docx" {
		t.Fatalf("expected resourceType 'docx', got %q", resourceType)
	}
}

func TestResolveDriveMemberAddTarget_RejectsUnsupportedURLPath(t *testing.T) {
	t.Parallel()

	_, _, err := resolveDriveMemberAddTarget("https://example.feishu.cn/calendar/calTok", "")
	if err == nil || !strings.Contains(err.Error(), "unsupported URL path") {
		t.Fatalf("expected unsupported URL path error, got: %v", err)
	}
}

func TestResolveDriveMemberAddTarget_AcceptsMinutesURL(t *testing.T) {
	t.Parallel()

	token, resourceType, err := resolveDriveMemberAddTarget("https://example.feishu.cn/minutes/obcnTok", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "obcnTok" {
		t.Fatalf("expected token 'obcnTok', got %q", token)
	}
	if resourceType != "minutes" {
		t.Fatalf("expected resourceType 'minutes', got %q", resourceType)
	}
}

func TestResolveDriveMemberAddTarget_RejectsInvalidExplicitType(t *testing.T) {
	t.Parallel()

	_, _, err := resolveDriveMemberAddTarget("mincnTok", "invalidtype")
	if err == nil || !strings.Contains(err.Error(), "--type must be one of") {
		t.Fatalf("expected invalid type error, got: %v", err)
	}
}

func TestResolveDriveMemberAddTarget_RejectsEmpty(t *testing.T) {
	t.Parallel()

	_, _, err := resolveDriveMemberAddTarget("", "")
	if err == nil || !strings.Contains(err.Error(), "--token is required") {
		t.Fatalf("expected --token required error, got: %v", err)
	}
}

// ── inferMemberTypeFromID unit tests ────────────────────────────────────────

func TestInferMemberTypeFromID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		memberID string
		want     string
	}{
		{"ou_xxx", "openid"},
		{"on_xxx", "unionid"},
		{"oc_xxx", "openchat"},
		{"od_xxx", "opendepartmentid"},
		{"user@example.com", "email"},
		{"ambiguous", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := inferMemberTypeFromID(tt.memberID)
		if got != tt.want {
			t.Errorf("inferMemberTypeFromID(%q) = %q, want %q", tt.memberID, got, tt.want)
		}
	}
}

func TestResolveDriveMemberAddMemberType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		memberIDs []string
		explicit  string
		wantType  string
		wantErr   string
	}{
		{
			name:      "single explicit openid",
			memberIDs: []string{"ou_x"},
			explicit:  "openid",
			wantType:  "openid",
		},
		{
			name:      "explicit openchat",
			memberIDs: []string{"oc_a"},
			explicit:  "openchat",
			wantType:  "openchat",
		},
		{
			name:      "explicit groupid",
			memberIDs: []string{"group_1"},
			explicit:  "groupid",
			wantType:  "groupid",
		},
		{
			name:      "explicit appid",
			memberIDs: []string{"cli_xxx"},
			explicit:  "appid",
			wantType:  "appid",
		},
		{
			name:      "explicit wikispaceid",
			memberIDs: []string{"space_xxx"},
			explicit:  "wikispaceid",
			wantType:  "wikispaceid",
		},
		{
			name:      "missing member-type rejected",
			memberIDs: []string{"ou_a"},
			wantErr:   "--member-type is required",
		},
		{
			name:      "prefix conflicts with explicit type",
			memberIDs: []string{"oc_chat"},
			explicit:  "openid",
			wantErr:   "implies --member-type openchat",
		},
		{
			name:      "email prefix matches explicit email",
			memberIDs: []string{"user@example.com"},
			explicit:  "email",
			wantType:  "email",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotType, err := resolveDriveMemberAddMemberType(tt.memberIDs, tt.explicit)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotType != tt.wantType {
				t.Fatalf("got type=%q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestResolveDriveMemberAddMemberKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		memberType string
		raw        string
		want       string
		wantErr    string
	}{
		{
			name:       "wikispaceid requires explicit wiki-space kind",
			memberType: "wikispaceid",
			wantErr:    "--member-kind is required when --member-type=wikispaceid",
		},
		{
			name:       "wikispaceid accepts member kind",
			memberType: "wikispaceid",
			raw:        "wiki_space_member",
			want:       "wiki_space_member",
		},
		{
			name:       "wikispaceid accepts uppercase member kind",
			memberType: "wikispaceid",
			raw:        "WIKI_SPACE_EDITOR",
			want:       "wiki_space_editor",
		},
		{
			name:       "reject invalid member kind",
			memberType: "wikispaceid",
			raw:        "user",
			wantErr:    "invalid value \"user\" for --member-kind",
		},
		{
			name:       "reject member kind for other member type",
			memberType: "openid",
			raw:        "wiki_space_viewer",
			wantErr:    "--member-kind only applies when --member-type=wikispaceid",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveDriveMemberAddMemberKind(tt.memberType, tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("memberKind = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeDriveMemberAddMemberType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"openid", "openid"},
		{"groupid", "groupid"},
		{"appid", "appid"},
		{"wikispaceid", "wikispaceid"},
	}
	for _, tt := range tests {
		if got := normalizeDriveMemberAddMemberType(tt.in); got != tt.want {
			t.Fatalf("normalizeDriveMemberAddMemberType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// ── splitAndTrimMembers unit tests ──────────────────────────────────────────

func TestSplitAndTrimMembers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want []string
	}{
		{"ou_a", []string{"ou_a"}},
		{"ou_a,ou_b,ou_c", []string{"ou_a", "ou_b", "ou_c"}},
		{" ou_a , ou_b ", []string{"ou_a", "ou_b"}},
		{"ou_a,,ou_b", []string{"ou_a", "ou_b"}},
	}
	for _, tt := range tests {
		got := splitAndTrimMembers(tt.raw)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitAndTrimMembers(%q) = %#v, want %#v", tt.raw, got, tt.want)
		}
	}
}

// ── spec body/query construction unit tests ─────────────────────────────────

func TestDriveMemberAddSpec_BuildsBodyAndQuery(t *testing.T) {
	t.Parallel()

	spec := driveMemberAddSpec{
		Token:        "doxTok",
		ResourceType: "docx",
		MemberIDs:    []string{"ou_x"},
		MemberType:   "openid",
		Perm:         "edit",
	}
	if got := spec.DryRunParams(); !reflect.DeepEqual(got, map[string]interface{}{"type": "docx"}) {
		t.Fatalf("DryRunParams() = %#v", got)
	}
	if got := spec.APIQueryParams(); !reflect.DeepEqual(got, map[string]interface{}{"type": "docx"}) {
		t.Fatalf("APIQueryParams() = %#v", got)
	}
	wantBody := map[string]interface{}{
		"member_id":   "ou_x",
		"member_type": "openid",
		"perm":        "edit",
		"type":        "user",
	}
	if got := buildMemberBody("ou_x", "openid", "", "edit", ""); !reflect.DeepEqual(got, wantBody) {
		t.Fatalf("buildMemberBody() = %#v, want %#v", got, wantBody)
	}
}

func TestDriveMemberAddSpec_BuildsWikiSpaceIDBody(t *testing.T) {
	t.Parallel()

	wantBody := map[string]interface{}{
		"member_id":   "spc_x",
		"member_type": "wikispaceid",
		"perm":        "view",
		"type":        "wiki_space_editor",
	}
	if got := buildMemberBody("spc_x", "wikispaceid", "wiki_space_editor", "view", ""); !reflect.DeepEqual(got, wantBody) {
		t.Fatalf("buildMemberBody() = %#v, want %#v", got, wantBody)
	}
}

func TestDriveMemberAddBodyType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		memberType          string
		wikiSpaceMemberKind string
		want                string
	}{
		{name: "regular user infers body type", memberType: "openid", want: "user"},
		{name: "regular group infers body type", memberType: "groupid", want: "group"},
		{name: "wiki space uses explicit member kind", memberType: "wikispaceid", wikiSpaceMemberKind: "wiki_space_viewer", want: "wiki_space_viewer"},
		{name: "wiki space does not infer fallback type", memberType: "wikispaceid", want: ""},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := driveMemberAddBodyType(tt.memberType, tt.wikiSpaceMemberKind); got != tt.want {
				t.Fatalf("driveMemberAddBodyType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDriveMemberAddSpec_HonorsExplicitNotificationFalse(t *testing.T) {
	t.Parallel()

	spec := driveMemberAddSpec{
		ResourceType:     "docx",
		NotificationSet:  true,
		NeedNotification: false,
	}
	if got := spec.DryRunParams(); !reflect.DeepEqual(got, map[string]interface{}{"type": "docx", "need_notification": false}) {
		t.Fatalf("DryRunParams() = %#v", got)
	}
	if got := spec.APIQueryParams(); !reflect.DeepEqual(got, map[string]interface{}{"type": "docx", "need_notification": "false"}) {
		t.Fatalf("APIQueryParams() = %#v", got)
	}
}

func TestDriveMemberAddOutputBackfillsProvidedMemberID(t *testing.T) {
	t.Parallel()

	spec := driveMemberAddSpec{
		Token:        "doxTok",
		ResourceType: "docx",
		MemberIDs:    []string{"ou_a", "ou_b"},
		MemberType:   "openid",
		Perm:         "view",
	}
	out := driveMemberAddOutput(spec, "ou_b", map[string]interface{}{"perm": "view"})
	if out["member_id"] != "ou_b" {
		t.Fatalf("member_id = %v, want ou_b", out["member_id"])
	}
}

func TestDriveMemberAddOutput_BackfillsAppIDMemberType(t *testing.T) {
	t.Parallel()

	spec := driveMemberAddSpec{
		Token:        "doxTok",
		ResourceType: "docx",
		MemberIDs:    []string{"cli_app_123"},
		MemberType:   "appid",
		Perm:         "view",
	}
	out := driveMemberAddOutput(spec, "cli_app_123", map[string]interface{}{"perm": "view"})
	if out["member_type"] != "appid" {
		t.Fatalf("member_type = %v, want appid", out["member_type"])
	}
}

func TestDriveMemberAddOutput_OmitsPermTypeForNonWiki(t *testing.T) {
	t.Parallel()

	spec := driveMemberAddSpec{
		Token:        "doxTok",
		ResourceType: "docx",
		MemberIDs:    []string{"ou_x"},
		MemberType:   "openid",
		Perm:         "view",
	}
	out := driveMemberAddOutput(spec, "ou_x", map[string]interface{}{
		"member_id":   "ou_x",
		"member_type": "openid",
		"perm":        "view",
		"perm_type":   "container",
		"type":        "user",
	})
	if _, ok := out["perm_type"]; ok {
		t.Fatalf("perm_type should be omitted for non-wiki output, got %#v", out["perm_type"])
	}
}

func TestBuildDriveMemberAddBatchResult_OmitsPermTypeForNonWiki(t *testing.T) {
	t.Parallel()

	spec := driveMemberAddSpec{
		Token:        "doxTok",
		ResourceType: "docx",
		MemberIDs:    []string{"ou_a", "ou_b"},
		MemberType:   "openid",
		Perm:         "view",
	}
	result := buildDriveMemberAddBatchResult(spec, map[string]interface{}{
		"members": []interface{}{
			map[string]interface{}{"member_id": "ou_a", "member_type": "openid", "perm": "view", "perm_type": "container", "type": "user"},
			map[string]interface{}{"member_id": "ou_b", "member_type": "openid", "perm": "view", "perm_type": "container", "type": "user"},
		},
	})
	members, ok := result["members"].([]map[string]interface{})
	if !ok {
		t.Fatalf("members = %#v, want []map[string]interface{}", result["members"])
	}
	for i, member := range members {
		if _, exists := member["perm_type"]; exists {
			t.Fatalf("members[%d].perm_type should be omitted for non-wiki output, got %#v", i, member["perm_type"])
		}
	}
}

// ── shortcut integration tests ──────────────────────────────────────────────

func TestDriveMemberAdd_PermDefaultsToView(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", "ou_x",
		"--member-type", "openid",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if got.API[0].Body["perm"] != "view" {
		t.Fatalf("perm = %v, want view", got.API[0].Body["perm"])
	}
}

func TestDriveMemberAdd_RejectsNotificationWithBot(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", "ou_x",
		"--member-type", "openid",
		"--perm", "view",
		"--need-notification",
		"--as", "bot",
		"--yes",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--need-notification is only valid with --as user") {
		t.Fatalf("expected bot notification validation error, got: %v", err)
	}
}

func TestDriveMemberAdd_RejectsDepartmentWithBot(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", "od_dept",
		"--member-type", "opendepartmentid",
		"--perm", "view",
		"--as", "bot",
		"--yes",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--member-type=opendepartmentid requires --as user") {
		t.Fatalf("expected bot+opendepartmentid validation error, got: %v", err)
	}
}

func TestDriveMemberAdd_AcceptsAmbiguousIDWithExplicitType(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/doxcnTok/members",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"member": map[string]interface{}{
					"member_id":   "ambiguous_id",
					"member_type": "openid",
					"perm":        "view",
					"type":        "user",
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", "ambiguous_id",
		"--member-type", "openid",
		"--perm", "view",
		"--as", "user",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveMemberAdd_DryRunAcceptsAppID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", "cli_app_123",
		"--member-type", "appid",
		"--perm", "view",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if got.API[0].Body["member_type"] != "appid" {
		t.Fatalf("member_type = %v, want appid", got.API[0].Body["member_type"])
	}
	if _, ok := got.API[0].Body["type"]; ok {
		t.Fatalf("type = %v, want omitted for appid", got.API[0].Body["type"])
	}
}

func TestDriveMemberAdd_DryRunAcceptsWikiSpaceID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", "spc_x",
		"--member-type", "wikispaceid",
		"--member-kind", "wiki_space_viewer",
		"--perm", "view",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if got.API[0].Body["member_type"] != "wikispaceid" || got.API[0].Body["type"] != "wiki_space_viewer" {
		t.Fatalf("body = %#v, want wikispaceid + wiki_space_viewer", got.API[0].Body)
	}
}

func TestDriveMemberAdd_RejectsWikiSpaceIDWithoutMemberKind(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", "spc_x",
		"--member-type", "wikispaceid",
		"--perm", "view",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--member-kind is required when --member-type=wikispaceid") {
		t.Fatalf("expected wikispaceid member-kind validation error, got: %v", err)
	}
}

func TestDriveMemberAdd_RejectsMemberKindForNonWikiSpaceID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", "ou_x",
		"--member-type", "openid",
		"--member-kind", "wiki_space_member",
		"--perm", "view",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--member-kind only applies when --member-type=wikispaceid") {
		t.Fatalf("expected non-wikispaceid member-kind validation error, got: %v", err)
	}
}

func TestDriveMemberAdd_RejectsBlankMemberIDList(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", ",,,",
		"--member-type", "openid",
		"--perm", "view",
		"--as", "user",
		"--yes",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "at least one non-blank ID") {
		t.Fatalf("expected blank member-id validation error, got: %v", err)
	}
}

func TestDriveMemberAdd_RejectsBatchOverLimit(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	ids := make([]string, 11)
	for i := range ids {
		ids[i] = fmt.Sprintf("ou_%d", i)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", strings.Join(ids, ","),
		"--member-type", "openid",
		"--perm", "view",
		"--as", "user",
		"--yes",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "at most 10") {
		t.Fatalf("expected batch limit error, got: %v", err)
	}
}

func TestDriveMemberAdd_RejectsDuplicateBatchMemberIDs(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", "ou_a,ou_b,ou_a",
		"--member-type", "openid",
		"--perm", "view",
		"--as", "user",
		"--yes",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "duplicate collaborator ID") {
		t.Fatalf("expected duplicate member-id validation error, got: %v", err)
	}
}

func TestDriveMemberAdd_DryRunInfersTypeAndDefaultsWikiPermType(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "https://example.feishu.cn/wiki/wikTok?from=share",
		"--member-id", "ou_x",
		"--member-type", "openid",
		"--perm", "full_access",
		"--need-notification=false",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if len(got.API) != 1 {
		t.Fatalf("api count = %d, want 1; stdout=%s", len(got.API), stdout.String())
	}
	api := got.API[0]
	if api.Method != "POST" || api.URL != "/open-apis/drive/v1/permissions/wikTok/members" {
		t.Fatalf("api = %#v", api)
	}
	if api.Params["type"] != "wiki" || api.Params["need_notification"] != false {
		t.Fatalf("params = %#v", api.Params)
	}
	if api.Body["member_id"] != "ou_x" || api.Body["member_type"] != "openid" || api.Body["perm"] != "full_access" || api.Body["type"] != "user" || api.Body["perm_type"] != "container" {
		t.Fatalf("body = %#v", api.Body)
	}
}

func TestDriveMemberAdd_DryRunAcceptsUppercaseEnumsForDocx(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "DOCX",
		"--member-id", "ou_x",
		"--member-type", "OPENID",
		"--perm", "EDIT",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if got.API[0].Params["type"] != "docx" {
		t.Fatalf("params.type = %v, want docx", got.API[0].Params["type"])
	}
	if got.API[0].Body["member_type"] != "openid" || got.API[0].Body["perm"] != "edit" {
		t.Fatalf("body = %#v, want canonical lowercase enum values", got.API[0].Body)
	}
}

func TestDriveMemberAdd_DryRunAcceptsUppercaseWikiPermType(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "wikcnTok",
		"--type", "WIKI",
		"--member-id", "ou_x",
		"--member-type", "OPENID",
		"--perm", "EDIT",
		"--perm-type", "CONTAINER",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if got.API[0].Params["type"] != "wiki" {
		t.Fatalf("params.type = %v, want wiki", got.API[0].Params["type"])
	}
	if got.API[0].Body["member_type"] != "openid" || got.API[0].Body["perm"] != "edit" || got.API[0].Body["perm_type"] != "container" {
		t.Fatalf("body = %#v, want canonical lowercase enum values", got.API[0].Body)
	}
}

func TestDriveMemberAdd_RejectsInvalidPermLocallyWithoutGlobalEnum(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "DOCX",
		"--member-id", "ou_x",
		"--member-type", "OPENID",
		"--perm", "INVALID_EDIT",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "invalid value \"INVALID_EDIT\" for --perm") {
		t.Fatalf("expected local invalid --perm validation error, got: %v", err)
	}
}

func TestDriveMemberAdd_PermTypeRejectedForNonWiki(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", "ou_x",
		"--member-type", "openid",
		"--perm", "edit",
		"--perm-type", "single_page",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatalf("expected validation error for --perm-type on non-wiki resource")
	}
	if got, want := err.Error(), "--perm-type only applies when resource type is wiki"; !strings.Contains(got, want) {
		t.Fatalf("error %q does not contain %q", got, want)
	}
}

func TestDriveMemberAdd_DryRunBatch(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "shtcnTok",
		"--type", "sheet",
		"--member-id", "ou_a,ou_b,ou_c",
		"--member-type", "openid",
		"--perm", "edit",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if len(got.API) != 1 {
		t.Fatalf("api count = %d, want 1", len(got.API))
	}
	api := got.API[0]
	if api.Method != "POST" || api.URL != "/open-apis/drive/v1/permissions/shtcnTok/members/batch_create" {
		t.Fatalf("api = %#v", api)
	}
	members, ok := api.Body["members"].([]interface{})
	if !ok || len(members) != 3 {
		t.Fatalf("body.members = %#v, want 3 items", api.Body["members"])
	}
}

func TestDriveMemberAdd_ExecuteSuccessFlattensMember(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())

	var capturedQuery string
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/doxcnTok/members",
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.RawQuery
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"member": map[string]interface{}{
					"member_id":   "ou_x",
					"member_type": "openid",
					"perm":        "view",
					"type":        "user",
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", "ou_x",
		"--member-type", "openid",
		"--perm", "view",
		"--need-notification",
		"--as", "user",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &captured); err != nil {
		t.Fatalf("decode captured body: %v\n%s", err, string(stub.CapturedBody))
	}
	wantBody := map[string]interface{}{"member_id": "ou_x", "member_type": "openid", "perm": "view", "type": "user"}
	if !reflect.DeepEqual(captured, wantBody) {
		t.Fatalf("captured body = %#v, want %#v", captured, wantBody)
	}
	if !strings.Contains(capturedQuery, "type=docx") || !strings.Contains(capturedQuery, "need_notification=true") {
		t.Fatalf("captured query = %q", capturedQuery)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["resource_token"] != "doxcnTok" || data["resource_type"] != "docx" ||
		data["member_id"] != "ou_x" || data["member_type"] != "openid" ||
		data["perm"] != "view" || data["member_kind"] != "user" {
		t.Fatalf("flattened output = %#v", data)
	}
	if !strings.Contains(stderr.String(), "Added Drive member") {
		t.Fatalf("stderr = %q, want success log", stderr.String())
	}
}

func TestDriveMemberAdd_ExecuteBatchSuccess(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())

	var capturedQuery string
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/bascnTok/members/batch_create",
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.RawQuery
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"members": []interface{}{
					map[string]interface{}{"member_id": "ou_a", "member_type": "openid", "perm": "view", "type": "user"},
					map[string]interface{}{"member_id": "ou_b", "member_type": "openid", "perm": "view", "type": "user"},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "bascnTok",
		"--type", "bitable",
		"--member-id", "ou_a,ou_b",
		"--member-type", "openid",
		"--perm", "view",
		"--as", "user",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["requested_count"] != float64(2) || data["succeeded_count"] != float64(2) || data["partial"] != false {
		t.Fatalf("batch output counts = %#v", data)
	}
	missing, ok := data["missing_member_ids"].([]interface{})
	if !ok || len(missing) != 0 {
		t.Fatalf("missing_member_ids = %#v, want empty array", data["missing_member_ids"])
	}
	members, ok := data["members"].([]interface{})
	if !ok || len(members) != 2 {
		t.Fatalf("members = %#v, want 2", data["members"])
	}
	first, _ := members[0].(map[string]interface{})
	second, _ := members[1].(map[string]interface{})
	if first["member_id"] != "ou_a" || second["member_id"] != "ou_b" {
		t.Fatalf("members = %#v, want request-order fallback IDs", members)
	}
	if !strings.Contains(stderr.String(), "Added 2 Drive member(s)") {
		t.Fatalf("stderr = %q, want success log", stderr.String())
	}
	if !strings.Contains(capturedQuery, "type=bitable") {
		t.Fatalf("captured query = %q, want type=bitable for bascn token", capturedQuery)
	}
}

func TestDriveMemberAdd_ExecuteBatchSuccessWithOutOfOrderResponse(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/bascnTok/members/batch_create",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"members": []interface{}{
					map[string]interface{}{"member_id": "ou_b", "member_type": "openid", "perm": "view", "type": "user"},
					map[string]interface{}{"member_id": "ou_a", "member_type": "openid", "perm": "view", "type": "user"},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "bascnTok",
		"--type", "bitable",
		"--member-id", "ou_a,ou_b",
		"--member-type", "openid",
		"--perm", "view",
		"--as", "user",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["requested_count"] != float64(2) || data["succeeded_count"] != float64(2) || data["partial"] != false {
		t.Fatalf("batch output counts = %#v", data)
	}
	missing, ok := data["missing_member_ids"].([]interface{})
	if !ok || len(missing) != 0 {
		t.Fatalf("missing_member_ids = %#v, want empty array", data["missing_member_ids"])
	}
	if _, ok := data["mismatched_member_ids"]; ok {
		t.Fatalf("mismatched_member_ids should not be present on success: %#v", data["mismatched_member_ids"])
	}
	members, ok := data["members"].([]interface{})
	if !ok || len(members) != 2 {
		t.Fatalf("members = %#v, want 2", data["members"])
	}
}

func TestDriveMemberAdd_ExecuteBatchPartialWhenResponseMissingMember(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/bascnTok/members/batch_create",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"members": []interface{}{
					map[string]interface{}{"member_id": "ou_a", "member_type": "openid", "perm": "view", "type": "user"},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "bascnTok",
		"--type", "bitable",
		"--member-id", "ou_a,ou_b",
		"--member-type", "openid",
		"--perm", "view",
		"--as", "user",
		"--yes",
	}, f, stdout)
	exitErr := assertDriveMemberAddPartialFailure(t, err)
	if exitErr.Code != output.ExitAPI {
		t.Fatalf("exit code = %d, want %d (ExitAPI)", exitErr.Code, output.ExitAPI)
	}

	// stdout must carry ok:false envelope with structured partial result.
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal stdout envelope: %v\nstdout: %s", err, stdout.String())
	}
	if env.OK {
		t.Fatalf("ok must be false on partial failure, got ok:true\nstdout: %s", stdout.String())
	}
	if v := common.GetInt(env.Data, "succeeded_count"); v != 1 {
		t.Fatalf("succeeded_count = %d, want 1", v)
	}
	if v := common.GetInt(env.Data, "requested_count"); v != 2 {
		t.Fatalf("requested_count = %d, want 2", v)
	}
	if v := common.GetBool(env.Data, "partial"); !v {
		t.Fatal("partial must be true")
	}
}

func TestDriveMemberAdd_ExecuteBatchPartialDoesNotInferMissingMemberID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/bascnTok/members/batch_create",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"members": []interface{}{
					map[string]interface{}{"member_type": "openid", "perm": "view", "type": "user"},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "bascnTok",
		"--type", "bitable",
		"--member-id", "ou_a,ou_b",
		"--member-type", "openid",
		"--perm", "view",
		"--as", "user",
		"--yes",
	}, f, stdout)
	exitErr := assertDriveMemberAddPartialFailure(t, err)
	if exitErr.Code != output.ExitAPI {
		t.Fatalf("exit code = %d, want %d (ExitAPI)", exitErr.Code, output.ExitAPI)
	}

	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal stdout envelope: %v\nstdout: %s", err, stdout.String())
	}
	if env.OK {
		t.Fatalf("ok must be false on partial failure, got ok:true\nstdout: %s", stdout.String())
	}
	if v := common.GetInt(env.Data, "succeeded_count"); v != 0 {
		t.Fatalf("succeeded_count = %d, want 0", v)
	}
	if v := common.GetInt(env.Data, "requested_count"); v != 2 {
		t.Fatalf("requested_count = %d, want 2", v)
	}
	if v := common.GetBool(env.Data, "partial"); !v {
		t.Fatal("partial must be true")
	}
}

func TestDriveMemberAdd_ExecuteBatchPartialWhenMemberIDMismatches(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/bascnTok/members/batch_create",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"members": []interface{}{
					map[string]interface{}{"member_id": "ou_a", "member_type": "openid", "perm": "view", "type": "user"},
					map[string]interface{}{"member_id": "ou_other", "member_type": "openid", "perm": "view", "type": "user"},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "bascnTok",
		"--type", "bitable",
		"--member-id", "ou_a,ou_b",
		"--member-type", "openid",
		"--perm", "view",
		"--as", "user",
		"--yes",
	}, f, stdout)
	exitErr := assertDriveMemberAddPartialFailure(t, err)
	if exitErr.Code != output.ExitAPI {
		t.Fatalf("exit code = %d, want %d (ExitAPI)", exitErr.Code, output.ExitAPI)
	}

	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal stdout envelope: %v\nstdout: %s", err, stdout.String())
	}
	if env.OK {
		t.Fatalf("ok must be false on partial failure, got ok:true\nstdout: %s", stdout.String())
	}
	if v := common.GetBool(env.Data, "partial"); !v {
		t.Fatal("partial must be true")
	}
	missing, _ := env.Data["missing_member_ids"].([]interface{})
	if len(missing) != 1 || missing[0] != "ou_b" {
		t.Fatalf("missing_member_ids = %#v, want [ou_b]", env.Data["missing_member_ids"])
	}
	mismatched, _ := env.Data["mismatched_member_ids"].([]interface{})
	if len(mismatched) != 1 {
		t.Fatalf("mismatched_member_ids = %#v, want 1 entry", env.Data["mismatched_member_ids"])
	}
	mismatchedEntry, _ := mismatched[0].(map[string]interface{})
	if mismatchedEntry["returned"] != "ou_other" {
		t.Fatalf("mismatched_member_ids[0].returned = %#v, want ou_other", mismatchedEntry["returned"])
	}
}

func TestDriveMemberAdd_ExecuteBatchInvalidOperationHasActionableHint(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/bascnTok/members/batch_create",
		Body: map[string]interface{}{
			"code": 1063003,
			"msg":  "Invalid operation",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "bascnTok",
		"--type", "bitable",
		"--member-id", "ou_a,ou_b",
		"--member-type", "openid",
		"--perm", "view",
		"--as", "user",
		"--yes",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *errs.APIError, got %T: %v", err, err)
	}
	if apiErr.Code != 1063003 {
		t.Fatalf("code = %d, want 1063003", apiErr.Code)
	}
	if !strings.Contains(apiErr.Message, "requested members may already be collaborators") {
		t.Fatalf("message = %q, want duplicate-permission guidance", apiErr.Message)
	}
	if !strings.Contains(apiErr.Hint, "retry only the missing collaborators") {
		t.Fatalf("hint = %q, want retry guidance", apiErr.Hint)
	}
}

func TestDriveMemberAdd_ExecuteBatchInvalidParameterHasConservativeHint(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/bascnTok/members/batch_create",
		Body: map[string]interface{}{
			"code": 1063001,
			"msg":  "Invalid parameter",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "bascnTok",
		"--type", "bitable",
		"--member-id", "ou_a,ou_missing",
		"--member-type", "openid",
		"--perm", "view",
		"--as", "user",
		"--yes",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *errs.APIError, got %T: %v", err, err)
	}
	if apiErr.Code != 1063001 {
		t.Fatalf("code = %d, want 1063001", apiErr.Code)
	}
	if !strings.Contains(apiErr.Message, "requested members may be invalid") {
		t.Fatalf("message = %q, want invalid-member guidance", apiErr.Message)
	}
	if !strings.Contains(apiErr.Hint, "belongs to the same tenant") || !strings.Contains(apiErr.Hint, "visible to the current identity") {
		t.Fatalf("hint = %q, want conservative validation guidance", apiErr.Hint)
	}
}

func TestDriveMemberAdd_ExecuteSuccessAsBot(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	var capturedQuery string
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/wikcnBotTok/members",
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.RawQuery
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"member": map[string]interface{}{
					"member_id":   "ou_bot_target",
					"member_type": "openid",
					"perm":        "edit",
					"type":        "user",
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "wikcnBotTok",
		"--type", "wiki",
		"--member-id", "ou_bot_target",
		"--member-type", "openid",
		"--perm", "edit",
		"--as", "bot",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Bot identity should NOT send need_notification in query.
	if strings.Contains(capturedQuery, "need_notification") {
		t.Fatalf("captured query = %q, want need_notification absent for bot", capturedQuery)
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &captured); err != nil {
		t.Fatalf("decode captured body: %v", err)
	}
	if captured["perm"] != "edit" {
		t.Fatalf("captured body perm = %v, want edit", captured["perm"])
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["resource_type"] != "wiki" || data["member_kind"] != "user" || data["perm"] != "edit" {
		t.Fatalf("flattened output = %#v", data)
	}
}

func assertDriveMemberAddPartialFailure(t *testing.T, err error) *output.PartialFailureError {
	t.Helper()
	if err == nil {
		t.Fatal("expected partial_failure error, got nil")
	}
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected *output.PartialFailureError, got %T: %v", err, err)
	}
	return pfErr
}

func TestDriveMemberAdd_RequiresYesForHighRiskWrite(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberAdd, []string{
		"+member-add",
		"--token", "doxcnTok",
		"--type", "docx",
		"--member-id", "ou_x",
		"--member-type", "openid",
		"--perm", "view",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "requires confirmation") {
		t.Fatalf("expected confirmation error, got: %v", err)
	}
}
