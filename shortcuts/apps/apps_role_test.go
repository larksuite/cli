// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func roleCRUDFlagDefs() map[string]string {
	return map[string]string{
		"app-id":      "string",
		"role-id":     "string",
		"name":        "string",
		"description": "string",
		"page-size":   "int",
		"page-token":  "string",
	}
}

func TestAppsRoleMetadata(t *testing.T) {
	tests := []struct {
		name    string
		command string
		risk    string
		scopes  []string
	}{
		{"list", AppsRoleList.Command, AppsRoleList.Risk, AppsRoleList.Scopes},
		{"get", AppsRoleGet.Command, AppsRoleGet.Risk, AppsRoleGet.Scopes},
		{"create", AppsRoleCreate.Command, AppsRoleCreate.Risk, AppsRoleCreate.Scopes},
		{"update", AppsRoleUpdate.Command, AppsRoleUpdate.Risk, AppsRoleUpdate.Scopes},
		{"delete", AppsRoleDelete.Command, AppsRoleDelete.Risk, AppsRoleDelete.Scopes},
	}
	wantCommands := map[string]string{
		"list":   "+role-list",
		"get":    "+role-get",
		"create": "+role-create",
		"update": "+role-update",
		"delete": "+role-delete",
	}
	wantRisks := map[string]string{
		"list":   "read",
		"get":    "read",
		"create": "write",
		"update": "write",
		"delete": "high-risk-write",
	}
	wantScopes := map[string]string{
		"list":   "spark:app:read",
		"get":    "spark:app:read",
		"create": "spark:app:write",
		"update": "spark:app:write",
		"delete": "spark:app:write",
	}
	for _, tt := range tests {
		if tt.command != wantCommands[tt.name] {
			t.Fatalf("%s command = %q, want %q", tt.name, tt.command, wantCommands[tt.name])
		}
		if tt.risk != wantRisks[tt.name] {
			t.Fatalf("%s risk = %q, want %q", tt.name, tt.risk, wantRisks[tt.name])
		}
		if len(tt.scopes) != 1 || tt.scopes[0] != wantScopes[tt.name] {
			t.Fatalf("%s scopes = %#v, want [%s]", tt.name, tt.scopes, wantScopes[tt.name])
		}
	}
	for name, shortcut := range map[string]struct {
		authTypes []string
		hasFormat bool
	}{
		"list":   {AppsRoleList.AuthTypes, AppsRoleList.HasFormat},
		"get":    {AppsRoleGet.AuthTypes, AppsRoleGet.HasFormat},
		"create": {AppsRoleCreate.AuthTypes, AppsRoleCreate.HasFormat},
		"update": {AppsRoleUpdate.AuthTypes, AppsRoleUpdate.HasFormat},
		"delete": {AppsRoleDelete.AuthTypes, AppsRoleDelete.HasFormat},
	} {
		if len(shortcut.authTypes) != 1 || shortcut.authTypes[0] != "user" {
			t.Fatalf("%s authTypes = %#v, want [user]", name, shortcut.authTypes)
		}
		if !shortcut.hasFormat {
			t.Fatalf("%s HasFormat = false, want true", name)
		}
	}
}

func TestAppsRoleIDFlagHelpIdentifiesAcceptedIDKinds(t *testing.T) {
	shortcuts := []common.Shortcut{
		AppsRoleList,
		AppsRoleGet,
		AppsRoleCreate,
		AppsRoleUpdate,
		AppsRoleDelete,
		AppsRoleMemberList,
		AppsRoleMemberAdd,
		AppsRoleMemberRemove,
		AppsRoleMatchList,
	}
	for _, shortcut := range shortcuts {
		found := false
		for _, flag := range shortcut.Flags {
			if flag.Name != "app-id" {
				continue
			}
			found = true
			if flag.Desc != roleAppIDRequiredDesc {
				t.Fatalf("%s --app-id description = %q, want %q", shortcut.Command, flag.Desc, roleAppIDRequiredDesc)
			}
		}
		if !found {
			t.Fatalf("%s is missing --app-id", shortcut.Command)
		}
	}

	for _, flag := range AppsRoleMatchList.Flags {
		if flag.Name == "user-id" {
			if flag.Desc != roleUserIDRequiredDesc {
				t.Fatalf("%s --user-id description = %q, want %q", AppsRoleMatchList.Command, flag.Desc, roleUserIDRequiredDesc)
			}
			return
		}
	}
	t.Fatalf("%s is missing --user-id", AppsRoleMatchList.Command)
}

func TestAppsRoleListTipsDescribeSafeNameResolution(t *testing.T) {
	tips := strings.Join(AppsRoleList.Tips, "\n")
	for _, want := range []string{"--name", "exact", "unique role_id", "+role-get"} {
		if !strings.Contains(tips, want) {
			t.Fatalf("AppsRoleList tips missing %q: %s", want, tips)
		}
	}
}

