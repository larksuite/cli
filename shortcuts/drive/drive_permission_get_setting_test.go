// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func newDrivePermissionGetSettingRuntime(t *testing.T, token, docType string) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "drive +permission-get-setting"}
	cmd.Flags().String("token", "", "")
	cmd.Flags().String("type", "", "")
	if token != "" {
		if err := cmd.Flags().Set("token", token); err != nil {
			t.Fatalf("set --token: %v", err)
		}
	}
	if docType != "" {
		if err := cmd.Flags().Set("type", docType); err != nil {
			t.Fatalf("set --type: %v", err)
		}
	}
	return common.TestNewRuntimeContext(cmd, driveTestConfig())
}

func TestDrivePermissionGetSettingSpecResolvesTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    string
		docType  string
		wantTok  string
		wantType string
	}{
		{
			name:     "folder URL",
			token:    "https://example.feishu.cn/drive/folder/fldTok?from=share",
			wantTok:  "fldTok",
			wantType: "folder",
		},
		{
			name:     "docx URL",
			token:    "https://example.feishu.cn/docx/doxTok",
			wantTok:  "doxTok",
			wantType: "docx",
		},
		{
			name:     "file URL",
			token:    "https://example.feishu.cn/file/boxTok",
			wantTok:  "boxTok",
			wantType: "file",
		},
		{
			name:     "wiki URL",
			token:    "https://example.feishu.cn/wiki/wikTok",
			wantTok:  "wikTok",
			wantType: "wiki",
		},
		{
			name:     "minutes URL",
			token:    "https://example.feishu.cn/minutes/obTok",
			wantTok:  "obTok",
			wantType: "minutes",
		},
		{
			name:     "mindnotes URL",
			token:    "https://example.feishu.cn/mindnotes/mndTok",
			wantTok:  "mndTok",
			wantType: "mindnote",
		},
		{
			name:     "bare folder token",
			token:    " fldTok ",
			docType:  " folder ",
			wantTok:  "fldTok",
			wantType: "folder",
		},
		{
			name:     "bare file token",
			token:    "boxTok",
			docType:  "file",
			wantTok:  "boxTok",
			wantType: "file",
		},
		{
			name:     "bare wiki token",
			token:    "wikTok",
			docType:  "wiki",
			wantTok:  "wikTok",
			wantType: "wiki",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newDrivePermissionGetSettingRuntime(t, tt.token, tt.docType)
			spec, err := readDrivePermissionGetSettingSpec(runtime)
			if err != nil {
				t.Fatalf("read spec: %v", err)
			}
			if spec.Token != tt.wantTok {
				t.Fatalf("Token = %q, want %q", spec.Token, tt.wantTok)
			}
			if spec.Type != tt.wantType {
				t.Fatalf("Type = %q, want %q", spec.Type, tt.wantType)
			}
		})
	}
}

func TestDrivePermissionGetSettingSpecValidationErrorsAreTyped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		token       string
		docType     string
		wantParam   string
		wantMessage string
	}{
		{
			name:        "missing token",
			wantParam:   "--token",
			wantMessage: "--token is required",
		},
		{
			name:        "bare token without type",
			token:       "doxTok",
			wantParam:   "--type",
			wantMessage: "--type is required",
		},
		{
			name:        "unsupported URL",
			token:       "https://example.feishu.cn/calendar/calTok",
			wantParam:   "--token",
			wantMessage: "unsupported --token URL",
		},
		{
			name:        "URL type conflict",
			token:       "https://example.feishu.cn/docx/doxTok",
			docType:     "sheet",
			wantParam:   "--type",
			wantMessage: "conflicts with URL path type",
		},
		{
			name:        "invalid bare token",
			token:       "../bad",
			docType:     "folder",
			wantParam:   "--token",
			wantMessage: "--token",
		},
		{
			name:        "invalid type",
			token:       "doxTok",
			docType:     "comment",
			wantParam:   "--type",
			wantMessage: "invalid --type",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newDrivePermissionGetSettingRuntime(t, tt.token, tt.docType)
			_, err := readDrivePermissionGetSettingSpec(runtime)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error is not typed: %T %v", err, err)
			}
			if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem = %s/%s, want validation/invalid_argument", problem.Category, problem.Subtype)
			}
			if validationErr, ok := err.(*errs.ValidationError); ok {
				if validationErr.Param != tt.wantParam {
					t.Fatalf("param = %q, want %q", validationErr.Param, tt.wantParam)
				}
			} else {
				t.Fatalf("error type = %T, want *errs.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantMessage)
			}
		})
	}
}

