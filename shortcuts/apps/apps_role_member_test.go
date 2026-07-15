// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func roleMemberFlagDefs() map[string]string {
	return map[string]string{
		"app-id":      "string",
		"role-id":     "string",
		"member-type": "string",
		"users":       "string",
		"departments": "string",
		"chats":       "string",
		"all":         "bool",
		"user-id":     "string",
	}
}

func TestAppsRoleMemberListTipsDescribeServerFilteringAndNativeTable(t *testing.T) {
	tips := strings.Join(AppsRoleMemberList.Tips, "\n")
	for _, want := range []string{
		"--member-type user|department|chat",
		"instead of filtering the full response",
		"only the selected member field",
		"omitted fields are unknown",
		"omit the flag for pre/post-write baselines",
		"--format table",
		"CLI-native member_type/member_id table",
		"no --limit or --page-size",
	} {
		if !strings.Contains(tips, want) {
			t.Fatalf("AppsRoleMemberList tips missing %q: %s", want, tips)
		}
	}
}

func TestAppsRoleMemberAddTipsDescribeAtomicTypedResolution(t *testing.T) {
	tips := strings.Join(AppsRoleMemberAdd.Tips, "\n")
	for _, want := range []string{
		"Resolve every name first",
		"users (ou_)",
		"departments (od-)",
		"chats (oc_)",
		"in one call",
		"stop without a partial write",
	} {
		if !strings.Contains(tips, want) {
			t.Fatalf("AppsRoleMemberAdd tips missing %q: %s", want, tips)
		}
	}
}

func TestAppsRoleMemberRemoveTipsDescribeClearVerification(t *testing.T) {
	tips := strings.Join(AppsRoleMemberRemove.Tips, "\n")
	for _, want := range []string{
		"resolve and verify that exact name",
		"lookup fails, stop",
		"only current member is the target",
		"--all clears members",
		"does not delete the role",
		"confirmed --all operation",
		"unfiltered +role-member-list",
		"users, departments, and chats are empty",
	} {
		if !strings.Contains(tips, want) {
			t.Fatalf("AppsRoleMemberRemove tips missing %q: %s", want, tips)
		}
	}
}

func TestAppsRoleMemberDryRunRequestShapes(t *testing.T) {
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
			name: "member-list",
			flags: map[string]string{
				"app-id": " app_x ", "role-id": " role_1 ", "member-type": "user",
			},
			sc:     AppsRoleMemberList,
			method: "GET",
			url:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
			params: map[string]interface{}{"member_type": "user"},
		},
		{
			name:   "member-add",
			flags:  map[string]string{"app-id": "app_x", "role-id": "role_1", "users": "ou_a, ou_b", "departments": "od-a", "chats": "oc_a"},
			sc:     AppsRoleMemberAdd,
			method: "POST",
			url:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_add",
			body: map[string]interface{}{
				"users":       []interface{}{"ou_a", "ou_b"},
				"departments": []interface{}{"od-a"},
				"chats":       []interface{}{"oc_a"},
			},
		},
		{
			name:   "member-remove",
			flags:  map[string]string{"app-id": "app_x", "role-id": "role_1", "users": "ou_a", "chats": "oc_a"},
			sc:     AppsRoleMemberRemove,
			method: "POST",
			url:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_remove",
			body: map[string]interface{}{
				"users": []interface{}{"ou_a"},
				"chats": []interface{}{"oc_a"},
			},
		},
		{
			name:   "member-remove-all",
			flags:  map[string]string{"app-id": "app_x", "role-id": "role_1", "all": "true"},
			sc:     AppsRoleMemberRemove,
			method: "POST",
			url:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_remove",
			body:   map[string]interface{}{"all": true},
		},
		{
			name:   "match-list",
			flags:  map[string]string{"app-id": "app_x", "role-id": "ignored", "user-id": " ou_user "},
			sc:     AppsRoleMatchList,
			method: "POST",
			url:    "/open-apis/spark/v1/apps/app_x/user_role_list",
			body:   map[string]interface{}{"target_user_id": "ou_user"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := validatedRoleMemberDryRunCall(t, tt.sc, tt.flags)
			if call.Method != tt.method || call.URL != tt.url {
				t.Fatalf("call = %s %s, want %s %s", call.Method, call.URL, tt.method, tt.url)
			}
			assertMapEqual(t, call.Params, tt.params)
			assertJSONEquivalent(t, call.Body, tt.body)
			if tt.name == "match-list" {
				if call.Params != nil {
					t.Fatalf("match-list must use body instead of query params: %#v", call.Params)
				}
				if _, ok := call.Body.(map[string]interface{})["role_id"]; ok {
					t.Fatalf("match-list body must not include role_id: %#v", call.Body)
				}
			}
		})
	}
}

