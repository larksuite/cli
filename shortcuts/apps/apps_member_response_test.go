// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/brand"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	configpkg "github.com/larksuite/cli/internal/config"
	"github.com/larksuite/cli/internal/httpmock"
	identitypkg "github.com/larksuite/cli/internal/identity"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
)

func newAppsMemberAPIRuntime(t *testing.T, shortcut common.Shortcut, values map[string]string) (*common.RuntimeContext, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	cfg := &configpkg.CliConfig{
		AppID:      "test-member-client",
		AppSecret:  "test-member-secret",
		Brand:      brand.Feishu,
		UserOpenId: "ou_test_operator",
	}
	factory, stdout, _, registry := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: shortcut.Command}
	cmd.SetContext(context.Background())
	for _, flag := range shortcut.Flags {
		switch flag.Type {
		case "bool":
			cmd.Flags().Bool(flag.Name, flag.Default == "true", flag.Desc)
		case "int":
			defaultValue := 0
			if flag.Default != "" {
				parsed, err := strconv.Atoi(flag.Default)
				if err != nil {
					t.Fatalf("parse --%s default %q: %v", flag.Name, flag.Default, err)
				}
				defaultValue = parsed
			}
			cmd.Flags().Int(flag.Name, defaultValue, flag.Desc)
		default:
			cmd.Flags().String(flag.Name, flag.Default, flag.Desc)
		}
	}
	cmd.Flags().String("format", "json", "")
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s=%q: %v", name, value, err)
		}
	}
	rctx := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, identitypkg.AsUser)
	rctx.Format = "json"
	return rctx, stdout, registry
}

func requireAppsMemberInvalidResponse(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("invalid response error = nil")
	}
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("invalid response error type = %T, want *errs.InternalError: %v", err, err)
	}
	if internalErr.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("invalid response subtype = %q, want %q", internalErr.Subtype, errs.SubtypeInvalidResponse)
	}
}

func TestAppsMemberProjectionFlattensExternalTypedIDs(t *testing.T) {
	user := "ou_user"
	department := "od-department"
	chat := "oc_chat"
	tests := []struct {
		name string
		raw  memberAPIRecord
		want memberOutput
	}{
		{
			name: "user",
			raw:  memberAPIRecord{MemberType: "user", UserOpenID: &user, Name: "User", Role: "view"},
			want: memberOutput{MemberType: "user", MemberID: "ou_user", Name: "User", Role: "view"},
		},
		{
			name: "department",
			raw:  memberAPIRecord{MemberType: "department", DepartmentID: &department, Role: "edit"},
			want: memberOutput{MemberType: "department", MemberID: "od-department", Role: "edit"},
		},
		{
			name: "chat",
			raw:  memberAPIRecord{MemberType: "chat", ChatID: &chat, Name: "Chat", Role: "full_access"},
			want: memberOutput{MemberType: "chat", MemberID: "oc_chat", Name: "Chat", Role: "full_access"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := projectMemberRecord(tc.raw)
			if err != nil {
				t.Fatalf("projectMemberRecord: %v", err)
			}
			if got != tc.want {
				t.Fatalf("projected member = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestAppsMemberProjectionFailsClosedOnMalformedTypedIDs(t *testing.T) {
	user := "ou_user"
	chat := "oc_chat"
	internal := "123456789"
	empty := ""
	tests := []struct {
		name string
		raw  memberAPIRecord
	}{
		{name: "missing typed ID", raw: memberAPIRecord{MemberType: "user", Role: "view"}},
		{name: "multiple typed IDs", raw: memberAPIRecord{MemberType: "user", UserOpenID: &user, ChatID: &chat, Role: "view"}},
		{name: "member type and ID mismatch", raw: memberAPIRecord{MemberType: "user", ChatID: &chat, Role: "view"}},
		{name: "internal numeric user ID", raw: memberAPIRecord{MemberType: "user", UserOpenID: &internal, Role: "view"}},
		{name: "empty typed ID", raw: memberAPIRecord{MemberType: "chat", ChatID: &empty, Role: "view"}},
		{name: "unknown member type", raw: memberAPIRecord{MemberType: "unknown", UserOpenID: &user, Role: "view"}},
		{name: "unsupported role", raw: memberAPIRecord{MemberType: "user", UserOpenID: &user, Role: "owner"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := projectMemberRecord(tc.raw)
			requireAppsMemberInvalidResponse(t, err)
		})
	}
}

func TestAppsMemberListExecuteUsesTypedProjectionWithoutLeakingRawFields(t *testing.T) {
	rctx, stdout, registry := newAppsMemberAPIRuntime(t, AppsMemberList, map[string]string{
		"app-id": "app_x", "member-type": "user",
	})
	stub := &httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/members",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"member_type": "user", "user_open_id": "ou_public", "name": "User", "role": "view",
						"meta_token": "sensitive-internal-token",
					},
				},
				"app": map[string]interface{}{"meta_token": "sensitive-internal-token"},
			},
		},
	}
	registry.Register(stub)
	if AppsMemberList.Execute == nil {
		t.Fatal("member-list Execute must be registered")
	}
	if err := AppsMemberList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("member-list Execute: %v", err)
	}
	registry.Verify(t)
	if got := stub.CapturedBody; len(got) != 0 {
		t.Fatalf("GET request body = %q, want empty", got)
	}
	out := stdout.String()
	for path, want := range map[string]string{
		"data.items.0.member_type": "user",
		"data.items.0.member_id":   "ou_public",
	} {
		if got := gjson.Get(out, path).String(); got != want {
			t.Errorf("output %s = %q, want %q: %s", path, got, want, out)
		}
	}
	if gjson.Get(out, "data.app").Exists() {
		t.Fatalf("member output unexpectedly contains app: %s", out)
	}
	if gjson.Get(out, "data.page_token").Exists() || gjson.Get(out, "data.has_more").Exists() {
		t.Fatalf("member output unexpectedly contains pagination: %s", out)
	}
	for _, forbidden := range []string{"collaborator_id", "user_open_id", "department_id", "chat_id", "123456789", "meta_token", "sensitive-internal-token"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("output exposed typed/internal field %q: %s", forbidden, out)
		}
	}
}

