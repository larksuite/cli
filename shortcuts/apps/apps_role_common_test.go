// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newRoleRCtx(t *testing.T, flagDefs map[string]string, flags map[string]string) (*common.RuntimeContext, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{
		AppID:      "test-app-" + strings.ToLower(t.Name()),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	factory, stdoutBuf, _, reg := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "test-role"}
	cmd.SetContext(context.Background())
	for name, typ := range flagDefs {
		switch typ {
		case "bool":
			cmd.Flags().Bool(name, false, "")
		case "int":
			cmd.Flags().Int(name, 0, "")
		case "string_array":
			cmd.Flags().StringArray(name, nil, "")
		default:
			cmd.Flags().String(name, "", "")
		}
	}
	for name, val := range flags {
		if err := cmd.Flags().Set(name, val); err != nil {
			t.Fatalf("set flag %q = %q: %v", name, val, err)
		}
	}
	rctx := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)
	return rctx, stdoutBuf, reg
}

func assertRoleValidationParam(t *testing.T, err error, param string) *errs.Problem {
	t.Helper()
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %#v, want typed problem", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("category = %q, want validation", problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want invalid_argument", problem.Subtype)
	}
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("err = %#v, want validation error", err)
	}
	if validation.Param != param {
		t.Fatalf("param = %q, want %s", validation.Param, param)
	}
	return problem
}

func assertRoleValidationParams(t *testing.T, err error, params ...string) *errs.Problem {
	t.Helper()
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %#v, want typed problem", err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %+v, want validation/invalid_argument", problem)
	}
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("err = %#v, want validation error", err)
	}
	if validation.Param != "" {
		t.Fatalf("param = %q, want omitted for multi-parameter constraint", validation.Param)
	}
	if len(validation.Params) != len(params) {
		t.Fatalf("params = %#v, want %v", validation.Params, params)
	}
	for index, want := range params {
		if validation.Params[index].Name != want || validation.Params[index].Reason == "" {
			t.Fatalf("params[%d] = %#v, want name=%q with a reason", index, validation.Params[index], want)
		}
	}
	return problem
}

func TestBuildRolePageParams_DefaultAndChanged(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, map[string]string{
		"page-size":  "int",
		"page-token": "string",
	}, map[string]string{})
	params, err := buildRolePageParams(rctx)
	if err != nil {
		t.Fatalf("buildRolePageParams() = %v", err)
	}
	if params["limit"] != defaultRolePageSize || params["offset"] != 0 {
		t.Fatalf("params = %#v, want limit=%d offset=0", params, defaultRolePageSize)
	}

	rctx, _, _ = newRoleRCtx(t, map[string]string{
		"page-size":  "int",
		"page-token": "string",
	}, map[string]string{"page-size": "20", "page-token": "40"})
	params, err = buildRolePageParams(rctx)
	if err != nil {
		t.Fatalf("buildRolePageParams(changed) = %v", err)
	}
	if params["limit"] != 20 || params["offset"] != 40 {
		t.Fatalf("params = %#v, want limit=20 offset=40", params)
	}
}

func TestBuildRolePageParams_RejectsInvalidToken(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, map[string]string{
		"page-size":  "int",
		"page-token": "string",
	}, map[string]string{"page-token": "abc"})
	_, err := buildRolePageParams(rctx)
	assertRoleValidationParam(t, err, "--page-token")
}

func TestBuildRolePageParams_RejectsPageSizeOverMax(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, map[string]string{
		"page-size":  "int",
		"page-token": "string",
	}, map[string]string{"page-size": "101"})
	_, err := buildRolePageParams(rctx)
	assertRoleValidationParam(t, err, "--page-size")
}

func TestValidateOptionalRoleID(t *testing.T) {
	for _, good := range []string{"", " role_001 ", "Role-ABC", "abc123", strings.Repeat("a", 64)} {
		if err := validateOptionalRoleID(good); err != nil {
			t.Fatalf("validateOptionalRoleID(%q) = %v", good, err)
		}
	}
	for _, bad := range []string{"bad/role", "bad role", strings.Repeat("a", 65)} {
		err := validateOptionalRoleID(bad)
		problem := assertRoleValidationParam(t, err, "--role-id")
		if !strings.Contains(problem.Hint, "omit --role-id") {
			t.Fatalf("hint = %q, want create-specific omit guidance", problem.Hint)
		}
	}
}