func TestAppsRoleMemberRemoveAllRejectsExplicitMembers(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"role-id": "role_1",
		"all":     "true",
		"users":   "ou_a",
	})
	assertRoleValidationParams(t, AppsRoleMemberRemove.Validate(context.Background(), rctx), "--all", "--users")
}

func TestAppsRoleMemberAddRejectsNoMembers(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"role-id": "role_1",
	})
	assertRoleValidationParams(t, AppsRoleMemberAdd.Validate(context.Background(), rctx), "--users", "--departments", "--chats")
}

func TestAppsRoleMemberRemoveRejectsNoMembersOrAll(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"role-id": "role_1",
	})
	assertRoleValidationParams(t, AppsRoleMemberRemove.Validate(context.Background(), rctx), "--users", "--departments", "--chats", "--all")
}

func TestAppsRoleMemberRequiredInputsUseTypedValidation(t *testing.T) {
	tests := []struct {
		name  string
		sc    common.Shortcut
		flags map[string]string
		param string
	}{
		{
			name:  "member list missing role id",
			sc:    AppsRoleMemberList,
			flags: map[string]string{"app-id": "app_x"},
			param: "--role-id",
		},
		{
			name:  "member add missing app id",
			sc:    AppsRoleMemberAdd,
			flags: map[string]string{"role-id": "role_1", "users": "ou_a"},
			param: "--app-id",
		},
		{
			name:  "match list missing app id",
			sc:    AppsRoleMatchList,
			flags: map[string]string{"user-id": "ou_user"},
			param: "--app-id",
		},
		{
			name:  "match list missing user id",
			sc:    AppsRoleMatchList,
			flags: map[string]string{"app-id": "app_x"},
			param: "--user-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _, _ := newRoleRCtx(t, roleMemberFlagDefs(), tt.flags)
			assertRoleValidationParam(t, tt.sc.Validate(context.Background(), rctx), tt.param)
		})
	}
}

func TestAppsRoleMatchListRejectsMissingUserID(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"user-id": " ",
	})
	assertRoleValidationParam(t, AppsRoleMatchList.Validate(context.Background(), rctx), "--user-id")
}

func TestAppsRoleMemberRejectsOverflowOnlyChats(t *testing.T) {
	chats := make([]string, maxRoleMembers+1)
	for i := range chats {
		chats[i] = "oc_test"
	}
	rctx, _, _ := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"role-id": "role_1",
		"chats":   strings.Join(chats, ","),
	})
	assertRoleValidationParams(t, AppsRoleMemberAdd.Validate(context.Background(), rctx), "--chats")
}

func TestAppsRoleMemberListExecuteGroups(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":      " app_x ",
		"role-id":     " role_1 ",
		"member-type": "chat",
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
		OnMatch: func(req *http.Request) {
			q := req.URL.Query()
			if q.Get("member_type") != "chat" || q.Get("limit") != "" || q.Get("offset") != "" {
				t.Fatalf("query = %s, want only member_type=chat", req.URL.RawQuery)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"users":       []interface{}{"ou_a"},
				"departments": []interface{}{"od-a"},
				"chats":       []interface{}{"oc_a"},
				"trace_id":    "kept",
			},
		},
	})

	if err := AppsRoleMemberList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	for _, key := range []string{"users", "departments"} {
		if _, exists := env.Data[key]; exists {
			t.Fatalf("filtered output must omit unknown %s: %#v", key, env.Data)
		}
	}
	assertStringList(t, env.Data["chats"], []string{"oc_a"})
	if env.Data["trace_id"] != "kept" {
		t.Fatalf("backend field lost: %#v", env.Data)
	}
	stderr, ok := rctx.IO().ErrOut.(*bytes.Buffer)
	if !ok {
		t.Fatalf("stderr = %T, want *bytes.Buffer", rctx.IO().ErrOut)
	}
	for _, want := range []string{
		"warning: --member-type=chat returns only the selected member field",
		"omitted member fields are unknown",
		"complete member baseline",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
	}
}

