// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestBaseWorkflowExecuteGet(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/workflows/wkf_1",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"workflow_id": "wkf_1", "title": "My Workflow"},
		},
	})
	if err := runShortcut(t, BaseWorkflowGet, []string{"+workflow-get", "--base-token", "app_x", "--workflow-id", "wkf_1"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"wkf_1"`) || !strings.Contains(got, `"My Workflow"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseWorkflowExecuteGetWithUserIDType(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "user_id_type=open_id",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"workflow_id": "wkf_1", "creator": map[string]interface{}{"open_id": "ou_abc"}},
		},
	})
	if err := runShortcut(t, BaseWorkflowGet, []string{"+workflow-get", "--base-token", "app_x", "--workflow-id", "wkf_1", "--user-id-type", "open_id"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"ou_abc"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseWorkflowExecuteGetValidate(t *testing.T) {
	t.Run("missing base-token", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseWorkflowGet, []string{"+workflow-get", "--workflow-id", "wkf_1"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "base-token") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("missing workflow-id", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseWorkflowGet, []string{"+workflow-get", "--base-token", "app_x"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "workflow-id") {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestBaseWorkflowExecuteListCompact(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/workflows/list",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"workflow_id":  "wkf_1",
						"title":        "My Workflow",
						"status":       "enabled",
						"trigger_type": "TimerTrigger",
						"creator_id":   "ou_creator",
						"update_time":  123,
					},
				},
				"has_more": false,
			},
		},
	})
	if err := runShortcut(t, BaseWorkflowList, []string{"+workflow-list", "--base-token", "app_x", "--compact"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{`"workflow_id"`, `"wkf_1"`, `"title"`, `"status"`, `"trigger_type"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %s: %s", want, got)
		}
	}
	for _, notWant := range []string{`"creator_id"`, `"update_time"`} {
		if strings.Contains(got, notWant) {
			t.Fatalf("stdout should be compact, found %s: %s", notWant, got)
		}
	}
}

func TestBaseWorkflowExecuteCreate(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/workflows",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"workflow_id": "wkf_new", "title": "My Workflow"},
		},
	})
	if err := runShortcut(t, BaseWorkflowCreate, []string{"+workflow-create", "--base-token", "app_x", "--json", `{"title":"My Workflow","steps":[]}`}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"wkf_new"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseWorkflowExecuteCreateValidate(t *testing.T) {
	t.Run("missing base-token", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseWorkflowCreate, []string{"+workflow-create", "--json", `{"title":"x"}`}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "base-token") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("invalid json", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseWorkflowCreate, []string{"+workflow-create", "--base-token", "app_x", "--json", `not-json`}, factory, stdout)
		if err == nil {
			t.Fatalf("expected error for invalid json")
		}
	})
}

func TestBaseWorkflowExecuteDisable(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/workflows/wkf_1/disable",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"workflow_id": "wkf_1", "status": "disabled"},
		},
	})
	if err := runShortcut(t, BaseWorkflowDisable, []string{"+workflow-disable", "--base-token", "app_x", "--workflow-id", "wkf_1"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"disabled"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseWorkflowExecuteDisableValidate(t *testing.T) {
	t.Run("missing base-token", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseWorkflowDisable, []string{"+workflow-disable", "--workflow-id", "wkf_1"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "base-token") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("missing workflow-id", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseWorkflowDisable, []string{"+workflow-disable", "--base-token", "app_x"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "workflow-id") {
			t.Fatalf("err=%v", err)
		}
	})
}