func TestRoleFlagHelpersTrim(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, map[string]string{
		"app-id":  "string",
		"role-id": "string",
	}, map[string]string{"app-id": " app_1 ", "role-id": " role_1 "})
	if got := roleAppID(rctx); got != "app_1" {
		t.Fatalf("roleAppID() = %q, want app_1", got)
	}
	if got := roleID(rctx); got != "role_1" {
		t.Fatalf("roleID() = %q, want role_1", got)
	}
	if err := validateRoleID(rctx); err != nil {
		t.Fatalf("validateRoleID() = %v, want nil", err)
	}
}

func TestValidateRoleAppIDRejectsEmpty(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, map[string]string{
		"app-id": "string",
	}, map[string]string{})
	problem := assertRoleValidationParam(t, validateRoleAppID(rctx), "--app-id")
	if problem.Message != "--app-id is required" {
		t.Fatalf("message = %q, want --app-id is required", problem.Message)
	}
	if problem.Hint == "" {
		t.Fatalf("hint is empty, want recovery guidance")
	}
}

func TestValidateRoleAppIDRejectsPathSegmentUnsafeChars(t *testing.T) {
	for _, appID := range []string{"app/bad", `app\bad`, "app bad", "app\u00a0bad", "app\nbad", "app\u0000bad"} {
		rctx, _, _ := newRoleRCtx(t, map[string]string{
			"app-id": "string",
		}, map[string]string{"app-id": appID})
		assertRoleValidationParam(t, validateRoleAppID(rctx), "--app-id")
	}
}

func TestValidateRoleAppIDRejectsLarkCredentialAppID(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, map[string]string{
		"app-id": "string",
	}, map[string]string{"app-id": "cli_app"})
	assertRoleValidationParam(t, validateRoleAppID(rctx), "--app-id")
}

func TestValidateRoleAppIDRequiresMiaodaPrefix(t *testing.T) {
	for _, appID := range []string{"app", "app_", "miaoda_123", "plain"} {
		rctx, _, _ := newRoleRCtx(t, map[string]string{
			"app-id": "string",
		}, map[string]string{"app-id": appID})
		problem := assertRoleValidationParam(t, validateRoleAppID(rctx), "--app-id")
		if !strings.Contains(problem.Message, "starting with app_") {
			t.Fatalf("appID=%q message=%q, want app_ guidance", appID, problem.Message)
		}
	}
}

func TestValidateRoleIDRejectsInvalidRequiredRoleID(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, map[string]string{
		"app-id":  "string",
		"role-id": "string",
	}, map[string]string{"app-id": "app_x", "role-id": "bad/role"})
	problem := assertRoleValidationParam(t, validateRoleID(rctx), "--role-id")
	if strings.Contains(problem.Hint, "omit --role-id") || !strings.Contains(problem.Hint, "+role-list") {
		t.Fatalf("hint = %q, want existing-role resolution guidance", problem.Hint)
	}
}

func TestValidateRoleIDRejectsMissingRequiredRoleID(t *testing.T) {
	rctx, _, _ := newRoleRCtx(t, map[string]string{
		"app-id":  "string",
		"role-id": "string",
	}, map[string]string{"app-id": "app_x"})
	problem := assertRoleValidationParam(t, validateRoleID(rctx), "--role-id")
	if problem.Message != "--role-id is required" {
		t.Fatalf("message = %q, want --role-id is required", problem.Message)
	}
}

func TestBuildRoleMemberGroupsAndBody(t *testing.T) {
	groups, err := buildRoleMemberGroups(" ou_a,ou_b ", " od-a ", " oc_a ")
	if err != nil {
		t.Fatalf("buildRoleMemberGroups() = %v", err)
	}
	if len(groups.Users) != 2 || len(groups.Departments) != 1 || len(groups.Chats) != 1 {
		t.Fatalf("groups = %#v", groups)
	}
	body := buildRoleMemberBody(groups)
	assertJSONEquivalent(t, body, map[string]interface{}{
		"users":       []interface{}{"ou_a", "ou_b"},
		"departments": []interface{}{"od-a"},
		"chats":       []interface{}{"oc_a"},
	})
}

func TestBuildRoleMemberGroupsRejectsEmpty(t *testing.T) {
	_, err := buildRoleMemberGroups(" , ", "", "")
	assertRoleValidationParams(t, err, "--users", "--departments", "--chats")
}

