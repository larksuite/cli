// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestSheetCreateBotAutoGrantSuccess(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, sheetCreateTestConfig(t, "ou_current_user"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"spreadsheet": map[string]interface{}{
					"spreadsheet_token": "shtcn_new_sheet",
					"url":               "https://example.feishu.cn/sheets/shtcn_new_sheet",
				},
			},
		},
	})

	permStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/shtcn_new_sheet/members",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	}
	reg.Register(permStub)

	err := runSheetCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "项目排期",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSheetCreateEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantGranted {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantGranted)
	}
	if grant["user_open_id"] != "ou_current_user" {
		t.Fatalf("permission_grant.user_open_id = %#v, want %q", grant["user_open_id"], "ou_current_user")
	}
	if grant["message"] != "Granted the current CLI user full_access (可管理权限) on the new spreadsheet." {
		t.Fatalf("permission_grant.message = %#v", grant["message"])
	}

	var body map[string]interface{}
	if err := json.Unmarshal(permStub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to parse permission request body: %v", err)
	}
	if body["member_type"] != "openid" || body["member_id"] != "ou_current_user" || body["perm"] != "full_access" || body["type"] != "user" {
		t.Fatalf("unexpected permission request body: %#v", body)
	}
}

func TestSheetCreateUserSkipsPermissionGrantAugmentation(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, sheetCreateTestConfig(t, "ou_current_user"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"spreadsheet": map[string]interface{}{
					"spreadsheet_token": "shtcn_new_sheet",
					"url":               "https://example.feishu.cn/sheets/shtcn_new_sheet",
				},
			},
		},
	})

	err := runSheetCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "项目排期",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSheetCreateEnvelope(t, stdout)
	if _, ok := data["permission_grant"]; ok {
		t.Fatalf("did not expect permission_grant in user mode output: %#v", data)
	}
}

func TestSheetCreateFallbackURLWhenBackendOmitsIt(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, sheetCreateTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"spreadsheet": map[string]interface{}{
					"spreadsheet_token": "shtcn_new_sheet",
					// "url" deliberately omitted to exercise the fallback.
				},
			},
		},
	})

	err := runSheetCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "项目排期",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSheetCreateEnvelope(t, stdout)
	if got, want := data["url"], "https://www.feishu.cn/sheets/shtcn_new_sheet"; got != want {
		t.Fatalf("url = %#v, want %q (brand-standard fallback)", got, want)
	}
}

func TestSheetCreateDryRunIncludesFolderToken(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t,
		map[string]string{
			"title":        "项目排期",
			"folder-token": "fldcn123",
			"headers":      "",
			"data":         "",
		},
		nil, nil)
	got := mustMarshalSheetsDryRun(t, SheetCreate.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"folder_token":"fldcn123"`) {
		t.Fatalf("DryRun should include folder_token, got: %s", got)
	}
}

func TestSheetCreatePreservesBackendURL(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, sheetCreateTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"spreadsheet": map[string]interface{}{
					"spreadsheet_token": "shtcn_new_sheet",
					"url":               "https://tenant.larkoffice.com/sheets/shtcn_new_sheet",
				},
			},
		},
	})

	err := runSheetCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "项目排期",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSheetCreateEnvelope(t, stdout)
	if got, want := data["url"], "https://tenant.larkoffice.com/sheets/shtcn_new_sheet"; got != want {
		t.Fatalf("url = %#v, want backend tenant URL %q (fallback must not overwrite)", got, want)
	}
}

func TestSheetCreateFallbackURLWhenBackendURLIsWhitespace(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, sheetCreateTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"spreadsheet": map[string]interface{}{
					"spreadsheet_token": "shtcn_new_sheet",
					"url":               "   ", // whitespace-only must trigger fallback, not pass through.
				},
			},
		},
	})

	err := runSheetCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "项目排期",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSheetCreateEnvelope(t, stdout)
	if got, want := data["url"], "https://www.feishu.cn/sheets/shtcn_new_sheet"; got != want {
		t.Fatalf("url = %#v, want %q (whitespace-only backend URL must yield fallback)", got, want)
	}
}

func TestSheetCreateTrimsPaddedBackendURL(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, sheetCreateTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"spreadsheet": map[string]interface{}{
					"spreadsheet_token": "shtcn_new_sheet",
					"url":               "  https://tenant.larkoffice.com/sheets/shtcn_new_sheet  ",
				},
			},
		},
	})

	err := runSheetCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "项目排期",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSheetCreateEnvelope(t, stdout)
	if got, want := data["url"], "https://tenant.larkoffice.com/sheets/shtcn_new_sheet"; got != want {
		t.Fatalf("url = %#v, want trimmed backend URL %q (whitespace must not leak into output)", got, want)
	}
}

func sheetCreateTestConfig(t *testing.T, userOpenID string) *core.CliConfig {
	t.Helper()

	replacer := strings.NewReplacer("/", "-", " ", "-")
	suffix := replacer.Replace(strings.ToLower(t.Name()))
	return &core.CliConfig{
		AppID:      "test-sheet-create-" + suffix,
		AppSecret:  "secret-sheet-create-" + suffix,
		Brand:      core.BrandFeishu,
		UserOpenId: userOpenID,
	}
}

func runSheetCreateShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()

	parent := &cobra.Command{Use: "sheets"}
	SheetCreate.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func decodeSheetCreateEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode output: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("missing data in output envelope: %#v", envelope)
	}
	return data
}