func TestDrivePermissionGetSettingDryRunIncludesGETRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    string
		docType  string
		wantURL  string
		wantType string
	}{
		{
			name:     "folder URL",
			token:    "https://example.feishu.cn/drive/folder/fldTok",
			wantURL:  "/open-apis/drive/v2/permissions/fldTok/public",
			wantType: "folder",
		},
		{
			name:     "bare folder token",
			token:    "fldTok",
			docType:  "folder",
			wantURL:  "/open-apis/drive/v2/permissions/fldTok/public",
			wantType: "folder",
		},
		{
			name:     "docx URL",
			token:    "https://example.feishu.cn/docx/doxTok",
			wantURL:  "/open-apis/drive/v2/permissions/doxTok/public",
			wantType: "docx",
		},
		{
			name:     "bare wiki token",
			token:    "wikTok",
			docType:  "wiki",
			wantURL:  "/open-apis/drive/v2/permissions/wikTok/public",
			wantType: "wiki",
		},
		{
			name:     "file URL",
			token:    "https://example.feishu.cn/file/boxTok",
			wantURL:  "/open-apis/drive/v2/permissions/boxTok/public",
			wantType: "file",
		},
		{
			name:     "minutes URL",
			token:    "https://example.feishu.cn/minutes/obTok",
			wantURL:  "/open-apis/drive/v2/permissions/obTok/public",
			wantType: "minutes",
		},
		{
			name:     "mindnotes URL",
			token:    "https://example.feishu.cn/mindnotes/mndTok",
			wantURL:  "/open-apis/drive/v2/permissions/mndTok/public",
			wantType: "mindnote",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newDrivePermissionGetSettingRuntime(t, tt.token, tt.docType)
			dry := DrivePermissionGetSetting.DryRun(context.Background(), runtime)
			if dry == nil {
				t.Fatal("DryRun returned nil")
			}
			data, err := json.Marshal(dry)
			if err != nil {
				t.Fatalf("marshal dry-run: %v", err)
			}
			out := string(data)
			for _, want := range []string{
				`"` + tt.wantURL + `"`,
				`"GET"`,
				`"type":"` + tt.wantType + `"`,
			} {
				if !strings.Contains(out, want) {
					t.Fatalf("dry-run output missing %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, `"folder_token"`) {
				t.Fatalf("dry-run output contains folder_token, want omitted:\n%s", out)
			}
		})
	}
}

func TestDrivePermissionGetSettingExecutePreservesPermissionPublic(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v2/permissions/doxTok/public?type=docx",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"permission_public": map[string]interface{}{
					"link_share_entity":          "closed",
					"external_access_entity":     "closed",
					"security_entity":            "anyone_can_view",
					"comment_entity":             "anyone_can_view",
					"share_entity":               "anyone",
					"manage_collaborator_entity": "collaborator_can_view",
					"lock_switch":                false,
					"server_future_field":        "preserved",
				},
			},
		},
	})

	err := mountAndRunDrive(t, DrivePermissionGetSetting, []string{
		"+permission-get-setting",
		"--token", "doxTok",
		"--type", "docx",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	for _, key := range []string{"type", "token", "url"} {
		if _, ok := data[key]; ok {
			t.Fatalf("data[%s] = %#v, want field omitted", key, data[key])
		}
	}
	permissionPublic, _ := data["permission_public"].(map[string]interface{})
	if permissionPublic == nil {
		t.Fatalf("permission_public missing in output: %#v", data)
	}
	for key, want := range map[string]interface{}{
		"link_share_entity":          "closed",
		"external_access_entity":     "closed",
		"security_entity":            "anyone_can_view",
		"comment_entity":             "anyone_can_view",
		"share_entity":               "anyone",
		"manage_collaborator_entity": "collaborator_can_view",
		"lock_switch":                false,
		"server_future_field":        "preserved",
	} {
		if permissionPublic[key] != want {
			t.Fatalf("permission_public[%s] = %#v, want %#v", key, permissionPublic[key], want)
		}
	}
}

func TestDrivePermissionGetSettingExecuteRejectsMissingPermissionPublic(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v2/permissions/doxTok/public?type=docx",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"unexpected": "response"},
		},
	})

	err := mountAndRunDrive(t, DrivePermissionGetSetting, []string{
		"+permission-get-setting",
		"--token", "doxTok",
		"--type", "docx",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected invalid response error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem = %#v, want internal/invalid_response", problem)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty on invalid response, got %s", stdout.String())
	}
}

func TestDrivePermissionGetSettingExecutePrettyFormatIncludesPermissionPublic(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v2/permissions/doxTok/public?type=docx",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"permission_public": map[string]interface{}{
					"link_share_entity":   "closed",
					"server_future_field": "preserved",
				},
			},
		},
	})

	err := mountAndRunDrive(t, DrivePermissionGetSetting, []string{
		"+permission-get-setting",
		"--token", "doxTok",
		"--type", "docx",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"Permission settings:",
		`"link_share_entity": "closed"`,
		`"server_future_field": "preserved"`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("pretty output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestDrivePermissionGetSettingDeclaresScopeAndIdentities(t *testing.T) {
	t.Parallel()

	if !reflect.DeepEqual(DrivePermissionGetSetting.Scopes, []string{"docs:permission.setting:read"}) {
		t.Fatalf("Scopes = %v, want docs:permission.setting:read", DrivePermissionGetSetting.Scopes)
	}
	if !reflect.DeepEqual(DrivePermissionGetSetting.AuthTypes, []string{"user", "bot"}) {
		t.Fatalf("AuthTypes = %v, want [user bot]", DrivePermissionGetSetting.AuthTypes)
	}
	for _, flag := range DrivePermissionGetSetting.Flags {
		if flag.Name == "token" && !flag.Required {
			t.Fatal("--token must be declared required")
		}
	}
}
