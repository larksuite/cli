// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestDriveMemberRemoveMetadata(t *testing.T) {
	t.Parallel()

	if DriveMemberRemove.Service != "drive" || DriveMemberRemove.Command != "+member-remove" {
		t.Fatalf("shortcut identity = %s %s", DriveMemberRemove.Service, DriveMemberRemove.Command)
	}
	if DriveMemberRemove.Risk != "high-risk-write" {
		t.Fatalf("risk = %q, want high-risk-write", DriveMemberRemove.Risk)
	}
	if !reflect.DeepEqual(DriveMemberRemove.Scopes, []string{"docs:permission.member:delete"}) {
		t.Fatalf("scopes = %#v", DriveMemberRemove.Scopes)
	}
	if !reflect.DeepEqual(DriveMemberRemove.AuthTypes, []string{"user", "bot"}) {
		t.Fatalf("auth types = %#v", DriveMemberRemove.AuthTypes)
	}
	wantIDTypes := []string{
		"email", "openid", "openchat", "opendepartmentid",
		"userid", "unionid", "groupid", "appid", "wikispaceid",
	}
	if !reflect.DeepEqual(driveMemberRemoveIDTypes, wantIDTypes) {
		t.Fatalf("member types = %#v, want %#v", driveMemberRemoveIDTypes, wantIDTypes)
	}
}

func TestDriveMemberRemoveSpecRequestShape(t *testing.T) {
	t.Parallel()

	spec := driveMemberRemoveSpec{
		Token:        "wikcnTok",
		ResourceType: "wiki",
		MemberID:     "ou_member",
		MemberType:   "openid",
		PermType:     "single_page",
	}
	wantParams := map[string]interface{}{"type": "wiki", "member_type": "openid"}
	if got := spec.APIQueryParams(); !reflect.DeepEqual(got, wantParams) {
		t.Fatalf("params = %#v, want %#v", got, wantParams)
	}
	wantBody := map[string]interface{}{"type": "user", "perm_type": "single_page"}
	if got := spec.Body(); !reflect.DeepEqual(got, wantBody) {
		t.Fatalf("body = %#v, want %#v", got, wantBody)
	}

	wikiSpaceSpec := driveMemberRemoveSpec{
		Token:        "wikcnTok",
		ResourceType: "wiki",
		MemberID:     "space_member",
		MemberType:   "wikispaceid",
		MemberKind:   "wiki_space_viewer",
	}
	if got := wikiSpaceSpec.Body(); !reflect.DeepEqual(got, map[string]interface{}{"type": "wiki_space_viewer"}) {
		t.Fatalf("wiki-space body = %#v", got)
	}
}

func TestDriveMemberRemoveOutputConditionalFields(t *testing.T) {
	t.Parallel()

	wikiSpace := driveMemberRemoveOutput(driveMemberRemoveSpec{
		Token:        "wikTok",
		ResourceType: "wiki",
		MemberID:     "space_member",
		MemberType:   "wikispaceid",
		MemberKind:   "wiki_space_editor",
	})
	if wikiSpace["removed"] != true || wikiSpace["member_kind"] != "wiki_space_editor" {
		t.Fatalf("wiki-space output = %#v", wikiSpace)
	}
	if _, ok := wikiSpace["perm_type"]; ok {
		t.Fatalf("wiki-space output must omit perm_type: %#v", wikiSpace)
	}

	userID := driveMemberRemoveOutput(driveMemberRemoveSpec{
		Token:        "doxTok",
		ResourceType: "docx",
		MemberID:     "custom_user",
		MemberType:   "userid",
	})
	if userID["member_kind"] != "user" {
		t.Fatalf("userid output = %#v, want member_kind user", userID)
	}
}

