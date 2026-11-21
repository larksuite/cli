// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

// requireAppsDeleteValidation asserts err is a ValidationError with the given
// subtype and param; Param lives on *errs.ValidationError, not on Problem.
func requireAppsDeleteValidation(t *testing.T, err error, subtype errs.Subtype, param string) *errs.ValidationError {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Subtype != subtype || ve.Param != param {
		t.Fatalf("subtype/param = %q/%q, want %q/%q", ve.Subtype, ve.Param, subtype, param)
	}
	return ve
}

func TestAppsDelete_Metadata(t *testing.T) {
	if AppsDelete.Command != "+delete" {
		t.Fatalf("command = %q", AppsDelete.Command)
	}
	if AppsDelete.Risk != "high-risk-write" {
		t.Fatalf("risk = %q, want high-risk-write so the framework enforces --yes", AppsDelete.Risk)
	}
	wantScopes := map[string]bool{"spark:app:read": true, "spark:app:write": true}
	if len(AppsDelete.Scopes) != len(wantScopes) {
		t.Fatalf("scopes = %v", AppsDelete.Scopes)
	}
	for _, s := range AppsDelete.Scopes {
		if !wantScopes[s] {
			t.Fatalf("unexpected scope %q in %v", s, AppsDelete.Scopes)
		}
	}
	if len(AppsDelete.AuthTypes) != 1 || AppsDelete.AuthTypes[0] != "user" {
		t.Fatalf("auth types = %v, want [user]", AppsDelete.AuthTypes)
	}
}

func TestAppsShortcuts_IncludesDelete(t *testing.T) {
	for _, sc := range Shortcuts() {
		if sc.Command == "+delete" {
			return
		}
	}
	t.Fatal("Shortcuts() does not register +delete")
}

func TestAppsDelete_RejectsBlankFlags(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantParam string
	}{
		{"blank app-id", []string{"+delete", "--app-id", "  ", "--confirm-name", "App", "--yes", "--as", "user"}, "--app-id"},
		{"blank confirm-name", []string{"+delete", "--app-id", "app_x", "--confirm-name", "  ", "--yes", "--as", "user"}, "--confirm-name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, _ := newAppsExecuteFactory(t)
			err := runAppsShortcut(t, AppsDelete, tc.args, factory, stdout)
			requireAppsDeleteValidation(t, err, errs.SubtypeInvalidArgument, tc.wantParam)
		})
	}
}

func TestAppsDelete_RequiresFlags(t *testing.T) {
	for _, flag := range []string{"app-id", "confirm-name"} {
		t.Run(flag, func(t *testing.T) {
			factory, stdout, _ := newAppsExecuteFactory(t)
			args := []string{"+delete", "--yes", "--as", "user"}
			if flag != "app-id" {
				args = append(args, "--app-id", "app_x")
			}
			if flag != "confirm-name" {
				args = append(args, "--confirm-name", "App")
			}
			err := runAppsShortcut(t, AppsDelete, args, factory, stdout)
			if err == nil || !strings.Contains(err.Error(), flag) {
				t.Fatalf("expected missing --%s error, got %v", flag, err)
			}
		})
	}
}

func TestAppsDelete_DryRunListsBothRequestsWithoutCalling(t *testing.T) {
	// No stubs registered: any real request fails with "httpmock: no stub".
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDelete,
		[]string{"+delete", "--app-id", "app_x", "--confirm-name", "App", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err = %v", err)
	}
	got := stdout.String()
	getIdx := strings.Index(got, `"GET"`)
	delIdx := strings.Index(got, `"DELETE"`)
	if getIdx < 0 || delIdx < 0 || getIdx > delIdx {
		t.Fatalf("dry-run must list GET before DELETE: %s", got)
	}
	if strings.Count(got, "/open-apis/spark/v1/apps/app_x") < 2 {
		t.Fatalf("dry-run must show the app path for both calls: %s", got)
	}
}

func TestAppsDelete_RequiresYesBeforeAnyRequest(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDelete,
		[]string{"+delete", "--app-id", "app_x", "--confirm-name", "App", "--as", "user"},
		factory, stdout)
	requireAppsProblem(t, err, errs.CategoryConfirmation)
}

