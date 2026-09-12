// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// ---------------------------------------------------------------------------
// Spec / validation
// ---------------------------------------------------------------------------

func TestRoleMemberReadSpecValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("blank base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "", "role-id": "rol_1", "member-ids": "ou_a"}, nil, nil)
		if err := BaseRoleMemberAdd.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--base-token must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("blank role-id", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "  ", "member-ids": "ou_a"}, nil, nil)
		if err := BaseRoleMemberRemove.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--role-id must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("blank member-ids", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "member-ids": "  ,  "}, nil, nil)
		if err := BaseRoleMemberAdd.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--member-ids") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("too many members", func(t *testing.T) {
		ids := make([]string, 101)
		for i := range ids {
			ids[i] = "ou_x"
		}
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "member-ids": strings.Join(ids, ",")}, nil, nil)
		if err := BaseRoleMemberAdd.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "at most 100") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("duplicate member", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "member-ids": "ou_a, ou_b, ou_a"}, nil, nil)
		if err := BaseRoleMemberAdd.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("invalid member-id-type", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "member-ids": "ou_a", "member-id-type": "email"}, nil, nil)
		if err := BaseRoleMemberAdd.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "invalid --member-id-type") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("prefix/type conflict", func(t *testing.T) {
		// ou_ prefix implies open_id; declaring chat_id must be rejected.
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "member-ids": "ou_a", "member-id-type": "chat_id"}, nil, nil)
		if err := BaseRoleMemberAdd.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "implies --member-id-type open_id") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("valid defaults to open_id", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "member-ids": "ou_a, ou_b"}, nil, nil)
		if err := BaseRoleMemberAdd.Validate(ctx, rt); err != nil {
			t.Fatalf("err=%v", err)
		}
		spec, err := readRoleMemberSpec(rt)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if spec.MemberIDType != "open_id" {
			t.Fatalf("memberIDType=%q want open_id", spec.MemberIDType)
		}
	})

	t.Run("chat id type accepts oc_ prefix", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "member-ids": "oc_a,oc_b", "member-id-type": "chat_id"}, nil, nil)
		if err := BaseRoleMemberAdd.Validate(ctx, rt); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("open_department_id uses dash prefix", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "member-ids": "od-abc", "member-id-type": "open_department_id"}, nil, nil)
		if err := BaseRoleMemberAdd.Validate(ctx, rt); err != nil {
			t.Fatalf("err=%v", err)
		}
		// od- prefix must not be misread as open_id (which uses ou_).
		spec, _ := readRoleMemberSpec(rt)
		if spec.MemberIDType != "open_department_id" {
			t.Fatalf("memberIDType=%q want open_department_id", spec.MemberIDType)
		}
	})
}

func TestRoleMemberListValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("blank role-id", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": ""}, nil, nil)
		if err := BaseRoleMemberList.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--role-id must not be blank") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("page size out of range", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1"}, nil, map[string]int{"limit": 101})
		if err := BaseRoleMemberList.Validate(ctx, rt); err == nil || !strings.Contains(err.Error(), "--limit must be in range 1-100") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("valid", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1"}, nil, map[string]int{"limit": 50})
		if err := BaseRoleMemberList.Validate(ctx, rt); err != nil {
			t.Fatalf("err=%v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// buildRoleMemberList: the silent-no-op guard. Every entry must carry BOTH the
// ID-namespace `type` and the `id`; the type is the ID namespace (open_id,
// chat_id, ...), not the collaborator category.
// ---------------------------------------------------------------------------

func TestBuildRoleMemberListCarriesTypeAndID(t *testing.T) {
	spec := roleMemberSpec{
		BaseToken:    "app_x",
		RoleID:       "rol_1",
		MemberIDs:    []string{"ou_a", "ou_b"},
		MemberIDType: "open_id",
	}
	members := buildRoleMemberList(spec)
	if len(members) != 2 {
		t.Fatalf("len=%d want 2", len(members))
	}
	for i, m := range members {
		if m["type"] != "open_id" {
			t.Fatalf("member[%d].type=%v want open_id (ID namespace, not category %q)", i, m["type"], "user")
		}
		if m["id"] == nil || m["id"] == "" {
			t.Fatalf("member[%d] missing id: %v", i, m)
		}
	}

	chatSpec := spec
	chatSpec.MemberIDs = []string{"oc_g"}
	chatSpec.MemberIDType = "chat_id"
	chatMembers := buildRoleMemberList(chatSpec)
	if chatMembers[0]["type"] != "chat_id" {
		t.Fatalf("chat member type=%v want chat_id", chatMembers[0]["type"])
	}
}

// ---------------------------------------------------------------------------
// DryRun
// ---------------------------------------------------------------------------

func TestRoleMemberListDryRun(t *testing.T) {
	rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "page-token": "tok_9"}, nil, map[string]int{"limit": 20})
	dr := BaseRoleMemberList.DryRun(context.Background(), rt)
	if dr == nil {
		t.Fatal("DryRun nil")
	}
	assertDryRunContains(t, dr,
		"GET /open-apis/bitable/v1/apps/app_x/roles/rol_1/members",
		"page_size=20", "page_token=tok_9")
}

func TestRoleMemberAddDryRun(t *testing.T) {
	rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "member-ids": "ou_a,ou_b"}, nil, nil)
	dr := BaseRoleMemberAdd.DryRun(context.Background(), rt)
	if dr == nil {
		t.Fatal("DryRun nil")
	}
	assertDryRunContains(t, dr,
		"POST /open-apis/bitable/v1/apps/app_x/roles/rol_1/members/batch_create",
		`"type":"open_id"`, `"id":"ou_a"`, `"id":"ou_b"`)
}

