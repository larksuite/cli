// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newExecuteFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *httpmock.Registry) {
	return newExecuteFactoryWithUserOpenID(t, "ou_testuser")
}

func newExecuteFactoryWithUserOpenID(t *testing.T, userOpenID string) (*cmdutil.Factory, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	config := &core.CliConfig{
		AppID:      "test-app-" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-"),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: userOpenID,
	}
	factory, stdout, _, reg := cmdutil.TestFactory(t, config)
	return factory, stdout, reg
}

func withBaseWorkingDir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() err=%v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q) err=%v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd err=%v", err)
		}
	})
}

func runShortcut(t *testing.T, shortcut common.Shortcut, args []string, factory *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	return runShortcutWithAuthTypes(t, shortcut, []string{"bot"}, args, factory, stdout)
}

func runShortcutWithAuthTypes(t *testing.T, shortcut common.Shortcut, authTypes []string, args []string, factory *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	if authTypes != nil {
		shortcut.AuthTypes = authTypes
	}
	parent := &cobra.Command{Use: "base"}
	shortcut.Mount(parent, factory)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	stdout.Reset()
	if stderr, ok := factory.IOStreams.ErrOut.(*bytes.Buffer); ok {
		stderr.Reset()
	}
	return parent.ExecuteContext(context.Background())
}

func registerEmptyAppBlockList(reg *httpmock.Registry, appToken, pageID string) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/base_apps/" + appToken + "/pages/" + pageID + "/blocks?page_size=100",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": []interface{}{}, "has_more": false},
		},
	})
}

func assertInvalidArgumentValidation(t *testing.T, err error, wantParam string, wantParams []string, messageContains string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected invalid-argument validation error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected invalid-argument validation problem, got %T %v", err, err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T %v", err, err)
	}
	if validationErr.Param != wantParam {
		t.Fatalf("param=%q, want %q", validationErr.Param, wantParam)
	}
	if wantParams != nil {
		if len(validationErr.Params) != len(wantParams) {
			t.Fatalf("params=%#v, want %v", validationErr.Params, wantParams)
		}
		for i, want := range wantParams {
			if validationErr.Params[i].Name != want {
				t.Fatalf("params=%#v, want %v", validationErr.Params, wantParams)
			}
		}
	}
	if messageContains != "" && !strings.Contains(err.Error(), messageContains) {
		t.Fatalf("err=%v, want message containing %q", err, messageContains)
	}
}

func TestBaseWorkspaceCreatePreservesResponseAndExposesReferences(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/workspaces",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"workspace_token": "ws_x",
				"name":            "Growth",
				"url":             "https://www.feishu.cn/base/workspace/ws_x",
			},
		},
	})

	if err := runShortcut(t, BaseWorkspaceCreate, []string{"+workspace-create", "--name", "Growth"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	data := decodeBaseEnvelope(t, stdout)
	if data["created"] != true || data["workspace_token"] != "ws_x" || data["url"] != "https://www.feishu.cn/base/workspace/ws_x" {
		t.Fatalf("unexpected result: %#v", data)
	}
	workspace, ok := data["workspace"].(map[string]interface{})
	if !ok || common.GetString(workspace, "workspace_token") != "ws_x" || common.GetString(workspace, "url") != "https://www.feishu.cn/base/workspace/ws_x" {
		t.Fatalf("workspace response not preserved: %#v", data["workspace"])
	}
}

func TestBaseWorkspaceMoveInReturnsServerDataWithoutSyntheticSuccess(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/workspaces/ws_x/move_in",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"entity_token": "bas_x",
			},
		},
	})
	if err := runShortcut(t, BaseWorkspaceMoveIn, []string{"+workspace-move-in", "--workspace-token", "ws_x", "--entity-token", "bas_x"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	if data["entity_token"] != "bas_x" {
		t.Fatalf("data=%#v, want server move-in data", data)
	}
	if _, exists := data["moved_in"]; exists {
		t.Fatalf("data=%#v, must not contain synthetic moved_in", data)
	}
}

func TestBaseWorkspaceExecuteCreate(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stderr, _ := factory.IOStreams.ErrOut.(*bytes.Buffer)
	permStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/app_x/members?need_notification=false&type=bitable",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	}
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_token": "app_x", "name": "Demo Base"},
		},
	})
	reg.Register(permStub)
	if err := runShortcut(t, BaseBaseCreate, []string{"+base-create", "--name", "Demo Base", "--folder-token", "fld_x", "--time-zone", "Asia/Shanghai"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	data := decodeBaseEnvelope(t, stdout)
	if data["created"] != true {
		t.Fatalf("created = %#v, want true", data["created"])
	}
	if !strings.Contains(stderr.String(), baseCreateHint) {
		t.Fatalf("stderr = %q, want %q", stderr.String(), baseCreateHint)
	}
	base, _ := data["base"].(map[string]interface{})
	if got := common.GetString(base, "app_token"); got != "app_x" {
		t.Fatalf("base.app_token = %q, want %q", got, "app_x")
	}
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantGranted {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantGranted)
	}
	if grant["user_open_id"] != "ou_testuser" {
		t.Fatalf("permission_grant.user_open_id = %#v, want %q", grant["user_open_id"], "ou_testuser")
	}
	if grant["message"] != "Granted the current CLI user full_access on the new base." {
		t.Fatalf("permission_grant.message = %#v", grant["message"])
	}

	body := decodeCapturedJSONBody(t, permStub)
	if body["member_type"] != "openid" || body["member_id"] != "ou_testuser" || body["perm"] != "full_access" || body["type"] != "user" {
		t.Fatalf("unexpected permission request body: %#v", body)
	}
}

func TestBaseAppCreateIsAtomic(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/base_apps",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_token": "app_x", "workspace_token": "ws_x"},
		},
	})
	if err := runShortcut(t, BaseAppCreate, []string{"+app-create", "--name", "Sales", "--workspace-token", "ws_x"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	data := decodeBaseEnvelope(t, stdout)
	if data["created"] != true || data["workspace_token"] != "ws_x" {
		t.Fatalf("unexpected result: %#v", data)
	}
	app, _ := data["app"].(map[string]interface{})
	if common.GetString(app, "app_token") != "app_x" {
		t.Fatalf("app=%#v", app)
	}
}

func TestBaseAppBlockGetData(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/base_apps/app_x/blocks/cht_x/data?base_token=bas_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"main_data": []interface{}{}},
		},
	}
	reg.Register(stub)

	err := runShortcut(t, BaseAppBlockGetData, []string{
		"+app-block-get-data",
		"--app-token", "app_x",
		"--base-token", "bas_x",
		"--block-id", "cht_x",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	data := decodeBaseEnvelope(t, stdout)
	if _, ok := data["main_data"]; !ok {
		t.Fatalf("unexpected response: %#v", data)
	}
}

func TestBaseAppBlockGetDataReturnsTypedAPIErrors(t *testing.T) {
	t.Run("non-zero API response", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/base_apps/app_x/blocks/cht_x/data?base_token=bas_x",
			Body: map[string]interface{}{
				"code": 1254001,
				"msg":  "invalid chart token",
			},
		})

		err := runShortcut(t, BaseAppBlockGetData, []string{
			"+app-block-get-data",
			"--app-token", "app_x",
			"--base-token", "bas_x",
			"--block-id", "cht_x",
		}, factory, stdout)
		assertProblemCode(t, err, 1254001, "invalid chart token")
	})

	t.Run("transport failure", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		cause := errors.New("connection reset")
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/base_apps/app_x/blocks/cht_x/data?base_token=bas_x",
			Error:  cause,
		})

		err := runShortcut(t, BaseAppBlockGetData, []string{
			"+app-block-get-data",
			"--app-token", "app_x",
			"--base-token", "bas_x",
			"--block-id", "cht_x",
		}, factory, stdout)
		problem, ok := errs.ProblemOf(err)
		if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTransport {
			t.Fatalf("problem=%#v, want typed network error", problem)
		}
		if !errors.Is(err, cause) {
			t.Fatalf("err=%v, want wrapped cause %v", err, cause)
		}
	})
}

func TestBaseAppBlockUpdateRejectsDuplicateName(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/base_apps/app_x/pages/pge_x/blocks?page_size=100",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"block_id": "blk_current", "name": "Current"},
					map[string]interface{}{"block_id": "blk_other", "name": "Taken"},
				},
				"has_more": false,
			},
		},
	})

	err := runShortcut(t, BaseAppBlockUpdate, []string{
		"+app-block-update",
		"--app-token", "app_x",
		"--page-id", "pge_x",
		"--block-id", "blk_current",
		"--name", "Taken",
	}, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--name", nil, "组件名称必须唯一")
}

func TestBaseAppBlockUpdateRejectsListBaseOutsideWorkspace(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/base_apps/app_x/pages/pge_x/blocks/blk_current",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"block_id": "blk_current", "type": "list"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/base_apps/app_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_token": "app_x", "workspace_token": "ws_x", "ref": map[string]interface{}{}},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/workspaces/ws_x/entities?entity_type=base&page_size=100",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": []interface{}{}, "has_more": false},
		},
	})

	err := runShortcut(t, BaseAppBlockUpdate, []string{
		"+app-block-update",
		"--app-token", "app_x",
		"--page-id", "pge_x",
		"--block-id", "blk_current",
		"--data-config", `{"base_token":"bas_outside","table_name":"Orders"}`,
	}, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--data-config", nil, "不在当前 Workspace")
}

func TestBaseAppBlockUpdateRejectsFieldForCurrentBlockType(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/base_apps/app_x/pages/pge_x/blocks/blk_current",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"block_id": "blk_current",
				"type":     "list",
				"sub_type": "standard",
				"data_config": map[string]interface{}{
					"base_token": "bas_x",
					"table_name": "Orders",
				},
			},
		},
	})

	err := runShortcut(t, BaseAppBlockUpdate, []string{
		"+app-block-update",
		"--app-token", "app_x",
		"--page-id", "pge_x",
		"--block-id", "blk_current",
		"--data-config", `{"text":"not valid for a list"}`,
	}, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--data-config", nil, "standard 列表不支持字段 text")
}

func TestBaseAppBlockCreateUsesWorkspaceIDAsWorkspaceToken(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	registerEmptyAppBlockList(reg, "app_x", "pge_x")
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/base_apps/app_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_token": "app_x", "workspace_id": "ws_x"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/workspaces/ws_x/entities?entity_type=base&page_size=100",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{map[string]interface{}{"token": "bas_x"}},
			},
		},
	})
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/base_apps/app_x/pages/pge_x/blocks",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"block_id": "blk_x", "type": "list", "sub_type": "card"},
		},
	}
	reg.Register(createStub)

	err := runShortcut(t, BaseAppBlockCreate, []string{
		"+app-block-create",
		"--app-token", "app_x",
		"--page-id", "pge_x",
		"--name", "Cards",
		"--type", "list",
		"--sub-type", "card",
		"--data-config", `{"base_token":"bas_x","table_name":"Orders","fields":[],"card_config":{}}`,
	}, factory, stdout)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	data := decodeBaseEnvelope(t, stdout)
	if data["created"] != true {
		t.Fatalf("created=%#v, want true", data["created"])
	}
	body := decodeCapturedJSONBody(t, createStub)
	config, _ := body["data_config"].(map[string]interface{})
	if config["base_token"] != "bas_x" || config["table_name"] != "Orders" {
		t.Fatalf("data_config=%#v", config)
	}
	fields, ok := config["fields"].([]interface{})
	if !ok || len(fields) != 0 {
		t.Fatalf("explicit fields must be preserved: %#v", config["fields"])
	}
}

func TestBaseAppBlockCreateListOmitsUnspecifiedOptionalFields(t *testing.T) {
	for _, tc := range []struct {
		subType     string
		optionalKey string
	}{
		{subType: "standard", optionalKey: "columns"},
		{subType: "grouped", optionalKey: "columns"},
		{subType: "collapsible", optionalKey: "columns"},
		{subType: "card", optionalKey: "fields"},
		{subType: "detail", optionalKey: "fields"},
	} {
		t.Run(tc.subType, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			registerEmptyAppBlockList(reg, "app_x", "pge_x")
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/base/v3/base_apps/app_x",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"app_token": "app_x",
						"ref": map[string]interface{}{
							"bas_x": []interface{}{"Orders"},
						},
					},
				},
			})
			createStub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/base/v3/base_apps/app_x/pages/pge_x/blocks",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"block_id": "blk_x"},
				},
			}
			reg.Register(createStub)

			err := runShortcut(t, BaseAppBlockCreate, []string{
				"+app-block-create",
				"--app-token", "app_x",
				"--page-id", "pge_x",
				"--name", "Orders",
				"--type", "list",
				"--sub-type", tc.subType,
				"--data-config", `{"base_token":"bas_x","table_name":"Orders"}`,
			}, factory, stdout)
			if err != nil {
				t.Fatalf("err=%v", err)
			}
			body := decodeCapturedJSONBody(t, createStub)
			config, _ := body["data_config"].(map[string]interface{})
			if _, exists := config[tc.optionalKey]; exists {
				t.Fatalf("unspecified %s must be omitted: %#v", tc.optionalKey, config)
			}
		})
	}
}

func TestBaseAppBlockCreateRejectsDuplicateNameAcrossPagination(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/base_apps/app_x/pages/pge_x/blocks?page_size=100",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":           []interface{}{map[string]interface{}{"block_id": "blk_1", "name": "Other"}},
				"has_more":        true,
				"next_page_token": "next_x",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/base_apps/app_x/pages/pge_x/blocks?page_size=100&page_token=next_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"block_id": "blk_2", "name": " cards "}},
				"has_more": false,
			},
		},
	})

	err := runShortcut(t, BaseAppBlockCreate, []string{
		"+app-block-create",
		"--app-token", "app_x",
		"--page-id", "pge_x",
		"--name", "Cards",
		"--type", "text",
	}, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--name", nil, "组件名称必须唯一")
}

func TestBaseWorkspaceExecuteCreateWithFields(t *testing.T) {
	oldDelay := baseCreateDefaultTableDeleteDelay
	baseCreateDefaultTableDeleteDelay = 0
	t.Cleanup(func() { baseCreateDefaultTableDeleteDelay = oldDelay })

	factory, stdout, reg := newExecuteFactory(t)
	stderr, _ := factory.IOStreams.ErrOut.(*bytes.Buffer)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_token": "app_x", "name": "Demo Base"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/tables",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"tables": []interface{}{
				map[string]interface{}{"id": "tbl_default", "name": "Table 1"},
			}},
		},
	})
	createTableStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"id": "tbl_custom", "name": "Tasks", "fields": []interface{}{
				map[string]interface{}{"id": "fld_title", "name": "Title", "type": "text"},
				map[string]interface{}{"id": "fld_status", "name": "Status", "type": "text"},
			}},
		},
	}
	reg.Register(createTableStub)
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_default",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/app_x/members?need_notification=false&type=bitable",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	err := runShortcut(
		t,
		BaseBaseCreate,
		[]string{"+base-create", "--name", "Demo Base", "--table-name", "Tasks", "--fields", `[{"name":"Title","type":"text"},{"name":"Status","type":"text"}]`},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	data := decodeBaseEnvelope(t, stdout)
	if data["created"] != true || data["default_table_deleted"] != true || data["deleted_default_table_id"] != "tbl_default" {
		t.Fatalf("unexpected create output: %#v", data)
	}
	table, _ := data["table"].(map[string]interface{})
	if got := common.GetString(table, "id"); got != "tbl_custom" {
		t.Fatalf("table.id = %q, want tbl_custom", got)
	}
	fields, _ := data["fields"].([]interface{})
	if len(fields) != 2 {
		t.Fatalf("fields len = %d, want 2; output=%#v", len(fields), data["fields"])
	}
	if strings.Contains(stderr.String(), baseCreateHint) {
		t.Fatalf("stderr should not contain default-table cleanup hint when --fields handled cleanup: %q", stderr.String())
	}

	if body := decodeCapturedJSONBody(t, createTableStub); body["name"] != "Tasks" {
		t.Fatalf("create table body = %#v", body)
	}
	body := decodeCapturedJSONBody(t, createTableStub)
	fieldsBody, _ := body["fields"].([]interface{})
	if len(fieldsBody) != 2 {
		t.Fatalf("create table fields body = %#v", body["fields"])
	}
}