func TestAppsRoleMemberListChatFilterFallsBackToUnfilteredQuery(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":      "app_x",
		"role-id":     "role_1",
		"member-type": "chat",
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("member_type"); got != "chat" {
				t.Fatalf("first member_type = %q, want chat", got)
			}
		},
		Body: map[string]interface{}{
			"code": roleErrUnsupportedMemberType,
			"msg":  "参数错误：不支持的 member_type",
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("member_type"); got != "" {
				t.Fatalf("fallback member_type = %q, want empty", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"users":       []interface{}{"ou_a"},
				"departments": []interface{}{"od-a"},
				"chats":       []interface{}{"oc_a"},
			},
		},
	})

	if err := AppsRoleMemberList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	for _, key := range []string{"users", "departments"} {
		if _, exists := env.Data[key]; exists {
			t.Fatalf("fallback projection must omit unknown %s: %#v", key, env.Data)
		}
	}
	assertStringList(t, env.Data["chats"], []string{"oc_a"})
	stderr, ok := rctx.IO().ErrOut.(*bytes.Buffer)
	if !ok {
		t.Fatalf("stderr = %T, want *bytes.Buffer", rctx.IO().ErrOut)
	}
	for _, want := range []string{"warning:", "only the chats field", "complete member baseline"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q: %q", want, stderr.String())
		}
	}
}

func TestAppsRoleMemberListFilteredResponseRequiresOnlySelectedGroup(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":      "app_x",
		"role-id":     "role_1",
		"member-type": "chat",
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"chats":    []interface{}{"oc_a"},
				"trace_id": "kept",
			},
		},
	})

	if err := AppsRoleMemberList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	assertStringList(t, env.Data["chats"], []string{"oc_a"})
	for _, key := range []string{"users", "departments"} {
		if _, exists := env.Data[key]; exists {
			t.Fatalf("filtered response must omit %s: %#v", key, env.Data)
		}
	}
	if env.Data["trace_id"] != "kept" {
		t.Fatalf("backend metadata lost: %#v", env.Data)
	}
}

func TestShouldRetryRoleMemberListWithoutFilterIsNarrow(t *testing.T) {
	documentedUnsupported := errs.NewAPIError(errs.SubtypeInvalidParameters, "unsupported member type").WithCode(roleErrUnsupportedMemberType)
	if !shouldRetryRoleMemberListWithoutFilter(documentedUnsupported, "chat") {
		t.Fatal("documented chat member_type incompatibility should use the unfiltered fallback")
	}
	currentUnsupported := errs.NewAPIError(errs.SubtypeUnknown, "parameter error").WithCode(400004040)
	if !shouldRetryRoleMemberListWithoutFilter(currentUnsupported, "chat") {
		t.Fatal("current chat member_type incompatibility should use the unfiltered fallback")
	}
	legacyUnsupported := errs.NewAPIError(errs.SubtypeUnknown, "unsupported member_type").WithCode(2)
	if !shouldRetryRoleMemberListWithoutFilter(legacyUnsupported, "chat") {
		t.Fatal("chat member_type incompatibility should use the unfiltered fallback")
	}
	if shouldRetryRoleMemberListWithoutFilter(currentUnsupported, "user") {
		t.Fatal("user filter must not use the chat-only compatibility fallback")
	}
	unrelated := errs.NewAPIError(errs.SubtypeUnknown, "invalid role_id").WithCode(2)
	if shouldRetryRoleMemberListWithoutFilter(unrelated, "chat") {
		t.Fatal("unrelated code=2 errors must pass through unchanged")
	}
}

func TestAppsRoleMemberListRejectsIncompleteOrMalformedBackendData(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{name: "missing chats", data: map[string]interface{}{"users": []interface{}{}, "departments": []interface{}{}}},
		{name: "users is not an array", data: map[string]interface{}{"users": "ou_a", "departments": []interface{}{}, "chats": []interface{}{}}},
		{name: "users contains non-string", data: map[string]interface{}{"users": []interface{}{7}, "departments": []interface{}{}, "chats": []interface{}{}}},
		{name: "department has wrong prefix", data: map[string]interface{}{"users": []interface{}{}, "departments": []interface{}{"ou_a"}, "chats": []interface{}{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
				"app-id":  "app_x",
				"role-id": "role_1",
			})
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "",
					"data": tt.data,
				},
			})

			err := AppsRoleMemberList.Execute(context.Background(), rctx)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("Execute() = %#v, want internal/invalid_response", err)
			}
		})
	}
}