func TestAppsRoleGetTipsDescribeSafeNameResolution(t *testing.T) {
	tips := strings.Join(AppsRoleGet.Tips, "\n")
	for _, want := range []string{"not a human-readable role name", "+role-list --name", "unique returned role_id"} {
		if !strings.Contains(tips, want) {
			t.Fatalf("AppsRoleGet tips missing %q: %s", want, tips)
		}
	}
	for _, want := range []string{"--name <exact_name>", "unique returned role_id"} {
		if !strings.Contains(roleItemHint, want) {
			t.Fatalf("roleItemHint missing %q: %s", want, roleItemHint)
		}
	}
}

func TestAppsRoleDeleteTipsRequireExplicitConfirmationAndConditionalReadback(t *testing.T) {
	tips := strings.Join(AppsRoleDelete.Tips, "\n")
	for _, want := range []string{
		"delete request alone is not explicit confirmation",
		"current member scope",
		"use --yes only after",
		"When independent verification is required",
		"+role-list --name",
		"confirm the deleted role_id is absent",
		"failed +role-get alone does not prove deletion",
	} {
		if !strings.Contains(tips, want) {
			t.Fatalf("AppsRoleDelete tips missing %q: %s", want, tips)
		}
	}
}

func TestAppsRoleCreateTipsDescribeConditionalIndependentReadback(t *testing.T) {
	tips := strings.Join(AppsRoleCreate.Tips, "\n")
	for _, want := range []string{
		"create response returns data.role",
		"data.role.role_id",
		"only when independent verification is required",
	} {
		if !strings.Contains(tips, want) {
			t.Fatalf("AppsRoleCreate tips missing %q: %s", want, tips)
		}
	}
}

func TestAppsRoleDryRunRequestShapes(t *testing.T) {
	tests := []struct {
		name   string
		flags  map[string]string
		sc     common.Shortcut
		method string
		url    string
		params map[string]interface{}
		body   map[string]interface{}
	}{
		{
			name: "list",
			flags: map[string]string{
				"app-id": "app_x", "name": " Admin ", "page-size": "20", "page-token": "40",
			},
			sc:     AppsRoleList,
			method: "GET",
			url:    "/open-apis/spark/v1/apps/app_x/roles",
			params: map[string]interface{}{"limit": float64(100), "offset": float64(0), "name": "Admin"},
		},
		{
			name:   "get",
			flags:  map[string]string{"app-id": "app_x", "role-id": "role_1"},
			sc:     AppsRoleGet,
			method: "GET",
			url:    "/open-apis/spark/v1/apps/app_x/roles/role_1",
		},
		{
			name:   "create",
			flags:  map[string]string{"app-id": "app_x", "name": " Admin ", "description": " Manage "},
			sc:     AppsRoleCreate,
			method: "POST",
			url:    "/open-apis/spark/v1/apps/app_x/roles",
			body:   map[string]interface{}{"name": "Admin", "description": "Manage"},
		},
		{
			name:   "update",
			flags:  map[string]string{"app-id": "app_x", "role-id": "role_1", "description": " New "},
			sc:     AppsRoleUpdate,
			method: "PATCH",
			url:    "/open-apis/spark/v1/apps/app_x/roles/role_1",
			body:   map[string]interface{}{"description": "New"},
		},
		{
			name:   "delete",
			flags:  map[string]string{"app-id": "app_x", "role-id": "role_1"},
			sc:     AppsRoleDelete,
			method: "DELETE",
			url:    "/open-apis/spark/v1/apps/app_x/roles/role_1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _, _ := newRoleRCtx(t, roleCRUDFlagDefs(), tt.flags)
			if tt.sc.Validate != nil {
				if err := tt.sc.Validate(context.Background(), rctx); err != nil {
					t.Fatalf("Validate() = %v", err)
				}
			}
			raw, err := json.Marshal(tt.sc.DryRun(context.Background(), rctx))
			if err != nil {
				t.Fatalf("marshal dry-run: %v", err)
			}
			call := firstDryRunCall(t, raw)
			if call.Method != tt.method || call.URL != tt.url {
				t.Fatalf("call = %s %s, want %s %s", call.Method, call.URL, tt.method, tt.url)
			}
			assertMapEqual(t, call.Params, tt.params)
			assertMapEqual(t, mapFromInterface(t, call.Body), tt.body)
		})
	}
}