// stubAppsDeleteGet registers the +get lookup +delete performs first.
func stubAppsDeleteGet(reg *httpmock.Registry, pathID, appID, name string) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/" + pathID,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{"app_id": appID, "name": name},
			},
		},
	})
}

// stubAppsDeleteDelete registers the DELETE call; data is the response "data".
func stubAppsDeleteDelete(reg *httpmock.Registry, appID string, data map[string]interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/spark/v1/apps/" + appID,
		Body:   map[string]interface{}{"code": 0, "data": data},
	}
	reg.Register(stub)
	return stub
}

func decodeAppsDeleteOutput(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var env map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("decode stdout %q: %v", raw, err)
	}
	data, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("stdout has no data object: %s", raw)
	}
	return data
}

func TestAppsDelete_MovesToTrash(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubAppsDeleteGet(reg, "app_x", "app_x", "审批系统")
	del := stubAppsDeleteDelete(reg, "app_x", map[string]interface{}{"in_trash": true})

	if err := runAppsShortcut(t, AppsDelete,
		[]string{"+delete", "--app-id", "app_x", "--confirm-name", "审批系统", "--yes", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err = %v", err)
	}
	if len(del.CapturedBodies) != 1 {
		t.Fatalf("DELETE calls = %d, want 1", len(del.CapturedBodies))
	}
	data := decodeAppsDeleteOutput(t, stdout.String())
	if data["app_id"] != "app_x" || data["name"] != "审批系统" || data["deleted"] != true || data["in_trash"] != true {
		t.Fatalf("output data = %v", data)
	}
}

func TestAppsDelete_PrettyOutputByInTrash(t *testing.T) {
	cases := []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{"in trash", map[string]interface{}{"in_trash": true}, "moved to trash: app_x (App)"},
		{"permanent", map[string]interface{}{"in_trash": false}, "deleted permanently: app_x (App)"},
		{"field absent", map[string]interface{}{}, "deleted: app_x (App)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			stubAppsDeleteGet(reg, "app_x", "app_x", "App")
			stubAppsDeleteDelete(reg, "app_x", tc.data)
			if err := runAppsShortcut(t, AppsDelete,
				[]string{"+delete", "--app-id", "app_x", "--confirm-name", "App", "--yes", "--format", "pretty", "--as", "user"},
				factory, stdout); err != nil {
				t.Fatalf("execute err = %v", err)
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("pretty output %q does not contain %q", stdout.String(), tc.want)
			}
		})
	}
}

func TestAppsDelete_JSONReportsPermanentDelete(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubAppsDeleteGet(reg, "app_x", "app_x", "App")
	stubAppsDeleteDelete(reg, "app_x", map[string]interface{}{"in_trash": false})
	if err := runAppsShortcut(t, AppsDelete,
		[]string{"+delete", "--app-id", "app_x", "--confirm-name", "App", "--yes", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err = %v", err)
	}
	data := decodeAppsDeleteOutput(t, stdout.String())
	if v, present := data["in_trash"]; !present || v != false {
		t.Fatalf("in_trash must be present and false: %v", data)
	}
	if data["deleted"] != true {
		t.Fatalf("output data = %v", data)
	}
}

func TestAppsDelete_OmitsInTrashWhenServerDoesNot(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubAppsDeleteGet(reg, "app_x", "app_x", "App")
	stubAppsDeleteDelete(reg, "app_x", map[string]interface{}{})
	if err := runAppsShortcut(t, AppsDelete,
		[]string{"+delete", "--app-id", "app_x", "--confirm-name", "App", "--yes", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err = %v", err)
	}
	data := decodeAppsDeleteOutput(t, stdout.String())
	if _, present := data["in_trash"]; present {
		t.Fatalf("in_trash must be omitted when the server does not return it: %v", data)
	}
	if data["deleted"] != true {
		t.Fatalf("output data = %v", data)
	}
}

func TestAppsDelete_RejectsMalformedDeleteResponse(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubAppsDeleteGet(reg, "app_x", "app_x", "App")
	stubAppsDeleteDelete(reg, "app_x", map[string]interface{}{"in_trash": "false"})
	err := runAppsShortcut(t, AppsDelete,
		[]string{"+delete", "--app-id", "app_x", "--confirm-name", "App", "--yes", "--as", "user"},
		factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryInternal)
	if p.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidResponse)
	}
}