func TestAppsRoleMemberListRejectsMissingNullOrNonObjectData(t *testing.T) {
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
			rctx, _, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
				"app-id":  "app_x",
				"role-id": "role_1",
			})
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
				Body:   tt.body,
			})

			err := AppsRoleMemberList.Execute(context.Background(), rctx)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("Execute() = %#v, want internal/invalid_response", err)
			}
		})
	}
}

func TestAppsRoleMemberListNormalizesVerifiedEmptyDataSentinel(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"role-id": "role_1",
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{},
		},
	})

	if err := AppsRoleMemberList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	assertStringList(t, env.Data["users"], []string{})
	assertStringList(t, env.Data["departments"], []string{})
	assertStringList(t, env.Data["chats"], []string{})
}

func TestAppsRoleMemberListFilteredEmptyDataOnlySynthesizesSelectedGroup(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":      "app_x",
		"role-id":     "role_1",
		"member-type": "chat",
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{},
		},
	})

	if err := AppsRoleMemberList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	assertStringList(t, env.Data["chats"], []string{})
	for _, key := range []string{"users", "departments"} {
		if _, exists := env.Data[key]; exists {
			t.Fatalf("filtered empty response must omit unknown %s: %#v", key, env.Data)
		}
	}
}

func TestAppsRoleMemberListPreservesExplicitEmptyGroups(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"role-id": "role_1",
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"users":       []interface{}{},
				"departments": []interface{}{},
				"chats":       []interface{}{},
			},
		},
	})

	if err := AppsRoleMemberList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	assertStringList(t, env.Data["users"], []string{})
	assertStringList(t, env.Data["departments"], []string{})
	assertStringList(t, env.Data["chats"], []string{})
}

func TestAppsRoleMemberListPrettyOutput(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"users":       []interface{}{"ou_a", "ou_b"},
				"departments": []interface{}{"od-a"},
				"chats":       []interface{}{},
			},
		},
	})

	if err := runAppsShortcut(t, AppsRoleMemberList,
		[]string{"+role-member-list", "--app-id", "app_x", "--role-id", "role_1", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{"users:", "  - ou_a", "  - ou_b", "departments:", "  - od-a", "chats: []"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q: %q", want, got)
		}
	}
}

func TestAppsRoleMemberListFilteredPrettyOmitsUnknownGroups(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"users":       []interface{}{"ou_a"},
				"departments": []interface{}{"od-a"},
				"chats":       []interface{}{"oc_a"},
			},
		},
	})

	if err := runAppsShortcut(t, AppsRoleMemberList,
		[]string{"+role-member-list", "--app-id", "app_x", "--role-id", "role_1", "--member-type", "chat", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "chats:\n  - oc_a") {
		t.Fatalf("pretty output missing selected chats: %q", got)
	}
	for _, absent := range []string{"users:", "departments:"} {
		if strings.Contains(got, absent) {
			t.Fatalf("pretty output must omit unknown group %q: %q", absent, got)
		}
	}
}

func TestAppsRoleMemberListStructuredFormatsUseMemberRows(t *testing.T) {
	tests := []struct {
		format string
		wants  []string
	}{
		{"table", []string{"member_id", "member_type", "user", "ou_a", "department", "od-a", "chat", "oc_a"}},
		{"csv", []string{"member_id,member_type", "ou_a,user", "od-a,department", "oc_a,chat"}},
		{"ndjson", []string{`"member_type":"user"`, `"member_id":"ou_a"`, `"member_type":"department"`, `"member_id":"od-a"`, `"member_type":"chat"`, `"member_id":"oc_a"`}},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_list",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "",
					"data": map[string]interface{}{
						"users":       []interface{}{"ou_a"},
						"departments": []interface{}{"od-a"},
						"chats":       []interface{}{"oc_a"},
					},
				},
			})

			if err := runAppsShortcut(t, AppsRoleMemberList,
				[]string{"+role-member-list", "--app-id", "app_x", "--role-id", "role_1", "--format", tt.format, "--as", "user"},
				factory, stdout); err != nil {
				t.Fatalf("execute err=%v", err)
			}
			got := stdout.String()
			if strings.Contains(got, "(empty)") {
				t.Fatalf("%s output must not be empty when users/departments/chats exist: %q", tt.format, got)
			}
			for _, want := range tt.wants {
				if !strings.Contains(got, want) {
					t.Fatalf("%s output missing %q: %q", tt.format, want, got)
				}
			}
		})
	}
}

