// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// ---------------------------------------------------------------------------
// Validate tests
// ---------------------------------------------------------------------------

func TestBaseRoleCreateValidate(t *testing.T) {
	ctx := context.Background()

	t.Run("blank base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "", "json": `{"role_name":"R"}`}, nil, nil)
		if err := BaseRoleCreate.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--base-token must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("whitespace base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "   ", "json": `{"role_name":"R"}`}, nil, nil)
		if err := BaseRoleCreate.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--base-token must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "json": "{"}, nil, nil)
		if err := BaseRoleCreate.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--json must be valid JSON") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("valid", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "json": `{"role_name":"Reviewer","role_type":"custom_role"}`}, nil, nil)
		if err := BaseRoleCreate.Validate(ctx, rt); err != nil {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestBaseRoleDeleteValidate(t *testing.T) {
	ctx := context.Background()

	t.Run("blank base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "", "role-id": "rol_1"}, nil, nil)
		if err := BaseRoleDelete.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--base-token must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("blank role-id", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": ""}, nil, nil)
		if err := BaseRoleDelete.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--role-id must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("whitespace role-id", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "  "}, nil, nil)
		if err := BaseRoleDelete.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--role-id must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("valid", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1"}, nil, nil)
		if err := BaseRoleDelete.Validate(ctx, rt); err != nil {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestBaseRoleGetValidate(t *testing.T) {
	ctx := context.Background()

	t.Run("blank base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "", "role-id": "rol_1"}, nil, nil)
		if err := BaseRoleGet.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--base-token must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("blank role-id", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": ""}, nil, nil)
		if err := BaseRoleGet.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--role-id must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("valid", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1"}, nil, nil)
		if err := BaseRoleGet.Validate(ctx, rt); err != nil {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestBaseRoleListValidate(t *testing.T) {
	ctx := context.Background()

	t.Run("blank base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": ""}, nil, nil)
		if err := BaseRoleList.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--base-token must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("valid", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x"}, nil, nil)
		if err := BaseRoleList.Validate(ctx, rt); err != nil {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestBaseRoleUpdateValidate(t *testing.T) {
	ctx := context.Background()

	t.Run("blank base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "", "role-id": "rol_1", "json": `{"role_name":"X"}`}, nil, nil)
		if err := BaseRoleUpdate.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--base-token must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("blank role-id", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "", "json": `{"role_name":"X"}`}, nil, nil)
		if err := BaseRoleUpdate.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--role-id must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "json": "["}, nil, nil)
		if err := BaseRoleUpdate.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--json must be valid JSON") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("valid", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "json": `{"role_name":"New Name"}`}, nil, nil)
		if err := BaseRoleUpdate.Validate(ctx, rt); err != nil {
			t.Fatalf("err=%v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// DryRun tests
// ---------------------------------------------------------------------------

func TestBaseRoleCreateDryRun(t *testing.T) {
	rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "json": `{"role_name":"Reviewer"}`}, nil, nil)
	dr := BaseRoleCreate.DryRun(context.Background(), rt)
	if dr == nil {
		t.Fatal("DryRun returned nil")
	}
}

func TestBaseRoleDeleteDryRun(t *testing.T) {
	rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1"}, nil, nil)
	dr := BaseRoleDelete.DryRun(context.Background(), rt)
	if dr == nil {
		t.Fatal("DryRun returned nil")
	}
}

func TestBaseRoleGetDryRun(t *testing.T) {
	rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1"}, nil, nil)
	dr := BaseRoleGet.DryRun(context.Background(), rt)
	if dr == nil {
		t.Fatal("DryRun returned nil")
	}
}

func TestBaseRoleListDryRun(t *testing.T) {
	rt := newBaseTestRuntime(map[string]string{"base-token": "app_x"}, nil, nil)
	dr := BaseRoleList.DryRun(context.Background(), rt)
	if dr == nil {
		t.Fatal("DryRun returned nil")
	}
}

func TestBaseRoleUpdateDryRun(t *testing.T) {
	rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "json": `{"role_name":"New"}`}, nil, nil)
	dr := BaseRoleUpdate.DryRun(context.Background(), rt)
	if dr == nil {
		t.Fatal("DryRun returned nil")
	}
}

// ---------------------------------------------------------------------------
// Shortcut metadata tests
// ---------------------------------------------------------------------------

func TestBaseRoleShortcutMetadata(t *testing.T) {
	tests := []struct {
		name    string
		s       common.Shortcut
		command string
		risk    string
		scopes  []string
	}{
		{"create", BaseRoleCreate, "+role-create", "write", []string{"base:role:create"}},
		{"delete", BaseRoleDelete, "+role-delete", "high-risk-write", []string{"base:role:delete"}},
		{"get", BaseRoleGet, "+role-get", "read", []string{"base:role:read"}},
		{"list", BaseRoleList, "+role-list", "read", []string{"base:role:read"}},
		{"update", BaseRoleUpdate, "+role-update", "high-risk-write", []string{"base:role:update"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.s.Command != tt.command {
				t.Fatalf("command=%q want=%q", tt.s.Command, tt.command)
			}
			if tt.s.Risk != tt.risk {
				t.Fatalf("risk=%q want=%q", tt.s.Risk, tt.risk)
			}
			if tt.s.Service != "base" {
				t.Fatalf("service=%q", tt.s.Service)
			}
			if len(tt.s.Scopes) != len(tt.scopes) || tt.s.Scopes[0] != tt.scopes[0] {
				t.Fatalf("scopes=%v want=%v", tt.s.Scopes, tt.scopes)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Execute tests (with httpmock)
// ---------------------------------------------------------------------------

func TestBaseRoleCreateExecute(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/roles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"role_id": "rol_new", "role_name": "Reviewer"},
			},
		},
	})
	args := []string{"+role-create", "--base-token", "app_x", "--json", `{"role_name":"Reviewer","role_type":"custom_role"}`}
	if err := runShortcut(t, BaseRoleCreate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "rol_new") || !strings.Contains(got, "Reviewer") {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseRoleDeleteExecute(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/base/v3/bases/app_x/roles/rol_1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": nil,
		},
	})
	args := []string{"+role-delete", "--base-token", "app_x", "--role-id", "rol_1", "--yes"}
	if err := runShortcut(t, BaseRoleDelete, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "success") {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseRoleGetExecute(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/roles/rol_1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"role_id":   "rol_1",
					"role_name": "Admin",
					"role_type": "system_role",
				},
			},
		},
	})
	args := []string{"+role-get", "--base-token", "app_x", "--role-id", "rol_1"}
	if err := runShortcut(t, BaseRoleGet, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "rol_1") || !strings.Contains(got, "Admin") {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseRoleListExecute(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/roles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"code": 0,
				"data": []interface{}{
					map[string]interface{}{"role_id": "rol_1", "role_name": "Admin"},
					map[string]interface{}{"role_id": "rol_2", "role_name": "Viewer"},
				},
			},
		},
	})
	args := []string{"+role-list", "--base-token", "app_x"}
	if err := runShortcut(t, BaseRoleList, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "rol_1") || !strings.Contains(got, "rol_2") {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseRoleUpdateExecute(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/roles/rol_1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"role_id": "rol_1", "role_name": "Editor"},
			},
		},
	})
	args := []string{"+role-update", "--base-token", "app_x", "--role-id", "rol_1", "--json", `{"role_name":"Editor"}`, "--yes"}
	if err := runShortcut(t, BaseRoleUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "rol_1") || !strings.Contains(got, "Editor") {
		t.Fatalf("stdout=%s", got)
	}
}

// ---------------------------------------------------------------------------
// Execute error paths
// ---------------------------------------------------------------------------

func TestBaseRoleCreateExecuteAPIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/roles",
		Body: map[string]interface{}{
			"code": 190001,
			"msg":  "bad request",
		},
	})
	args := []string{"+role-create", "--base-token", "app_x", "--json", `{"role_name":"Bad"}`}
	if err := runShortcut(t, BaseRoleCreate, args, factory, stdout); err == nil || !strings.Contains(err.Error(), "190001") {
		t.Fatalf("err=%v", err)
	}
}

