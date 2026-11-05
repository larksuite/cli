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

type miaodaDriveCase struct {
	shortcut common.Shortcut
	args     []string
	method   string
	path     string
	body     map[string]interface{}
}

func miaodaDriveCases() []miaodaDriveCase {
	return []miaodaDriveCase{
		{DriveApplyPermission, []string{"--perm", "view"}, "POST", "/open-apis/drive/v1/permissions/metaTok/members/apply", map[string]interface{}{"perm": "view"}},
		{DrivePermissionGetSetting, nil, "GET", "/open-apis/drive/v2/permissions/metaTok/public", nil},
		{DriveMemberList, nil, "GET", "/open-apis/drive/v1/permissions/metaTok/members", nil},
		{DriveMemberAdd, []string{"--member-id", "ou_test", "--member-type", "openid"}, "POST", "/open-apis/drive/v1/permissions/metaTok/members", map[string]interface{}{"member_id": "ou_test", "member_type": "openid", "perm": "view", "type": "user"}},
		{DriveMemberAdd, []string{"--member-id", "ou_test,ou_other", "--member-type", "openid"}, "POST", "/open-apis/drive/v1/permissions/metaTok/members/batch_create", map[string]interface{}{"members": []interface{}{
			map[string]interface{}{"member_id": "ou_test", "member_type": "openid", "perm": "view", "type": "user"},
			map[string]interface{}{"member_id": "ou_other", "member_type": "openid", "perm": "view", "type": "user"},
		}}},
		{DriveMemberRemove, []string{"--member-id", "ou_test", "--member-type", "openid"}, "DELETE", "/open-apis/drive/v1/permissions/metaTok/members/ou_test", map[string]interface{}{"type": "user"}},
		{DriveSecureLabelUpdate, []string{"--label-id", "123"}, "PATCH", "/open-apis/drive/v2/files/metaTok/secure_label", map[string]interface{}{"id": "123"}},
	}
}

func TestDriveMiaodaURLRecovery(t *testing.T) {
	for _, tc := range miaodaDriveCases() {
		t.Run(tc.shortcut.Command+tc.path, func(t *testing.T) {
			for _, raw := range []string{"https://example.com/app/app_test?from=share", "/app/app_test", "https://example.com/app/app_test/"} {
				for _, dryRun := range []bool{false, true} {
					t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
					f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
					reg.Register(&httpmock.Stub{URL: "/open-apis/", Optional: true, OnMatch: func(req *http.Request) { t.Errorf("unexpected API request: %s", req.URL.Path) }})
					args := []string{tc.shortcut.Command, "--token", raw, "--as", "user"}
					args = append(args, tc.args...)
					if tc.shortcut.Risk == "high-risk-write" {
						args = append(args, "--yes")
					}
					if dryRun {
						args = append(args, "--dry-run", "--type", "miaoda_apps")
					}
					err := mountAndRunDrive(t, tc.shortcut, args, f, stdout)
					var validationErr *errs.ValidationError
					if !errors.As(err, &validationErr) || validationErr.Param != "--token" || validationErr.Subtype != errs.SubtypeInvalidArgument {
						t.Fatalf("error = %T %v, want token validation error", err, err)
					}
					for _, want := range []string{"lark-cli apps +get --app-id", "data.app.meta_token", "data.app.meta_doc_type"} {
						if !strings.Contains(validationErr.Hint, want) {
							t.Fatalf("hint missing %q: %s", want, validationErr.Hint)
						}
					}
					if stdout.Len() != 0 {
						t.Fatalf("unexpected success output: %s", stdout.String())
					}
				}
			}
		})
	}
}

func TestDriveMiaodaMetaTargetRequests(t *testing.T) {
	for _, tc := range miaodaDriveCases() {
		for _, docType := range []string{"apps", "miaoda_apps"} {
			for _, dryRun := range []bool{true, false} {
				t.Run(tc.shortcut.Command+tc.path+docType+map[bool]string{true: "/dry", false: "/execute"}[dryRun], func(t *testing.T) {
					t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
					f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
					calls := 0
					stub := &httpmock.Stub{
						Method: tc.method, URL: tc.path,
						Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
							"permission_public": map[string]interface{}{},
							"members":           []interface{}{map[string]interface{}{"member_id": "ou_test"}, map[string]interface{}{"member_id": "ou_other"}},
						}},
						OnMatch: func(req *http.Request) {
							calls++
							if req.URL.Path != tc.path || req.URL.Query().Get("type") != docType {
								t.Errorf("unexpected request: %s", req.URL)
							}
						},
					}
					if !dryRun {
						reg.Register(stub)
					}
					args := []string{tc.shortcut.Command, "--token", "metaTok", "--type", docType, "--as", "user"}
					args = append(args, tc.args...)
					if tc.shortcut.Risk == "high-risk-write" {
						args = append(args, "--yes")
					}
					if dryRun {
						args = append(args, "--dry-run")
					}
					if err := mountAndRunDrive(t, tc.shortcut, args, f, stdout); err != nil {
						t.Fatal(err)
					}
					var body map[string]interface{}
					if dryRun {
						var envelope struct {
							Data struct {
								API []common.DryRunAPICall `json:"api"`
							} `json:"data"`
						}
						if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
							t.Fatal(err)
						}
						if len(envelope.Data.API) != 1 {
							t.Fatalf("expected one Drive request: %s", stdout.String())
						}
						call := envelope.Data.API[0]
						if call.Method != tc.method || call.URL != tc.path || call.Params["type"] != docType {
							t.Fatalf("unexpected dry-run request: %+v", call)
						}
						if call.Body != nil {
							body = call.Body.(map[string]interface{})
						}
					} else {
						if calls != 1 {
							t.Fatalf("API calls = %d, want 1", calls)
						}
						if len(stub.CapturedBody) != 0 {
							if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
								t.Fatal(err)
							}
						}
					}
					if !reflect.DeepEqual(body, tc.body) {
						t.Fatalf("body = %#v, want %#v", body, tc.body)
					}
				})
			}
		}
	}
}

func TestDriveMiaodaURLDetectionBoundary(t *testing.T) {
	for _, raw := range []string{
		"metaTok",
		"https://example.com/app/baseToken",
		"https://example.com/page/metaTok",
		"https://example.com/docx/docTok?next=/app/app_test",
		"https://example.com/app/app_%zz",
		"ftp://example.com/app/app_test",
	} {
		if err := rejectDriveMiaodaURL(raw); err != nil {
			t.Fatalf("misclassified %q: %v", raw, err)
		}
	}
	err := rejectDriveMiaodaURL("https://example.com/app/app_$(touch%20bad)")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || strings.Contains(validationErr.Hint, "$(") {
		t.Fatalf("unsafe hint: %v", err)
	}
}