func TestAppsRoleMemberAddExecuteSendsGroupedBody(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":      "app_x",
		"role-id":     "role_1",
		"users":       "ou_request",
		"departments": "od-request",
		"chats":       "oc_request",
	})
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_add",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"users":       []interface{}{"ou_backend"},
				"departments": []interface{}{"od-backend"},
				"chats":       []interface{}{"oc_backend"},
				"audit_id":    "audit_123",
			},
		},
	}
	reg.Register(stub)

	if err := AppsRoleMemberAdd.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	assertJSONEquivalent(t, mustDecodeJSON(t, stub.CapturedBody), map[string]interface{}{
		"users":       []interface{}{"ou_request"},
		"departments": []interface{}{"od-request"},
		"chats":       []interface{}{"oc_request"},
	})
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	if env.Data["audit_id"] != "audit_123" {
		t.Fatalf("backend field lost: %#v", env.Data)
	}
	assertStringList(t, env.Data["users"], []string{"ou_backend"})
	assertStringList(t, env.Data["departments"], []string{"od-backend"})
	assertStringList(t, env.Data["chats"], []string{"oc_backend"})
}

func TestAppsRoleMemberAddPrettyOutput(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_add",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"users": []interface{}{"ou_backend"},
				"chats": []interface{}{"oc_backend"},
			},
		},
	})

	if err := runAppsShortcut(t, AppsRoleMemberAdd,
		[]string{"+role-member-add", "--app-id", "app_x", "--role-id", "role_1", "--users", "ou_request", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{"users:", "  - ou_backend", "chats:", "  - oc_backend"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "departments:") {
		t.Fatalf("pretty output must omit member groups absent from the write response: %q", got)
	}
}

func TestAppsRoleMemberAddPreservesBackendDataOnly(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"role-id": "role_1",
		"users":   "ou_request",
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_add",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"audit_id": "audit_123",
				"status":   "accepted",
			},
		},
	})

	if err := AppsRoleMemberAdd.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	if env.Data["audit_id"] != "audit_123" || env.Data["status"] != "accepted" {
		t.Fatalf("backend fields lost: %#v", env.Data)
	}
	for _, key := range []string{"users", "departments", "chats"} {
		if _, ok := env.Data[key]; ok {
			t.Fatalf("%s should not be derived from requested members: %#v", key, env.Data)
		}
	}
}

func TestAppsRoleMemberMutationsRejectMalformedReturnedGroups(t *testing.T) {
	tests := []struct {
		name    string
		sc      common.Shortcut
		url     string
		flags   map[string]string
		badData map[string]interface{}
	}{
		{
			name:  "add truncating numeric user id",
			sc:    AppsRoleMemberAdd,
			url:   "/open-apis/spark/v1/apps/app_x/roles/role_1/member_add",
			flags: map[string]string{"app-id": "app_x", "role-id": "role_1", "users": "ou_request"},
			badData: map[string]interface{}{
				"users": []interface{}{1.9},
			},
		},
		{
			name:  "remove scalar chat id",
			sc:    AppsRoleMemberRemove,
			url:   "/open-apis/spark/v1/apps/app_x/roles/role_1/member_remove",
			flags: map[string]string{"app-id": "app_x", "role-id": "role_1", "chats": "oc_request"},
			badData: map[string]interface{}{
				"chats": "oc_backend",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, stdoutBuf, reg := newRoleRCtx(t, roleMemberFlagDefs(), tt.flags)
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    tt.url,
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "",
					"data": tt.badData,
				},
			})

			err := tt.sc.Execute(context.Background(), rctx)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("Execute() = %#v, want internal/invalid_response", err)
			}
			if stdoutBuf.Len() != 0 {
				t.Fatalf("malformed mutation response must not produce stdout: %q", stdoutBuf.String())
			}
		})
	}
}