func TestAppsRoleCreatePrettyAcceptsRoleIDAcknowledgement(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/roles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"role": map[string]interface{}{
					"role_id": "role_backend",
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsRoleCreate,
		[]string{"+role-create", "--app-id", "app_x", "--name", "Admin", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "Created role role_backend") {
		t.Fatalf("pretty output should read role_id from response data.role, got: %q", got)
	}
	if strings.Contains(got, "Admin") {
		t.Fatalf("pretty output must not present the request name as response-confirmed data, got: %q", got)
	}
}

func TestAppsRoleCreateRejectsMismatchedExplicitRoleID(t *testing.T) {
	rctx, _, reg := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"name":    "Admin",
		"role-id": "role_request",
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/roles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"role": map[string]interface{}{
					"name":    "Admin",
					"role_id": "role_backend",
				},
			},
		},
	})

	err := AppsRoleCreate.Execute(context.Background(), rctx)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("Execute() = %#v, want internal/invalid_response", err)
	}
}

func TestAppsRoleGetPrettyOutput(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"role": map[string]interface{}{
					"role_id":     "role_1",
					"name":        "Admin",
					"description": "Manage",
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsRoleGet,
		[]string{"+role-get", "--app-id", "app_x", "--role-id", "role_1", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{"role_id: role_1", "name: Admin", "description: Manage"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q: %q", want, got)
		}
	}
}

func TestAppsRoleUpdatePrettyAcceptsRoleIDAcknowledgement(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"role": map[string]interface{}{
					"role_id": "role_1",
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsRoleUpdate,
		[]string{"+role-update", "--app-id", "app_x", "--role-id", "role_1", "--name", "Operator", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "Updated role role_1") {
		t.Fatalf("pretty output = %q, want updated role acknowledgement", got)
	}
	if strings.Contains(got, "Operator") {
		t.Fatalf("pretty output must not present the request name as response-confirmed data, got: %q", got)
	}
}

func TestAppsRoleReadWriteCommandsRejectEmptySuccessData(t *testing.T) {
	tests := []struct {
		name   string
		sc     common.Shortcut
		method string
		url    string
		flags  map[string]string
	}{
		{
			name:   "get",
			sc:     AppsRoleGet,
			method: "GET",
			url:    "/open-apis/spark/v1/apps/app_x/roles/role_1",
			flags:  map[string]string{"app-id": "app_x", "role-id": "role_1"},
		},
		{
			name:   "create",
			sc:     AppsRoleCreate,
			method: "POST",
			url:    "/open-apis/spark/v1/apps/app_x/roles",
			flags:  map[string]string{"app-id": "app_x", "name": "Admin"},
		},
		{
			name:   "update",
			sc:     AppsRoleUpdate,
			method: "PATCH",
			url:    "/open-apis/spark/v1/apps/app_x/roles/role_1",
			flags:  map[string]string{"app-id": "app_x", "role-id": "role_1", "name": "Operator"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _, reg := newRoleRCtx(t, roleCRUDFlagDefs(), tt.flags)
			reg.Register(&httpmock.Stub{
				Method: tt.method,
				URL:    tt.url,
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "",
					"data": map[string]interface{}{},
				},
			})

			err := tt.sc.Execute(context.Background(), rctx)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("Execute() = %#v, want internal/invalid_response", err)
			}
		})
	}
}

func TestParseRoleDetailResponseDataRejectsMalformedRole(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{name: "nil data", data: nil},
		{name: "missing role", data: map[string]interface{}{}},
		{name: "null role", data: map[string]interface{}{"role": nil}},
		{name: "role is not object", data: map[string]interface{}{"role": []interface{}{}}},
		{name: "missing role id", data: map[string]interface{}{"role": map[string]interface{}{"name": "Admin"}}},
		{name: "role id is not string", data: map[string]interface{}{"role": map[string]interface{}{"role_id": 1, "name": "Admin"}}},
		{name: "role id is empty", data: map[string]interface{}{"role": map[string]interface{}{"role_id": " ", "name": "Admin"}}},
		{name: "role id mismatches request", data: map[string]interface{}{"role": map[string]interface{}{"role_id": "role_2", "name": "Admin"}}},
		{name: "missing name", data: map[string]interface{}{"role": map[string]interface{}{"role_id": "role_1"}}},
		{name: "name is not string", data: map[string]interface{}{"role": map[string]interface{}{"role_id": "role_1", "name": 1}}},
		{name: "name is empty", data: map[string]interface{}{"role": map[string]interface{}{"role_id": "role_1", "name": " "}}},
		{name: "description is null", data: map[string]interface{}{"role": map[string]interface{}{"role_id": "role_1", "name": "Admin", "description": nil}}},
		{name: "description is not string", data: map[string]interface{}{"role": map[string]interface{}{"role_id": "role_1", "name": "Admin", "description": 1}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRoleDetailResponseData(tt.data, "role_1")
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("parseRoleDetailResponseData() = %#v, want internal/invalid_response", err)
			}
		})
	}
}