func TestAppsDelete_MetaTokenDeletesResolvedAppID(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubAppsDeleteGet(reg, "meta_tok", "app_real", "App")
	del := stubAppsDeleteDelete(reg, "app_real", map[string]interface{}{"in_trash": true})
	if err := runAppsShortcut(t, AppsDelete,
		[]string{"+delete", "--app-id", "meta_tok", "--confirm-name", "App", "--yes", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err = %v", err)
	}
	if len(del.CapturedBodies) != 1 {
		t.Fatalf("DELETE /apps/app_real calls = %d, want 1", len(del.CapturedBodies))
	}
	if data := decodeAppsDeleteOutput(t, stdout.String()); data["app_id"] != "app_real" {
		t.Fatalf("app_id = %v, want app_real", data["app_id"])
	}
}

func TestAppsDelete_NameComparison(t *testing.T) {
	cases := []struct {
		name        string
		confirmName string
		wantDeleted bool
	}{
		{"surrounding whitespace is trimmed", "  App  ", true},
		{"case differs", "app", false},
		{"different name", "Other", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			stubAppsDeleteGet(reg, "app_x", "app_x", "App")
			if tc.wantDeleted {
				stubAppsDeleteDelete(reg, "app_x", map[string]interface{}{"in_trash": true})
			}
			// Without a DELETE stub, an unexpected DELETE fails with "no stub",
			// which the failed_precondition assertion below would not accept.
			err := runAppsShortcut(t, AppsDelete,
				[]string{"+delete", "--app-id", "app_x", "--confirm-name", tc.confirmName, "--yes", "--as", "user"},
				factory, stdout)
			if tc.wantDeleted {
				if err != nil {
					t.Fatalf("execute err = %v", err)
				}
				return
			}
			requireAppsDeleteValidation(t, err, errs.SubtypeFailedPrecondition, "--confirm-name")
		})
	}
}

func TestAppsDelete_NameMismatchDoesNotLeakRealName(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubAppsDeleteGet(reg, "app_x", "app_x", "SecretRealName")
	err := runAppsShortcut(t, AppsDelete,
		[]string{"+delete", "--app-id", "app_x", "--confirm-name", "Guess", "--yes", "--as", "user"},
		factory, stdout)
	p := requireAppsDeleteValidation(t, err, errs.SubtypeFailedPrecondition, "--confirm-name")
	if strings.Contains(p.Message, "SecretRealName") || strings.Contains(p.Hint, "SecretRealName") {
		t.Fatalf("error leaks the real name: message=%q hint=%q", p.Message, p.Hint)
	}
	if !strings.Contains(p.Hint, "lark-cli apps +get --app-id app_x") {
		t.Fatalf("hint = %q, want it to point at +get", p.Hint)
	}
}

func TestAppsDelete_RejectsUnusableGetResponse(t *testing.T) {
	cases := []struct {
		name  string
		input string
		appID string
		appNm string
	}{
		{"missing name", "app_x", "app_x", ""},
		{"missing app_id", "app_x", "", "App"},
		{"app_id without app_ prefix", "meta_tok", "weird_1", "App"},
		{"app_id differs from the app_id passed", "app_a", "app_b", "App"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			stubAppsDeleteGet(reg, tc.input, tc.appID, tc.appNm)
			err := runAppsShortcut(t, AppsDelete,
				[]string{"+delete", "--app-id", tc.input, "--confirm-name", "App", "--yes", "--as", "user"},
				factory, stdout)
			p := requireAppsProblem(t, err, errs.CategoryInternal)
			if p.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidResponse)
			}
		})
	}
}

func stubAppsDeleteBizError(reg *httpmock.Registry, method, appID string, code int, msg string) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: method,
		URL:    "/open-apis/spark/v1/apps/" + appID,
		Body:   map[string]interface{}{"code": code, "msg": msg},
	}
	reg.Register(stub)
	return stub
}