func TestBaseRoleListExecuteTransportError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/roles",
		Status: 500,
		Body:   "internal server error",
	})
	args := []string{"+role-list", "--base-token", "app_x"}
	if err := runShortcut(t, BaseRoleList, args, factory, stdout); err == nil {
		t.Fatalf("expected transport error")
	}
}

func TestBaseRoleListExecuteAPIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/roles",
		Body: map[string]interface{}{
			"code": 190002,
			"msg":  "not found",
		},
	})
	args := []string{"+role-list", "--base-token", "app_x"}
	if err := runShortcut(t, BaseRoleList, args, factory, stdout); err == nil || !strings.Contains(err.Error(), "190002") {
		t.Fatalf("err=%v", err)
	}
}

func TestBaseRoleDeleteExecuteAPIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/base/v3/bases/app_x/roles/rol_1",
		Body: map[string]interface{}{
			"code": 190003,
			"msg":  "forbidden",
		},
	})
	args := []string{"+role-delete", "--base-token", "app_x", "--role-id", "rol_1", "--yes"}
	if err := runShortcut(t, BaseRoleDelete, args, factory, stdout); err == nil || !strings.Contains(err.Error(), "190003") {
		t.Fatalf("err=%v", err)
	}
}

func TestBaseRoleUpdateExecuteAPIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/roles/rol_1",
		Body: map[string]interface{}{
			"code": 190004,
			"msg":  "invalid params",
		},
	})
	args := []string{"+role-update", "--base-token", "app_x", "--role-id", "rol_1", "--json", `{"role_name":"X"}`, "--yes"}
	if err := runShortcut(t, BaseRoleUpdate, args, factory, stdout); err == nil || !strings.Contains(err.Error(), "190004") {
		t.Fatalf("err=%v", err)
	}
}

