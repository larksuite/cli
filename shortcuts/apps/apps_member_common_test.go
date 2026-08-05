// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

type appsMemberDryRunCall struct {
	Method string                 `json:"method"`
	URL    string                 `json:"url"`
	Params map[string]interface{} `json:"params"`
	Body   map[string]interface{} `json:"body"`
}

func appsMemberDryRunCallFor(t *testing.T, shortcut common.Shortcut, values map[string]string) appsMemberDryRunCall {
	t.Helper()
	if shortcut.DryRun == nil {
		t.Fatalf("%s DryRun must be registered", shortcut.Command)
	}
	rctx := newAppsMemberRuntime(t, shortcut, values)
	if shortcut.Validate != nil {
		if err := shortcut.Validate(context.Background(), rctx); err != nil {
			t.Fatalf("%s validation: %v", shortcut.Command, err)
		}
	}
	raw, err := json.Marshal(shortcut.DryRun(context.Background(), rctx))
	if err != nil {
		t.Fatalf("marshal %s dry-run: %v", shortcut.Command, err)
	}
	var envelope struct {
		API []appsMemberDryRunCall `json:"api"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode %s dry-run: %v", shortcut.Command, err)
	}
	if len(envelope.API) != 1 {
		t.Fatalf("%s dry-run calls = %d, want 1: %s", shortcut.Command, len(envelope.API), raw)
	}
	return envelope.API[0]
}

func newAppsMemberRuntime(t *testing.T, shortcut common.Shortcut, values map[string]string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: shortcut.Command}
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
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s=%q: %v", name, value, err)
		}
	}
	return common.TestNewRuntimeContext(cmd, &core.CliConfig{})
}

func requireAppsMemberValidationError(t *testing.T, err error, param string) *errs.ValidationError {
	t.Helper()
	if err == nil {
		t.Fatal("validation error = nil")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("validation error type = %T, want *errs.ValidationError: %v", err, err)
	}
	if validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("validation subtype = %q, want %q", validationErr.Subtype, errs.SubtypeInvalidArgument)
	}
	if validationErr.Param != param {
		t.Errorf("validation param = %q, want %q", validationErr.Param, param)
	}
	if validationErr.Hint == "" {
		t.Error("validation hint must be actionable")
	}
	return validationErr
}

func TestAppsMemberAPIErrorNormalization(t *testing.T) {
	tests := []struct {
		name        string
		code        int
		wantSubtype errs.Subtype
		wantMessage string
		wantHint    string
	}{
		{
			name: "internal feature not available", code: 40005, wantSubtype: errs.SubtypeFeatureNotAvailable,
			wantMessage: "Collaborator management is not available for this app via lark-cli.",
			wantHint:    "Open this app in Miaoda and manage collaborators from its permission settings.",
		},
		{
			name: "OpenAPI feature not available", code: 3340005, wantSubtype: errs.SubtypeFeatureNotAvailable,
			wantMessage: "Collaborator management is not available for this app via lark-cli.",
			wantHint:    "Open this app in Miaoda and manage collaborators from its permission settings.",
		},
		{name: "internal app not found", code: 40400, wantSubtype: errs.SubtypeNotFound},
		{name: "OpenAPI app not found", code: 3340400, wantSubtype: errs.SubtypeNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := errs.NewAPIError(errs.SubtypeUnknown, "server message").WithCode(tc.code).WithLogID("log-member")
			got := normalizeMemberAPIError(input)
			problem, ok := errs.ProblemOf(got)
			if !ok {
				t.Fatalf("normalizeMemberAPIError() = %T, want typed problem", got)
			}
			if problem.Code != tc.code || problem.Subtype != tc.wantSubtype || problem.LogID != "log-member" || problem.Retryable {
				t.Fatalf("problem = %+v", problem)
			}
			if tc.wantMessage != "" && problem.Message != tc.wantMessage {
				t.Fatalf("message = %q, want %q", problem.Message, tc.wantMessage)
			}
			if problem.Hint != tc.wantHint {
				t.Fatalf("hint = %q, want %q", problem.Hint, tc.wantHint)
			}
		})
	}
}

func TestAppsMemberFlagsExposeExactEnums(t *testing.T) {
	tests := []struct {
		shortcut common.Shortcut
		flag     string
		want     []string
	}{
		{AppsMemberList, "role", []string{"view", "edit", "full_access"}},
		{AppsMemberList, "member-type", []string{"user", "department", "chat"}},
		{AppsMemberAdd, "member-type", []string{"openid", "openchat", "opendepartmentid"}},
		{AppsMemberAdd, "perm", []string{"view", "edit", "full_access"}},
		{AppsMemberUpdate, "member-type", []string{"openid", "openchat", "opendepartmentid"}},
		{AppsMemberUpdate, "perm", []string{"view", "edit", "full_access"}},
		{AppsMemberRemove, "member-type", []string{"openid", "openchat", "opendepartmentid"}},
		{AppsMemberSettingsSet, "external-access", []string{"enabled", "disabled"}},
		{AppsMemberSettingsSet, "external-invite", []string{"enabled", "disabled"}},
		{AppsMemberSettingsSet, "link-share", []string{"closed", "tenant-readable", "tenant-editable", "anyone-readable"}},
		{AppsMemberSettingsSet, "manage-collaborators-by", []string{"anyone", "same-tenant", "full-access"}},
		{AppsMemberSettingsSet, "comment-by", []string{"viewer", "editor"}},
		{AppsMemberSettingsSet, "copy-download-by", []string{"viewer", "editor", "full-access"}},
	}

	for _, tc := range tests {
		t.Run(tc.shortcut.Command+"/"+tc.flag, func(t *testing.T) {
			var got []string
			for _, flag := range tc.shortcut.Flags {
				if flag.Name == tc.flag {
					got = flag.Enum
					break
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s --%s enum = %#v, want %#v", tc.shortcut.Command, tc.flag, got, tc.want)
			}
		})
	}
}

func TestAppsMemberAppIDValidationIsOwnedByShortcut(t *testing.T) {
	for _, shortcut := range []common.Shortcut{
		AppsMemberList, AppsMemberAdd, AppsMemberUpdate, AppsMemberRemove,
		AppsMemberSettingsGet, AppsMemberSettingsSet,
	} {
		t.Run(shortcut.Command, func(t *testing.T) {
			for _, flag := range shortcut.Flags {
				if flag.Name == "app-id" {
					if flag.Required {
						t.Fatal("app-id must use shortcut validation so errors include param and hint")
					}
					return
				}
			}
			t.Fatal("app-id flag is missing")
		})
	}
}

func TestAppsMemberPublicCopyIsGeneric(t *testing.T) {
	tests := []struct {
		shortcut common.Shortcut
		values   map[string]string
	}{
		{AppsMemberList, map[string]string{"app-id": "app_x"}},
		{AppsMemberAdd, map[string]string{"app-id": "app_x", "member-type": "openid", "member-id": "ou_user", "perm": "view"}},
		{AppsMemberUpdate, map[string]string{"app-id": "app_x", "member-type": "openid", "member-id": "ou_user", "perm": "edit"}},
		{AppsMemberRemove, map[string]string{"app-id": "app_x", "member-type": "openid", "member-id": "ou_user"}},
		{AppsMemberSettingsGet, map[string]string{"app-id": "app_x"}},
		{AppsMemberSettingsSet, map[string]string{"app-id": "app_x", "external-access": "enabled"}},
	}
	for _, tc := range tests {
		t.Run(tc.shortcut.Command, func(t *testing.T) {
			if strings.Contains(strings.ToLower(tc.shortcut.Description), "creative") {
				t.Fatalf("description exposes an internal app mode: %q", tc.shortcut.Description)
			}
			call := appsMemberDryRunCallFor(t, tc.shortcut, tc.values)
			raw, err := json.Marshal(call)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.ToLower(string(raw)), "creative") {
				t.Fatalf("dry-run exposes an internal app mode: %s", raw)
			}
		})
	}
}

func TestAppsMemberListValidationAndParams(t *testing.T) {
	valid := newAppsMemberRuntime(t, AppsMemberList, map[string]string{
		"app-id": "  app_test  ", "role": "edit", "member-type": "chat",
	})
	if AppsMemberList.Validate == nil {
		t.Fatal("member-list Validate must be registered")
	}
	if err := AppsMemberList.Validate(context.Background(), valid); err != nil {
		t.Fatalf("valid member-list flags: %v", err)
	}
	params, err := buildMemberListParams(valid)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}
	want := map[string]interface{}{
		"role": "edit", "member_type": "chat",
	}
	if !reflect.DeepEqual(params, want) {
		t.Fatalf("params = %#v, want %#v", params, want)
	}
}

func TestAppsMemberIdentityValidationMapsExactlyOneTypedField(t *testing.T) {
	tests := []struct {
		memberType string
		memberID   string
		want       memberIdentityRequest
	}{
		{memberType: "openid", memberID: "ou_member", want: memberIdentityRequest{UserOpenID: "ou_member"}},
		{memberType: "openchat", memberID: "oc_member", want: memberIdentityRequest{ChatID: "oc_member"}},
		{memberType: "opendepartmentid", memberID: "od-member", want: memberIdentityRequest{DepartmentID: "od-member"}},
	}
	for _, tc := range tests {
		t.Run(tc.memberType, func(t *testing.T) {
			got, err := buildMemberIdentity(tc.memberType, tc.memberID)
			if err != nil {
				t.Fatalf("buildMemberIdentity: %v", err)
			}
			if got != tc.want {
				t.Fatalf("identity = %#v, want %#v", got, tc.want)
			}
		})
	}

	invalid := []struct {
		memberType string
		memberID   string
		param      string
	}{
		{memberType: "openid", memberID: "12345", param: "--member-id"},
		{memberType: "openchat", memberID: "ou_member", param: "--member-id"},
		{memberType: "opendepartmentid", memberID: "oc_member", param: "--member-id"},
		{memberType: "mystery", memberID: "ou_member", param: "--member-type"},
		{memberType: "openid", memberID: "", param: "--member-id"},
	}
	for _, tc := range invalid {
		t.Run("reject/"+tc.memberType+"/"+tc.memberID, func(t *testing.T) {
			_, err := buildMemberIdentity(tc.memberType, tc.memberID)
			requireAppsMemberValidationError(t, err, tc.param)
		})
	}
}

func TestAppsMemberMutationValidation(t *testing.T) {
	valid := []struct {
		shortcut common.Shortcut
		values   map[string]string
	}{
		{AppsMemberAdd, map[string]string{"app-id": " app_test ", "member-type": "openid", "member-id": "ou_member", "perm": "view", "need-notification": "false"}},
		{AppsMemberUpdate, map[string]string{"app-id": "app_test", "member-type": "openchat", "member-id": "oc_member", "perm": "full_access"}},
		{AppsMemberRemove, map[string]string{"app-id": "app_test", "member-type": "opendepartmentid", "member-id": "od-member"}},
	}
	for _, tc := range valid {
		t.Run(tc.shortcut.Command, func(t *testing.T) {
			if tc.shortcut.Validate == nil {
				t.Fatal("Validate must be registered")
			}
			if err := tc.shortcut.Validate(context.Background(), newAppsMemberRuntime(t, tc.shortcut, tc.values)); err != nil {
				t.Fatalf("valid mutation flags: %v", err)
			}
		})
	}

	rctx := newAppsMemberRuntime(t, AppsMemberAdd, map[string]string{
		"app-id": "cli_credential", "member-type": "openid", "member-id": "ou_member", "perm": "view",
	})
	requireAppsMemberValidationError(t, AppsMemberAdd.Validate(context.Background(), rctx), "--app-id")
}

func TestAppsMemberAppIDValidationPreservesResourceNameCause(t *testing.T) {
	rctx := newAppsMemberRuntime(t, AppsMemberList, map[string]string{"app-id": "app_test?query"})
	err := AppsMemberList.Validate(context.Background(), rctx)
	validationErr := requireAppsMemberValidationError(t, err, "--app-id")
	cause := errors.Unwrap(validationErr)
	if cause == nil {
		t.Fatal("resource-name validation cause = nil")
	}
	if !strings.Contains(cause.Error(), "invalid characters") {
		t.Fatalf("resource-name validation cause = %q, want invalid characters", cause)
	}
}

func TestAppsMemberSettingsSetRequiresAtLeastOneExplicitField(t *testing.T) {
	empty := newAppsMemberRuntime(t, AppsMemberSettingsSet, map[string]string{"app-id": "app_test"})
	if AppsMemberSettingsSet.Validate == nil {
		t.Fatal("member-settings-set Validate must be registered")
	}
	err := AppsMemberSettingsSet.Validate(context.Background(), empty)
	if err == nil {
		t.Fatal("settings-set without changes must fail")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Hint == "" {
		t.Fatalf("settings-set error = %#v, want actionable invalid_argument", err)
	}

	for _, field := range []struct{ name, value string }{
		{name: "external-access", value: "enabled"},
		{name: "external-invite", value: "disabled"},
		{name: "link-share", value: "tenant-readable"},
		{name: "manage-collaborators-by", value: "same-tenant"},
		{name: "comment-by", value: "viewer"},
		{name: "copy-download-by", value: "full-access"},
	} {
		t.Run(field.name, func(t *testing.T) {
			rctx := newAppsMemberRuntime(t, AppsMemberSettingsSet, map[string]string{"app-id": "app_test", field.name: field.value})
			if err := AppsMemberSettingsSet.Validate(context.Background(), rctx); err != nil {
				t.Fatalf("explicit --%s should validate: %v", field.name, err)
			}
		})
	}
}

func TestAppsMemberDryRunRequestsUseExactRoutesAndTypedBodies(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		call := appsMemberDryRunCallFor(t, AppsMemberList, map[string]string{
			"app-id": " app_报告 ", "role": "view", "member-type": "user",
		})
		if call.Method != "GET" || call.URL != "/open-apis/spark/v1/apps/app_%E6%8A%A5%E5%91%8A/members" {
			t.Fatalf("list request = %s %s", call.Method, call.URL)
		}
		want := map[string]interface{}{"role": "view", "member_type": "user"}
		if !reflect.DeepEqual(call.Params, want) || call.Body != nil {
			t.Fatalf("list params/body = %#v / %#v, want %#v / nil", call.Params, call.Body, want)
		}
	})

	mutations := []struct {
		name     string
		shortcut common.Shortcut
		values   map[string]string
		method   string
		url      string
		body     map[string]interface{}
	}{
		{
			name: "add-user-with-explicit-false-notification", shortcut: AppsMemberAdd,
			values: map[string]string{"app-id": "app_x", "member-type": "openid", "member-id": "ou_member", "perm": "edit", "need-notification": "false"},
			method: "POST", url: "/open-apis/spark/v1/apps/app_x/members",
			body: map[string]interface{}{"user_open_id": "ou_member", "role": "edit", "need_notification": false},
		},
		{
			name: "add-chat-omits-notification", shortcut: AppsMemberAdd,
			values: map[string]string{"app-id": "app_x", "member-type": "openchat", "member-id": "oc_member", "perm": "view"},
			method: "POST", url: "/open-apis/spark/v1/apps/app_x/members",
			body: map[string]interface{}{"chat_id": "oc_member", "role": "view"},
		},
		{
			name: "update-department", shortcut: AppsMemberUpdate,
			values: map[string]string{"app-id": "app_x", "member-type": "opendepartmentid", "member-id": "od-member", "perm": "full_access"},
			method: "PATCH", url: "/open-apis/spark/v1/apps/app_x/members",
			body: map[string]interface{}{"department_id": "od-member", "role": "full_access"},
		},
		{
			name: "remove-user", shortcut: AppsMemberRemove,
			values: map[string]string{"app-id": "app_x", "member-type": "openid", "member-id": "ou_member"},
			method: "POST", url: "/open-apis/spark/v1/apps/app_x/members/remove",
			body: map[string]interface{}{"user_open_id": "ou_member"},
		},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			call := appsMemberDryRunCallFor(t, tc.shortcut, tc.values)
			if call.Method != tc.method || call.URL != tc.url || !reflect.DeepEqual(call.Body, tc.body) || call.Params != nil {
				t.Fatalf("request = %s %s params=%#v body=%#v, want %s %s params=nil body=%#v", call.Method, call.URL, call.Params, call.Body, tc.method, tc.url, tc.body)
			}
		})
	}

	t.Run("settings-get", func(t *testing.T) {
		call := appsMemberDryRunCallFor(t, AppsMemberSettingsGet, map[string]string{"app-id": "app_x"})
		if call.Method != "GET" || call.URL != "/open-apis/spark/v1/apps/app_x/member-settings" || call.Params != nil || call.Body != nil {
			t.Fatalf("settings get request = %#v", call)
		}
	})

	t.Run("settings-set-partial", func(t *testing.T) {
		call := appsMemberDryRunCallFor(t, AppsMemberSettingsSet, map[string]string{
			"app-id": "app_x", "external-access": "disabled", "comment-by": "editor",
		})
		want := map[string]interface{}{"external_access": "disabled", "comment_by": "editor"}
		if call.Method != "PATCH" || call.URL != "/open-apis/spark/v1/apps/app_x/member-settings" || !reflect.DeepEqual(call.Body, want) {
			t.Fatalf("settings set request = %#v, want body %#v", call, want)
		}
	})
}