func TestParseRoleWriteResponseDataAcceptsMinimalAcknowledgement(t *testing.T) {
	got, err := parseRoleWriteResponseData(map[string]interface{}{
		"role": map[string]interface{}{"role_id": "role_1"},
	}, "role_1")
	if err != nil {
		t.Fatalf("parseRoleWriteResponseData() error = %v", err)
	}
	if got.RoleID != "role_1" || got.Name != "" || got.Description != "" {
		t.Fatalf("parseRoleWriteResponseData() = %#v, want role_id-only acknowledgement", got)
	}
}

func TestParseRoleWriteResponseDataRejectsMalformedOptionalFields(t *testing.T) {
	tests := []struct {
		name string
		role map[string]interface{}
	}{
		{name: "name is null", role: map[string]interface{}{"role_id": "role_1", "name": nil}},
		{name: "name is not string", role: map[string]interface{}{"role_id": "role_1", "name": 1}},
		{name: "name is empty", role: map[string]interface{}{"role_id": "role_1", "name": " "}},
		{name: "description is null", role: map[string]interface{}{"role_id": "role_1", "description": nil}},
		{name: "description is not string", role: map[string]interface{}{"role_id": "role_1", "description": 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRoleWriteResponseData(map[string]interface{}{"role": tt.role}, "role_1")
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("parseRoleWriteResponseData() = %#v, want internal/invalid_response", err)
			}
		})
	}
}

func TestAppsRoleDeletePrettyOutputUsesRequestRoleID(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{},
		},
	})

	if err := runAppsShortcut(t, AppsRoleDelete,
		[]string{"+role-delete", "--app-id", "app_x", "--role-id", "role_1", "--yes", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "Deleted role role_1") {
		t.Fatalf("pretty output = %q, want deleted role summary", got)
	}
}

func TestAppsRoleDeleteNormalizesEmptyBackendData(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"role-id": "role_1",
	})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{},
		},
	})

	if err := AppsRoleDelete.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	if env.Data["role_id"] != "role_1" || env.Data["deleted"] != true {
		t.Fatalf("delete output = %#v, want normalized role_id/deleted", env.Data)
	}
}

func TestAppsRoleDeleteRejectsMissingNullOrNonObjectData(t *testing.T) {
	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{name: "missing", body: map[string]interface{}{"code": 0, "msg": ""}},
		{name: "null", body: map[string]interface{}{"code": 0, "msg": "", "data": nil}},
		{name: "array", body: map[string]interface{}{"code": 0, "msg": "", "data": []interface{}{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _, reg := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
				"app-id":  "app_x",
				"role-id": "role_1",
			})
			reg.Register(&httpmock.Stub{
				Method: "DELETE",
				URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1",
				Body:   tt.body,
			})

			err := AppsRoleDelete.Execute(context.Background(), rctx)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("Execute() = %#v, want internal/invalid_response", err)
			}
		})
	}
}

func TestAppsRoleDeleteRejectsContradictoryBackendData(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{name: "non-empty response missing role id", data: map[string]interface{}{"deleted": true}},
		{name: "non-empty response missing deleted", data: map[string]interface{}{"role_id": "role_1"}},
		{name: "mismatched role id", data: map[string]interface{}{"role_id": "role_other", "deleted": true}},
		{name: "role id is not a string", data: map[string]interface{}{"role_id": 1, "deleted": true}},
		{name: "explicit false", data: map[string]interface{}{"role_id": "role_1", "deleted": false}},
		{name: "deleted is not boolean", data: map[string]interface{}{"role_id": "role_1", "deleted": "true"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _, reg := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
				"app-id":  "app_x",
				"role-id": "role_1",
			})
			reg.Register(&httpmock.Stub{
				Method: "DELETE",
				URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "",
					"data": tt.data,
				},
			})

			err := AppsRoleDelete.Execute(context.Background(), rctx)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("Execute() = %#v, want internal/invalid_response", err)
			}
		})
	}
}

func TestAppsRoleListPrettyDoesNotDisplayMemberCount(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"role_id":      "role_1",
						"name":         "Admin",
						"description":  "Manage",
						"member_count": 12345,
					},
				},
				"has_more": false,
				"total":    1,
			},
		},
	})

	if err := runAppsShortcut(t, AppsRoleList,
		[]string{"+role-list", "--app-id", "app_x", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if strings.Contains(got, "MEMBERS") || strings.Contains(got, "12345") {
		t.Fatalf("pretty output must not display member_count, got: %q", got)
	}
	if !strings.Contains(got, "ROLE ID") || !strings.Contains(got, "role_1") {
		t.Fatalf("pretty output missing role fields: %q", got)
	}
}

func TestAppsRoleCreateBodyOmitsRoleIDUnlessChanged(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id": "app_x",
		"name":   "Admin",
	})
	body := buildRoleCreateBody(rctx)
	if _, ok := body["role_id"]; ok {
		t.Fatalf("role_id should be omitted when --role-id is not changed: %#v", body)
	}

	rctx, _, _ = newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"name":    "Admin",
		"role-id": " role_1 ",
	})
	body = buildRoleCreateBody(rctx)
	if body["role_id"] != "role_1" {
		t.Fatalf("role_id = %#v, want role_1; body=%#v", body["role_id"], body)
	}
}