func TestBaseRoleGetExecuteBusinessError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/roles/rol_bad",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"code":    100001,
				"message": "role not found",
			},
		},
	})
	args := []string{"+role-get", "--base-token", "app_x", "--role-id", "rol_bad"}
	if err := runShortcut(t, BaseRoleGet, args, factory, stdout); err == nil || !strings.Contains(err.Error(), "100001") || !strings.Contains(err.Error(), "role not found") {
		t.Fatalf("err=%v", err)
	}
}

// ---------------------------------------------------------------------------
// handleRoleResponse unit tests
// ---------------------------------------------------------------------------

func newRoleResponseRuntime(t *testing.T) *common.RuntimeContext {
	t.Helper()
	factory, _, _ := newExecuteFactory(t)
	cfg, _ := factory.Config()
	return &common.RuntimeContext{
		Cmd:     &cobra.Command{Use: "test"},
		Config:  cfg,
		Factory: factory,
	}
}

func TestHandleRoleResponse(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		if err := handleRoleResponse(rt, []byte("{bad"), "test"); err == nil || !strings.Contains(err.Error(), "failed to parse response") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("outer error code", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		if err := handleRoleResponse(rt, []byte(`{"code":999,"msg":"outer error"}`), "test"); err == nil || !strings.Contains(err.Error(), "999") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("outer error code with empty msg and data.error.message", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		body := `{"code":1,"data":{"error":{"hint":"failed to update","message":"the name already exists!","type":""}},"msg":""}`
		err := handleRoleResponse(rt, []byte(body), "test")
		if err == nil || !strings.Contains(err.Error(), "the name already exists!") {
			t.Fatalf("err=%v, want error containing 'the name already exists!'", err)
		}
	})

	t.Run("outer error code with empty msg and no data error", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		body := `{"code":2,"data":{},"msg":""}`
		err := handleRoleResponse(rt, []byte(body), "test")
		if err == nil || !strings.Contains(err.Error(), "[2]") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("null data", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		if err := handleRoleResponse(rt, []byte(`{"code":0,"msg":"ok","data":null}`), "test"); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("empty string data", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		if err := handleRoleResponse(rt, []byte(`{"code":0,"msg":"ok","data":""}`), "test"); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("empty data field", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		if err := handleRoleResponse(rt, []byte(`{"code":0,"msg":"ok"}`), "test"); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("double encoded json string", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		body := `{"code":0,"msg":"ok","data":"{\"role_id\":\"rol_1\"}"}`
		if err := handleRoleResponse(rt, []byte(body), "test"); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("non-parseable string data", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		body := `{"code":0,"msg":"ok","data":"just a plain string"}`
		if err := handleRoleResponse(rt, []byte(body), "test"); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("business code zero with inner data", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		body := `{"code":0,"msg":"ok","data":{"code":0,"data":{"role_id":"rol_1"}}}`
		if err := handleRoleResponse(rt, []byte(body), "test"); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("business code zero with double-encoded inner data", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		body := `{"code":0,"msg":"ok","data":{"code":0,"data":"{\"role_id\":\"rol_1\"}"}}`
		if err := handleRoleResponse(rt, []byte(body), "test"); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("business code zero without inner data", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		body := `{"code":0,"msg":"ok","data":{"code":0,"message":"ok"}}`
		if err := handleRoleResponse(rt, []byte(body), "test"); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("business code non-zero", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		body := `{"code":0,"msg":"ok","data":{"code":50001,"message":"permission denied"}}`
		if err := handleRoleResponse(rt, []byte(body), "test"); err == nil || !strings.Contains(err.Error(), "50001") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("data is array", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		body := `{"code":0,"msg":"ok","data":[{"role_id":"rol_1"},{"role_id":"rol_2"}]}`
		if err := handleRoleResponse(rt, []byte(body), "test"); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("data is object without code field", func(t *testing.T) {
		rt := newRoleResponseRuntime(t)
		body := `{"code":0,"msg":"ok","data":{"role_id":"rol_1","role_name":"Admin"}}`
		if err := handleRoleResponse(rt, []byte(body), "test"); err != nil {
			t.Fatalf("err=%v", err)
		}
	})
}
