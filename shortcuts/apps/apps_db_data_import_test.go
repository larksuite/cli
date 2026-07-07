// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

const dbDataImportURL = "/open-apis/spark/v1/apps/app_x/db/data_import"

// chdirTemp 切到临时工作目录（--file 走 cwd 内相对路径），返回该目录。
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

// TestAppsDBDataImport_RequiresAppID 验证空白 --app-id 报 --app-id 的 ValidationError。
func TestAppsDBDataImport_RequiresAppID(t *testing.T) {
	chdirTemp(t)
	_ = os.WriteFile("orders.csv", []byte("id\n1\n"), 0o600)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBDataImport,
		[]string{"+db-data-import", "--app-id", "  ", "--file", "orders.csv", "--yes", "--as", "user"}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != "--app-id" {
		t.Fatalf("Param = %q, want --app-id", ve.Param)
	}
}

// TestAppsDBDataImport_RejectsUnsupportedFormat 验证非 csv/json 文件（.txt）报不支持格式的校验错误。
func TestAppsDBDataImport_RejectsUnsupportedFormat(t *testing.T) {
	chdirTemp(t)
	_ = os.WriteFile("data.txt", []byte("x\n"), 0o600)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBDataImport,
		[]string{"+db-data-import", "--app-id", "app_x", "--file", "data.txt", "--yes", "--as", "user"}, factory, stdout)
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected unsupported-format validation, got %v", err)
	}
}

// TestAppsDBDataImport_RequiresConfirmation 验证缺 --yes 时报 requires confirmation 错误。
func TestAppsDBDataImport_RequiresConfirmation(t *testing.T) {
	chdirTemp(t)
	_ = os.WriteFile("orders.csv", []byte("id\n1\n"), 0o600)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBDataImport,
		[]string{"+db-data-import", "--app-id", "app_x", "--file", "orders.csv", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "requires confirmation") {
		t.Fatalf("expected confirmation_required, got %v", err)
	}
}

// TestAppsDBDataImport_RejectsOversizeFile 验证超过 1MB 上限的文件报 --file 的 ValidationError。
func TestAppsDBDataImport_RejectsOversizeFile(t *testing.T) {
	chdirTemp(t)
	// >1MB → size 校验
	big := append([]byte("id\n"), make([]byte, dbDataImportMaxBytes+1)...)
	_ = os.WriteFile("big.csv", big, 0o600)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBDataImport,
		[]string{"+db-data-import", "--app-id", "app_x", "--file", "big.csv", "--yes", "--as", "user"}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected 1MB limit error, got %T %v", err, err)
	}
	if ve.Param != "--file" {
		t.Fatalf("Param = %q, want --file", ve.Param)
	}
}

// dry-run：multipart 上传——file_name + file 走 body，env + table 走 query（table 缺省取文件名）。
// TestAppsDBDataImport_DryRunMultipartShape 验证 dry-run 的 multipart 形态：file_name+file 走 body、env+table 走 query 且不再发 format。
func TestAppsDBDataImport_DryRunMultipartShape(t *testing.T) {
	chdirTemp(t)
	_ = os.WriteFile("orders.csv", []byte("id\n1\n"), 0o600)
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBDataImport,
		[]string{"+db-data-import", "--app-id", "app_x", "--file", "orders.csv", "--environment", "dev", "--dry-run", "--yes", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	a := env.API[0]
	if a.Method != "POST" || a.URL != dbDataImportURL {
		t.Fatalf("dry-run = %s %s", a.Method, a.URL)
	}
	if a.Body["file_name"] != "orders.csv" || a.Body["file"] == nil {
		t.Fatalf("dry-run body should carry file_name + file: %v", a.Body)
	}
	if _, ok := a.Body["format"]; ok {
		t.Fatalf("format must no longer be sent: %v", a.Body)
	}
	if a.Params["env"] != "dev" || a.Params["table"] != "orders" {
		t.Fatalf("dry-run params (env+table) = %v", a.Params)
	}
}

// TestAppsDBDataImport_DryRunOmitsEnvWhenUnset 验证不传 --environment 时 dry-run 的 query
// 不带 env 键（交服务端按应用形态自动选分支），但仍携带 table。
func TestAppsDBDataImport_DryRunOmitsEnvWhenUnset(t *testing.T) {
	chdirTemp(t)
	_ = os.WriteFile("orders.csv", []byte("id\n1\n"), 0o600)
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBDataImport,
		[]string{"+db-data-import", "--app-id", "app_x", "--file", "orders.csv", "--dry-run", "--yes", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	p := env.API[0].Params
	if _, ok := p["env"]; ok {
		t.Fatalf("no --environment → env key must be omitted, got params=%v", p)
	}
	if p["table"] != "orders" {
		t.Fatalf("table should still default to file basename, got params=%v", p)
	}
}

// TestAppsDBDataImport_Success 验证成功导入后输出含 table、rows 与回显的 file 名。
func TestAppsDBDataImport_Success(t *testing.T) {
	chdirTemp(t)
	_ = os.WriteFile("orders.csv", []byte("id,name\n1,a\n2,b\n"), 0o600)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: dbDataImportURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"table": "orders", "rows": 2}},
	})
	if err := runAppsShortcut(t, AppsDBDataImport,
		[]string{"+db-data-import", "--app-id", "app_x", "--file", "orders.csv", "--table", "orders", "--yes", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"table": "orders"`) || !strings.Contains(got, `"rows": 2`) || !strings.Contains(got, `"file": "orders.csv"`) {
		t.Fatalf("output missing fields:\n%s", got)
	}
}

// TestAppsDBDataImport_TableDefaultsToFileBasename 验证未传 --table 时表名缺省取文件名去扩展名（customers.json→customers）。
func TestAppsDBDataImport_TableDefaultsToFileBasename(t *testing.T) {
	chdirTemp(t)
	_ = os.WriteFile("customers.json", []byte(`[{"id":1}]`), 0o600)
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBDataImport,
		[]string{"+db-data-import", "--app-id", "app_x", "--file", "customers.json", "--dry-run", "--yes", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	if env.API[0].Params["table"] != "customers" {
		t.Fatalf("expected table=customers (from file basename) in params, got %v", env.API[0].Params)
	}
}