func TestAppsRoleCreateBodyPreservesExplicitEmptyDescription(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":      "app_x",
		"name":        "Admin",
		"description": "",
	})
	body := buildRoleCreateBody(rctx)
	description, ok := body["description"]
	if !ok {
		t.Fatalf("description should be present when --description is explicitly provided: %#v", body)
	}
	if description != "" {
		t.Fatalf("description = %#v, want empty string", description)
	}
}

func TestAppsRoleDryRunPreservesExplicitEmptyDescription(t *testing.T) {
	tests := []struct {
		name  string
		sc    common.Shortcut
		flags map[string]string
	}{
		{
			name: "create",
			sc:   AppsRoleCreate,
			flags: map[string]string{
				"app-id":      "app_x",
				"name":        "Admin",
				"description": "",
			},
		},
		{
			name: "update",
			sc:   AppsRoleUpdate,
			flags: map[string]string{
				"app-id":      "app_x",
				"role-id":     "role_1",
				"description": "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _, _ := newRoleRCtx(t, roleCRUDFlagDefs(), tt.flags)
			if tt.sc.Validate != nil {
				if err := tt.sc.Validate(context.Background(), rctx); err != nil {
					t.Fatalf("Validate() = %v", err)
				}
			}
			raw, err := json.Marshal(tt.sc.DryRun(context.Background(), rctx))
			if err != nil {
				t.Fatalf("marshal dry-run: %v", err)
			}
			call := firstDryRunCall(t, raw)
			body := mapFromInterface(t, call.Body)
			description, ok := body["description"]
			if !ok {
				t.Fatalf("description should be present when explicitly set empty: %#v", body)
			}
			if description != "" {
				t.Fatalf("description = %#v, want empty string", description)
			}
		})
	}
}

func TestAppsRoleUpdateBodyOnlyChangedFields(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":      "app_x",
		"role-id":     "role_1",
		"description": " updated ",
	})
	body := buildRoleUpdateBody(rctx)
	if len(body) != 1 || body["description"] != "updated" {
		t.Fatalf("body = %#v, want only trimmed description", body)
	}
	if _, ok := body["name"]; ok {
		t.Fatalf("name should be omitted when unchanged: %#v", body)
	}
}

func TestAppsRoleUpdateEmptyNameValidationParam(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"role-id": "role_1",
		"name":    " ",
	})
	assertRoleValidationParam(t, AppsRoleUpdate.Validate(context.Background(), rctx), "--name")
}

func TestAppsRoleListEmptyNameValidationParam(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id": "app_x",
		"name":   " ",
	})
	assertRoleValidationParam(t, AppsRoleList.Validate(context.Background(), rctx), "--name")
}

func TestAppsRoleUpdateMissingNameAndDescriptionValidationParams(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"role-id": "role_1",
	})
	assertRoleValidationParams(t, AppsRoleUpdate.Validate(context.Background(), rctx), "--name", "--description")
}

func TestAppsRoleCreateInvalidOptionalRoleIDParam(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"name":    "Admin",
		"role-id": "bad/role",
	})
	assertRoleValidationParam(t, AppsRoleCreate.Validate(context.Background(), rctx), "--role-id")
}

func TestAppsRoleCreateBlankOptionalRoleIDValidationParam(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"name":    "Admin",
		"role-id": " ",
	})
	assertRoleValidationParam(t, AppsRoleCreate.Validate(context.Background(), rctx), "--role-id")
}

func TestAppsRoleRequiredInputsUseTypedValidation(t *testing.T) {
	tests := []struct {
		name  string
		sc    common.Shortcut
		flags map[string]string
		param string
	}{
		{
			name:  "list missing app id",
			sc:    AppsRoleList,
			flags: map[string]string{},
			param: "--app-id",
		},
		{
			name:  "get missing role id",
			sc:    AppsRoleGet,
			flags: map[string]string{"app-id": "app_x"},
			param: "--role-id",
		},
		{
			name:  "create missing name",
			sc:    AppsRoleCreate,
			flags: map[string]string{"app-id": "app_x"},
			param: "--name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _, _ := newRoleRCtx(t, roleCRUDFlagDefs(), tt.flags)
			assertRoleValidationParam(t, tt.sc.Validate(context.Background(), rctx), tt.param)
		})
	}
}

