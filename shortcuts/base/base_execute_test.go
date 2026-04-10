// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newExecuteFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	config := &core.CliConfig{
		AppID:      "test-app-" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-"),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_testuser",
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
	shortcut.AuthTypes = []string{"bot"}
	parent := &cobra.Command{Use: "base"}
	shortcut.Mount(parent, factory)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	stdout.Reset()
	return parent.ExecuteContext(context.Background())
}

func TestBaseWorkspaceExecuteCreate(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_token": "app_x", "name": "Demo Base"},
		},
	})
	if err := runShortcut(t, BaseBaseCreate, []string{"+base-create", "--name", "Demo Base", "--folder-token", "fld_x", "--time-zone", "Asia/Shanghai"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"created": true`) || !strings.Contains(got, `"app_token": "app_x"`) {
		t.Fatalf("stdout=%s", got)
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
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_src/copy",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"base_token": "app_new", "name": "Copied Base", "url": "https://example.com/base/app_new"},
			},
		})
		args := []string{"+base-copy", "--base-token", "app_src", "--name", "Copied Base", "--folder-token", "fld_x", "--time-zone", "Asia/Shanghai", "--without-content"}
		if err := runShortcut(t, BaseBaseCopy, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"copied": true`) || !strings.Contains(got, `"app_new"`) {
			t.Fatalf("stdout=%s", got)
		}
	})
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
	if err := runShortcut(t, BaseFieldUpdate, []string{"+field-update", "--base-token", "app_x", "--table-id", "tbl_x", "--field-id", "fld_x", "--json", `{"name":"Amount","type":"number"}`}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"updated": true`) || !strings.Contains(got, `"fld_x"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseTableExecuteCreate(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"id": "tbl_new", "name": "Orders"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_new/fields",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"fields": []interface{}{map[string]interface{}{"id": "fld_primary", "name": "Primary"}}},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_new/fields/fld_primary",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"id": "fld_primary", "name": "OrderNo", "type": "text"},
		},
	})
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
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/rec_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"record_id": "rec_x", "fields": map[string]interface{}{"Name": "Alice"}},
		},
	})
	if err := runShortcut(t, BaseRecordUpsert, []string{"+record-upsert", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_x", "--json", `{"fields":{"Name":"Alice"}}`}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
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
		if err := runShortcut(t, BaseViewSetGroup, []string{"+view-set-group", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--json", `[{"field":"fld_status","desc":false}]`}, factory, stdout); err != nil {
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
		if err := runShortcut(t, BaseViewSetSort, []string{"+view-set-sort", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--json", `[{"field":"fld_amount","desc":true}]`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"sort"`) || !strings.Contains(got, `"fld_amount"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

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
		if got := stdout.String(); !strings.Contains(got, `"created": true`) || !strings.Contains(got, `"fld_new"`) {
			t.Fatalf("stdout=%s", got)
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
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables",
			Status: 404,
			Body:   "404 page not found",
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
	t.Run("list", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "limit=1&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"records": map[string]interface{}{
					"schema":     []interface{}{"Name", "Age"},
					"record_ids": []interface{}{"rec_1"},
					"rows":       []interface{}{[]interface{}{"Alice", 18}},
				}},
			},
		})
		if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "1"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"records"`) || !strings.Contains(got, `"Alice"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

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
		if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--limit", "1", "--field-id", "Name", "--field-id", "Age"}, factory, stdout); err != nil {
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
		if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "1", "--field-id", "A,B", "--field-id", "C"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"A,B"`) || !strings.Contains(got, `"rec_json_fields"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("list new shape", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "limit=1&offset=0",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"fields":         []interface{}{"Name", "Age"},
					"record_id_list": []interface{}{"rec_2"},
					"data":           []interface{}{[]interface{}{"Bob", 20}},
					"total":          1,
				},
			},
		})
		if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "1"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"Bob"`) || !strings.Contains(got, `"rec_2"`) {
			t.Fatalf("stdout=%s", got)
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
				"--json", `{"view_id":"vew_x","keyword":"Created","search_fields":["Title","fld_owner"],"select_fields":["Title","fld_owner"],"offset":0,"limit":2}`,
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
			!strings.Contains(body, `"offset":0`) ||
			!strings.Contains(body, `"limit":2`) {
			t.Fatalf("captured body=%s", body)
		}
	})

	t.Run("list legacy fields flag rejected", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--fields", "Name"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "unknown flag: --fields") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("list legacy fields flag rejected in dry-run", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--fields", "Name", "--dry-run"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "unknown flag: --fields") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("get", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/rec_1",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"records": map[string]interface{}{
					"schema":     []interface{}{"Name", "Age"},
					"record_ids": []interface{}{"rec_1"},
					"rows":       []interface{}{[]interface{}{"Alice", 18}},
				}},
			},
		})
		if err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_ids"`) || !strings.Contains(got, `"Name"`) || strings.Contains(got, `"raw"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("get passthrough fallback", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/rec_2",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"unexpected": "shape", "record_id": "rec_2"},
			},
		})
		if err := runShortcut(t, BaseRecordGet, []string{"+record-get", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_2"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"unexpected": "shape"`) || strings.Contains(got, `"raw"`) || strings.Contains(got, `"record":`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("create", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"record_id": "rec_new", "fields": map[string]interface{}{"Name": "Alice"}},
			},
		})
		if err := runShortcut(t, BaseRecordUpsert, []string{"+record-upsert", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"fields":{"Name":"Alice"}}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
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
					"fields":         []interface{}{"Name"},
					"record_id_list": []interface{}{"rec_1", "rec_2"},
					"data":           []interface{}{[]interface{}{"Alice"}, []interface{}{"Bob"}},
				},
			},
		})
		if err := runShortcut(t, BaseRecordBatchCreate, []string{"+record-batch-create", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"fields":["Name"],"rows":[["Alice"],["Bob"]]}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"rec_1"`) || !strings.Contains(got, `"Alice"`) {
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
					"has_more":       false,
					"record_id_list": []interface{}{"rec_1"},
					"update":         map[string]interface{}{"Status": "Done"},
				},
			},
		})
		if err := runShortcut(t, BaseRecordBatchUpdate, []string{"+record-batch-update", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"record_id_list":["rec_1"],"patch":{"Status":"Done"}}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"update"`) || !strings.Contains(got, `"Done"`) {
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
				"data": map[string]interface{}{
					"record_id_list": []interface{}{"rec_1"},
				},
			},
		}
		reg.Register(updateStub)
		if err := runShortcut(t, BaseRecordBatchUpdate, []string{"+record-batch-update", "--base-token", "app_x", "--table-id", "tbl_x", "--json", `{"record_id_list":["rec_1"],"patch":{"Name":"Alice","Status":"Done"}}`}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"record_id_list"`) || !strings.Contains(got, `"rec_1"`) {
			t.Fatalf("stdout=%s", got)
		}
		body := string(updateStub.CapturedBody)
		if !strings.Contains(body, `"record_id_list":["rec_1"]`) || !strings.Contains(body, `"patch":{"Name":"Alice","Status":"Done"}`) {
			t.Fatalf("request body=%s", body)
		}
	})

	t.Run("delete", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "DELETE",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/rec_1",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		})
		if err := runShortcut(t, BaseRecordDelete, []string{"+record-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--record-id", "rec_1", "--yes"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"deleted": true`) || !strings.Contains(got, `"record_id": "rec_1"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("upload attachment", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)

		tmpFile, err := os.CreateTemp(t.TempDir(), "base-attachment-*.txt")
		if err != nil {
			t.Fatalf("CreateTemp() err=%v", err)
		}
		if _, err := tmpFile.WriteString("hello attachment"); err != nil {
			t.Fatalf("WriteString() err=%v", err)
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
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/rec_x",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id": "rec_x",
					"fields": map[string]interface{}{
						"附件": []interface{}{
							map[string]interface{}{
								"file_token":                "existing_tok",
								"name":                      "existing.pdf",
								"size":                      2048,
								"image_width":               640,
								"image_height":              480,
								"deprecated_set_attachment": false,
							},
						},
					},
				},
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
		updateStub := &httpmock.Stub{
			Method: "PATCH",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/rec_x",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id": "rec_x",
					"fields": map[string]interface{}{
						"附件": []interface{}{
							map[string]interface{}{
								"file_token":                "existing_tok",
								"name":                      "existing.pdf",
								"size":                      2048,
								"image_width":               640,
								"image_height":              480,
								"deprecated_set_attachment": true,
							},
							map[string]interface{}{
								"file_token":                "file_tok_1",
								"name":                      "report.txt",
								"deprecated_set_attachment": true,
							},
						},
					},
				},
			},
		}
		reg.Register(updateStub)

		if err := runShortcut(t, BaseRecordUploadAttachment, []string{
			"+record-upload-attachment",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--record-id", "rec_x",
			"--field-id", "fld_att",
			"--file", "./" + filepath.Base(tmpFile.Name()),
			"--name", "report.txt",
		}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"updated": true`) || !strings.Contains(got, `"file_tok_1"`) || !strings.Contains(got, `"report.txt"`) {
			t.Fatalf("stdout=%s", got)
		}

		uploadBody := string(uploadStub.CapturedBody)
		if !strings.Contains(uploadBody, `name="parent_type"`) || !strings.Contains(uploadBody, "bitable_file") || !strings.Contains(uploadBody, `name="parent_node"`) || !strings.Contains(uploadBody, "app_x") {
			t.Fatalf("upload body=%s", uploadBody)
		}

		updateBody := string(updateStub.CapturedBody)
		if !strings.Contains(updateBody, `"附件"`) ||
			!strings.Contains(updateBody, `"file_token":"existing_tok"`) ||
			!strings.Contains(updateBody, `"name":"existing.pdf"`) ||
			!strings.Contains(updateBody, `"size":2048`) ||
			!strings.Contains(updateBody, `"image_width":640`) ||
			!strings.Contains(updateBody, `"image_height":480`) ||
			!strings.Contains(updateBody, `"deprecated_set_attachment":true`) ||
			!strings.Contains(updateBody, `"file_token":"file_tok_1"`) ||
			!strings.Contains(updateBody, `"name":"report.txt"`) {
			t.Fatalf("update body=%s", updateBody)
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
		if err == nil || !strings.Contains(err.Error(), "invalid JSON object") {
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
	if err := runShortcut(t, BaseRecordList, []string{"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--view-id", "vew_x", "--offset", "200", "--limit", "1"}, factory, stdout); err != nil {
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