func TestBaseWorkspaceExecuteCreateWithFieldsDefaultTableName(t *testing.T) {
	oldDelay := baseCreateDefaultTableDeleteDelay
	baseCreateDefaultTableDeleteDelay = 0
	t.Cleanup(func() { baseCreateDefaultTableDeleteDelay = oldDelay })

	factory, stdout, reg := newExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_token": "app_x", "name": "Demo Base"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/tables",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"tables": []interface{}{
				map[string]interface{}{"id": "tbl_default", "name": "Table 1"},
			}},
		},
	})
	createTableStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"id": "tbl_custom", "name": "Table 1", "fields": []interface{}{
				map[string]interface{}{"id": "fld_title", "name": "Title", "type": "text"},
			}},
		},
	}
	reg.Register(createTableStub)
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_default",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/app_x/members?need_notification=false&type=bitable",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	err := runShortcut(
		t,
		BaseBaseCreate,
		[]string{"+base-create", "--name", "Demo Base", "--fields", `[{"name":"Title","type":"text"}]`},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	body := decodeCapturedJSONBody(t, createTableStub)
	if body["name"] != "Table 1" {
		t.Fatalf("create table body = %#v, want name Table 1", body)
	}
}

func TestBaseWorkspaceExecuteCreateWithTableNameOnly(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stderr, _ := factory.IOStreams.ErrOut.(*bytes.Buffer)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_token": "app_x", "name": "Demo Base"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/tables",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"tables": []interface{}{
				map[string]interface{}{"id": "tbl_default", "name": "Table 1"},
			}},
		},
	})
	renameStub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_default",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"id": "tbl_default", "name": "Tasks"},
		},
	}
	reg.Register(renameStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/app_x/members?need_notification=false&type=bitable",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	err := runShortcut(
		t,
		BaseBaseCreate,
		[]string{"+base-create", "--name", "Demo Base", "--table-name", "Tasks"},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	data := decodeBaseEnvelope(t, stdout)
	if data["created"] != true || data["default_table_renamed"] != true || data["renamed_default_table_id"] != "tbl_default" {
		t.Fatalf("unexpected create output: %#v", data)
	}
	if data["default_table_deleted"] == true {
		t.Fatalf("table-name-only should not delete the default table: %#v", data)
	}
	table, _ := data["table"].(map[string]interface{})
	if got := common.GetString(table, "name"); got != "Tasks" {
		t.Fatalf("table.name = %q, want Tasks", got)
	}
	if strings.Contains(stderr.String(), baseCreateHint) {
		t.Fatalf("stderr should not contain default schema hint when --table-name handled rename: %q", stderr.String())
	}
	body := decodeCapturedJSONBody(t, renameStub)
	if body["name"] != "Tasks" {
		t.Fatalf("rename table body = %#v", body)
	}
}

func TestBaseWorkspaceExecuteGetAndCopy(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"base_token": "app_x", "name": "Demo Base"},
			},
		})
		if err := runShortcut(t, BaseBaseGet, []string{"+base-get", "--base-token", "app_x"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"base"`) || !strings.Contains(got, `"Demo Base"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("copy", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		permStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/permissions/app_new/members?need_notification=false&type=bitable",
			Body: map[string]interface{}{
				"code": 0,
				"msg":  "ok",
			},
		}
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_src/copy",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"base_token": "app_new", "name": "Copied Base", "url": "https://example.com/base/app_new"},
			},
		})
		reg.Register(permStub)
		args := []string{"+base-copy", "--base-token", "app_src", "--name", "Copied Base", "--folder-token", "fld_x", "--time-zone", "Asia/Shanghai", "--without-content"}
		if err := runShortcut(t, BaseBaseCopy, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["copied"] != true {
			t.Fatalf("copied = %#v, want true", data["copied"])
		}
		base, _ := data["base"].(map[string]interface{})
		if got := common.GetString(base, "base_token"); got != "app_new" {
			t.Fatalf("base.base_token = %q, want %q", got, "app_new")
		}
		grant, _ := data["permission_grant"].(map[string]interface{})
		if grant["status"] != common.PermissionGrantGranted {
			t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantGranted)
		}
		if grant["user_open_id"] != "ou_testuser" {
			t.Fatalf("permission_grant.user_open_id = %#v, want %q", grant["user_open_id"], "ou_testuser")
		}

		body := decodeCapturedJSONBody(t, permStub)
		if body["member_type"] != "openid" || body["member_id"] != "ou_testuser" || body["perm"] != "full_access" || body["type"] != "user" {
			t.Fatalf("unexpected permission request body: %#v", body)
		}
	})
}

func TestBaseWorkspaceExecuteCreateBotAutoGrantSkippedWithoutCurrentUser(t *testing.T) {
	factory, stdout, reg := newExecuteFactoryWithUserOpenID(t, "")
	stderr, _ := factory.IOStreams.ErrOut.(*bytes.Buffer)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_token": "app_x", "name": "Demo Base"},
		},
	})

	if err := runShortcut(t, BaseBaseCreate, []string{"+base-create", "--name", "Demo Base"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	if !strings.Contains(stderr.String(), baseCreateHint) {
		t.Fatalf("stderr = %q, want %q", stderr.String(), baseCreateHint)
	}
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantSkipped {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantSkipped)
	}
	if _, ok := grant["user_open_id"]; ok {
		t.Fatalf("did not expect user_open_id when current user is missing: %#v", grant)
	}
}

func TestBaseWorkspaceExecuteCreateBotAutoGrantFailureDoesNotFailCreate(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_token": "app_x", "name": "Demo Base"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/app_x/members?need_notification=false&type=bitable",
		Body: map[string]interface{}{
			"code": 230001,
			"msg":  "no permission",
		},
	})

	if err := runShortcut(t, BaseBaseCreate, []string{"+base-create", "--name", "Demo Base"}, factory, stdout); err != nil {
		t.Fatalf("Base creation should still succeed when auto-grant fails, got: %v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantFailed {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantFailed)
	}
	if !strings.Contains(grant["message"].(string), "retry later") {
		t.Fatalf("permission_grant.message = %q, want retry guidance", grant["message"])
	}
}

func TestBaseWorkspaceExecuteCreateUserSkipsPermissionGrantAugmentation(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_token": "app_x", "name": "Demo Base"},
		},
	})

	if err := runShortcutWithAuthTypes(t, BaseBaseCreate, authTypes(), []string{"+base-create", "--name", "Demo Base", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	if _, ok := data["permission_grant"]; ok {
		t.Fatalf("did not expect permission_grant in user mode output: %#v", data)
	}
}

func TestBaseWorkspaceExecuteCopyBotAutoGrantSkippedWithoutCurrentUser(t *testing.T) {
	factory, stdout, reg := newExecuteFactoryWithUserOpenID(t, "")
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_src/copy",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"base_token": "app_new", "name": "Copied Base"},
		},
	})

	if err := runShortcut(t, BaseBaseCopy, []string{"+base-copy", "--base-token", "app_src"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantSkipped {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantSkipped)
	}
}

func TestBaseWorkspaceExecuteCopyBotAutoGrantFailureDoesNotFailCopy(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_src/copy",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_token": "app_new", "name": "Copied Base"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/app_new/members?need_notification=false&type=bitable",
		Body: map[string]interface{}{
			"code": 230001,
			"msg":  "no permission",
		},
	})

	if err := runShortcut(t, BaseBaseCopy, []string{"+base-copy", "--base-token", "app_src"}, factory, stdout); err != nil {
		t.Fatalf("Base copy should still succeed when auto-grant fails, got: %v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantFailed {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantFailed)
	}
}

func TestBaseWorkspaceExecuteCopyUserSkipsPermissionGrantAugmentation(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_src/copy",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"base_token": "app_new", "name": "Copied Base"},
		},
	})

	if err := runShortcutWithAuthTypes(t, BaseBaseCopy, authTypes(), []string{"+base-copy", "--base-token", "app_src", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	if _, ok := data["permission_grant"]; ok {
		t.Fatalf("did not expect permission_grant in user mode output: %#v", data)
	}
}

func TestBaseWorkspaceDryRunCreateAndCopyPermissionGrantHints(t *testing.T) {
	t.Run("create bot", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		if err := runShortcut(t, BaseBaseCreate, []string{"+base-create", "--name", "Demo Base", "--dry-run"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		wantDesc := "After Base creation succeeds in bot mode, the CLI will also try to grant the current CLI user full_access on the new Base."
		if got := stdout.String(); !strings.Contains(got, wantDesc) {
			t.Fatalf("stdout=%s, want desc %q", got, wantDesc)
		}
	})

	t.Run("copy bot", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		if err := runShortcut(t, BaseBaseCopy, []string{"+base-copy", "--base-token", "app_src", "--dry-run"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		wantDesc := "After Base copy succeeds in bot mode, the CLI will also try to grant the current CLI user full_access on the new Base."
		if got := stdout.String(); !strings.Contains(got, wantDesc) {
			t.Fatalf("stdout=%s, want desc %q", got, wantDesc)
		}
	})

	t.Run("create user", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		if err := runShortcutWithAuthTypes(t, BaseBaseCreate, authTypes(), []string{"+base-create", "--name", "Demo Base", "--as", "user", "--dry-run"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); strings.Contains(got, "grant the current CLI user full_access") {
			t.Fatalf("stdout=%s", got)
		}
	})
}

func decodeBaseEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
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

func decodeCapturedJSONBody(t *testing.T, stub *httpmock.Stub) map[string]interface{} {
	t.Helper()

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to decode captured request body: %v\nraw=%s", err, string(stub.CapturedBody))
	}
	return body
}

func TestBaseBlockExecuteShortcuts(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	listStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/blocks/list",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"blocks": []interface{}{
					map[string]interface{}{"id": "blk_doc", "type": "docx", "name": "Spec"},
					map[string]interface{}{"id": "blk_folder", "type": "folder", "name": "Folder"},
				},
				"total": 2,
			},
		},
	}
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/blocks",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"block_id": "blk_doc", "type": "docx", "name": "Spec"},
		},
	}
	moveStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/blocks/blk_doc/move",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"block_id": "blk_doc", "parent_id": "bfl_1"},
		},
	}
	renameStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/blocks/blk_doc/rename",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"block_id": "blk_doc", "name": "Final Spec"},
		},
	}
	deleteStub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/base/v3/bases/app_x/blocks/blk_doc",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"block_id": "blk_doc"},
		},
	}
	for _, stub := range []*httpmock.Stub{listStub, createStub, moveStub, renameStub, deleteStub} {
		reg.Register(stub)
	}

	if err := runShortcut(t, BaseBaseBlockList, []string{"+base-block-list", "--base-token", "app_x", "--parent-id", "bfl_1", "--type", "docx"}, factory, stdout); err != nil {
		t.Fatalf("list err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"total": 1`) || !strings.Contains(got, `"blk_doc"`) || strings.Contains(got, `"blk_folder"`) {
		t.Fatalf("list stdout=%s", got)
	}
	if body := decodeCapturedJSONBody(t, listStub); body["parent_id"] != "bfl_1" || body["type"] != nil {
		t.Fatalf("list body=%#v", body)
	}

	if err := runShortcut(t, BaseBaseBlockCreate, []string{"+base-block-create", "--base-token", "app_x", "--type", "docx", "--name", " Spec ", "--parent-id", "bfl_1"}, factory, stdout); err != nil {
		t.Fatalf("create err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"created": true`) || !strings.Contains(got, `"blk_doc"`) {
		t.Fatalf("create stdout=%s", got)
	}
	createBody := decodeCapturedJSONBody(t, createStub)
	if createBody["type"] != "docx" || createBody["name"] != "Spec" || createBody["parent_id"] != "bfl_1" {
		t.Fatalf("create body=%#v", createBody)
	}

	if err := runShortcut(t, BaseBaseBlockMove, []string{"+base-block-move", "--base-token", "app_x", "--block-id", "blk_doc", "--parent-id", "bfl_1", "--after-id", "blk_prev"}, factory, stdout); err != nil {
		t.Fatalf("move err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"moved": true`) {
		t.Fatalf("move stdout=%s", got)
	}
	moveBody := decodeCapturedJSONBody(t, moveStub)
	if moveBody["parent_id"] != "bfl_1" || moveBody["after_id"] != "blk_prev" {
		t.Fatalf("move body=%#v", moveBody)
	}

	if err := runShortcut(t, BaseBaseBlockRename, []string{"+base-block-rename", "--base-token", "app_x", "--block-id", "blk_doc", "--name", " Final Spec "}, factory, stdout); err != nil {
		t.Fatalf("rename err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"renamed": true`) || !strings.Contains(got, `"Final Spec"`) {
		t.Fatalf("rename stdout=%s", got)
	}
	if body := decodeCapturedJSONBody(t, renameStub); body["name"] != "Final Spec" {
		t.Fatalf("rename body=%#v", body)
	}

	if err := runShortcut(t, BaseBaseBlockDelete, []string{"+base-block-delete", "--base-token", "app_x", "--block-id", "blk_doc", "--yes"}, factory, stdout); err != nil {
		t.Fatalf("delete err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"deleted": true`) || !strings.Contains(got, `"blk_doc"`) {
		t.Fatalf("delete stdout=%s", got)
	}
}

func TestBaseBlockValidationReturnsTypedErrors(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	tests := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
		params   []string
	}{
		{
			name:     "create blank name",
			shortcut: BaseBaseBlockCreate,
			args:     []string{"+base-block-create", "--base-token", "app_x", "--type", "docx", "--name", " "},
			params:   []string{"--name"},
		},
		{
			name:     "move conflicting sibling anchors",
			shortcut: BaseBaseBlockMove,
			args:     []string{"+base-block-move", "--base-token", "app_x", "--block-id", "blk_doc", "--before-id", "blk_a", "--after-id", "blk_b"},
			params:   []string{"--before-id", "--after-id"},
		},
		{
			name:     "rename blank name",
			shortcut: BaseBaseBlockRename,
			args:     []string{"+base-block-rename", "--base-token", "app_x", "--block-id", "blk_doc", "--name", " "},
			params:   []string{"--name"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runShortcut(t, tt.shortcut, tt.args, factory, stdout)
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed problem, got %T %v", err, err)
			}
			if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("category/subtype=%s/%s", p.Category, p.Subtype)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError, got %T %v", err, err)
			}
			if validationErr.Param != tt.params[0] {
				t.Fatalf("param=%q, want %q", validationErr.Param, tt.params[0])
			}
			if len(validationErr.Params) != len(tt.params) {
				t.Fatalf("params=%#v, want %v", validationErr.Params, tt.params)
			}
			for i, param := range tt.params {
				if validationErr.Params[i].Name != param {
					t.Fatalf("params=%#v, want %v", validationErr.Params, tt.params)
				}
				if validationErr.Params[i].Reason == "" {
					t.Fatalf("params[%d] missing reason: %#v", i, validationErr.Params)
				}
			}
		})
	}
}

func TestBaseHistoryExecute(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/record_history",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": []interface{}{map[string]interface{}{"record_id": "rec_x"}}},
		},
	})
	if err := runShortcut(t, BaseRecordHistoryList, []string{"+record-history-list", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_x", "--page-size", "10"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"record_id": "rec_x"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseHistoryRecordFormatsUseStandardFormatter(t *testing.T) {
	for _, format := range []string{"table", "csv", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/base/v3/bases/app_x/record_history",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items": []interface{}{map[string]interface{}{
							"activity_type": "update",
							"operator":      "Operator",
						}},
					},
				},
			})
			if err := runShortcut(t, BaseRecordHistoryList, []string{
				"+record-history-list", "--base-token", "app_x", "--table-id", "tbl_x",
				"--record-id", "rec_x", "--format", format,
			}, factory, stdout); err != nil {
				t.Fatalf("err=%v", err)
			}

			got := stdout.String()
			if strings.Contains(got, `"ok"`) || !strings.Contains(got, "activity_type") || !strings.Contains(got, "Operator") {
				t.Fatalf("history %s output did not use the standard formatter:\n%s", format, got)
			}
		})
	}
}

