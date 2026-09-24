// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func dashboardPushRuntime(overrides map[string]string, receivers []string) *common.RuntimeContext {
	flags := map[string]string{
		"base-token": "app_x", "dashboard-id": "dsh_sales", "title": "Sales dashboard",
		"send-at": "2026-09-15 09:00", "repeat": dashboardPushDaily,
		"client-token": "dashboard-sales-v1", "content-mode": dashboardPushImageMode,
	}
	for key, value := range overrides {
		flags[key] = value
	}
	return newBaseTestRuntimeWithArrays(flags, map[string][]string{"receiver": receivers}, nil, nil)
}

func dashboardPushArgs(extra ...string) []string {
	args := []string{
		"+dashboard-push-create", "--base-token", "app_x", "--dashboard-id", "dsh_sales",
		"--title", "Sales dashboard", "--send-at", "2026-09-15 09:00", "--repeat", "DAILY",
		"--receiver", "ou_user", "--client-token", "dashboard-sales-v1", "--content-mode", "image",
	}
	return append(args, extra...)
}

func TestDashboardPushCreateMetadata(t *testing.T) {
	if BaseDashboardPushCreate.Risk != "write" ||
		!reflect.DeepEqual(BaseDashboardPushCreate.AuthTypes, []string{"user", "bot"}) ||
		!reflect.DeepEqual(BaseDashboardPushCreate.Scopes, []string{"base:workflow:create", "base:workflow:update"}) {
		t.Fatalf("metadata = risk:%q auth:%v scopes:%v", BaseDashboardPushCreate.Risk, BaseDashboardPushCreate.AuthTypes, BaseDashboardPushCreate.Scopes)
	}
	flags := make(map[string]common.Flag, len(BaseDashboardPushCreate.Flags))
	for _, flag := range BaseDashboardPushCreate.Flags {
		flags[flag.Name] = flag
	}
	for _, required := range []string{"base-token", "dashboard-id", "title", "send-at", "repeat", "receiver", "client-token"} {
		if !flags[required].Required {
			t.Fatalf("flag --%s must be required", required)
		}
	}
	if flags["receiver"].Type != "string_array" || flags["content-mode"].Default != dashboardPushImageMode ||
		!reflect.DeepEqual(flags["content-mode"].Enum, []string{dashboardPushImageMode}) {
		t.Fatalf("flag metadata = %#v", flags)
	}
}

func TestBuildDashboardPushWorkflow(t *testing.T) {
	tests := []struct {
		name      string
		repeat    string
		receivers []string
		neverEnd  bool
		wantTypes []string
	}{
		{name: "one-time user", repeat: dashboardPushNoRepeat, receivers: []string{"ou_user"}, neverEnd: false, wantTypes: []string{"user"}},
		{name: "daily mixed receivers", repeat: dashboardPushDaily, receivers: []string{"ou_user", "oc_group"}, neverEnd: true, wantTypes: []string{"user", "group"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow, err := buildDashboardPushWorkflow(dashboardPushRuntime(map[string]string{"repeat": tt.repeat}, tt.receivers))
			if err != nil {
				t.Fatalf("build error: %v", err)
			}
			if len(workflow.Steps) != 2 || workflow.Steps[0].Type != "TimerTrigger" || workflow.Steps[1].Type != "LarkMessageAction" {
				t.Fatalf("steps = %#v", workflow.Steps)
			}
			if got := workflow.Steps[0].Data["is_never_end"]; got != tt.neverEnd {
				t.Fatalf("is_never_end = %#v, want %v", got, tt.neverEnd)
			}
			gotReceivers := workflow.Steps[1].Data["receiver"].([]dashboardPushValue)
			gotTypes := make([]string, 0, len(gotReceivers))
			for _, receiver := range gotReceivers {
				gotTypes = append(gotTypes, receiver.ValueType)
			}
			if !reflect.DeepEqual(gotTypes, tt.wantTypes) {
				t.Fatalf("receiver types = %v, want %v", gotTypes, tt.wantTypes)
			}
			content := workflow.Steps[1].Data["content"].([]dashboardPushValue)
			if len(content) != 1 || content[0].Value != "$.dashboard.image" || content[0].ExtraInfo["dashboard_name"] != "dsh_sales" {
				t.Fatalf("content = %#v", content)
			}
		})
	}
}

func TestBuildDashboardPushWorkflowRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
		receivers []string
		wantParam string
	}{
		{name: "invalid time", overrides: map[string]string{"send-at": "2026-9-15 9:00"}, receivers: []string{"ou_user"}, wantParam: "--send-at"},
		{name: "invalid frequency", overrides: map[string]string{"repeat": "WEEKLY"}, receivers: []string{"ou_user"}, wantParam: "--repeat"},
		{name: "missing receiver", receivers: nil, wantParam: "--receiver"},
		{name: "wrong receiver prefix", receivers: []string{"user_1"}, wantParam: "--receiver"},
		{name: "duplicate receiver", receivers: []string{"ou_user", "ou_user"}, wantParam: "--receiver"},
		{name: "AI mode", overrides: map[string]string{"content-mode": "image_and_ai_summary"}, receivers: []string{"ou_user"}, wantParam: "--content-mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildDashboardPushWorkflow(dashboardPushRuntime(tt.overrides, tt.receivers))
			if err == nil {
				t.Fatal("expected validation error")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryValidation || !strings.Contains(problem.Message, tt.wantParam) {
				t.Fatalf("error = %T %v", err, err)
			}
		})
	}
}