func TestBuildRoleMemberGroupsRejectsInvalidMemberIDWithSourceParam(t *testing.T) {
	tests := []struct {
		name        string
		users       string
		departments string
		chats       string
		wantParam   string
	}{
		{name: "users slash", users: "ou/bad", wantParam: "--users"},
		{name: "users email", users: "alice@example.com", wantParam: "--users"},
		{name: "users wrong prefix", users: "user_123", wantParam: "--users"},
		{name: "users prefix only", users: "ou_", wantParam: "--users"},
		{name: "departments wrong prefix", departments: "ou_user", wantParam: "--departments"},
		{name: "departments prefix only", departments: "od-", wantParam: "--departments"},
		{name: "legacy departments prefix", departments: "od_department", wantParam: "--departments"},
		{name: "chats wrong prefix", chats: "od-department", wantParam: "--chats"},
		{name: "chats prefix only", chats: "oc_", wantParam: "--chats"},
		{
			name:        "departments",
			departments: "od-bad value",
			wantParam:   "--departments",
		},
		{
			name:      "chats",
			chats:     "oc?bad",
			wantParam: "--chats",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildRoleMemberGroups(tt.users, tt.departments, tt.chats)
			assertRoleValidationParam(t, err, tt.wantParam)
		})
	}
}

func TestBuildRoleMemberGroupsRejectsMoreThanMax(t *testing.T) {
	users := make([]string, maxRoleMembers+1)
	for i := range users {
		users[i] = "ou_test"
	}
	_, err := buildRoleMemberGroups(strings.Join(users, ","), "", "")
	assertRoleValidationParams(t, err, "--users")
}

func TestBuildRoleMemberGroupsRejectsMoreThanMaxOnlyChats(t *testing.T) {
	chats := make([]string, maxRoleMembers+1)
	for i := range chats {
		chats[i] = "oc_test"
	}
	_, err := buildRoleMemberGroups("", "", strings.Join(chats, ","))
	problem := assertRoleValidationParams(t, err, "--chats")
	if !strings.Contains(problem.Message, "role members cannot exceed 100") {
		t.Fatalf("message = %q, want role members limit", problem.Message)
	}
	if !strings.Contains(problem.Hint, "does not split") || !strings.Contains(problem.Hint, "atomic request") {
		t.Fatalf("hint = %q, want no automatic batching guidance", problem.Hint)
	}
}

func TestBuildRoleMemberGroupsOverflowNamesEveryContributingFlag(t *testing.T) {
	users := strings.TrimSuffix(strings.Repeat("ou_user,", 60), ",")
	chats := strings.TrimSuffix(strings.Repeat("oc_chat,", 41), ",")
	_, err := buildRoleMemberGroups(users, "", chats)
	assertRoleValidationParams(t, err, "--users", "--chats")
}

func TestRoleMemberKindsAreCompleteAndStable(t *testing.T) {
	want := []roleMemberKind{
		{memberType: "user", dataKey: "users", flagName: "--users", prefix: "ou_"},
		{memberType: "department", dataKey: "departments", flagName: "--departments", prefix: "od-"},
		{memberType: "chat", dataKey: "chats", flagName: "--chats", prefix: "oc_"},
	}
	if len(roleMemberKinds) != len(want) {
		t.Fatalf("roleMemberKinds = %#v, want %#v", roleMemberKinds, want)
	}
	for index := range want {
		if roleMemberKinds[index] != want[index] {
			t.Fatalf("roleMemberKinds[%d] = %#v, want %#v", index, roleMemberKinds[index], want[index])
		}
	}
}

func TestRoleDisplayValueSanitizesAndFlattens(t *testing.T) {
	got := roleDisplayValue("  Admin\n\x1b[31mred\x1b[0m\tvalue  ")
	if got != "Admin red value" {
		t.Fatalf("roleDisplayValue() = %q, want flattened safe text", got)
	}
}

func TestRoleNextPageToken(t *testing.T) {
	if got := roleNextPageToken(40, 20, true); got != "60" {
		t.Fatalf("roleNextPageToken(hasMore) = %q, want 60", got)
	}
	if got := roleNextPageToken(40, 20, false); got != "" {
		t.Fatalf("roleNextPageToken(!hasMore) = %q, want empty", got)
	}
}

