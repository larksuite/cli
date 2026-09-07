// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func newDriveMemberListRuntime(t *testing.T, token, docType, fields, permType string) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "drive +member-list"}
	cmd.Flags().String("token", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("fields", "", "")
	cmd.Flags().String("perm-type", "", "")
	for name, value := range map[string]string{
		"token":     token,
		"type":      docType,
		"fields":    fields,
		"perm-type": permType,
	} {
		if value == "" {
			continue
		}
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	return common.TestNewRuntimeContext(cmd, driveTestConfig())
}

func TestDriveMemberListSpecResolvesTargets(t *testing.T) {
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
			name:     "bare folder token",
			token:    " fldTok ",
			docType:  " folder ",
			wantTok:  "fldTok",
			wantType: "folder",
		},
		{
			name:     "mindnotes URL",
			token:    "https://example.feishu.cn/mindnotes/mndTok",
			wantTok:  "mndTok",
			wantType: "mindnote",
		},
		{
			name:     "minutes URL",
			token:    "https://example.feishu.cn/minutes/obTok",
			wantTok:  "obTok",
			wantType: "minutes",
		},
		{
			name:     "apps page URL",
			token:    "https://example.feishu.cn/page/appMetaTok/?from=share",
			wantTok:  "appMetaTok",
			wantType: "apps",
		},
		{
			name:     "bare token with explicit apps type",
			token:    "appBareMetaTok",
			docType:  "apps",
			wantTok:  "appBareMetaTok",
			wantType: "apps",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newDriveMemberListRuntime(t, tt.token, tt.docType, "", "")
			spec, err := readDriveMemberListSpec(runtime)
			if err != nil {
				t.Fatalf("read spec: %v", err)
			}
			if spec.Token != tt.wantTok || spec.Type != tt.wantType {
				t.Fatalf("spec token/type = %q/%q, want %q/%q", spec.Token, spec.Type, tt.wantTok, tt.wantType)
			}
		})
	}
}

func TestDriveMemberListSpecValidationErrorsAreTyped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		token       string
		docType     string
		fields      string
		permType    string
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
			docType:     "folder",
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
			wantMessage: "invalid value",
		},
		{
			name:        "invalid fields",
			token:       "doxTok",
			docType:     "docx",
			fields:      "name,unknown",
			wantParam:   "--fields",
			wantMessage: "invalid value",
		},
		{
			name:        "star mixed with fields",
			token:       "doxTok",
			docType:     "docx",
			fields:      "*,name",
			wantParam:   "--fields",
			wantMessage: "cannot be combined",
		},
		{
			name:        "perm type rejected for non-wiki",
			token:       "doxTok",
			docType:     "docx",
			permType:    "single_page",
			wantParam:   "--perm-type",
			wantMessage: "only applies when resource type is wiki",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newDriveMemberListRuntime(t, tt.token, tt.docType, tt.fields, tt.permType)
			_, err := readDriveMemberListSpec(runtime)
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
			validationErr, ok := err.(*errs.ValidationError)
			if !ok {
				t.Fatalf("error type = %T, want *errs.ValidationError", err)
			}
			if validationErr.Param != tt.wantParam {
				t.Fatalf("param = %q, want %q", validationErr.Param, tt.wantParam)
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantMessage)
			}
		})
	}
}

func TestDriveMemberListSpecParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    string
		docType  string
		fields   string
		permType string
		want     map[string]interface{}
	}{
		{
			name:    "default omits optional params",
			token:   "doxTok",
			docType: "docx",
			want:    map[string]interface{}{"type": "docx"},
		},
		{
			name:    "fields canonicalized and deduplicated",
			token:   "doxTok",
			docType: "docx",
			fields:  "Name,avatar,name",
			want:    map[string]interface{}{"type": "docx", "fields": "name,avatar"},
		},
		{
			name:     "wiki accepts perm type",
			token:    "wikTok",
			docType:  "WIKI",
			fields:   "*",
			permType: "SINGLE_PAGE",
			want:     map[string]interface{}{"type": "wiki", "fields": "*", "perm_type": "single_page"},
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newDriveMemberListRuntime(t, tt.token, tt.docType, tt.fields, tt.permType)
			spec, err := readDriveMemberListSpec(runtime)
			if err != nil {
				t.Fatalf("read spec: %v", err)
			}
			if got := spec.params(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("params = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDriveMemberListDryRunIncludesGETRequest(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveMemberList, []string{
		"+member-list",
		"--token", "https://example.feishu.cn/drive/folder/fldTok",
		"--fields", "*",
		"--dry-run",
		"--as", "bot",
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
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if len(got.Data.API) != 1 {
		t.Fatalf("api count = %d, want 1", len(got.Data.API))
	}
	api := got.Data.API[0]
	if api.Method != "GET" || api.URL != "/open-apis/drive/v1/permissions/fldTok/members" {
		t.Fatalf("api = %#v", api)
	}
	if api.Params["type"] != "folder" || api.Params["fields"] != "*" {
		t.Fatalf("params = %#v", api.Params)
	}
	if _, ok := api.Params["perm_type"]; ok {
		t.Fatalf("perm_type should be omitted for folder: %#v", api.Params)
	}
}

func TestDriveMemberListExecutePreservesRawData(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())

	var capturedQuery string
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/permissions/doxTok/members",
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.RawQuery
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"member_id":      "ou_x",
						"member_type":    "openid",
						"perm":           "view",
						"type":           "user",
						"name":           "zhangsan",
						"server_future":  "preserved",
						"external_label": true,
					},
				},
				"server_top_level": "preserved",
			},
		},
	})

	err := mountAndRunDrive(t, DriveMemberList, []string{
		"+member-list",
		"--token", "doxTok",
		"--type", "docx",
		"--fields", "name,type,external_label",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(capturedQuery, "type=docx") ||
		!strings.Contains(capturedQuery, "fields=name%2Ctype%2Cexternal_label") {
		t.Fatalf("captured query = %q", capturedQuery)
	}
	data := decodeDriveEnvelope(t, stdout)
	if data["server_top_level"] != "preserved" {
		t.Fatalf("server_top_level = %#v", data["server_top_level"])
	}
	for _, key := range []string{"token", "type", "count"} {
		if _, ok := data[key]; ok {
			t.Fatalf("data[%s] = %#v, want omitted", key, data[key])
		}
	}
	items, _ := data["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("items = %#v, want one item", data["items"])
	}
	item, _ := items[0].(map[string]interface{})
	if item["server_future"] != "preserved" || item["external_label"] != true {
		t.Fatalf("item future fields not preserved: %#v", item)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no count log", stderr.String())
	}
}

func TestDriveMemberListDeclaresScopeAndIdentities(t *testing.T) {
	t.Parallel()

	if !reflect.DeepEqual(DriveMemberList.Scopes, []string{"docs:permission.member:retrieve"}) {
		t.Fatalf("Scopes = %v, want docs:permission.member:retrieve", DriveMemberList.Scopes)
	}
	if !reflect.DeepEqual(DriveMemberList.AuthTypes, []string{"user", "bot"}) {
		t.Fatalf("AuthTypes = %v, want [user bot]", DriveMemberList.AuthTypes)
	}
}

func TestDriveMemberListPrettyOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/permissions/wikTok/members",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"member_id":   "ou_x",
						"member_type": "openid",
						"perm":        "view",
						"perm_type":   "single_page",
						"type":        "user",
						"name":        "zhangsan",
					},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveMemberList, []string{
		"+member-list",
		"--token", "wikTok",
		"--type", "wiki",
		"--perm-type", "single_page",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"[1] ou_x", "member_type: openid", "perm_type:   single_page", "name:        zhangsan"} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q:\n%s", want, out)
		}
	}
}