func TestDriveMemberRemoveDryRunInfersURLAndDefaultsWikiPermType(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberRemove, []string{
		"+member-remove",
		"--token", "https://example.feishu.cn/wiki/wikcnTok?from=share",
		"--member-id", "ou_member",
		"--member-type", "openid",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		Data struct {
			API []struct {
				Method string                 `json:"method"`
				URL    string                 `json:"url"`
				Params map[string]interface{} `json:"params"`
				Body   map[string]interface{} `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if len(got.Data.API) != 1 {
		t.Fatalf("api count = %d, want 1", len(got.Data.API))
	}
	call := got.Data.API[0]
	if call.Method != "DELETE" || call.URL != "/open-apis/drive/v1/permissions/wikcnTok/members/ou_member" {
		t.Fatalf("call = %#v", call)
	}
	if call.Params["type"] != "wiki" || call.Params["member_type"] != "openid" {
		t.Fatalf("params = %#v", call.Params)
	}
	if call.Body["type"] != "user" || call.Body["perm_type"] != "container" {
		t.Fatalf("body = %#v", call.Body)
	}
}

func TestDriveMemberRemoveDryRunInfersAppsFromPageURLAndBareToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		typ   string
	}{
		{name: "page URL infers apps", token: "https://example.feishu.cn/page/appRemoveTok?from=share"},
		{name: "bare token with explicit type", token: "appRemoveTok", typ: "apps"},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
			args := []string{
				"+member-remove",
				"--token", tt.token,
				"--member-id", "ou_member",
				"--member-type", "openid",
				"--dry-run",
				"--as", "user",
			}
			if tt.typ != "" {
				args = append(args, "--type", tt.typ)
			}
			if err := mountAndRunDrive(t, DriveMemberRemove, args, f, stdout); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var got struct {
				Data struct {
					API []struct {
						Method string                 `json:"method"`
						URL    string                 `json:"url"`
						Params map[string]interface{} `json:"params"`
						Body   map[string]interface{} `json:"body"`
					} `json:"api"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
			}
			if len(got.Data.API) != 1 {
				t.Fatalf("api count = %d, want 1", len(got.Data.API))
			}
			call := got.Data.API[0]
			if call.Method != "DELETE" || call.URL != "/open-apis/drive/v1/permissions/appRemoveTok/members/ou_member" {
				t.Fatalf("call = %#v", call)
			}
			if call.Params["type"] != "apps" || call.Params["member_type"] != "openid" {
				t.Fatalf("params = %#v", call.Params)
			}
			if call.Body["type"] != "user" {
				t.Fatalf("body = %#v", call.Body)
			}
			if _, ok := call.Body["perm_type"]; ok {
				t.Fatalf("apps must not carry perm_type, body = %#v", call.Body)
			}
		})
	}
}

func TestDriveMemberRemoveValidation(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		defaultAs string
		wantParam string
		wantText  string
	}{
		{
			name:      "rejects slash in normalized token",
			args:      []string{"--token", "token/with/slash", "--type", "docx", "--member-id", "ou_user", "--member-type", "openid"},
			wantParam: "--token",
			wantText:  "cannot contain '/'",
		},
		{
			name:      "rejects slash in member ID",
			args:      []string{"--token", "doxTok", "--type", "docx", "--member-id", "userid/with/slash", "--member-type", "userid"},
			wantParam: "--member-id",
			wantText:  "cannot contain '/'",
		},
		{
			name:      "rejects multiple members",
			args:      []string{"--token", "doxTok", "--type", "docx", "--member-id", "ou_a,ou_b", "--member-type", "openid"},
			wantParam: "--member-id",
			wantText:  "exactly one collaborator ID",
		},
		{
			name:      "rejects member prefix mismatch",
			args:      []string{"--token", "doxTok", "--type", "docx", "--member-id", "oc_chat", "--member-type", "openid"},
			wantParam: "--member-id",
			wantText:  "implies --member-type openchat",
		},
		{
			name:      "rejects wiki-space ID outside wiki",
			args:      []string{"--token", "doxTok", "--type", "docx", "--member-id", "space_x", "--member-type", "wikispaceid", "--member-kind", "wiki_space_member"},
			wantParam: "--member-type",
			wantText:  "only applies when resource type is wiki",
		},
		{
			name:      "requires member kind for wiki space ID",
			args:      []string{"--token", "wikTok", "--type", "wiki", "--member-id", "space_x", "--member-type", "wikispaceid"},
			wantParam: "--member-kind",
			wantText:  "--member-kind is required",
		},
		{
			name:      "rejects member kind for ordinary member",
			args:      []string{"--token", "wikTok", "--type", "wiki", "--member-id", "ou_x", "--member-type", "openid", "--member-kind", "wiki_space_member"},
			wantParam: "--member-kind",
			wantText:  "only applies",
		},
		{
			name:      "rejects perm type outside wiki",
			args:      []string{"--token", "doxTok", "--type", "docx", "--member-id", "ou_x", "--member-type", "openid", "--perm-type", "single_page"},
			wantParam: "--perm-type",
			wantText:  "only applies when resource type is wiki",
		},
		{
			name:      "rejects perm type on apps",
			args:      []string{"--token", "appRemoveTok", "--type", "apps", "--member-id", "ou_x", "--member-type", "openid", "--perm-type", "single_page"},
			wantParam: "--perm-type",
			wantText:  `--perm-type only applies when resource type is wiki; got "apps"`,
		},
		{
			name:      "rejects perm type for wiki space member",
			args:      []string{"--token", "wikTok", "--type", "wiki", "--member-id", "space_x", "--member-type", "wikispaceid", "--member-kind", "wiki_space_member", "--perm-type", "container"},
			wantParam: "--perm-type",
			wantText:  "not supported when --member-type=wikispaceid",
		},
		{
			name:      "rejects department with bot",
			args:      []string{"--token", "doxTok", "--type", "docx", "--member-id", "od_dept", "--member-type", "opendepartmentid"},
			defaultAs: "bot",
			wantParam: "--member-type",
			wantText:  "requires --as user",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
			args := append([]string{"+member-remove"}, tt.args...)
			args = append(args, "--dry-run", "--as", tt.defaultAs)
			if tt.defaultAs == "" {
				args[len(args)-1] = "user"
			}
			err := mountAndRunDrive(t, DriveMemberRemove, args, f, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("error = %v, want text %q", err, tt.wantText)
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed validation error, got %T: %v", err, err)
			}
			if problem.Category != errs.CategoryValidation {
				t.Fatalf("problem = %#v", problem)
			}
			if problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem subtype = %q, want %q", problem.Subtype, errs.SubtypeInvalidArgument)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) || validationErr.Param != tt.wantParam {
				t.Fatalf("validation error = %#v, want param %q", validationErr, tt.wantParam)
			}
		})
	}
}