func TestDashboardPushCreateDryRunPlansCreateThenEnable(t *testing.T) {
	runtime := dashboardPushRuntime(nil, []string{"ou_user", "oc_group"})
	plan := BaseDashboardPushCreate.DryRun(context.Background(), runtime)
	var data map[string]interface{}
	raw, err := json.Marshal(plan)
	if err != nil || json.Unmarshal(raw, &data) != nil {
		t.Fatalf("marshal dry-run: %v, raw=%s", err, raw)
	}
	api := data["api"].([]interface{})
	if len(api) != 2 {
		t.Fatalf("api calls = %#v", api)
	}
	create := api[0].(map[string]interface{})
	enable := api[1].(map[string]interface{})
	if create["method"] != "POST" || enable["method"] != "PATCH" || !strings.HasSuffix(enable["url"].(string), "/%3Ccreated_workflow_id%3E/enable") {
		t.Fatalf("api plan = %#v", api)
	}
}

func TestDashboardPushCreateExecutesCreateThenEnable(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	create := &httpmock.Stub{Method: "POST", URL: "/open-apis/base/v3/bases/app_x/workflows", Body: map[string]interface{}{
		"code": 0, "data": map[string]interface{}{"workflow_id": "wkf_dashboard"},
	}}
	enable := &httpmock.Stub{Method: "PATCH", URL: "/open-apis/base/v3/bases/app_x/workflows/wkf_dashboard/enable", Body: map[string]interface{}{
		"code": 0, "data": map[string]interface{}{"workflow_id": "wkf_dashboard"},
	}}
	reg.Register(create)
	reg.Register(enable)
	if err := runShortcut(t, BaseDashboardPushCreate, dashboardPushArgs(), factory, stdout); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(create.CapturedBodies) != 1 || len(enable.CapturedBodies) != 1 {
		t.Fatalf("create calls=%d enable calls=%d", len(create.CapturedBodies), len(enable.CapturedBodies))
	}
	if got := stdout.String(); !strings.Contains(got, `"wkf_dashboard"`) || !strings.Contains(got, `"enabled": true`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestDashboardPushCreateFailureDoesNotEnable(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/base/v3/bases/app_x/workflows", Body: map[string]interface{}{
		"code": 1254003, "msg": "invalid dashboard",
	}})
	enable := &httpmock.Stub{Method: "PATCH", URL: "/enable", Optional: true, Body: map[string]interface{}{"code": 0}}
	reg.Register(enable)
	if err := runShortcut(t, BaseDashboardPushCreate, dashboardPushArgs(), factory, stdout); err == nil {
		t.Fatal("expected create error")
	}
	if len(enable.CapturedBodies) != 0 {
		t.Fatalf("enable calls=%d, want 0", len(enable.CapturedBodies))
	}
}

func TestDashboardPushEnableFailureClassification(t *testing.T) {
	tests := []struct {
		name       string
		enableStub *httpmock.Stub
		outcome    string
		status     string
		enabled    any
	}{
		{name: "authorization rejection", enableStub: &httpmock.Stub{Body: map[string]interface{}{"code": 99991679, "msg": "missing scope"}}, outcome: "rejected", status: "disabled", enabled: false},
		{name: "transport unknown", enableStub: &httpmock.Stub{Error: errors.New("connection reset")}, outcome: "unknown", status: "unknown", enabled: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/base/v3/bases/app_x/workflows", Body: map[string]interface{}{
				"code": 0, "data": map[string]interface{}{"workflow_id": "wkf_partial"},
			}})
			tt.enableStub.Method = "PATCH"
			tt.enableStub.URL = "/open-apis/base/v3/bases/app_x/workflows/wkf_partial/enable"
			reg.Register(tt.enableStub)
			err := runShortcut(t, BaseDashboardPushCreate, dashboardPushArgs(), factory, stdout)
			var partial *output.PartialFailureError
			if !errors.As(err, &partial) {
				t.Fatalf("error = %T %v, want partial failure", err, err)
			}
			var envelope struct {
				Data map[string]interface{} `json:"data"`
			}
			if unmarshalErr := json.Unmarshal(stdout.Bytes(), &envelope); unmarshalErr != nil {
				t.Fatalf("decode stdout: %v: %s", unmarshalErr, stdout.String())
			}
			if envelope.Data["workflow_id"] != "wkf_partial" || envelope.Data["created"] != true ||
				envelope.Data["enable_outcome"] != tt.outcome || envelope.Data["status"] != tt.status ||
				!reflect.DeepEqual(envelope.Data["enabled"], tt.enabled) {
				t.Fatalf("partial data = %#v", envelope.Data)
			}
			if _, ok := envelope.Data["error"].(map[string]interface{}); !ok {
				t.Fatalf("typed error missing: %#v", envelope.Data)
			}
		})
	}
}