func TestWithRoleErrorHintUsesDocumentedRecoveryAndPreservesEnvelope(t *testing.T) {
	tests := []struct {
		name      string
		code      int
		operation roleErrorOperation
		wantHint  string
		forbid    string
	}{
		{name: "invalid parameters", code: roleErrInvalidParameters, operation: roleOperationList, wantHint: roleAppHint},
		{name: "administrator required", code: roleErrAdminRequired, operation: roleOperationList, wantHint: "app administrator"},
		{name: "administrator or developer required", code: roleErrManagerRequired, operation: roleOperationGet, wantHint: "administrator or app developer"},
		{name: "invalid create role id", code: roleErrInvalidRoleID, operation: roleOperationCreate, wantHint: "omit --role-id"},
		{name: "role missing", code: roleErrRoleNotFound, operation: roleOperationGet, wantHint: "+role-list"},
		{name: "stale match role", code: roleErrRoleNotFound, operation: roleOperationMatchList, wantHint: "may no longer be valid", forbid: "--role-id"},
		{name: "duplicate role id", code: roleErrRoleAlreadyExists, operation: roleOperationCreate, wantHint: "different --role-id"},
		{name: "role limit", code: roleErrRoleLimitExceeded, operation: roleOperationCreate, wantHint: "delete an unused app role"},
		{name: "invalid role name", code: roleErrInvalidRoleName, operation: roleOperationUpdate, wantHint: "adjust --name"},
		{name: "invalid role description", code: roleErrInvalidRoleDescription, operation: roleOperationUpdate, wantHint: "adjust --description"},
		{name: "unsupported member type", code: roleErrUnsupportedMemberType, operation: roleOperationMemberList, wantHint: "user, department, or chat"},
		{name: "invalid member id", code: roleErrInvalidMemberID, operation: roleOperationMemberAdd, wantHint: "member IDs"},
		{name: "invalid match target", code: roleErrInvalidMemberID, operation: roleOperationMatchList, wantHint: "--user-id", forbid: "--role-id"},
		{name: "user quota", code: roleErrUserLimitExceeded, operation: roleOperationMemberAdd, wantHint: "reduce the users"},
		{name: "department quota", code: roleErrDepartmentLimitExceeded, operation: roleOperationMemberAdd, wantHint: "reduce the departments"},
		{name: "chat quota", code: roleErrChatLimitExceeded, operation: roleOperationMemberAdd, wantHint: "reduce the chats"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errclass.BuildAPIError(map[string]any{
				"code":   tt.code,
				"msg":    "role request failed",
				"log_id": "log-role-hint",
			}, errclass.ClassifyContext{Identity: "user"})
			err = withRoleErrorHint(err, tt.operation)
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("err = %#v, want typed problem", err)
			}
			if problem.Code != tt.code || problem.LogID != "log-role-hint" || problem.Retryable {
				t.Fatalf("problem envelope changed: %+v", problem)
			}
			if !strings.Contains(problem.Hint, tt.wantHint) {
				t.Fatalf("hint = %q, want substring %q", problem.Hint, tt.wantHint)
			}
			if tt.forbid != "" && strings.Contains(problem.Hint, tt.forbid) {
				t.Fatalf("hint = %q, must not contain %q", problem.Hint, tt.forbid)
			}
		})
	}
}

func TestWithRoleErrorHintPreservesServerDetail(t *testing.T) {
	err := errclass.BuildAPIError(map[string]any{
		"code": roleErrInvalidRoleName,
		"msg":  "invalid role name",
		"error": map[string]any{
			"details": []any{map[string]any{"value": "name exceeds the service limit"}},
		},
	}, errclass.ClassifyContext{Identity: "user"})
	err = withRoleErrorHint(err, roleOperationCreate)
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %#v, want typed problem", err)
	}
	for _, want := range []string{"name exceeds the service limit", "adjust --name"} {
		if !strings.Contains(problem.Hint, want) {
			t.Fatalf("hint = %q, want %q", problem.Hint, want)
		}
	}
}

func TestWithRoleErrorHintPreservesAuthorizationDetail(t *testing.T) {
	var err error = errs.NewPermissionError(errs.SubtypePermissionDenied, "administrator access required").
		WithCode(roleErrAdminRequired).
		WithHint("server detail: only owners may change this app")
	err = withRoleErrorHint(err, roleOperationUpdate)
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %#v, want typed problem", err)
	}
	for _, want := range []string{"server detail: only owners", "ask an app administrator"} {
		if !strings.Contains(problem.Hint, want) {
			t.Fatalf("hint = %q, want %q", problem.Hint, want)
		}
	}
}