func TestAppsRoleCreateMissingNameDoesNotSuggestInventingOne(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":      "app_x",
		"description": "Can inspect BI reports",
	})
	problem := assertRoleValidationParam(t, AppsRoleCreate.Validate(context.Background(), rctx), "--name")
	if !strings.Contains(problem.Hint, "do not infer a name") {
		t.Fatalf("hint = %q, want non-invention guidance", problem.Hint)
	}
}

func TestAppsRoleListExecutePagination(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":     " app_x ",
		"page-size":  "2",
		"page-token": "4",
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles",
		OnMatch: func(req *http.Request) {
			q := req.URL.Query()
			if q.Get("limit") != "2" || q.Get("offset") != "4" || q.Get("name") != "" {
				t.Fatalf("query = %s, want limit=2 offset=4", req.URL.RawQuery)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"role_id":      "role_1",
						"name":         "Admin",
						"description":  "Manage",
						"member_count": 3,
					},
				},
				"has_more":   true,
				"page_token": "opaque_backend_token",
				"total":      "9",
				"trace_id":   "kept",
			},
		},
	})

	if err := AppsRoleList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	if env.Data["page_token"] != "6" {
		t.Fatalf("page_token = %#v, want 6; output=%s", env.Data["page_token"], stdoutBuf.String())
	}
	if env.Data["has_more"] != true || env.Data["total"] != float64(9) || env.Data["trace_id"] != "kept" {
		t.Fatalf("unexpected normalized data: %#v", env.Data)
	}
	items, ok := env.Data["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v, want one role", env.Data["items"])
	}
	role, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("role item = %#v, want object", items[0])
	}
	if role["member_count"] != float64(3) {
		t.Fatalf("role-list JSON should preserve backend member_count, got %#v", role)
	}
}

func TestAppsRoleListNameFilterScansAllPagesAndPaginatesMatches(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":     "app_x",
		"name":       "Target",
		"page-size":  "1",
		"page-token": "1",
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles",
		OnMatch: func(req *http.Request) {
			q := req.URL.Query()
			if q.Get("limit") != "100" || q.Get("offset") != "0" || q.Get("name") != "Target" {
				t.Fatalf("first scan query = %s", req.URL.RawQuery)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"role_id": "role_target_1", "name": "Target"},
					map[string]interface{}{"role_id": "role_other", "name": "Other"},
				},
				"has_more": true,
				"total":    3,
				"trace_id": "kept",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles",
		OnMatch: func(req *http.Request) {
			q := req.URL.Query()
			if q.Get("limit") != "100" || q.Get("offset") != "100" || q.Get("name") != "Target" {
				t.Fatalf("second scan query = %s", req.URL.RawQuery)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"role_id": "role_target_2", "name": "Target"},
				},
				"has_more": false,
				"total":    3,
			},
		},
	})

	if err := AppsRoleList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	items, ok := env.Data["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v, want one paged exact match", env.Data["items"])
	}
	role, ok := items[0].(map[string]interface{})
	if !ok || role["role_id"] != "role_target_2" || role["name"] != "Target" {
		t.Fatalf("role = %#v, want second exact match", items[0])
	}
	if env.Data["total"] != float64(2) || env.Data["has_more"] != false || env.Data["page_token"] != "" || env.Data["trace_id"] != "kept" {
		t.Fatalf("normalized filtered output = %#v", env.Data)
	}
}