func TestAppsRoleMemberRemovePrettyOutput(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_remove",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"users": []interface{}{"ou_backend"},
			},
		},
	})

	if err := runAppsShortcut(t, AppsRoleMemberRemove,
		[]string{"+role-member-remove", "--app-id", "app_x", "--role-id", "role_1", "--users", "ou_request", "--yes", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{"users:", "  - ou_backend"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q: %q", want, got)
		}
	}
	for _, absent := range []string{"departments:", "chats:"} {
		if strings.Contains(got, absent) {
			t.Fatalf("pretty output must omit absent group %q: %q", absent, got)
		}
	}
}

func TestAppsRoleMemberMutationPrettyWithoutGroupsRequiresReadback(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_add",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"audit_id": "audit_123",
			},
		},
	})

	if err := runAppsShortcut(t, AppsRoleMemberAdd,
		[]string{"+role-member-add", "--app-id", "app_x", "--role-id", "role_1", "--users", "ou_request", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "+role-member-list") {
		t.Fatalf("pretty output must require an independent member readback: %q", got)
	}
	for _, absent := range []string{"users:", "departments:", "chats:"} {
		if strings.Contains(got, absent) {
			t.Fatalf("pretty output must not synthesize absent group %q: %q", absent, got)
		}
	}
}

func TestAppsRoleMemberRemoveAllExecuteSendsAllAndPreservesBackendData(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"role-id": "role_1",
		"all":     "true",
	})
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/roles/role_1/member_remove",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"removed":  true,
				"audit_id": "audit_456",
			},
		},
	}
	reg.Register(stub)

	if err := AppsRoleMemberRemove.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	assertJSONEquivalent(t, mustDecodeJSON(t, stub.CapturedBody), map[string]interface{}{"all": true})
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	if env.Data["removed"] != true || env.Data["audit_id"] != "audit_456" {
		t.Fatalf("remove all output should preserve backend data, got: %#v", env.Data)
	}
}

func TestAppsRoleMatchListAPIErrorGetsAppsHint(t *testing.T) {
	rctx, _, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"user-id": "ou_user",
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/user_role_list",
		OnMatch: func(req *http.Request) {
			assertJSONEquivalent(t, mustDecodeJSON(t, readRequestBody(t, req)), map[string]interface{}{
				"target_user_id": "ou_user",
			})
		},
		Body: map[string]interface{}{
			"code": 99991663,
			"msg":  "permission denied",
		},
	})

	err := AppsRoleMatchList.Execute(context.Background(), rctx)
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
	if problem.Hint != roleMatchHint {
		t.Fatalf("hint = %q, want %q", problem.Hint, roleMatchHint)
	}
}

func TestAppsRoleMatchListPrettyOutput(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/user_role_list",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"roles": []interface{}{
					map[string]interface{}{
						"role_id":      "role_1",
						"name":         "Admin",
						"description":  "System role",
						"member_count": 99,
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsRoleMatchList,
		[]string{"+role-match-list", "--app-id", "app_x", "--user-id", "ou_user", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{"ROLE ID", "NAME", "DESCRIPTION", "role_1", "Admin", "System role"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "MEMBERS") {
		t.Fatalf("pretty output must not include member count column: %q", got)
	}
	if strings.Contains(got, "99") {
		t.Fatalf("pretty output must not display member_count value: %q", got)
	}
}

func TestAppsRoleMatchListJSONPreservesMemberCount(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"user-id": "ou_user",
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/user_role_list",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"roles": []interface{}{
					map[string]interface{}{
						"role_id":      "role_1",
						"name":         "Admin",
						"description":  "System role",
						"member_count": 99,
					},
				},
				"trace_id": "kept",
			},
		},
	})

	if err := AppsRoleMatchList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	if env.Data["trace_id"] != "kept" {
		t.Fatalf("top-level backend fields should be preserved: %#v", env.Data)
	}
	roles, ok := env.Data["roles"].([]interface{})
	if !ok || len(roles) != 1 {
		t.Fatalf("roles = %#v, want one role", env.Data["roles"])
	}
	role, ok := roles[0].(map[string]interface{})
	if !ok {
		t.Fatalf("role = %#v, want object", roles[0])
	}
	if role["member_count"] != float64(99) {
		t.Fatalf("role-match-list JSON must preserve member_count: %#v", role)
	}
	if role["role_id"] != "role_1" || role["name"] != "Admin" {
		t.Fatalf("role fields lost: %#v", role)
	}
}