func TestDriveMemberRemoveAcceptsUserID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberRemove, []string{
		"+member-remove",
		"--token", "doxTok",
		"--type", "docx",
		"--member-id", "ou_tenant_defined_user",
		"--member-type", "userid",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		Data struct {
			API []struct {
				Params map[string]interface{} `json:"params"`
				Body   map[string]interface{} `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if len(got.Data.API) != 1 {
		t.Fatalf("api count = %d, want 1", len(got.Data.API))
	}
	if got.Data.API[0].Params["member_type"] != "userid" {
		t.Fatalf("params = %#v", got.Data.API[0].Params)
	}
	if got.Data.API[0].Body["type"] != "user" {
		t.Fatalf("body = %#v", got.Data.API[0].Body)
	}
}

func TestDriveMemberRemoveAcceptsAppID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberRemove, []string{
		"+member-remove",
		"--token", "doxTok",
		"--type", "docx",
		"--member-id", "cli_app_123",
		"--member-type", "appid",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		Data struct {
			API []struct {
				Params map[string]interface{} `json:"params"`
				Body   map[string]interface{} `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if len(got.Data.API) != 1 {
		t.Fatalf("api count = %d, want 1", len(got.Data.API))
	}
	if got.Data.API[0].Params["member_type"] != "appid" {
		t.Fatalf("params = %#v", got.Data.API[0].Params)
	}
	if _, ok := got.Data.API[0].Body["type"]; ok {
		t.Fatalf("body = %#v, want type omitted for appid", got.Data.API[0].Body)
	}
}

func TestDriveMemberRemoveExecuteSuccess(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
	var capturedQuery string
	stub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/drive/v1/permissions/wikcnSecretToken/members/ou_secret_member",
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.RawQuery
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveMemberRemove, []string{
		"+member-remove",
		"--token", "wikcnSecretToken",
		"--type", "wiki",
		"--member-id", "ou_secret_member",
		"--member-type", "openid",
		"--perm-type", "single_page",
		"--as", "user",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedQuery, "type=wiki") || !strings.Contains(capturedQuery, "member_type=openid") {
		t.Fatalf("query = %q", capturedQuery)
	}
	var capturedBody map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &capturedBody); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	wantBody := map[string]interface{}{"type": "user", "perm_type": "single_page"}
	if !reflect.DeepEqual(capturedBody, wantBody) {
		t.Fatalf("body = %#v, want %#v", capturedBody, wantBody)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["removed"] != true || data["resource_token"] != "wikcnSecretToken" || data["resource_type"] != "wiki" ||
		data["member_id"] != "ou_secret_member" || data["member_type"] != "openid" ||
		data["member_kind"] != "user" || data["perm_type"] != "single_page" {
		t.Fatalf("output = %#v", data)
	}
	if strings.Contains(stderr.String(), "wikcnSecretToken") || strings.Contains(stderr.String(), "ou_secret_member") {
		t.Fatalf("stderr exposes unmasked identifiers: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), common.MaskToken("wikcnSecretToken")) || !strings.Contains(stderr.String(), common.MaskToken("ou_secret_member")) {
		t.Fatalf("stderr missing masked identifiers: %q", stderr.String())
	}
}

func TestDriveMemberRemoveTypedPermissionErrorPassesThrough(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/drive/v1/permissions/doxTok/members/ou_member",
		Body: map[string]interface{}{
			"code": 1063002,
			"msg":  "Permission denied",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunDrive(t, DriveMemberRemove, []string{
		"+member-remove",
		"--token", "doxTok",
		"--type", "docx",
		"--member-id", "ou_member",
		"--member-type", "openid",
		"--as", "user",
		"--yes",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected API error")
	}
	var permissionErr *errs.PermissionError
	if !errors.As(err, &permissionErr) {
		t.Fatalf("expected *errs.PermissionError, got %T: %v", err, err)
	}
	if permissionErr.Code != 1063002 {
		t.Fatalf("code = %d, want 1063002", permissionErr.Code)
	}
}

func TestDriveMemberRemoveRequiresConfirmation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberRemove, []string{
		"+member-remove",
		"--token", "doxTok",
		"--type", "docx",
		"--member-id", "ou_member",
		"--member-type", "openid",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "requires confirmation") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
}