func TestAppsRoleListRejectsMalformedPageShape(t *testing.T) {
	validRole := map[string]interface{}{"role_id": "role_1", "name": "Admin"}
	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{name: "missing items", data: map[string]interface{}{"has_more": false, "total": 0}},
		{name: "items is not array", data: map[string]interface{}{"items": "role_1", "has_more": false, "total": 1}},
		{name: "item is not object", data: map[string]interface{}{"items": []interface{}{"role_1"}, "has_more": false, "total": 1}},
		{name: "item missing role id", data: map[string]interface{}{"items": []interface{}{map[string]interface{}{"name": "Admin"}}, "has_more": false, "total": 1}},
		{name: "legacy id does not replace role id", data: map[string]interface{}{"items": []interface{}{map[string]interface{}{"id": "role_1", "name": "Admin"}}, "has_more": false, "total": 1}},
		{name: "role id is empty", data: map[string]interface{}{"items": []interface{}{map[string]interface{}{"role_id": " ", "name": "Admin"}}, "has_more": false, "total": 1}},
		{name: "role id is not string", data: map[string]interface{}{"items": []interface{}{map[string]interface{}{"role_id": 1, "name": "Admin"}}, "has_more": false, "total": 1}},
		{name: "item missing name", data: map[string]interface{}{"items": []interface{}{map[string]interface{}{"role_id": "role_1"}}, "has_more": false, "total": 1}},
		{name: "name is empty", data: map[string]interface{}{"items": []interface{}{map[string]interface{}{"role_id": "role_1", "name": " "}}, "has_more": false, "total": 1}},
		{name: "name is not string", data: map[string]interface{}{"items": []interface{}{map[string]interface{}{"role_id": "role_1", "name": 1}}, "has_more": false, "total": 1}},
		{name: "missing has more", data: map[string]interface{}{"items": []interface{}{}, "total": 0}},
		{name: "has more is not boolean", data: map[string]interface{}{"items": []interface{}{}, "has_more": "false", "total": 0}},
		{name: "missing total", data: map[string]interface{}{"items": []interface{}{}, "has_more": false}},
		{name: "total string has whitespace", data: map[string]interface{}{"items": []interface{}{validRole}, "has_more": false, "total": " 1"}},
		{name: "total string has sign", data: map[string]interface{}{"items": []interface{}{validRole}, "has_more": false, "total": "+1"}},
		{name: "total string is fractional", data: map[string]interface{}{"items": []interface{}{validRole}, "has_more": false, "total": "1.0"}},
		{name: "total string exceeds int range", data: map[string]interface{}{"items": []interface{}{}, "has_more": false, "total": "9223372036854775808"}},
		{name: "total is fractional", data: map[string]interface{}{"items": []interface{}{validRole}, "has_more": false, "total": 1.5}},
		{name: "total is negative", data: map[string]interface{}{"items": []interface{}{}, "has_more": false, "total": -1}},
		{name: "total exceeds int range", data: map[string]interface{}{"items": []interface{}{}, "has_more": false, "total": 9.223372036854776e18}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _, reg := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{"app-id": "app_x"})
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/spark/v1/apps/app_x/roles",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "",
					"data": tt.data,
				},
			})

			err := AppsRoleList.Execute(context.Background(), rctx)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("Execute() = %#v, want internal/invalid_response", err)
			}
		})
	}
}

func TestAppsRoleListNameFilterRejectsInvalidPagination(t *testing.T) {
	tests := []struct {
		name          string
		secondPage    []interface{}
		secondHasMore bool
	}{
		{name: "empty page", secondPage: []interface{}{}, secondHasMore: true},
		{name: "repeated page", secondPage: []interface{}{map[string]interface{}{"role_id": "role_1", "name": "Target"}}, secondHasMore: true},
		{
			name: "partially overlapping final page",
			secondPage: []interface{}{
				map[string]interface{}{"role_id": "role_1", "name": "Target"},
				map[string]interface{}{"role_id": "role_2", "name": "Target"},
			},
			secondHasMore: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _, reg := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
				"app-id": "app_x",
				"name":   "Target",
			})
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/spark/v1/apps/app_x/roles",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "",
					"data": map[string]interface{}{
						"items":    []interface{}{map[string]interface{}{"role_id": "role_1", "name": "Target"}},
						"has_more": true,
						"total":    3,
					},
				},
			})
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/spark/v1/apps/app_x/roles",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "",
					"data": map[string]interface{}{
						"items":    tt.secondPage,
						"has_more": tt.secondHasMore,
						"total":    3,
					},
				},
			})

			err := AppsRoleList.Execute(context.Background(), rctx)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("Execute() = %#v, want internal/invalid_response", err)
			}
			if !strings.Contains(problem.Hint, "incomplete exact-name scan") {
				t.Fatalf("hint = %q, want incomplete scan guidance", problem.Hint)
			}
		})
	}
}

func TestAppsRoleListNameFilterRejectsContradictoryTotal(t *testing.T) {
	tests := []struct {
		name          string
		firstTotal    int
		secondTotal   int
		secondItems   []interface{}
		secondHasMore bool
	}{
		{
			name:        "total changes across pages",
			firstTotal:  3,
			secondTotal: 4,
			secondItems: []interface{}{map[string]interface{}{"role_id": "role_2", "name": "Target"}},
		},
		{
			name:        "final page ends before total",
			firstTotal:  3,
			secondTotal: 3,
			secondItems: []interface{}{map[string]interface{}{"role_id": "role_2", "name": "Target"}},
		},
		{
			name:          "has more after reaching total",
			firstTotal:    2,
			secondTotal:   2,
			secondItems:   []interface{}{map[string]interface{}{"role_id": "role_2", "name": "Target"}},
			secondHasMore: true,
		},
		{
			name:        "items exceed total",
			firstTotal:  2,
			secondTotal: 2,
			secondItems: []interface{}{
				map[string]interface{}{"role_id": "role_2", "name": "Target"},
				map[string]interface{}{"role_id": "role_3", "name": "Target"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _, reg := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
				"app-id": "app_x",
				"name":   "Target",
			})
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/spark/v1/apps/app_x/roles",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "",
					"data": map[string]interface{}{
						"items":    []interface{}{map[string]interface{}{"role_id": "role_1", "name": "Target"}},
						"has_more": true,
						"total":    tt.firstTotal,
					},
				},
			})
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/spark/v1/apps/app_x/roles",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "",
					"data": map[string]interface{}{
						"items":    tt.secondItems,
						"has_more": tt.secondHasMore,
						"total":    tt.secondTotal,
					},
				},
			})

			err := AppsRoleList.Execute(context.Background(), rctx)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("Execute() = %#v, want internal/invalid_response", err)
			}
			if !strings.Contains(problem.Hint, "incomplete exact-name scan") {
				t.Fatalf("hint = %q, want incomplete scan guidance", problem.Hint)
			}
		})
	}
}