func TestAppsRoleMatchListPreservesExplicitEmptyRoles(t *testing.T) {
	rctx, stdoutBuf, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"user-id": "ou_user",
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/user_role_list",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{"roles": []interface{}{}, "trace_id": "kept"},
		},
	})

	if err := AppsRoleMatchList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	env := decodeRoleEnvelope(t, stdoutBuf.String())
	roles, ok := env.Data["roles"].([]interface{})
	if !ok || len(roles) != 0 {
		t.Fatalf("roles = %#v, want explicit empty array", env.Data["roles"])
	}
	if env.Data["trace_id"] != "kept" {
		t.Fatalf("top-level backend fields should be preserved: %#v", env.Data)
	}
}

func TestAppsRoleMatchListRejectsMalformedRoles(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{name: "missing roles", data: map[string]interface{}{}},
		{name: "roles is not array", data: map[string]interface{}{"roles": "role_1"}},
		{name: "role is not object", data: map[string]interface{}{"roles": []interface{}{"role_1"}}},
		{name: "role missing role id", data: map[string]interface{}{"roles": []interface{}{map[string]interface{}{"name": "Admin"}}}},
		{name: "legacy id does not replace role id", data: map[string]interface{}{"roles": []interface{}{map[string]interface{}{"id": "role_1", "name": "Admin"}}}},
		{name: "role id is empty", data: map[string]interface{}{"roles": []interface{}{map[string]interface{}{"role_id": " ", "name": "Admin"}}}},
		{name: "role id is not string", data: map[string]interface{}{"roles": []interface{}{map[string]interface{}{"role_id": 1, "name": "Admin"}}}},
		{name: "role missing name", data: map[string]interface{}{"roles": []interface{}{map[string]interface{}{"role_id": "role_1"}}}},
		{name: "name is empty", data: map[string]interface{}{"roles": []interface{}{map[string]interface{}{"role_id": "role_1", "name": " "}}}},
		{name: "name is not string", data: map[string]interface{}{"roles": []interface{}{map[string]interface{}{"role_id": "role_1", "name": 1}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _, reg := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
				"app-id":  "app_x",
				"user-id": "ou_user",
			})
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/spark/v1/apps/app_x/user_role_list",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "",
					"data": tt.data,
				},
			})

			err := AppsRoleMatchList.Execute(context.Background(), rctx)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("Execute() = %#v, want internal/invalid_response", err)
			}
		})
	}
}

func TestAppsRoleMatchListRejectsNonOpenUserID(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, roleMemberFlagDefs(), map[string]string{
		"app-id":  "app_x",
		"user-id": "alice@example.com",
	})
	assertRoleValidationParam(t, AppsRoleMatchList.Validate(context.Background(), rctx), "--user-id")
}

func validatedRoleMemberDryRunCall(t *testing.T, sc common.Shortcut, flags map[string]string) dryRunCall {
	t.Helper()
	rctx, _, _ := newRoleRCtx(t, roleMemberFlagDefs(), flags)
	if sc.Validate != nil {
		if err := sc.Validate(context.Background(), rctx); err != nil {
			t.Fatalf("Validate() = %v", err)
		}
	}
	raw, err := json.Marshal(sc.DryRun(context.Background(), rctx))
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	return firstDryRunCall(t, raw)
}

func assertJSONEquivalent(t *testing.T, got, want interface{}) {
	t.Helper()
	gotNorm := normalizeJSONValue(t, got)
	wantNorm := normalizeJSONValue(t, want)
	if !reflect.DeepEqual(gotNorm, wantNorm) {
		t.Fatalf("json value = %#v, want %#v", gotNorm, wantNorm)
	}
}

func normalizeJSONValue(t *testing.T, value interface{}) interface{} {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json value: %v", err)
	}
	return mustDecodeJSON(t, raw)
}

func mustDecodeJSON(t *testing.T, raw []byte) interface{} {
	t.Helper()
	var value interface{}
	if len(raw) == 0 {
		t.Fatalf("empty JSON body")
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("unmarshal JSON %s: %v", raw, err)
	}
	return value
}

func readRequestBody(t *testing.T, req *http.Request) []byte {
	t.Helper()
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	return raw
}

func assertStringList(t *testing.T, value interface{}, want []string) {
	t.Helper()
	got := roleIDValues(value)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("list = %#v, want %#v", got, want)
	}
}