func TestAppsMemberListExecuteNeverLeaksMetaTokenAcrossFormats(t *testing.T) {
	for _, format := range []string{"json", "table", "csv", "ndjson", "pretty"} {
		t.Run(format, func(t *testing.T) {
			rctx, stdout, registry := newAppsMemberAPIRuntime(t, AppsMemberList, map[string]string{
				"app-id": "app_x",
			})
			rctx.Format = format
			registry.Register(&httpmock.Stub{
				Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/members",
				Body: map[string]interface{}{
					"code": 0, "msg": "",
					"data": map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{
								"member_type": "user", "user_open_id": "ou_public", "role": "view",
								"meta_token": "sensitive-internal-token",
							},
						},
						"app": map[string]interface{}{"meta_token": "sensitive-internal-token"},
					},
				},
			})

			if err := AppsMemberList.Execute(context.Background(), rctx); err != nil {
				t.Fatalf("member-list Execute: %v", err)
			}
			registry.Verify(t)
			out := stdout.String()
			if gjson.Get(out, "data.app").Exists() {
				t.Fatalf("%s output unexpectedly contains app: %s", format, out)
			}
			for _, forbidden := range []string{"meta_token", "sensitive-internal-token"} {
				if strings.Contains(out, forbidden) {
					t.Errorf("%s output exposed internal field %q: %s", format, forbidden, out)
				}
			}
		})
	}
}

func TestAppsMemberMutationExecuteProjectsResponses(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		values   map[string]string
		method   string
		url      string
		data     map[string]interface{}
		want     map[string]string
		wantBool map[string]bool
	}{
		{
			name: "add", shortcut: AppsMemberAdd,
			values: map[string]string{"app-id": "app_x", "member-type": "openid", "member-id": "ou_added", "perm": "view"},
			method: "POST", url: "/open-apis/spark/v1/apps/app_x/members",
			data: map[string]interface{}{"member": map[string]interface{}{"member_type": "user", "user_open_id": "ou_added", "role": "view"}, "changed": true},
			want: map[string]string{"data.member.member_id": "ou_added"}, wantBool: map[string]bool{"data.changed": true},
		},
		{
			name: "update", shortcut: AppsMemberUpdate,
			values: map[string]string{"app-id": "app_x", "member-type": "openchat", "member-id": "oc_updated", "perm": "edit"},
			method: "PATCH", url: "/open-apis/spark/v1/apps/app_x/members",
			data: map[string]interface{}{"member": map[string]interface{}{"member_type": "chat", "chat_id": "oc_updated", "role": "edit"}, "before_role": "view", "after_role": "edit", "changed": true},
			want: map[string]string{"data.member.member_id": "oc_updated", "data.before_role": "view", "data.after_role": "edit"},
		},
		{
			name: "remove", shortcut: AppsMemberRemove,
			values: map[string]string{"app-id": "app_x", "member-type": "opendepartmentid", "member-id": "od-removed"},
			method: "POST", url: "/open-apis/spark/v1/apps/app_x/members/remove",
			data: map[string]interface{}{"member": map[string]interface{}{"member_type": "department", "department_id": "od-removed", "role": "view"}, "changed": true},
			want: map[string]string{"data.member.member_id": "od-removed"}, wantBool: map[string]bool{"data.changed": true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rctx, stdout, registry := newAppsMemberAPIRuntime(t, tc.shortcut, tc.values)
			stub := &httpmock.Stub{Method: tc.method, URL: tc.url, Body: map[string]interface{}{"code": 0, "msg": "", "data": tc.data}}
			registry.Register(stub)
			if tc.shortcut.Execute == nil {
				t.Fatalf("%s Execute must be registered", tc.shortcut.Command)
			}
			if err := tc.shortcut.Execute(context.Background(), rctx); err != nil {
				t.Fatalf("%s Execute: %v", tc.shortcut.Command, err)
			}
			registry.Verify(t)
			if !json.Valid(stub.CapturedBody) {
				t.Fatalf("request body is not JSON: %s", stub.CapturedBody)
			}
			out := stdout.String()
			for path, want := range tc.want {
				if got := gjson.Get(out, path).String(); got != want {
					t.Errorf("output %s = %q, want %q: %s", path, got, want, out)
				}
			}
			for path, want := range tc.wantBool {
				if got := gjson.Get(out, path).Bool(); got != want {
					t.Errorf("output %s = %t, want %t: %s", path, got, want, out)
				}
			}
			for _, forbidden := range []string{"user_open_id", "department_id", "chat_id", "123456789"} {
				if strings.Contains(out, forbidden) {
					t.Errorf("output exposed typed/internal field %q: %s", forbidden, out)
				}
			}
		})
	}
}