func TestAppsRoleDeleteExecutePreservesBackendData(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"role-id": "role_1",
	})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"role_id":  "role_1",
				"deleted":  true,
				"audit_id": "audit_123",
			},
		},
	})

	if err := AppsRoleDelete.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	if env.Data["role_id"] != "role_1" || env.Data["deleted"] != true || env.Data["audit_id"] != "audit_123" {
		t.Fatalf("delete output should preserve backend data, got: %#v", env.Data)
	}
}

func TestAppsRoleListPrettySanitizesTerminalContent(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"role_id":     "role_1",
						"name":        "Admin\nFORGED",
						"description": "\x1b[31mred\x1b[0m\tvalue",
					},
				},
				"has_more": false,
				"total":    1,
			},
		},
	})

	if err := runAppsShortcut(t, AppsRoleList,
		[]string{"+role-list", "--app-id", "app_x", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if strings.Contains(got, "\x1b") || strings.Count(got, "\n") != 2 {
		t.Fatalf("pretty output contains terminal injection or extra rows: %q", got)
	}
	for _, want := range []string{"Admin FORGED", "red value"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing sanitized value %q: %q", want, got)
		}
	}
}

func TestAppsRoleAPIErrorGetsAppsHint(t *testing.T) {
	rctx, _, reg := newRoleRCtx(t, roleCRUDFlagDefs(), map[string]string{
		"app-id": "app_x",
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles",
		Body: map[string]interface{}{
			"code": 99991663,
			"msg":  "permission denied",
		},
	})

	err := AppsRoleList.Execute(context.Background(), rctx)
	if err == nil {
		t.Fatalf("Execute() = nil, want API error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %#v, want typed problem", err)
	}
	if problem.Category != errs.CategoryAuthentication || problem.Subtype != errs.SubtypeTokenInvalid || problem.Code != 99991663 {
		t.Fatalf("problem = %+v, want authentication/token_invalid code 99991663", problem)
	}
	if problem.Hint != roleAppHint {
		t.Fatalf("hint = %q, want %q", problem.Hint, roleAppHint)
	}
}

type dryRunCall struct {
	Method string                 `json:"method"`
	URL    string                 `json:"url"`
	Params map[string]interface{} `json:"params"`
	Body   interface{}            `json:"body"`
}

type dryRunEnvelope struct {
	API []dryRunCall `json:"api"`
}

func firstDryRunCall(t *testing.T, raw []byte) dryRunCall {
	t.Helper()
	var env dryRunEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal dry-run %s: %v", raw, err)
	}
	if len(env.API) != 1 {
		t.Fatalf("dry-run calls = %d, want 1: %s", len(env.API), raw)
	}
	return env.API[0]
}

func mapFromInterface(t *testing.T, value interface{}) map[string]interface{} {
	t.Helper()
	if value == nil {
		return nil
	}
	m, ok := value.(map[string]interface{})
	if !ok {
		t.Fatalf("value = %#v, want map[string]interface{}", value)
	}
	return m
}

func assertMapEqual(t *testing.T, got, want map[string]interface{}) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("map = %#v, want %#v", got, want)
	}
	for k, wantValue := range want {
		if got[k] != wantValue {
			t.Fatalf("map[%s] = %#v, want %#v; full map=%#v", k, got[k], wantValue, got)
		}
	}
}

type roleEnvelope struct {
	OK   bool                   `json:"ok"`
	Data map[string]interface{} `json:"data"`
}

func decodeRoleEnvelope(t *testing.T, raw string) roleEnvelope {
	t.Helper()
	var env roleEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal output: %v\nraw: %s", err, raw)
	}
	if !env.OK {
		t.Fatalf("ok = false, raw: %s", raw)
	}
	return env
}