func TestRoleMemberRemoveDryRun(t *testing.T) {
	rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "role-id": "rol_1", "member-ids": "oc_g", "member-id-type": "chat_id"}, nil, nil)
	dr := BaseRoleMemberRemove.DryRun(context.Background(), rt)
	if dr == nil {
		t.Fatal("DryRun nil")
	}
	assertDryRunContains(t, dr,
		"POST /open-apis/bitable/v1/apps/app_x/roles/rol_1/members/batch_delete",
		`"type":"chat_id"`, `"id":"oc_g"`)
}

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func TestRoleMemberShortcutMetadata(t *testing.T) {
	tests := []struct {
		s       common.Shortcut
		command string
		risk    string
		scopes  []string
	}{
		{BaseRoleMemberList, "+role-member-list", "read", []string{"base:collaborator:read"}},
		{BaseRoleMemberAdd, "+role-member-add", "write", []string{"base:collaborator:create"}},
		{BaseRoleMemberRemove, "+role-member-remove", "high-risk-write", []string{"base:collaborator:delete"}},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			if tt.s.Command != tt.command {
				t.Fatalf("command=%q want=%q", tt.s.Command, tt.command)
			}
			if tt.s.Risk != tt.risk {
				t.Fatalf("risk=%q want=%q", tt.s.Risk, tt.risk)
			}
			if tt.s.Service != "base" {
				t.Fatalf("service=%q", tt.s.Service)
			}
			if len(tt.s.Scopes) != 1 || tt.s.Scopes[0] != tt.scopes[0] {
				t.Fatalf("scopes=%v want=%v", tt.s.Scopes, tt.scopes)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Execute (httpmock)
// ---------------------------------------------------------------------------

func TestRoleMemberAddExecute(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	var captured map[string]interface{}
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/bitable/v1/apps/app_x/roles/rol_1/members/batch_create",
		OnMatch: func(req *http.Request) {
			raw, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(raw, &captured)
		},
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	args := []string{"+role-member-add", "--base-token", "app_x", "--role-id", "rol_1", "--member-ids", "ou_a,ou_b"}
	if err := runShortcut(t, BaseRoleMemberAdd, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	list, _ := captured["member_list"].([]interface{})
	if len(list) != 2 {
		t.Fatalf("member_list len=%d want 2; body=%v", len(list), captured)
	}
	first, _ := list[0].(map[string]interface{})
	if first["type"] != "open_id" || first["id"] != "ou_a" {
		t.Fatalf("first member=%v want type=open_id id=ou_a", first)
	}
}

func TestRoleMemberListExecute(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/bitable/v1/apps/app_x/roles/rol_1/members",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"member_id": "ou_a", "open_id": "ou_a", "member_name": "Alice", "member_type": "user"},
				},
				"has_more":   false,
				"page_token": "",
				"total":      1,
			},
		},
	})
	args := []string{"+role-member-list", "--base-token", "app_x", "--role-id", "rol_1"}
	if err := runShortcut(t, BaseRoleMemberList, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "Alice") || !strings.Contains(got, "ou_a") {
		t.Fatalf("stdout=%s", got)
	}
}

func TestRoleMemberRemoveExecute(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/bitable/v1/apps/app_x/roles/rol_1/members/batch_delete",
		Body:   map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	args := []string{"+role-member-remove", "--base-token", "app_x", "--role-id", "rol_1", "--member-ids", "ou_a", "--yes"}
	if err := runShortcut(t, BaseRoleMemberRemove, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestRoleMemberAddExecuteAPIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/bitable/v1/apps/app_x/roles/rol_1/members/batch_create",
		Body: map[string]interface{}{
			"code": 1254048,
			"msg":  "invalid member",
		},
	})
	args := []string{"+role-member-add", "--base-token", "app_x", "--role-id", "rol_1", "--member-ids", "ou_bad"}
	assertProblemCode(t, runShortcut(t, BaseRoleMemberAdd, args, factory, stdout), 1254048, "add role members failed")
}