func TestAppsMemberSettingsExecuteUsesTypedResponses(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		values   map[string]string
		method   string
		data     map[string]interface{}
		want     map[string]string
		wantBool map[string]bool
	}{
		{
			name: "get", shortcut: AppsMemberSettingsGet,
			values: map[string]string{"app-id": "app_x"}, method: "GET",
			data: map[string]interface{}{
				"settings": map[string]interface{}{"external_access": "enabled", "link_share": "tenant-readable", "comment_by": "viewer"},
			},
			want: map[string]string{
				"data.settings.external_access": "enabled",
				"data.settings.link_share":      "tenant-readable",
				"data.settings.comment_by":      "viewer",
			},
		},
		{
			name: "set", shortcut: AppsMemberSettingsSet,
			values: map[string]string{"app-id": "app_x", "comment-by": "viewer"}, method: "PATCH",
			data: map[string]interface{}{
				"settings": map[string]interface{}{"comment_by": "viewer", "copy_download_by": "editor"},
				"changes":  []interface{}{map[string]interface{}{"field": "comment_by", "before": "editor", "after": "viewer"}},
				"changed":  true,
			},
			want: map[string]string{
				"data.settings.comment_by":       "viewer",
				"data.settings.copy_download_by": "editor",
				"data.changes.0.field":           "comment_by",
			},
			wantBool: map[string]bool{"data.changed": true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rctx, stdout, registry := newAppsMemberAPIRuntime(t, tc.shortcut, tc.values)
			registry.Register(&httpmock.Stub{Method: tc.method, URL: "/open-apis/spark/v1/apps/app_x/member-settings", Body: map[string]interface{}{"code": 0, "msg": "", "data": tc.data}})
			if tc.shortcut.Execute == nil {
				t.Fatalf("%s Execute must be registered", tc.shortcut.Command)
			}
			if err := tc.shortcut.Execute(context.Background(), rctx); err != nil {
				t.Fatalf("%s Execute: %v", tc.shortcut.Command, err)
			}
			registry.Verify(t)
			out := stdout.String()
			if gjson.Get(out, "data.app").Exists() {
				t.Fatalf("%s output unexpectedly contains app: %s", tc.name, out)
			}
			for path, want := range tc.want {
				if got := gjson.Get(out, path).String(); got != want {
					t.Errorf("output %s = %q, want %q: %s", path, got, want, out)
				}
			}
			for path, want := range tc.wantBool {
				if got := gjson.Get(out, path).Bool(); got != want {
					t.Errorf("output %s = %t, want %t: %s", path, got, want, out)
				}
			}
		})
	}
}

func TestAppsMemberExecuteFailsClosedBeforeOutput(t *testing.T) {
	rctx, stdout, registry := newAppsMemberAPIRuntime(t, AppsMemberAdd, map[string]string{
		"app-id": "app_x", "member-type": "openid", "member-id": "ou_requested", "perm": "view",
	})
	registry.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/members",
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"member":  map[string]interface{}{"member_type": "user", "user_open_id": "123456789", "role": "view"},
			"changed": true,
		}},
	})
	if AppsMemberAdd.Execute == nil {
		t.Fatal("member-add Execute must be registered")
	}
	err := AppsMemberAdd.Execute(context.Background(), rctx)
	requireAppsMemberInvalidResponse(t, err)
	if stdout.Len() != 0 {
		t.Fatalf("malformed member response must not emit data: %s", stdout.String())
	}
}

func TestAppsMemberSettingsProjectionRejectsUnknownEnumsButAllowsAbsentFields(t *testing.T) {
	valid := memberSettingsResponse{}
	if err := validateMemberSettingsResponse(valid); err != nil {
		t.Fatalf("absent optional settings should be allowed: %v", err)
	}
	unknown := "internet-editable"
	requireAppsMemberInvalidResponse(t, validateMemberSettingsResponse(memberSettingsResponse{LinkShare: &unknown}))
}