func TestBaseHistoryPrettyExecute(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/record_history",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more":         true,
				"next_max_version": 7,
				"items": []interface{}{
					map[string]interface{}{
						"activity_type": "update",
						"create_time":   int64(1774196856),
						"operator":      "Operator\nName",
						"field_changes": []interface{}{map[string]interface{}{
							"field_name": "Status",
							"before":     "",
							"after":      "done\nvalue",
						}},
					},
					map[string]interface{}{
						"activity_type": "create",
						"create_time":   int64(1774196857),
						"operator":      "Creator",
						"field_changes": nil,
					},
				},
			},
		},
	})
	if err := runShortcut(t, BaseRecordHistoryList, []string{
		"+record-history-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--record-id", "rec_x", "--page-size", "2", "--format", "pretty",
	}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	wantTime := time.Unix(1774196856, 0).Local().Format("2006-01-02 15:04:05 -07:00")
	got := stdout.String()
	for _, want := range []string{
		"1. " + wantTime + " — Operator Name — Status: - -> done value",
		"2. ",
		"— Creator — create",
		"More history is available; continue with --max-version 7.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty history output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"create_time"`) || strings.Contains(got, `"ok"`) {
		t.Fatalf("pretty history output leaked JSON envelope:\n%s", got)
	}
}

func TestFormatRecordHistoryPrettyValidatesNextMaxVersion(t *testing.T) {
	page := func(cursor interface{}, includeCursor bool) map[string]interface{} {
		data := map[string]interface{}{
			"has_more": true,
			"items": []interface{}{map[string]interface{}{
				"activity_type": "update",
			}},
		}
		if includeCursor {
			data["next_max_version"] = cursor
		}
		return data
	}

	got, err := formatRecordHistoryPretty(page(7, true), time.UTC)
	if err != nil {
		t.Fatalf("valid cursor: %v", err)
	}
	if !strings.Contains(got, "continue with --max-version 7.") {
		t.Fatalf("valid cursor guidance missing:\n%s", got)
	}

	for _, test := range []struct {
		name          string
		cursor        interface{}
		includeCursor bool
	}{
		{name: "missing"},
		{name: "zero", cursor: 0, includeCursor: true},
		{name: "negative", cursor: -1, includeCursor: true},
		{name: "fractional", cursor: 1.5, includeCursor: true},
		{name: "string", cursor: "7", includeCursor: true},
		{name: "object", cursor: map[string]interface{}{"value": 7}, includeCursor: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := formatRecordHistoryPretty(page(test.cursor, test.includeCursor), time.UTC)
			if err == nil {
				t.Fatal("expected invalid-response error")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("expected internal invalid-response error, got %T %v", err, err)
			}
		})
	}
}