func TestAppsDelete_GetNotFoundStopsBeforeDelete(t *testing.T) {
	for _, code := range []int{40400, 3340400} {
		factory, stdout, reg := newAppsExecuteFactory(t)
		stubAppsDeleteBizError(reg, "GET", "app_x", code, "app not found")
		err := runAppsShortcut(t, AppsDelete,
			[]string{"+delete", "--app-id", "app_x", "--confirm-name", "App", "--yes", "--as", "user"},
			factory, stdout)
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("code %d: expected typed problem, got %T %v", code, err, err)
		}
		if p.Subtype != errs.SubtypeNotFound || p.Hint != appIDListHint || p.Code != code {
			t.Fatalf("code %d: subtype/hint/code = %q/%q/%d", code, p.Subtype, p.Hint, p.Code)
		}
	}
}

func TestAppsDelete_UnsupportedAppType(t *testing.T) {
	for _, method := range []string{"GET", "DELETE"} {
		for _, code := range []int{40002, 3340002} {
			factory, stdout, reg := newAppsExecuteFactory(t)
			if method == "GET" {
				stubAppsDeleteBizError(reg, "GET", "app_x", code, "unsupported app type")
			} else {
				stubAppsDeleteGet(reg, "app_x", "app_x", "App")
				stubAppsDeleteBizError(reg, "DELETE", "app_x", code, "unsupported app type")
			}
			err := runAppsShortcut(t, AppsDelete,
				[]string{"+delete", "--app-id", "app_x", "--confirm-name", "App", "--yes", "--as", "user"},
				factory, stdout)
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("%s %d: expected typed problem, got %T %v", method, code, err, err)
			}
			if p.Subtype != errs.SubtypeFailedPrecondition || p.Hint != unsupportedAppTypeHint || p.Code != code {
				t.Fatalf("%s %d: subtype/hint/code = %q/%q/%d", method, code, p.Subtype, p.Hint, p.Code)
			}
		}
	}
}

func TestAppsDelete_AlreadyDeletedOnDelete(t *testing.T) {
	for _, code := range []int{40300, 3340300} {
		factory, stdout, reg := newAppsExecuteFactory(t)
		stubAppsDeleteGet(reg, "app_x", "app_x", "App")
		stubAppsDeleteBizError(reg, "DELETE", "app_x", code, "app already deleted")
		err := runAppsShortcut(t, AppsDelete,
			[]string{"+delete", "--app-id", "app_x", "--confirm-name", "App", "--yes", "--as", "user"},
			factory, stdout)
		var permErr *errs.PermissionError
		if errors.As(err, &permErr) {
			t.Fatalf("code %d: DELETE 40300 must not point users to request permission: %v", code, err)
		}
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("code %d: expected typed problem, got %T %v", code, err, err)
		}
		if p.Subtype != errs.SubtypeNotFound || p.Code != code || p.Hint != appAlreadyDeletedHint {
			t.Fatalf("code %d: subtype/code/hint = %q/%d/%q", code, p.Subtype, p.Code, p.Hint)
		}
	}
}

func TestAppsDelete_NotFoundOnDelete(t *testing.T) {
	for _, code := range []int{40400, 3340400} {
		factory, stdout, reg := newAppsExecuteFactory(t)
		stubAppsDeleteGet(reg, "app_x", "app_x", "App")
		stubAppsDeleteBizError(reg, "DELETE", "app_x", code, "app not found")
		err := runAppsShortcut(t, AppsDelete,
			[]string{"+delete", "--app-id", "app_x", "--confirm-name", "App", "--yes", "--as", "user"},
			factory, stdout)
		p, ok := errs.ProblemOf(err)
		if !ok || p.Subtype != errs.SubtypeNotFound || p.Hint != appAlreadyDeletedHint || p.Code != code {
			t.Fatalf("code %d: err = %v", code, err)
		}
	}
}

func TestAppsDelete_GetPermissionDeniedIsNotRetyped(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stubAppsDeleteBizError(reg, "GET", "app_x", 40300, "permission denied")
	err := runAppsShortcut(t, AppsDelete,
		[]string{"+delete", "--app-id", "app_x", "--confirm-name", "App", "--yes", "--as", "user"},
		factory, stdout)
	var permErr *errs.PermissionError
	if errors.As(err, &permErr) {
		t.Fatalf("GET 40300 must not be re-typed as PermissionError: %v", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T %v", err, err)
	}
	if p.Code != 40300 || p.Hint != appIDListHint {
		t.Fatalf("code/hint = %d/%q", p.Code, p.Hint)
	}
}