func TestBaseFieldExecuteUpdate(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"id": "fld_x", "name": "Amount", "type": "number"},
		},
	})
	if err := runShortcut(t, BaseFieldUpdate, []string{"+field-update", "--base-token", "app_x", "--table-id", "tbl_x", "--field-id", "fld_x", "--json", `{"name":"Amount","type":"number"}`, "--yes"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{`"updated": true`, `"fld_x"`, `"field_get_recommended": true`, `"next_step": "field_get"`, `"verification_hint"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q:\n%s", want, got)
		}
	}
}

func TestFieldUpdateResultAlwaysRecommendsReadback(t *testing.T) {
	tests := []struct {
		name         string
		field        interface{}
		submitted    map[string]interface{}
		hintContains []string
	}{
		{
			name:         "direct complex server type overrides simple submitted type",
			field:        map[string]interface{}{"type": "auto_number"},
			submitted:    map[string]interface{}{"type": "number"},
			hintContains: []string{`submitted type "number"`, `server returned type "auto_number"`},
		},
		{
			name:         "nested simple server type still recommends readback",
			field:        map[string]interface{}{"field": map[string]interface{}{"type": "number"}},
			submitted:    map[string]interface{}{"type": "auto_number"},
			hintContains: []string{`submitted type "auto_number"`, `server returned type "number"`},
		},
		{
			name:         "submitted simple type still recommends readback when response omits type",
			field:        map[string]interface{}{"id": "fld_x"},
			submitted:    map[string]interface{}{"type": "text"},
			hintContains: []string{`type "text"`, "cannot determine the previous type"},
		},
		{
			name:         "missing type is conservative",
			field:        map[string]interface{}{"id": "fld_x"},
			submitted:    map[string]interface{}{"name": "Amount"},
			hintContains: []string{"unknown or uncommon field type", "+field-get"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fieldUpdateResult(map[string]interface{}{"field": tc.field, "updated": true}, tc.submitted)
			if got["field_get_recommended"] != true || got["next_step"] != "field_get" {
				t.Fatalf("result=%#v, want readback recommendation", got)
			}
			hint, _ := got["verification_hint"].(string)
			for _, want := range tc.hintContains {
				if !strings.Contains(hint, want) {
					t.Fatalf("verification_hint=%q, want substring %q", hint, want)
				}
			}
		})
	}
}

func TestBaseFieldExecuteUpdateNoopReturnsAPIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x",
		Body: map[string]interface{}{
			"code": 800070003,
			"msg":  "no operation produced",
		},
	})
	err := runShortcut(t, BaseFieldUpdate, []string{"+field-update", "--base-token", "app_x", "--table-id", "tbl_x", "--field-id", "fld_x", "--json", `{"name":"Amount","type":"number"}`, "--yes"}, factory, stdout)
	if err == nil {
		t.Fatal("expected the API no-op response to surface as an error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed API error, got %T %v", err, err)
	}
	if p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeUnknown || p.Code != 800070003 {
		t.Fatalf("category/subtype/code=%s/%s/%d", p.Category, p.Subtype, p.Code)
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T %v", err, err)
	}
	if got := stdout.String(); strings.TrimSpace(got) != "" {
		t.Fatalf("no success envelope should be emitted on a no-op API error:\n%s", got)
	}
}

func TestBaseFieldExecuteUpdateAutoNumberUsesV3FieldJSON(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"field": map[string]interface{}{"id": "fld_x", "name": "编号", "type": "auto_number"},
			},
		},
	}
	reg.Register(stub)
	jsonBody := `{"name":"编号","type":"auto_number","style":{"rules":[{"type":"text","text":"TASK-"},{"type":"created_time","date_format":"yyyyMM"},{"type":"text","text":"-"},{"type":"incremental_number","length":4}]}}`
	if err := runShortcut(t, BaseFieldUpdate, []string{"+field-update", "--base-token", "app_x", "--table-id", "tbl_x", "--field-id", "fld_x", "--json", jsonBody, "--yes"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	gotBody := string(stub.CapturedBody)
	for _, want := range []string{
		`"name":"编号"`,
		`"type":"auto_number"`,
		`"rules":[`,
		`"date_format":"yyyyMM"`,
		`"length":4`,
	} {
		if !strings.Contains(gotBody, want) {
			t.Fatalf("request body missing %q:\n%s", want, gotBody)
		}
	}
	for _, forbidden := range []string{"auto_serial", "reformat_existing_records", `"type":1005`} {
		if strings.Contains(gotBody, forbidden) {
			t.Fatalf("request body must not contain v1 field %q:\n%s", forbidden, gotBody)
		}
	}
	got := stdout.String()
	for _, want := range []string{`"updated": true`, `"fld_x"`, `"field_get_recommended": true`, `"next_step": "field_get"`, `"verification_hint"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{`"reformat_existing_records"`} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("stdout must not expose %q:\n%s", forbidden, got)
		}
	}
}

func TestBaseFieldExecuteUpdateDoesNotRejectExtraJSONKeys(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"id": "fld_x", "name": "编号", "type": "auto_number"},
		},
	}
	reg.Register(stub)
	// Unknown v3 keys are forwarded unchanged; the server remains the source of
	// truth for whether a field-update property is supported.
	jsonBody := `{"name":"编号","type":"auto_number","style":{"rules":[{"type":"incremental_number","length":4}]},"reformat_existing_records":true}`
	if err := runShortcut(t, BaseFieldUpdate, []string{"+field-update", "--base-token", "app_x", "--table-id", "tbl_x", "--field-id", "fld_x", "--json", jsonBody, "--yes"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if gotBody := string(stub.CapturedBody); !strings.Contains(gotBody, `"reformat_existing_records":true`) {
		t.Fatalf("request body must preserve unknown v3 key:\n%s", gotBody)
	}
	if got := stdout.String(); !strings.Contains(got, `"updated": true`) {
		t.Fatalf("expected successful update, got: %s", got)
	}
}

func TestBaseObjectJSONShortcutsRejectArrayInDryRun(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{
			name:     "field update",
			shortcut: BaseFieldUpdate,
			args:     []string{"+field-update", "--base-token", "app_x", "--table-id", "tbl_x", "--field-id", "fld_x", "--json", `[]`, "--dry-run"},
		},
		{
			name:     "record search",
			shortcut: BaseRecordSearch,
			args:     []string{"+record-search", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `[]`, "--dry-run"},
		},
		{
			name:     "record upsert",
			shortcut: BaseRecordUpsert,
			args:     []string{"+record-upsert", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `[]`, "--dry-run"},
		},
		{
			name:     "record batch create",
			shortcut: BaseRecordBatchCreate,
			args:     []string{"+record-batch-create", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `[]`, "--dry-run"},
		},
		{
			name:     "record batch update",
			shortcut: BaseRecordBatchUpdate,
			args:     []string{"+record-batch-update", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `[]`, "--dry-run"},
		},
		{
			name:     "view set filter",
			shortcut: BaseViewSetFilter,
			args:     []string{"+view-set-filter", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--json", `[]`, "--dry-run"},
		},
		{
			name:     "view set visible fields",
			shortcut: BaseViewSetVisibleFields,
			args:     []string{"+view-set-visible-fields", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--json", `[]`, "--dry-run"},
		},
		{
			name:     "view set card",
			shortcut: BaseViewSetCard,
			args:     []string{"+view-set-card", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--json", `[]`, "--dry-run"},
		},
		{
			name:     "view set timebar",
			shortcut: BaseViewSetTimebar,
			args:     []string{"+view-set-timebar", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--json", `[]`, "--dry-run"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			err := runShortcut(t, tt.shortcut, tt.args, factory, stdout)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "--json must be a JSON object") {
				t.Fatalf("err=%v", err)
			}
			if !strings.Contains(err.Error(), "match the documented shape") {
				t.Fatalf("err=%v", err)
			}
			if strings.Contains(err.Error(), "array") {
				t.Fatalf("err should not mention array: %v", err)
			}
			if got := stdout.String(); got != "" {
				t.Fatalf("stdout=%q, want empty", got)
			}
		})
	}
}

func TestBaseTableExecuteCreate(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	createTableStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"id":   "tbl_new",
				"name": "Orders",
				"fields": []interface{}{
					map[string]interface{}{"id": "fld_primary", "name": "OrderNo", "type": "text"},
				},
			},
		},
	}
	reg.Register(createTableStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_new/views",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"id": "vew_main", "name": "Main", "type": "grid"},
		},
	})
	args := []string{"+table-create", "--base-token", "app_x", "--name", "Orders", "--fields", `[{"name":"OrderNo","type":"text"}]`, "--view", `{"name":"Main","type":"grid"}`}
	if err := runShortcut(t, BaseTableCreate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"table"`) || !strings.Contains(got, `"vew_main"`) {
		t.Fatalf("stdout=%s", got)
	}
	body := decodeCapturedJSONBody(t, createTableStub)
	fieldsBody, _ := body["fields"].([]interface{})
	if body["name"] != "Orders" || len(fieldsBody) != 1 {
		t.Fatalf("create table body = %#v", body)
	}
}

func TestBaseTableExecuteUpdate(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"id": "tbl_x", "name": "Orders Updated"},
		},
	})
	if err := runShortcut(t, BaseTableUpdate, []string{"+table-update", "--base-token", "app_x", "--table-id", "tbl_x", "--name", "Orders Updated"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"updated": true`) || !strings.Contains(got, `"Orders Updated"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseRecordExecuteUpsertUpdate(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	updateStub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/rec_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"record_id": "rec_x", "fields": map[string]interface{}{"Name": "Alice"}},
		},
	}
	reg.Register(updateStub)
	if err := runShortcut(t, BaseRecordUpsert, []string{"+record-upsert", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_x", "--json", `{"Name":"Alice"}`}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	body := decodeCapturedJSONBody(t, updateStub)
	if body["Name"] != "Alice" {
		t.Fatalf("request body=%v", body)
	}
	if _, ok := body["fields"]; ok {
		t.Fatalf("request body must not contain fields wrapper: %v", body)
	}
	if got := stdout.String(); !strings.Contains(got, `"updated": true`) || !strings.Contains(got, `"rec_x"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseViewExecuteRename(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"id": "vew_x", "name": "Renamed", "type": "grid"},
		},
	})
	if err := runShortcut(t, BaseViewRename, []string{"+view-rename", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--name", "Renamed"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"Renamed"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseViewExecutePropertyActions(t *testing.T) {
	t.Run("set-group", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "PUT",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_x/group",
			Body: map[string]interface{}{
				"code": 0,
				"data": []interface{}{map[string]interface{}{"field": "fld_status", "desc": false}},
			},
		})
		if err := runShortcut(t, BaseViewSetGroup, []string{"+view-set-group", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--json", `{"group_config":[{"field":"fld_status","desc":false}]}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"group"`) || !strings.Contains(got, `"fld_status"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("set-sort", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "PUT",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_x/sort",
			Body: map[string]interface{}{
				"code": 0,
				"data": []interface{}{map[string]interface{}{"field": "fld_amount", "desc": true}},
			},
		})
		if err := runShortcut(t, BaseViewSetSort, []string{"+view-set-sort", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--json", `{"sort_config":[{"field":"fld_amount","desc":true}]}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"sort"`) || !strings.Contains(got, `"fld_amount"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

}

func TestFieldCreateBatchDelayUsesLowerBoundOfWriteConflictGuidance(t *testing.T) {
	want := 500 * time.Millisecond
	if fieldCreateBatchDelay != want {
		t.Fatalf("fieldCreateBatchDelay=%s, want %s", fieldCreateBatchDelay, want)
	}
}

func TestFieldCreateThrottleDelayCountsRequestTime(t *testing.T) {
	startedAt := time.Unix(0, 0)
	for _, tc := range []struct {
		name string
		now  time.Time
		want time.Duration
	}{
		{name: "first request", want: 0},
		{name: "fast response waits only for remainder", now: startedAt.Add(200 * time.Millisecond), want: 300 * time.Millisecond},
		{name: "slow response needs no extra wait", now: startedAt.Add(600 * time.Millisecond), want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previousStartedAt := startedAt
			if tc.name == "first request" {
				previousStartedAt = time.Time{}
			}
			if got := fieldCreateThrottleDelay(previousStartedAt, tc.now); got != tc.want {
				t.Fatalf("fieldCreateThrottleDelay()=%s, want %s", got, tc.want)
			}
		})
	}
}

func TestBaseFieldExecuteCRUD(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "limit=1&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"fields": []interface{}{
					map[string]interface{}{"id": "fld_2", "name": "Amount", "type": "number"},
				}, "total": 2},
			},
		})
		if err := runShortcut(t, BaseFieldList, []string{"+field-list", "--base-token", "app_x", "--table-id", "tbl_x", "--offset", "0", "--limit", "1"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"total": 2`) || !strings.Contains(got, `"fields"`) || !strings.Contains(got, `"name": "Amount"`) || strings.Contains(got, `"items"`) || strings.Contains(got, `"offset"`) || strings.Contains(got, `"limit"`) || strings.Contains(got, `"count"`) || strings.Contains(got, `"field_name": "Amount"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("get", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "fld_x", "name": "Amount", "type": "number"},
			},
		})
		if err := runShortcut(t, BaseFieldGet, []string{"+field-get", "--base-token", "app_x", "--table-id", "tbl_x", "--field-id", "fld_x"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"field"`) || !strings.Contains(got, `"fld_x"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("create", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "fld_new", "name": "Status", "type": "text"},
			},
		})
		if err := runShortcut(t, BaseFieldCreate, []string{"+field-create", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"name":"Status","type":"text"}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		for _, want := range []string{`"created": true`, `"fld_new"`, `"field_get_recommended": false`, `"next_step": "done"`, `"verification_hint"`} {
			if !strings.Contains(got, want) {
				t.Fatalf("stdout missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("create generated field recommends readback", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "fld_auto", "name": "编号", "type": "auto_number"},
			},
		})
		if err := runShortcut(t, BaseFieldCreate, []string{"+field-create", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"name":"编号","type":"auto_number"}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		for _, want := range []string{`"created": true`, `"fld_auto"`, `"field_get_recommended": true`, `"next_step": "field_get"`, `"verification_hint"`} {
			if !strings.Contains(got, want) {
				t.Fatalf("stdout missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("create array sequentially", func(t *testing.T) {
		oldDelay := fieldCreateBatchDelay
		fieldCreateBatchDelay = 0
		t.Cleanup(func() { fieldCreateBatchDelay = oldDelay })

		factory, stdout, reg := newExecuteFactory(t)
		firstStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields",
			BodyFilter: func(body []byte) bool {
				return strings.Contains(string(body), `"name":"A"`)
			},
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"id":            "fld_a",
					"name":          "A",
					"type":          "text",
					"default_value": nil,
					"description":   "verbose server field metadata",
					"style":         map[string]interface{}{"type": "plain"},
				},
			},
		}
		secondStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields",
			BodyFilter: func(body []byte) bool {
				return strings.Contains(string(body), `"name":"B"`)
			},
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "fld_b", "name": "B", "type": "text"},
			},
		}
		reg.Register(firstStub)
		reg.Register(secondStub)

		err := runShortcut(t, BaseFieldCreate, []string{"+field-create", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `[{"name":"A","type":"text"},{"name":"B","type":"text"}]`}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["created"] != true || data["total"] != float64(2) {
			t.Fatalf("unexpected output: %#v", data)
		}
		fields, _ := data["fields"].([]interface{})
		if len(fields) != 2 {
			t.Fatalf("fields len=%d output=%#v", len(fields), data)
		}
		firstField, _ := fields[0].(map[string]interface{})
		if firstField["id"] != "fld_a" || firstField["name"] != "A" || firstField["type"] != "text" {
			t.Fatalf("batch output should preserve field identity, got %#v", firstField)
		}
		if firstField["description"] != "verbose server field metadata" || firstField["default_value"] != nil {
			t.Fatalf("batch output should preserve server field metadata, got %#v", firstField)
		}
		style, _ := firstField["style"].(map[string]interface{})
		if style["type"] != "plain" {
			t.Fatalf("batch output should preserve server field style, got %#v", firstField)
		}
		if data["field_get_recommended"] != false || data["next_step"] != "done" || data["verification_hint"] == nil {
			t.Fatalf("simple batch create must carry field_get_recommended:false + next_step:done + verification_hint: %#v", data)
		}
		hint := common.GetString(data, "verification_hint")
		for _, want := range []string{"do not list or get fields", "filter +field-list with --jq"} {
			if !strings.Contains(hint, want) {
				t.Fatalf("verification_hint=%q, want %q", hint, want)
			}
		}
		if !strings.Contains(string(firstStub.CapturedBody), `"name":"A"`) || !strings.Contains(string(secondStub.CapturedBody), `"name":"B"`) {
			t.Fatalf("unexpected request bodies: %s / %s", firstStub.CapturedBody, secondStub.CapturedBody)
		}
	})

	t.Run("create array reports progress when a later field fails", func(t *testing.T) {
		oldDelay := fieldCreateBatchDelay
		fieldCreateBatchDelay = 0
		t.Cleanup(func() { fieldCreateBatchDelay = oldDelay })

		runPartial := func(input, createdType, failedName string, failedResponse map[string]interface{}) map[string]interface{} {
			t.Helper()
			factory, stdout, reg := newExecuteFactory(t)
			register := func(name string, response map[string]interface{}) {
				reg.Register(&httpmock.Stub{
					Method:     "POST",
					URL:        "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields",
					BodyFilter: func(body []byte) bool { return strings.Contains(string(body), `"name":"`+name+`"`) },
					Body:       response,
				})
			}
			register("A", map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"id":            "fld_a",
					"name":          "A",
					"type":          createdType,
					"default_value": nil,
					"description":   "verbose server field metadata",
					"style":         map[string]interface{}{"type": "plain"},
				},
			})
			register(failedName, failedResponse)
			err := runShortcut(t, BaseFieldCreate, []string{
				"+field-create", "--base-token", "app_x", "--table-id", "tbl_x", "--json", input,
			}, factory, stdout)
			var partialErr *output.PartialFailureError
			if !errors.As(err, &partialErr) {
				t.Fatalf("expected partial failure error, got %T: %v", err, err)
			}
			var envelope struct {
				OK   bool                   `json:"ok"`
				Data map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("decode partial failure output: %v\nstdout=%s", err, stdout.String())
			}
			if envelope.OK || envelope.Data == nil {
				t.Fatalf("unexpected partial failure envelope: %#v", envelope)
			}
			return envelope.Data
		}

		conflictResponse := map[string]interface{}{
			"code": 1254090,
			"msg":  "field already exists",
			"error": map[string]interface{}{
				"log_id":         "202607300001",
				"troubleshooter": "https://open.feishu.cn/document/troubleshoot/field-exists",
				"details":        []interface{}{map[string]interface{}{"value": "choose a different field name"}},
			},
		}
		data := runPartial(`[{"name":"A","type":"auto_number"},{"name":"B","type":"text"},{"name":"C","type":"text"}]`, "auto_number", "B", conflictResponse)
		summary, _ := data["summary"].(map[string]interface{})
		if summary["requested"] != float64(3) || summary["attempted"] != float64(2) ||
			summary["created"] != float64(1) || summary["failed"] != float64(1) || summary["not_attempted"] != float64(1) {
			t.Fatalf("unexpected summary: %#v", summary)
		}

		items, _ := data["items"].([]interface{})
		if len(items) != 3 {
			t.Fatalf("items=%#v, want three outcomes", items)
		}
		created, _ := items[0].(map[string]interface{})
		createdField, _ := created["field"].(map[string]interface{})
		failed, _ := items[1].(map[string]interface{})
		failedField, _ := failed["field"].(map[string]interface{})
		notAttempted, _ := items[2].(map[string]interface{})
		notAttemptedField, _ := notAttempted["field"].(map[string]interface{})
		if created["status"] != "created" || createdField["id"] != "fld_a" ||
			failed["status"] != "failed" || failed["index"] != float64(1) || failedField["name"] != "B" ||
			notAttempted["status"] != "not_attempted" || notAttemptedField["name"] != "C" {
			t.Fatalf("unexpected item outcomes: %#v", items)
		}
		if len(createdField) != 3 {
			t.Fatalf("partial failure should keep only compact created-field identity, got %#v", createdField)
		}
		if !strings.Contains(common.GetString(failed, "error"), "field already exists") {
			t.Fatalf("failed item must include the API error: %#v", failed)
		}
		for key, want := range map[string]interface{}{
			"type":           "api",
			"subtype":        "unknown",
			"code":           float64(1254090),
			"hint":           "choose a different field name",
			"retryable":      false,
			"log_id":         "202607300001",
			"troubleshooter": "https://open.feishu.cn/document/troubleshoot/field-exists",
		} {
			if failed[key] != want {
				t.Fatalf("failed[%q]=%#v, want %#v; failed=%#v", key, failed[key], want, failed)
			}
		}
		if _, ok := failed["error_type"]; ok {
			t.Fatalf("failed item must use canonical type/subtype fields: %#v", failed)
		}
		writeConflictData := runPartial(`[{"name":"A","type":"text"},{"name":"W","type":"text"}]`, "text", "W", map[string]interface{}{
			"code": 1254291,
			"msg":  "write conflict",
		})
		items, _ = writeConflictData["items"].([]interface{})
		writeConflict, _ := items[1].(map[string]interface{})
		if writeConflict["type"] != "api" || writeConflict["subtype"] != "conflict" || writeConflict["retryable"] != true ||
			!strings.Contains(common.GetString(writeConflict, "hint"), "retry later") {
			t.Fatalf("1254291 must remain a retryable conflict with wait guidance: %#v", writeConflict)
		}
		if !strings.Contains(data["hint"].(string), "Automatically retry a failed item unchanged only when retryable is true") {
			t.Fatalf("hint=%#v", data["hint"])
		}
		if data["field_get_recommended"] != true || data["next_step"] != "inspect_items" || data["verification_hint"] == nil {
			t.Fatalf("partial success with auto_number must recommend readback: %#v", data)
		}

		simpleData := runPartial(`[{"name":"A","type":"text"},{"name":"B","type":"text"},{"name":"C","type":"text"}]`, "text", "B", conflictResponse)
		if simpleData["field_get_recommended"] != false || simpleData["next_step"] != "inspect_items" {
			t.Fatalf("simple-field partial failure must inspect items, not report done: %#v", simpleData)
		}

		permissionData := runPartial(`[{"name":"A","type":"text"},{"name":"P","type":"text"}]`, "text", "P", map[string]interface{}{
			"code": 99991672,
			"msg":  "app scope not applied",
			"error": map[string]interface{}{
				"permission_violations": []interface{}{map[string]interface{}{"subject": "base:field:create"}},
			},
		})
		items, _ = permissionData["items"].([]interface{})
		permissionFailure, _ := items[1].(map[string]interface{})
		missingScopes, _ := permissionFailure["missing_scopes"].([]interface{})
		if len(missingScopes) != 1 || missingScopes[0] != "base:field:create" ||
			permissionFailure["identity"] != "bot" || common.GetString(permissionFailure, "console_url") == "" {
			t.Fatalf("permission failure must retain typed extensions: %#v", permissionFailure)
		}

		policyData := runPartial(`[{"name":"A","type":"text"},{"name":"S","type":"text"}]`, "text", "S", map[string]interface{}{
			"code": 21000,
			"msg":  "challenge required",
			"data": map[string]interface{}{
				"challenge_url": "https://passport.feishu.cn/challenge/field-create",
				"hint":          "complete MFA in the browser, then retry",
			},
		})
		items, _ = policyData["items"].([]interface{})
		policyFailure, _ := items[1].(map[string]interface{})
		if policyFailure["type"] != "policy" || policyFailure["subtype"] != "challenge_required" ||
			policyFailure["code"] != float64(21000) || policyFailure["retryable"] != false ||
			policyFailure["challenge_url"] != "https://passport.feishu.cn/challenge/field-create" ||
			common.GetString(policyFailure, "hint") != "complete MFA in the browser, then retry" {
			t.Fatalf("policy failure must retain typed extensions: %#v", policyFailure)
		}
	})

	t.Run("create array presents partial failure recovery", func(t *testing.T) {
		oldDelay := fieldCreateBatchDelay
		fieldCreateBatchDelay = 0
		t.Cleanup(func() { fieldCreateBatchDelay = oldDelay })

		tests := []struct {
			name string
			plan *surface.Plan
		}{
			{name: "visible"},
			{
				name: "concealed",
				plan: surface.NewPlan(map[surface.CommandID]surface.CommandState{
					surface.CommandAuthLogin: surface.CommandConcealed,
				}),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				factory, stdout, reg := newExecuteFactory(t)
				factory.Recovery = recovery.NewProjector(func() *surface.Plan { return tt.plan })
				reg.Register(&httpmock.Stub{
					Method:     "POST",
					URL:        "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields",
					BodyFilter: func(body []byte) bool { return strings.Contains(string(body), `"name":"A"`) },
					Body: map[string]interface{}{
						"code": 0,
						"data": map[string]interface{}{"id": "fld_a", "name": "A", "type": "text"},
					},
				})
				reg.Register(&httpmock.Stub{
					Method:     "POST",
					URL:        "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields",
					BodyFilter: func(body []byte) bool { return strings.Contains(string(body), `"name":"B"`) },
					Body:       map[string]interface{}{"code": 230027, "msg": "operation unauthorized"},
				})

				err := runShortcutWithAuthTypes(t, BaseFieldCreate, []string{"bot", "user"}, []string{
					"+field-create", "--base-token", "app_x", "--table-id", "tbl_x", "--as", "user",
					"--json", `[{"name":"A","type":"text"},{"name":"B","type":"text"}]`,
				}, factory, stdout)
				var partialErr *output.PartialFailureError
				if !errors.As(err, &partialErr) {
					t.Fatalf("expected partial failure error, got %T: %v", err, err)
				}

				var envelope struct {
					OK   bool `json:"ok"`
					Data struct {
						Items []map[string]interface{} `json:"items"`
						Hint  string                   `json:"hint"`
					} `json:"data"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatalf("decode partial failure output: %v\nstdout=%s", err, stdout.String())
				}
				if envelope.OK || len(envelope.Data.Items) != 2 {
					t.Fatalf("unexpected partial failure envelope: %#v", envelope)
				}

				failed := envelope.Data.Items[1]
				wantHint := errclass.PermissionRecovery(
					[]string{"base:field:create"}, "user", errs.SubtypeUserUnauthorized, "",
				).Render(tt.plan)
				gotHint := common.GetString(failed, "hint")
				if gotHint != wantHint {
					t.Errorf("failed hint = %q, want %q", gotHint, wantHint)
				}
				if failed["type"] != "authorization" || failed["subtype"] != "user_unauthorized" || failed["identity"] != "user" || failed["retryable"] != false {
					t.Errorf("failed typed metadata = %#v", failed)
				}
				for _, want := range []string{
					"Automatically retry a failed item unchanged only when retryable is true",
					"otherwise follow its hint to authorize or correct the input before resubmitting it",
				} {
					if !strings.Contains(envelope.Data.Hint, want) {
						t.Errorf("partial failure hint = %q, want %q", envelope.Data.Hint, want)
					}
				}
				if strings.Contains(envelope.Data.Hint, "Do not retry failed items") {
					t.Errorf("partial failure hint must allow recovery before resubmission: %q", envelope.Data.Hint)
				}
				if _, ok := failed["missing_scopes"]; ok {
					t.Errorf("presentation must not fabricate missing_scopes: %#v", failed)
				}
				if tt.plan == nil {
					if !strings.Contains(gotHint, `auth login --scope "base:field:create"`) {
						t.Errorf("visible recovery lost precise auth path: %q", gotHint)
					}
				} else if strings.Contains(gotHint, "auth login") || !strings.Contains(gotHint, "base:field:create") {
					t.Errorf("concealed recovery leaked command or lost scope: %q", gotHint)
				}
			})
		}
	})

	t.Run("create array with generated field recommends readback", func(t *testing.T) {
		oldDelay := fieldCreateBatchDelay
		fieldCreateBatchDelay = 0
		t.Cleanup(func() { fieldCreateBatchDelay = oldDelay })

		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields",
			BodyFilter: func(body []byte) bool {
				return strings.Contains(string(body), `"name":"Title"`)
			},
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "fld_title", "name": "Title", "type": "text"},
			},
		})
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields",
			BodyFilter: func(body []byte) bool {
				return strings.Contains(string(body), `"name":"编号"`)
			},
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "fld_no", "name": "编号", "type": "auto_number"},
			},
		})

		if err := runShortcut(t, BaseFieldCreate, []string{"+field-create", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `[{"name":"Title","type":"text"},{"name":"编号","type":"auto_number"}]`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["created"] != true || data["total"] != float64(2) {
			t.Fatalf("unexpected output: %#v", data)
		}
		if _, ok := data["fields"].([]interface{}); !ok {
			t.Fatalf("batch create must keep fields array: %#v", data)
		}
		if data["field_get_recommended"] != true || data["next_step"] != "field_get" || data["verification_hint"] == nil {
			t.Fatalf("batch with auto_number must recommend readback: %#v", data)
		}
	})

	t.Run("delete", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "DELETE",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		})
		if err := runShortcut(t, BaseFieldDelete, []string{"+field-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--field-id", "fld_x", "--yes"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"deleted": true`) || !strings.Contains(got, `"field_id": "fld_x"`) {
			t.Fatalf("stdout=%s", got)
		}
	})
}

type fieldCreateCollidingTypedError struct {
	errs.Problem
	Index     int    `json:"index"`
	Status    string `json:"status"`
	Field     string `json:"field"`
	ErrorText string `json:"error"`
}

func TestFieldCreateTypedErrorExtensionsAliasLedgerCollisions(t *testing.T) {
	extensions := fieldCreateTypedErrorExtensions(&fieldCreateCollidingTypedError{
		Problem: errs.Problem{
			Category: errs.CategoryConfig,
			Subtype:  errs.SubtypeInvalidConfig,
			Message:  "invalid config",
		},
		Index:     0,
		Status:    "created",
		Field:     "app_id",
		ErrorText: "masked failure",
	})

	for key, want := range map[string]interface{}{
		"error_index":  float64(0),
		"error_status": "created",
		"error_field":  "app_id",
		"error_error":  "masked failure",
	} {
		if extensions[key] != want {
			t.Errorf("%s=%#v, want %#v", key, extensions[key], want)
		}
	}
	for _, key := range []string{"index", "status", "field", "error"} {
		if _, exists := extensions[key]; exists {
			t.Errorf("ledger key %q must not be exposed by typed extensions: %#v", key, extensions)
		}
	}
}

func TestBaseTableExecuteReadAndDelete(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "limit=1&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"tables": []interface{}{
					map[string]interface{}{"id": "tbl_a", "name": "Alpha"},
				}, "total": 2},
			},
		})
		if err := runShortcut(t, BaseTableList, []string{"+table-list", "--base-token", "app_x", "--limit", "1"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"total": 2`) || !strings.Contains(got, `"tables"`) || !strings.Contains(got, `"name": "Alpha"`) || strings.Contains(got, `"items"`) || strings.Contains(got, `"offset"`) || strings.Contains(got, `"limit"`) || strings.Contains(got, `"count"`) || strings.Contains(got, `"table_name": "Alpha"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("list-http-404", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method:  "GET",
			URL:     "/open-apis/base/v3/bases/app_x/tables",
			Status:  404,
			RawBody: []byte("404 page not found"),
			Headers: map[string][]string{
				"Content-Type": {"text/plain"},
			},
		})
		err := runShortcut(t, BaseTableList, []string{"+table-list", "--base-token", "app_x"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "HTTP 404") || !strings.Contains(err.Error(), "404 page not found") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("get", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "tbl_x", "name": "Orders", "primary_field": "fld_x"},
			},
		})
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"fields": []interface{}{map[string]interface{}{"id": "fld_x", "name": "OrderNo", "type": "text"}}},
			},
		})
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/views",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"views": []interface{}{map[string]interface{}{"id": "vew_x", "name": "Main", "type": "grid"}}},
			},
		})
		if err := runShortcut(t, BaseTableGet, []string{"+table-get", "--base-token", "app_x", "--table-id", "tbl_x"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"name": "Orders"`) || !strings.Contains(got, `"primary_field": "fld_x"`) || !strings.Contains(got, `"id": "fld_x"`) || !strings.Contains(got, `"name": "OrderNo"`) || !strings.Contains(got, `"id": "vew_x"`) || !strings.Contains(got, `"name": "Main"`) || strings.Contains(got, `"field_name": "OrderNo"`) || strings.Contains(got, `"view_name": "Main"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("delete", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "DELETE",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		})
		if err := runShortcut(t, BaseTableDelete, []string{"+table-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--yes"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"deleted": true`) || !strings.Contains(got, `"table_id": "tbl_x"`) {
			t.Fatalf("stdout=%s", got)
		}
	})
}

func TestBaseRecordExecuteReadCreateDelete(t *testing.T) {
	t.Run("list with fields and view", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "field_id=Name&field_id=Age&limit=1&offset=0&view_id=vew_x",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Name", "Age"},
					"record_id_list": []interface{}{"rec_fields"},
					"data":           []interface{}{[]interface{}{"Alice", 18}},
					"total":          1,
				},
			},
		})
		if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--limit", "1", "--field-id", "Name", "--field-id", "Age", "--format", "json"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"rec_fields"`) || !strings.Contains(got, `"Alice"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("list with comma field", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "field_id=A%2CB&field_id=C&limit=1&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"A,B", "C"},
					"record_id_list": []interface{}{"rec_json_fields"},
					"data":           []interface{}{[]interface{}{"value-1", "value-2"}},
					"total":          1,
				},
			},
		})
		if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "1", "--field-id", "A,B", "--field-id", "C", "--format", "json"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"A,B"`) || !strings.Contains(got, `"rec_json_fields"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("list field names alias", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "field_id=Name&field_id=Age&limit=1&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Name", "Age"},
					"record_id_list": []interface{}{"rec_alias"},
					"data":           []interface{}{[]interface{}{"Alice", 18}},
					"total":          1,
				},
			},
		})
		if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "1", "--field-names", "Name,Age", "--format", "json"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"rec_alias"`) || !strings.Contains(got, `"Alice"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("list field names alias preserves quoted commas and at-sign names", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "field_id=A%2CB&field_id=%40Owner&limit=1&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"A,B", "@Owner"},
					"record_id_list": []interface{}{"rec_alias_special"},
					"data":           []interface{}{[]interface{}{"value-1", "value-2"}},
					"total":          1,
				},
			},
		})
		if err := runShortcut(t, BaseRecordList, []string{
			"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "1",
			"--field-names", `"A,B",@Owner`, "--format", "json",
		}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"rec_alias_special"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("list json format", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "limit=1&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Name", "Age"},
					"field_id_list":  []interface{}{"fld_name", "fld_age"},
					"record_id_list": []interface{}{"rec_2"},
					"data":           []interface{}{[]interface{}{"Bob", 20}},
					"total":          1,
				},
			},
		})
		if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "1", "--format", "json"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"Bob"`) || !strings.Contains(got, `"rec_2"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("list json alias", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "limit=1&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Name"},
					"field_id_list":  []interface{}{"fld_name"},
					"record_id_list": []interface{}{"rec_alias"},
					"data":           []interface{}{[]interface{}{"Carol"}},
					"total":          1,
				},
			},
		})
		if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "1", "--json"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"Carol"`) || !strings.Contains(got, `"rec_alias"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("list markdown format", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "field_id=Name&field_id=Age&field_id=Formula&limit=2&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Name", "Age"},
					"field_id_list":  []interface{}{"fld_name", "fld_age"},
					"record_id_list": []interface{}{"rec_1", "rec_2"},
					"data": []interface{}{
						[]interface{}{"Alice", 18},
						[]interface{}{"Bob", 20},
					},
					"has_more": false,
					"query_context": map[string]interface{}{
						"record_scope": "all_records",
						"field_scope":  "selected_fields",
					},
					"ignored_fields": []interface{}{map[string]interface{}{
						"id":     "fld_formula",
						"name":   "Formula",
						"reason": "UNSUPPORTED: formula field cannot be read through OpenAPI because this base uses an old schema version without backend formula computation.",
					}},
				},
			},
		})
		if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "2", "--field-id", "Name", "--field-id", "Age", "--field-id", "Formula"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		for _, want := range []string{
			"`_record_id` is metadata for record operations, not a table field.",
			"| _record_id | Name | Age |",
			"| rec_1 | Alice | 18 |",
			"Meta: count=2; has_more=false; record_scope=all_records; field_scope=selected_fields; ignored_fields=1",
			`Ignored fields: {"id":"fld_formula","name":"Formula","reason":"UNSUPPORTED: formula field cannot be read through OpenAPI because this base uses an old schema version without backend formula computation."}`,
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("stdout missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("search", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		searchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/search",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Title", "Owner"},
					"field_id_list":  []interface{}{"fld_title", "fld_owner"},
					"record_id_list": []interface{}{"rec_1"},
					"data":           []interface{}{[]interface{}{"Created by AI", "Alice"}},
					"has_more":       false,
					"query_context": map[string]interface{}{
						"record_scope": "filtered_records",
						"field_scope":  "selected_fields",
						"search_scope": "fld_title(Title)",
					},
				},
			},
		}
		reg.Register(searchStub)
		if err := runShortcut(
			t,
			BaseRecordSearch,
			[]string{
				"+record-search",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--json", `{"view_id":"vew_x","keyword":"Created","search_fields":["Title","fld_owner"],"select_fields":["Title","fld_owner"],"filter":{"logic":"and","conditions":[["Status","!=","Done"]]},"sort":{"sort_config":[{"field":"Updated At","desc":true},{"field":"Title","desc":false}]},"offset":0,"limit":2}`,
				"--format", "json",
			},
			factory,
			stdout,
		); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"rec_1"`) || !strings.Contains(got, `"query_context"`) {
			t.Fatalf("stdout=%s", got)
		}
		body := string(searchStub.CapturedBody)
		if !strings.Contains(body, `"view_id":"vew_x"`) ||
			!strings.Contains(body, `"keyword":"Created"`) ||
			!strings.Contains(body, `"search_fields":["Title","fld_owner"]`) ||
			!strings.Contains(body, `"select_fields":["Title","fld_owner"]`) ||
			!strings.Contains(body, `"filter":{"conditions":[["Status","!=","Done"]],"logic":"and"}`) ||
			!strings.Contains(body, `"sort":[{"desc":true,"field":"Updated At"},{"desc":false,"field":"Title"}]`) ||
			!strings.Contains(body, `"offset":0`) ||
			!strings.Contains(body, `"limit":2`) {
			t.Fatalf("captured body=%s", body)
		}
	})

	t.Run("search with flag filter sort and projection", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		searchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/search",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Title", "Status"},
					"field_id_list":  []interface{}{"fld_title", "fld_status"},
					"record_id_list": []interface{}{"rec_1"},
					"data":           []interface{}{[]interface{}{"Created by AI", "Todo"}},
					"has_more":       false,
				},
			},
		}
		reg.Register(searchStub)
		if err := runShortcut(
			t,
			BaseRecordSearch,
			[]string{
				"+record-search",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--keyword", "Created",
				"--search-field", "Title",
				"--field-id", "Title",
				"--field-id", "Status",
				"--filter-json", `{"logic":"and","conditions":[["Status","==","Todo"],["Score",">=",80]]}`,
				"--sort-json", `[{"field":"Updated At","desc":true},{"field":"Title","desc":false}]`,
				"--limit", "20",
				"--format", "json",
			},
			factory,
			stdout,
		); err != nil {
			t.Fatalf("err=%v", err)
		}
		var body map[string]interface{}
		if err := json.Unmarshal(searchStub.CapturedBody, &body); err != nil {
			t.Fatalf("captured body json err=%v body=%s", err, string(searchStub.CapturedBody))
		}
		if body["keyword"] != "Created" || body["limit"].(float64) != 20 {
			t.Fatalf("captured body=%#v", body)
		}
		filter := body["filter"].(map[string]interface{})
		if filter["logic"] != "and" {
			t.Fatalf("filter=%#v", filter)
		}
		conditions := filter["conditions"].([]interface{})
		if len(conditions) != 2 {
			t.Fatalf("conditions=%#v", conditions)
		}
		sortConfig := body["sort"].([]interface{})
		if len(sortConfig) != 2 {
			t.Fatalf("sort=%#v", sortConfig)
		}
		firstSort := sortConfig[0].(map[string]interface{})
		if firstSort["field"] != "Updated At" || firstSort["desc"] != true {
			t.Fatalf("sort=%#v", sortConfig)
		}
	})

	t.Run("search with filter json file", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		tmp := t.TempDir()
		withBaseWorkingDir(t, tmp)
		if err := os.WriteFile(filepath.Join(tmp, "filter.json"), []byte(`{"logic":"or","conditions":[["Status","==","Todo"]]}`), 0600); err != nil {
			t.Fatalf("write filter err=%v", err)
		}
		searchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/search",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Title"},
					"record_id_list": []interface{}{"rec_1"},
					"data":           []interface{}{[]interface{}{"A"}},
					"has_more":       false,
				},
			},
		}
		reg.Register(searchStub)
		if err := runShortcut(
			t,
			BaseRecordSearch,
			[]string{
				"+record-search",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--keyword", "A",
				"--search-field", "Title",
				"--filter-json", "@filter.json",
				"--format", "json",
			},
			factory,
			stdout,
		); err != nil {
			t.Fatalf("err=%v", err)
		}
		body := string(searchStub.CapturedBody)
		if !strings.Contains(body, `"filter":{"conditions":[["Status","==","Todo"]],"logic":"or"}`) {
			t.Fatalf("captured body=%s", body)
		}
	})

	t.Run("search markdown format", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/search",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Title", "Owner"},
					"field_id_list":  []interface{}{"fld_title", "fld_owner"},
					"record_id_list": []interface{}{"rec_1"},
					"data":           []interface{}{[]interface{}{"Created by AI", "Alice"}},
					"has_more":       false,
					"query_context": map[string]interface{}{
						"record_scope": "view_filtered_records",
						"field_scope":  "selected_fields",
						"search_scope": "fld_title(Title)",
					},
				},
			},
		})
		if err := runShortcut(
			t,
			BaseRecordSearch,
			[]string{
				"+record-search",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--json", `{"keyword":"Created","search_fields":["Title"],"select_fields":["Title","Owner"],"limit":2}`,
			},
			factory,
			stdout,
		); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		for _, want := range []string{
			"| _record_id | Title | Owner |",
			"| rec_1 | Created by AI | Alice |",
			"Meta: count=1; has_more=false; record_scope=view_filtered_records; field_scope=selected_fields; search_scope=fld_title(Title)",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("stdout missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("list fields alias accepts JSON array projection", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "field_id=Name&field_id=Age&limit=1&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Name", "Age"},
					"record_id_list": []interface{}{"rec_fields"},
					"data":           []interface{}{[]interface{}{"Alice", 18}},
					"total":          1,
				},
			},
		})
		if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "1", "--fields", `["Name","Age"]`, "--format", "json"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"rec_fields"`) || !strings.Contains(got, `"Alice"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("list field names alias accepts repeated projection", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "field_id=Name&field_id=Age&limit=1&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Name", "Age"},
					"record_id_list": []interface{}{"rec_fields"},
					"data":           []interface{}{[]interface{}{"Alice", 18}},
					"total":          1,
				},
			},
		})
		if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "1", "--field-names", "Name", "--field-names", "Age", "--format", "json"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"rec_fields"`) || !strings.Contains(got, `"Alice"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("list projection aliases report only supplied ambiguous inputs", func(t *testing.T) {
		baseArgs := []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x"}
		cases := []struct {
			name       string
			args       []string
			wantParam  string
			wantParams []string
		}{
			{name: "canonical and fields alias", args: []string{"--field-id", "Name", "--fields", `["Age"]`}, wantParam: "--field-id", wantParams: []string{"--field-id", "--fields"}},
			{name: "canonical and field names alias", args: []string{"--field-id", "Name", "--field-names", "Age"}, wantParam: "--field-id", wantParams: []string{"--field-id", "--field-names"}},
			{name: "compatibility aliases", args: []string{"--fields", `["Name"]`, "--field-names", "Age"}, wantParam: "--fields", wantParams: []string{"--fields", "--field-names"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				factory, stdout, _ := newExecuteFactory(t)
				args := append(append([]string{}, baseArgs...), tc.args...)
				err := runShortcut(t, BaseRecordList, args, factory, stdout)
				assertInvalidArgumentValidation(t, err, tc.wantParam, tc.wantParams, "mutually exclusive")
				var validationErr *errs.ValidationError
				if !errors.As(err, &validationErr) || validationErr.Hint != "Use only --field-id for projection." {
					t.Fatalf("hint=%q, want canonical projection guidance", validationErr.Hint)
				}
			})
		}
	})

	t.Run("search json conflict reports each supplied projection parameter", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseRecordSearch, []string{
			"+record-search", "--base-token", "app_x", "--table-id", "tbl_x",
			"--json", `{"keyword":"Alice","search_fields":["Name"]}`,
			"--field-names", "Age",
		}, factory, stdout)
		assertInvalidArgumentValidation(t, err, "--json", []string{"--json", "--field-names"}, "mutually exclusive")
		var validationErr *errs.ValidationError
		if !errors.As(err, &validationErr) || !strings.Contains(validationErr.Hint, "inside --json") {
			t.Fatalf("hint=%q, want JSON-body guidance", validationErr.Hint)
		}
	})

	t.Run("search json conflict reports canonical pagination flag", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseRecordSearch, []string{
			"+record-search", "--base-token", "app_x", "--table-id", "tbl_x",
			"--json", `{"keyword":"Alice","search_fields":["Name"]}`,
			"--limit", "10", "--page-size", "201",
		}, factory, stdout)
		assertInvalidArgumentValidation(t, err, "--json", []string{"--json", "--page-size"}, "mutually exclusive")
	})

	t.Run("list canonical and alias projections reject duplicates consistently", func(t *testing.T) {
		cases := []struct {
			name  string
			args  []string
			param string
		}{
			{name: "canonical", args: []string{"--field-id", "Cost--USD", "--field-id", "Cost--USD"}, param: "--field-id"},
			{name: "fields alias", args: []string{"--fields", `["Cost--USD","Cost--USD"]`}, param: "--fields"},
			{name: "field names alias", args: []string{"--field-names", "Cost--USD", "--field-names", "Cost--USD"}, param: "--field-names"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				factory, stdout, _ := newExecuteFactory(t)
				args := append([]string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x"}, tc.args...)
				err := runShortcut(t, BaseRecordList, args, factory, stdout)
				assertInvalidArgumentValidation(t, err, tc.param, []string{tc.param}, "duplicate field id")
			})
		}
	})

	t.Run("search fields alias accepts JSON array projection", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		searchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/search",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Name", "Age"},
					"record_id_list": []interface{}{"rec_search"},
					"data":           []interface{}{[]interface{}{"Alice", 18}},
				},
			},
		}
		reg.Register(searchStub)
		if err := runShortcut(t, BaseRecordSearch, []string{
			"+record-search", "--base-token", "app_x", "--table-id", "tbl_x",
			"--keyword", "Alice", "--search-field", "Name", "--fields", `["Name","Age"]`, "--format", "json",
		}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if body := string(searchStub.CapturedBody); !strings.Contains(body, `"select_fields":["Name","Age"]`) {
			t.Fatalf("captured body=%s", body)
		}
	})

	t.Run("get field names alias accepts repeated projection", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_1"},
					"fields":         []interface{}{"Name", "Age"},
					"data":           []interface{}{[]interface{}{"Alice", 18}},
				},
			},
		}
		reg.Register(batchStub)
		if err := runShortcut(t, BaseRecordGet, []string{
			"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1",
			"--field-names", "Name", "--field-names", "Age", "--format", "json",
		}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if body := string(batchStub.CapturedBody); !strings.Contains(body, `"select_fields":["Name","Age"]`) {
			t.Fatalf("request body=%s", body)
		}
	})

	t.Run("get", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_1"},
					"fields":         []interface{}{"Name", "Age"},
					"data":           []interface{}{[]interface{}{"Alice", 18}},
				},
			},
		}
		reg.Register(batchStub)
		if err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		for _, want := range []string{
			"`_record_id` is metadata for record operations, not a table field.",
			"- `_record_id`: rec_1",
			"- `Name`: Alice",
			"- `Age`: 18",
			"Meta: count=1",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("stdout missing %q:\n%s", want, got)
			}
		}
		body := string(batchStub.CapturedBody)
		if !strings.Contains(body, `"record_id_list":["rec_1"]`) {
			t.Fatalf("request body=%s", body)
		}
	})

	t.Run("get json format", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_1"},
					"fields":         []interface{}{"Name", "Age"},
					"data":           []interface{}{[]interface{}{"Alice", 18}},
				},
			},
		}
		reg.Register(batchStub)
		if err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1", "--format", "json"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"fields"`) || !strings.Contains(got, `"Alice"`) || !strings.Contains(got, `"Age"`) || strings.Contains(got, `"record":`) || strings.Contains(got, `"raw"`) {
			t.Fatalf("stdout=%s", got)
		}
		if got := stdout.String(); !strings.Contains(got, `"rec_1"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("get with selected fields", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_1"},
					"fields":         []interface{}{"Name", "Age"},
					"data":           []interface{}{[]interface{}{"Alice", 18}},
				},
			},
		}
		reg.Register(batchStub)
		if err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1", "--field-id", "Name", "--field-id", "Age", "--format", "json"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"fields"`) || !strings.Contains(got, `"Name"`) || !strings.Contains(got, `"Age"`) || !strings.Contains(got, `"Alice"`) || strings.Contains(got, `"record":`) {
			t.Fatalf("stdout=%s", got)
		}
		body := string(batchStub.CapturedBody)
		if !strings.Contains(body, `"record_id_list":["rec_1"]`) || !strings.Contains(body, `"select_fields":["Name","Age"]`) {
			t.Fatalf("request body=%s", body)
		}
	})

	t.Run("get batch with repeated record-id flags", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_2", "rec_1"},
					"fields":         []interface{}{"Name"},
					"data":           []interface{}{[]interface{}{"Bob"}, []interface{}{"Alice"}},
				},
			},
		}
		reg.Register(batchStub)
		if err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_2", "--record-id", "rec_1", "--field-id", "Name"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		for _, want := range []string{
			"| _record_id | Name |",
			"| rec_2 | Bob |",
			"| rec_1 | Alice |",
			"Meta: count=2",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("stdout missing %q:\n%s", want, got)
			}
		}
		body := string(batchStub.CapturedBody)
		if !strings.Contains(body, `"record_id_list":["rec_2","rec_1"]`) || !strings.Contains(body, `"select_fields":["Name"]`) {
			t.Fatalf("request body=%s", body)
		}
	})

	t.Run("get batch json format", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_2", "rec_1"},
					"fields":         []interface{}{"Name"},
					"data":           []interface{}{[]interface{}{"Bob"}, []interface{}{"Alice"}},
				},
			},
		}
		reg.Register(batchStub)
		if err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_2", "--record-id", "rec_1", "--field-id", "Name", "--format", "json"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"rec_2"`) || !strings.Contains(got, `"Bob"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("get batch with json selector", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_3"},
					"fields":         []interface{}{"Name"},
					"data":           []interface{}{[]interface{}{"Carol"}},
				},
			},
		}
		reg.Register(batchStub)
		if err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"record_id_list":["rec_3"],"select_fields":["Name"]}`, "--format", "json"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"Carol"`) {
			t.Fatalf("stdout=%s", got)
		}
		body := string(batchStub.CapturedBody)
		if !strings.Contains(body, `"record_id_list":["rec_3"]`) || !strings.Contains(body, `"select_fields":["Name"]`) {
			t.Fatalf("request body=%s", body)
		}
	})

	t.Run("get single returns batch_get error when batch_get is unavailable", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
			Status: 404,
			Body:   map[string]interface{}{"code": 404, "msg": "not found"},
		}
		reg.Register(batchStub)
		err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1"}, factory, stdout)
		if err == nil {
			t.Fatalf("expected batch_get error")
		}
		if !strings.Contains(string(batchStub.CapturedBody), `"record_id_list":["rec_1"]`) {
			t.Fatalf("request body=%s", string(batchStub.CapturedBody))
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout=%s", stdout.String())
		}
	})

	t.Run("get single missing record renders not found markdown", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list":   []interface{}{"rec_missing"},
					"fields":           []interface{}{"Name"},
					"data":             []interface{}{[]interface{}{nil}},
					"has_more":         false,
					"record_not_found": []interface{}{"rec_missing"},
				},
			},
		}
		reg.Register(batchStub)
		if err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_missing"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		for _, want := range []string{
			"Record not found.",
			"- `_record_id`: rec_missing",
			"Meta: count=1; has_more=false; record_not_found=1",
			"Missing records: rec_missing",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("stdout missing %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, "- `Name`:") {
			t.Fatalf("missing record output should not render business fields:\n%s", got)
		}
	})

	t.Run("get batch returns batch_get error when batch_get is unavailable", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
			Status: 404,
			Body:   map[string]interface{}{"code": 404, "msg": "not found"},
		}
		reg.Register(batchStub)
		err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_2", "--record-id", "rec_1", "--field-id", "Name"}, factory, stdout)
		if err == nil {
			t.Fatalf("expected batch_get error")
		}
		body := string(batchStub.CapturedBody)
		if !strings.Contains(body, `"record_id_list":["rec_2","rec_1"]`) || !strings.Contains(body, `"select_fields":["Name"]`) {
			t.Fatalf("request body=%s", body)
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout=%s", stdout.String())
		}
	})

	t.Run("get batch with json record ids and field flags", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_4"},
					"fields":         []interface{}{"Status"},
					"data":           []interface{}{[]interface{}{"Done"}},
				},
			},
		}
		reg.Register(batchStub)
		if err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"record_id_list":["rec_4"]}`, "--field-id", "Status", "--format", "json"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"Done"`) {
			t.Fatalf("stdout=%s", got)
		}
		body := string(batchStub.CapturedBody)
		if !strings.Contains(body, `"record_id_list":["rec_4"]`) || !strings.Contains(body, `"select_fields":["Status"]`) {
			t.Fatalf("request body=%s", body)
		}
	})

	t.Run("get rejects duplicate record ids", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1", "--record-id", "rec_1"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "duplicate record id") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("get rejects duplicate field ids", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1", "--field-id", "Name", "--field-id", "Name"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "duplicate field id") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("get rejects mixed record-id and json", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1", "--json", `{"record_id_list":["rec_2"]}`}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("get rejects mixed field-id and json select_fields", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"record_id_list":["rec_2"],"select_fields":["Name"]}`, "--field-id", "Age"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "select_fields") || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("get rejects empty selection", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "provide at least one --record-id") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("create", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		createStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"record_id": "rec_new", "fields": map[string]interface{}{"Name": "Alice"}},
			},
		}
		reg.Register(createStub)
		if err := runShortcut(t, BaseRecordUpsert, []string{"+record-upsert", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"Name":"Alice"}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		body := decodeCapturedJSONBody(t, createStub)
		if body["Name"] != "Alice" {
			t.Fatalf("request body=%v", body)
		}
		if _, ok := body["fields"]; ok {
			t.Fatalf("request body must not contain fields wrapper: %v", body)
		}
		if got := stdout.String(); !strings.Contains(got, `"created": true`) || !strings.Contains(got, `"rec_new"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("batch create", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_create",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_1", "rec_2"},
				},
			},
		})
		if err := runShortcut(t, BaseRecordBatchCreate, []string{"+record-batch-create", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"create_records":[{"Name":"Alice"},{"Name":"Bob"}]}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"rec_1"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("batch update", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_update",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"ignored_fields": []interface{}{map[string]interface{}{
						"id":     "fld_formula",
						"name":   "Formula",
						"reason": "READONLY: formula field cannot be written through OpenAPI.",
					}},
				},
			},
		})
		if err := runShortcut(t, BaseRecordBatchUpdate, []string{"+record-batch-update", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"update_records":{"rec_1":{"Status":["Done"],"Formula":"ignored"}}}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"ignored_fields"`) || !strings.Contains(got, `"Formula"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("batch update passthrough", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		updateStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_update",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{},
			},
		}
		reg.Register(updateStub)
		input := `{"update_records":{"recA":{"Status":["Done"]},"recB":{"Score":20}}}`
		if err := runShortcut(t, BaseRecordBatchUpdate, []string{"+record-batch-update", "--base-token", "app_x", "--table-id", "tbl_x", "--json", input}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		body := string(updateStub.CapturedBody)
		if !strings.Contains(body, `"update_records":{"recA":{"Status":["Done"]},"recB":{"Score":20}}`) {
			t.Fatalf("request body=%s", body)
		}
	})

	t.Run("delete", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_delete",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_1"},
				},
			},
		}
		reg.Register(batchStub)
		if err := runShortcut(t, BaseRecordDelete, []string{"+record-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1", "--yes"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"rec_1"`) || strings.Contains(got, `"deleted": true`) {
			t.Fatalf("stdout=%s", got)
		}
		if !strings.Contains(string(batchStub.CapturedBody), `"record_id_list":["rec_1"]`) {
			t.Fatalf("request body=%s", string(batchStub.CapturedBody))
		}
	})

	t.Run("delete returns batch_delete error when unavailable", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_delete",
			Status: 404,
			Body:   map[string]interface{}{"code": 404, "msg": "not found"},
		}
		reg.Register(batchStub)
		err := runShortcut(t, BaseRecordDelete, []string{"+record-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1", "--yes"}, factory, stdout)
		if err == nil {
			t.Fatalf("expected batch_delete error")
		}
		if !strings.Contains(string(batchStub.CapturedBody), `"record_id_list":["rec_1"]`) {
			t.Fatalf("request body=%s", string(batchStub.CapturedBody))
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout=%s", stdout.String())
		}
	})

	t.Run("delete batch with repeated record-id flags", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_delete",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_2", "rec_1"},
				},
			},
		}
		reg.Register(batchStub)
		if err := runShortcut(t, BaseRecordDelete, []string{"+record-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_2", "--record-id", "rec_1", "--yes"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"rec_2"`) {
			t.Fatalf("stdout=%s", got)
		}
		body := string(batchStub.CapturedBody)
		if !strings.Contains(body, `"record_id_list":["rec_2","rec_1"]`) {
			t.Fatalf("request body=%s", body)
		}
	})

	t.Run("delete batch with json selector", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		batchStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_delete",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_3"},
				},
			},
		}
		reg.Register(batchStub)
		if err := runShortcut(t, BaseRecordDelete, []string{"+record-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"record_id_list":["rec_3"]}`, "--yes"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"rec_3"`) {
			t.Fatalf("stdout=%s", got)
		}
		body := string(batchStub.CapturedBody)
		if !strings.Contains(body, `"record_id_list":["rec_3"]`) {
			t.Fatalf("request body=%s", body)
		}
	})

	t.Run("delete requires yes for batch", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseRecordDelete, []string{"+record-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_2", "--record-id", "rec_1"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "requires confirmation") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("delete rejects duplicate record ids", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseRecordDelete, []string{"+record-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1", "--record-id", "rec_1", "--yes"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "duplicate record id") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("delete rejects mixed record-id and json", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseRecordDelete, []string{"+record-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1", "--json", `{"record_id_list":["rec_2"]}`, "--yes"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("upload attachment", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)

		tmpFile, err := os.CreateTemp(t.TempDir(), "base-attachment-*.png")
		if err != nil {
			t.Fatalf("CreateTemp() err=%v", err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 3, 2))
		img.Set(0, 0, color.RGBA{R: 255, A: 255})
		if err := png.Encode(tmpFile, img); err != nil {
			t.Fatalf("png.Encode() err=%v", err)
		}
		if err := tmpFile.Close(); err != nil {
			t.Fatalf("Close() err=%v", err)
		}
		withBaseWorkingDir(t, filepath.Dir(tmpFile.Name()))

		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_att",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "fld_att", "name": "附件", "type": "attachment"},
			},
		})
		uploadStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/medias/upload_all",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"file_token": "file_tok_1"},
			},
		}
		reg.Register(uploadStub)
		appendStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/append_attachments",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"attachments": map[string]interface{}{
						"rec_x": map[string]interface{}{
							"fld_att": []interface{}{
								map[string]interface{}{
									"file_token": "file_tok_1",
									"name":       "base-attachment.png",
									"size":       73,
								},
							},
						},
					},
				},
			},
		}
		reg.Register(appendStub)

		if err := runShortcut(t, BaseRecordUploadAttachment, []string{
			"+record-upload-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--field-id", "fld_att",
			"--file", "./" + filepath.Base(tmpFile.Name()),
		}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"file_tok_1"`) || strings.Contains(got, `"updated"`) || strings.Contains(got, `"uploaded"`) {
			t.Fatalf("stdout=%s", got)
		}

		uploadBody := string(uploadStub.CapturedBody)
		if !strings.Contains(uploadBody, `name="parent_type"`) || !strings.Contains(uploadBody, "bitable_file") || !strings.Contains(uploadBody, `name="parent_node"`) || !strings.Contains(uploadBody, "app_x") {
			t.Fatalf("upload body=%s", uploadBody)
		}

		appendBody := string(appendStub.CapturedBody)
		if !strings.Contains(appendBody, `"rec_x"`) ||
			!strings.Contains(appendBody, `"fld_att"`) ||
			!strings.Contains(appendBody, `"file_token":"file_tok_1"`) ||
			!strings.Contains(appendBody, `"image_width":3`) ||
			!strings.Contains(appendBody, `"image_height":2`) {
			t.Fatalf("append body=%s", appendBody)
		}
	})

	t.Run("upload attachment uses multipart for large file", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)

		tmpFile, err := os.CreateTemp(t.TempDir(), "base-attachment-large-*.bin")
		if err != nil {
			t.Fatalf("CreateTemp() err=%v", err)
		}
		if err := tmpFile.Truncate(common.MaxDriveMediaUploadSinglePartSize + 1); err != nil {
			t.Fatalf("Truncate() err=%v", err)
		}
		if err := tmpFile.Close(); err != nil {
			t.Fatalf("Close() err=%v", err)
		}
		withBaseWorkingDir(t, filepath.Dir(tmpFile.Name()))

		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_att",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "fld_att", "name": "附件", "type": "attachment"},
			},
		})

		prepareStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/medias/upload_prepare",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"upload_id":  "upload_big_1",
					"block_size": float64(8 * 1024 * 1024),
					"block_num":  float64(3),
				},
			},
		}
		reg.Register(prepareStub)

		partStubs := make([]*httpmock.Stub, 0, 3)
		for i := 0; i < 3; i++ {
			stub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/drive/v1/medias/upload_part",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "ok",
				},
			}
			partStubs = append(partStubs, stub)
			reg.Register(stub)
		}

		finishStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/medias/upload_finish",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"file_token": "file_tok_big"},
			},
		}
		reg.Register(finishStub)

		appendStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/append_attachments",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"attachments": map[string]interface{}{
						"rec_x": map[string]interface{}{
							"fld_att": []interface{}{
								map[string]interface{}{"file_token": "file_tok_big"},
							},
						},
					},
				},
			},
		}
		reg.Register(appendStub)

		if err := runShortcut(t, BaseRecordUploadAttachment, []string{
			"+record-upload-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--field-id", "fld_att",
			"--file", "./" + filepath.Base(tmpFile.Name()),
		}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}

		if got := stdout.String(); !strings.Contains(got, `"file_tok_big"`) || strings.Contains(got, `"updated"`) || strings.Contains(got, `"uploaded"`) {
			t.Fatalf("stdout=%s", got)
		}

		prepareBody := string(prepareStub.CapturedBody)
		if !strings.Contains(prepareBody, `"file_name":"`+filepath.Base(tmpFile.Name())+`"`) ||
			!strings.Contains(prepareBody, `"parent_type":"bitable_file"`) ||
			!strings.Contains(prepareBody, `"parent_node":"app_x"`) ||
			!strings.Contains(prepareBody, `"size":20971521`) {
			t.Fatalf("prepare body=%s", prepareBody)
		}

		firstPartBody := string(partStubs[0].CapturedBody)
		if !strings.Contains(firstPartBody, `name="upload_id"`) ||
			!strings.Contains(firstPartBody, "upload_big_1") ||
			!strings.Contains(firstPartBody, `name="seq"`) ||
			!strings.Contains(firstPartBody, "\r\n0\r\n") ||
			!strings.Contains(firstPartBody, `name="size"`) ||
			!strings.Contains(firstPartBody, "8388608") {
			t.Fatalf("first part body=%s", firstPartBody)
		}

		lastPartBody := string(partStubs[2].CapturedBody)
		if !strings.Contains(lastPartBody, `name="seq"`) ||
			!strings.Contains(lastPartBody, "\r\n2\r\n") ||
			!strings.Contains(lastPartBody, `name="size"`) ||
			!strings.Contains(lastPartBody, "4194305") {
			t.Fatalf("last part body=%s", lastPartBody)
		}

		finishBody := string(finishStub.CapturedBody)
		if !strings.Contains(finishBody, `"upload_id":"upload_big_1"`) ||
			!strings.Contains(finishBody, `"block_num":3`) {
			t.Fatalf("finish body=%s", finishBody)
		}

		appendBody := string(appendStub.CapturedBody)
		if !strings.Contains(appendBody, `"rec_x"`) ||
			!strings.Contains(appendBody, `"fld_att"`) ||
			!strings.Contains(appendBody, `"file_token":"file_tok_big"`) {
			t.Fatalf("append body=%s", appendBody)
		}
	})

	t.Run("upload attachment rejects non-attachment field", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)

		tmpFile, err := os.CreateTemp(t.TempDir(), "base-not-attachment-*.txt")
		if err != nil {
			t.Fatalf("CreateTemp() err=%v", err)
		}
		if _, err := tmpFile.WriteString("hello"); err != nil {
			t.Fatalf("WriteString() err=%v", err)
		}
		if err := tmpFile.Close(); err != nil {
			t.Fatalf("Close() err=%v", err)
		}
		withBaseWorkingDir(t, filepath.Dir(tmpFile.Name()))

		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_status",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "fld_status", "name": "状态", "type": "text"},
			},
		})

		err = runShortcut(t, BaseRecordUploadAttachment, []string{
			"+record-upload-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--field-id", "fld_status",
			"--file", "./" + filepath.Base(tmpFile.Name()),
		}, factory, stdout)
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "expected attachment") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("upload attachment rejects file larger than 2GB", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)

		tmpFile, err := os.CreateTemp(t.TempDir(), "base-too-large-*.bin")
		if err != nil {
			t.Fatalf("CreateTemp() err=%v", err)
		}
		if err := tmpFile.Truncate(2*1024*1024*1024 + 1); err != nil {
			t.Fatalf("Truncate() err=%v", err)
		}
		if err := tmpFile.Close(); err != nil {
			t.Fatalf("Close() err=%v", err)
		}
		withBaseWorkingDir(t, filepath.Dir(tmpFile.Name()))

		err = runShortcut(t, BaseRecordUploadAttachment, []string{
			"+record-upload-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--field-id", "fld_att",
			"--file", "./" + filepath.Base(tmpFile.Name()),
		}, factory, stdout)
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "exceeds 2GB limit") {
			t.Fatalf("err=%v", err)
		}
		if !strings.Contains(err.Error(), filepath.Base(tmpFile.Name())) {
			t.Fatalf("err=%v should name the offending file", err)
		}
	})

	t.Run("upload attachment rejects deprecated name flag", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)

		tmpFile, err := os.CreateTemp(t.TempDir(), "base-name-*.txt")
		if err != nil {
			t.Fatalf("CreateTemp() err=%v", err)
		}
		if err := tmpFile.Close(); err != nil {
			t.Fatalf("Close() err=%v", err)
		}
		withBaseWorkingDir(t, filepath.Dir(tmpFile.Name()))

		err = runShortcut(t, BaseRecordUploadAttachment, []string{
			"+record-upload-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--field-id", "fld_att",
			"--file", "./" + filepath.Base(tmpFile.Name()),
			"--name", "renamed.txt",
		}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "--name is no longer supported") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("download attachment uses extra info", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)

		extra := `{"bitablePerm":{"tableId":"tbl_x","attachments":{"fld_att":{"rec_x":["box_a"]}}}}`
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/get_attachments",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"attachments": map[string]interface{}{
						"rec_x": map[string]interface{}{
							"fld_att": []interface{}{
								map[string]interface{}{
									"file_token": "box_a",
									"name":       "pic.png",
									"size":       7,
									"extra_info": extra,
								},
							},
						},
					},
				},
			},
		})
		downloadStub := &httpmock.Stub{
			Method:      "GET",
			URL:         "/open-apis/drive/v1/medias/box_a/download?" + url.Values{"extra": []string{extra}}.Encode(),
			RawBody:     []byte("payload"),
			ContentType: "image/png",
		}
		reg.Register(downloadStub)

		tmpDir := t.TempDir()
		withBaseWorkingDir(t, tmpDir)
		if err := os.Mkdir("downloads", 0700); err != nil {
			t.Fatalf("Mkdir() err=%v", err)
		}

		if err := runShortcut(t, BaseRecordDownloadAttachment, []string{
			"+record-download-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--file-token", "box_a",
			"--output", "downloads",
		}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "downloads", "pic.png")); err != nil {
			t.Fatalf("expected downloaded file: %v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		gotItems, _ := data["downloaded"].([]interface{})
		if len(gotItems) != 1 {
			t.Fatalf("downloaded=%#v", data["downloaded"])
		}
		got, _ := gotItems[0].(map[string]interface{})
		if got["file_token"] != "box_a" || got["saved_path"] == "" || got["extra_info_used"] != nil {
			t.Fatalf("download output=%#v", got)
		}
	})

	t.Run("download all row attachments when file token omitted", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)

		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/get_attachments",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"attachments": map[string]interface{}{
						"rec_x": map[string]interface{}{
							"fld_att": []interface{}{
								map[string]interface{}{"file_token": "box_a", "name": "a.txt", "size": 7},
								map[string]interface{}{"file_token": "box_b", "name": "b.txt", "size": 8},
							},
						},
					},
				},
			},
		})
		reg.Register(&httpmock.Stub{
			Method:      "GET",
			URL:         "/open-apis/drive/v1/medias/box_a/download",
			RawBody:     []byte("payload-a"),
			ContentType: "text/plain",
		})
		reg.Register(&httpmock.Stub{
			Method:      "GET",
			URL:         "/open-apis/drive/v1/medias/box_b/download",
			RawBody:     []byte("payload-b"),
			ContentType: "text/plain",
		})

		tmpDir := t.TempDir()
		withBaseWorkingDir(t, tmpDir)
		if err := os.Mkdir("downloads", 0700); err != nil {
			t.Fatalf("Mkdir() err=%v", err)
		}

		if err := runShortcut(t, BaseRecordDownloadAttachment, []string{
			"+record-download-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--output", "downloads",
		}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "downloads", "a.txt")); err != nil {
			t.Fatalf("expected downloaded file a.txt: %v", err)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "downloads", "b.txt")); err != nil {
			t.Fatalf("expected downloaded file b.txt: %v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		gotItems, _ := data["downloaded"].([]interface{})
		if len(gotItems) != 2 {
			t.Fatalf("downloaded=%#v", data["downloaded"])
		}
	})

	t.Run("download without file token requires output directory", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		tmpDir := t.TempDir()
		withBaseWorkingDir(t, tmpDir)

		err := runShortcut(t, BaseRecordDownloadAttachment, []string{
			"+record-download-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--output", "file.txt",
		}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "--output must be an existing directory") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("download surfaces unsafe output path instead of directory hint", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		tmpDir := t.TempDir()
		withBaseWorkingDir(t, tmpDir)

		err := runShortcut(t, BaseRecordDownloadAttachment, []string{
			"+record-download-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--output", "../escape",
		}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "unsafe output path") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("download all disambiguates duplicate attachment names with file token", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/get_attachments",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"attachments": map[string]interface{}{
						"rec_x": map[string]interface{}{
							"fld_att": []interface{}{
								map[string]interface{}{"file_token": "box_a", "name": "same.txt", "size": 7},
								map[string]interface{}{"file_token": "box_a", "name": "same.txt", "size": 7},
								map[string]interface{}{"file_token": "box_b", "name": "same.txt", "size": 8},
							},
						},
					},
				},
			},
		})
		reg.Register(&httpmock.Stub{
			Method:      "GET",
			URL:         "/open-apis/drive/v1/medias/box_a/download",
			RawBody:     []byte("payload-a"),
			ContentType: "text/plain",
		})
		reg.Register(&httpmock.Stub{
			Method:      "GET",
			URL:         "/open-apis/drive/v1/medias/box_b/download",
			RawBody:     []byte("payload-b"),
			ContentType: "text/plain",
		})

		tmpDir := t.TempDir()
		withBaseWorkingDir(t, tmpDir)
		if err := os.Mkdir("downloads", 0700); err != nil {
			t.Fatalf("Mkdir() err=%v", err)
		}

		if err := runShortcut(t, BaseRecordDownloadAttachment, []string{
			"+record-download-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--output", "downloads",
		}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "downloads", "same_box_a.txt")); err != nil {
			t.Fatalf("expected downloaded file same_box_a.txt: %v", err)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "downloads", "same_box_b.txt")); err != nil {
			t.Fatalf("expected downloaded file same_box_b.txt: %v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		gotItems, _ := data["downloaded"].([]interface{})
		if len(gotItems) != 2 {
			t.Fatalf("downloaded=%#v", data["downloaded"])
		}
	})

	t.Run("download duplicate requested file token only once", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/get_attachments",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"attachments": map[string]interface{}{
						"rec_x": map[string]interface{}{
							"fld_att": []interface{}{
								map[string]interface{}{"file_token": "box_a", "name": "a.txt", "size": 7},
							},
						},
					},
				},
			},
		})
		reg.Register(&httpmock.Stub{
			Method:      "GET",
			URL:         "/open-apis/drive/v1/medias/box_a/download",
			RawBody:     []byte("payload-a"),
			ContentType: "text/plain",
		})

		tmpDir := t.TempDir()
		withBaseWorkingDir(t, tmpDir)
		if err := runShortcut(t, BaseRecordDownloadAttachment, []string{
			"+record-download-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--file-token", "box_a",
			"--file-token", "box_a",
			"--output", "a.txt",
		}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		gotItems, _ := data["downloaded"].([]interface{})
		if len(gotItems) != 1 {
			t.Fatalf("downloaded=%#v", data["downloaded"])
		}
	})

	t.Run("download all preflights local target conflicts before writing", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/get_attachments",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"attachments": map[string]interface{}{
						"rec_x": map[string]interface{}{
							"fld_att": []interface{}{
								map[string]interface{}{"file_token": "box_a", "name": "a.txt", "size": 7},
								map[string]interface{}{"file_token": "box_b", "name": "b.txt", "size": 8},
							},
						},
					},
				},
			},
		})

		tmpDir := t.TempDir()
		withBaseWorkingDir(t, tmpDir)
		if err := os.Mkdir("downloads", 0700); err != nil {
			t.Fatalf("Mkdir() err=%v", err)
		}
		if err := os.WriteFile(filepath.Join("downloads", "b.txt"), []byte("existing"), 0600); err != nil {
			t.Fatalf("WriteFile() err=%v", err)
		}

		err := runShortcut(t, BaseRecordDownloadAttachment, []string{
			"+record-download-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--output", "downloads",
		}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "output file already exists: downloads/b.txt") {
			t.Fatalf("err=%v", err)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "downloads", "a.txt")); err == nil {
			t.Fatalf("a.txt should not be written after preflight conflict")
		}
	})

	t.Run("download reports progress and log_id when later attachment fails", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/get_attachments",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"attachments": map[string]interface{}{
						"rec_x": map[string]interface{}{
							"fld_att": []interface{}{
								map[string]interface{}{"file_token": "box_a", "name": "a.txt", "size": 7},
								map[string]interface{}{"file_token": "box_b", "name": "b.txt", "size": 8},
							},
						},
					},
				},
			},
		})
		reg.Register(&httpmock.Stub{
			Method:      "GET",
			URL:         "/open-apis/drive/v1/medias/box_a/download",
			RawBody:     []byte("payload-a"),
			ContentType: "text/plain",
		})
		reg.Register(&httpmock.Stub{
			Method:  "GET",
			URL:     "/open-apis/drive/v1/medias/box_b/download",
			Status:  403,
			RawBody: []byte("server error"),
			Headers: http.Header{"X-Tt-Logid": []string{"202605270001"}},
		})

		tmpDir := t.TempDir()
		withBaseWorkingDir(t, tmpDir)
		if err := os.Mkdir("downloads", 0700); err != nil {
			t.Fatalf("Mkdir() err=%v", err)
		}

		err := runShortcut(t, BaseRecordDownloadAttachment, []string{
			"+record-download-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--output", "downloads",
		}, factory, stdout)
		if err == nil {
			t.Fatalf("err=%v", err)
		}
		var partialErr *output.PartialFailureError
		if !errors.As(err, &partialErr) {
			t.Fatalf("expected partial failure error, got %T %v", err, err)
		}

		var envelope map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("failed to decode partial failure output: %v\nraw=%s", err, stdout.String())
		}
		if envelope["ok"] != false {
			t.Fatalf("ok=%#v, want false; envelope=%#v", envelope["ok"], envelope)
		}
		data, _ := envelope["data"].(map[string]interface{})
		if msg, _ := data["message"].(string); !strings.Contains(msg, "download failed after 1 attachment(s) succeeded and 1 failed") {
			t.Fatalf("message=%q", msg)
		}
		downloaded, _ := data["downloaded"].([]interface{})
		failed, _ := data["failed"].([]interface{})
		if len(downloaded) != 1 || len(failed) != 1 {
			t.Fatalf("data=%#v", data)
		}
		downloadedItem, _ := downloaded[0].(map[string]interface{})
		failedItem, _ := failed[0].(map[string]interface{})
		if downloadedItem["file_token"] != "box_a" || failedItem["file_token"] != "box_b" {
			t.Fatalf("data=%#v", data)
		}
		if data["log_id"] != "202605270001" {
			t.Fatalf("data=%#v, want log_id", data)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "downloads", "a.txt")); err != nil {
			t.Fatalf("expected first file to remain: %v", err)
		}
	})

	t.Run("remove attachment", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_att",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "fld_att", "name": "附件", "type": "attachment"},
			},
		})
		removeStub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/remove_attachments",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"attachments": map[string]interface{}{
						"rec_x": map[string]interface{}{"fld_att": []interface{}{}},
					},
				},
			},
		}
		reg.Register(removeStub)

		if err := runShortcut(t, BaseRecordRemoveAttachment, []string{
			"+record-remove-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--field-id", "fld_att",
			"--file-token", "box_a",
			"--file-token", "box_b",
			"--yes",
		}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); strings.Contains(got, `"removed"`) || strings.Contains(got, `"updated"`) {
			t.Fatalf("stdout=%s", got)
		}
		body := string(removeStub.CapturedBody)
		if !strings.Contains(body, `"rec_x"`) ||
			!strings.Contains(body, `"fld_att"`) ||
			!strings.Contains(body, `"file_token":"box_a"`) ||
			!strings.Contains(body, `"file_token":"box_b"`) {
			t.Fatalf("remove body=%s", body)
		}
	})
}

func TestBaseViewExecuteReadCreateDeleteAndFilter(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "limit=1&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"views": []interface{}{map[string]interface{}{"id": "vew_1", "name": "Main", "type": "grid"}}, "total": 3},
			},
		})
		if err := runShortcut(t, BaseViewList, []string{"+view-list", "--base-token", "app_x", "--table-id", "tbl_x", "--offset", "0", "--limit", "1"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"total": 3`) || !strings.Contains(got, `"views"`) || !strings.Contains(got, `"name": "Main"`) || strings.Contains(got, `"items"`) || strings.Contains(got, `"offset"`) || strings.Contains(got, `"limit"`) || strings.Contains(got, `"count"`) || strings.Contains(got, `"view_name": "Main"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("get", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_1",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "vew_1", "name": "Main", "type": "grid"},
			},
		})
		if err := runShortcut(t, BaseViewGet, []string{"+view-get", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_1"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"view"`) || !strings.Contains(got, `"vew_1"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("create", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/views",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "vew_1", "name": "Main", "type": "grid"},
			},
		})
		if err := runShortcut(t, BaseViewCreate, []string{"+view-create", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"name":"Main","type":"grid"}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"views"`) || !strings.Contains(got, `"vew_1"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("delete", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "DELETE",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_1",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		})
		if err := runShortcut(t, BaseViewDelete, []string{"+view-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_1", "--yes"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"deleted": true`) || !strings.Contains(got, `"view_id": "vew_1"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("set-filter", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "PUT",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_1/filter",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"field_name": "Status"}}},
			},
		})
		if err := runShortcut(t, BaseViewSetFilter, []string{"+view-set-filter", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_1", "--json", `{"conditions":[{"field_name":"Status"}]}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"filter"`) || !strings.Contains(got, `"Status"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("get-visible-fields", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_1/visible_fields",
			Body: map[string]interface{}{
				"code": 0,
				"data": []interface{}{"fld_primary", "fld_status"},
			},
		})
		if err := runShortcut(t, BaseViewGetVisibleFields, []string{"+view-get-visible-fields", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_1"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"visible_fields"`) || !strings.Contains(got, `"fld_primary"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("set-visible-fields-array-invalid", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(
			t,
			BaseViewSetVisibleFields,
			[]string{"+view-set-visible-fields", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_1", "--json", `["fld_status"]`},
			factory,
			stdout,
		)
		if err == nil || !strings.Contains(err.Error(), "--json must be a JSON object") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("set-visible-fields-object", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		updateStub := &httpmock.Stub{
			Method: "PUT",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_1/visible_fields",
			Body: map[string]interface{}{
				"code": 0,
				"data": []interface{}{"fld_primary", "fld_status"},
			},
		}
		reg.Register(updateStub)
		if err := runShortcut(t, BaseViewSetVisibleFields, []string{"+view-set-visible-fields", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_1", "--json", `{"visible_fields":["fld_status"]}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		body := string(updateStub.CapturedBody)
		if !strings.Contains(body, `"visible_fields":["fld_status"]`) {
			t.Fatalf("request body=%s", body)
		}
		if strings.Contains(body, `{"visible_fields":{"visible_fields":`) {
			t.Fatalf("request body double wrapped: %s", body)
		}
	})
}

func TestBaseTableExecuteListFallbackShapes(t *testing.T) {
	t.Run("items-payload", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"items": []interface{}{map[string]interface{}{"id": "tbl_items", "name": "ItemsOnly"}}},
			},
		})
		if err := runShortcut(t, BaseTableList, []string{"+table-list", "--base-token", "app_x"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"ItemsOnly"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("single-object-payload", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"id": "tbl_single", "name": "SingleOnly"},
			},
		})
		if err := runShortcut(t, BaseTableList, []string{"+table-list", "--base-token", "app_x"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"SingleOnly"`) {
			t.Fatalf("stdout=%s", got)
		}
	})
}

func TestBaseRecordExecuteListWithViewPagination(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "view_id=vew_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"records": map[string]interface{}{
				"schema":     []interface{}{"Name", "Index"},
				"record_ids": []interface{}{"rec_last"},
				"rows":       []interface{}{[]interface{}{"Tail", 200}},
			}, "total": 201},
		},
	})
	if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--offset", "200", "--limit", "1", "--format", "json"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"rec_last"`) || !strings.Contains(got, `"total": 201`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseHistoryExecuteWithLinkFieldLimit(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "max_version=2",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": []interface{}{map[string]interface{}{"record_id": "rec_x", "field_name": "History"}}},
		},
	})
	if err := runShortcut(t, BaseRecordHistoryList, []string{"+record-history-list", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_x", "--page-size", "10", "--max-version", "2"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"field_name": "History"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseFieldExecuteSearchOptions(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_amount/options",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"options": []interface{}{map[string]interface{}{"id": "opt_1", "name": "已完成"}}, "total": 1},
		},
	})
	if err := runShortcut(t, BaseFieldSearchOptions, []string{"+field-search-options", "--base-token", "app_x", "--table-id", "tbl_x", "--field-id", "fld_amount", "--keyword", "已", "--limit", "10"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"options"`) || !strings.Contains(got, `"已完成"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseViewExecutePropertyGettersAndExtendedSetters(t *testing.T) {
	t.Run("get-group", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_x/group", Body: map[string]interface{}{"code": 0, "data": []interface{}{map[string]interface{}{"field": "fld_status", "desc": false}}}})
		if err := runShortcut(t, BaseViewGetGroup, []string{"+view-get-group", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"group"`) || !strings.Contains(got, `"fld_status"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("get-filter", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_x/filter", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"field_name": "Status"}}}}})
		if err := runShortcut(t, BaseViewGetFilter, []string{"+view-get-filter", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"filter"`) || !strings.Contains(got, `"Status"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("get-sort", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_x/sort", Body: map[string]interface{}{"code": 0, "data": []interface{}{map[string]interface{}{"field": "fld_priority", "desc": true}}}})
		if err := runShortcut(t, BaseViewGetSort, []string{"+view-get-sort", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"sort"`) || !strings.Contains(got, `"fld_priority"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("get-timebar", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_time/timebar", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"start_time": "fld_start", "end_time": "fld_end", "title": "fld_title"}}})
		if err := runShortcut(t, BaseViewGetTimebar, []string{"+view-get-timebar", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_time"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"timebar"`) || !strings.Contains(got, `"fld_start"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("set-timebar", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{Method: "PUT", URL: "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_time/timebar", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"start_time": "fld_start", "end_time": "fld_end", "title": "fld_title"}}})
		args := []string{"+view-set-timebar", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_time", "--json", `{"start_time":"fld_start","end_time":"fld_end","title":"fld_title"}`}
		if err := runShortcut(t, BaseViewSetTimebar, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"timebar"`) || !strings.Contains(got, `"fld_end"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("get-card", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_card/card", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"cover_field": "fld_cover"}}})
		if err := runShortcut(t, BaseViewGetCard, []string{"+view-get-card", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_card"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"card"`) || !strings.Contains(got, `"fld_cover"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("set-card", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{Method: "PUT", URL: "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_card/card", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"cover_field": "fld_cover"}}})
		if err := runShortcut(t, BaseViewSetCard, []string{"+view-set-card", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_card", "--json", `{"cover_field":"fld_cover"}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"card"`) || !strings.Contains(got, `"fld_cover"`) {
			t.Fatalf("stdout=%s", got)
		}
	})
}
