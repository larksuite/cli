// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestBaseButtonRuleScopesDoNotRequireWorkflowAccess(t *testing.T) {
	tests := []struct {
		name       string
		shortcut   common.Shortcut
		wantScopes []string
	}{
		{name: "bind", shortcut: BaseButtonRuleBind, wantScopes: []string{"base:field:read", "base:field:update"}},
		{name: "get", shortcut: BaseButtonRuleGet, wantScopes: []string{"base:field:read"}},
		{name: "unbind", shortcut: BaseButtonRuleUnbind, wantScopes: []string{"base:field:read", "base:field:update"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !slices.Equal(tt.shortcut.Scopes, tt.wantScopes) {
				t.Fatalf("Scopes=%v want=%v", tt.shortcut.Scopes, tt.wantScopes)
			}
		})
	}
}

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

func TestBaseWorkflowExecuteCreatePreservesAIClassificationAgentData(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/workflows",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"workflow_id": "wkf_ai", "title": "Feedback classify"},
		},
	}
	reg.Register(stub)

	body := `{
		"title": "Feedback classify",
		"steps": [
			{"id": "step_trigger", "type": "AddRecordTrigger", "next": "step_classify", "data": {"table_name": "Feedback"}},
			{
				"id": "step_classify",
				"type": "AIClassificationBranch",
				"children": {"links": [
					{"kind": "case", "label": "branch_1", "desc": "Bug", "to": "step_bug"},
					{"kind": "case", "label": "branch_2", "desc": "Feature", "to": "step_feature"},
					{"kind": "case", "label": "default", "desc": "默认分支", "to": "step_other"}
				]},
				"data": {
					"classes": [
						{"name": "Bug", "desc": "Broken behavior"},
						{"name": "Feature", "desc": "New capability"}
					],
					"content": [
						{"value_type": "text", "value": "Classify feedback: "},
						{"value_type": "ref", "value": "$.step_trigger.fldFeedback"}
					],
					"classification_rule": "Use Other when unsure.",
					"no_match_action": "classifyToOther",
					"future_server_field": {"keep": true}
				}
			},
			{"id": "step_bug", "type": "SetRecordAction", "next": null, "data": {}},
			{"id": "step_feature", "type": "SetRecordAction", "next": null, "data": {}},
			{"id": "step_other", "type": "LarkMessageAction", "next": null, "data": {}}
		]
	}`
	if err := runShortcut(t, BaseWorkflowCreate, []string{"+workflow-create", "--base-token", "app_x", "--json", body}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := string(stub.CapturedBody); !strings.Contains(got, `"type":"AIClassificationBranch"`) || !strings.Contains(got, `"future_server_field":{"keep":true}`) {
		t.Fatalf("AI classification payload was not forwarded verbatim enough: %s", got)
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

func TestBaseWorkflowExecuteValidateAIClassificationAgentData(t *testing.T) {
	base := func(data string, children string) string {
		return `{
			"title": "Feedback classify",
			"steps": [
				{"id": "step_trigger", "type": "AddRecordTrigger", "next": "step_classify", "data": {}},
				{"id": "step_classify", "type": "AIClassificationBranch", "children": ` + children + `, "data": ` + data + `},
				{"id": "step_bug", "type": "SetRecordAction", "next": null, "data": {}},
				{"id": "step_feature", "type": "SetRecordAction", "next": null, "data": {}},
				{"id": "step_other", "type": "LarkMessageAction", "next": null, "data": {}}
			]
		}`
	}
	validChildren := `{"links":[{"kind":"case","label":"branch_1","desc":"Bug","to":"step_bug"},{"kind":"case","label":"branch_2","desc":"Feature","to":"step_feature"}]}`
	validData := `{
		"classes": [
			{"name": "Bug", "desc": "Broken behavior"},
			{"name": "Feature", "desc": "New capability"}
		],
		"content": [{"value_type": "text", "value": "Classify"}],
		"classification_rule": "Use the closest category.",
		"no_match_action": "fail"
	}`

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "draft data is not public protocol",
			body: base(`{"prompt":[{"value_type":"text","value":"Classify"}],"childBranchList":[{"name":"Bug"},{"name":"Feature"}],"no_match_action":"fail"}`, validChildren),
			want: "data.classes must be an array",
		},
		{
			name: "exclusive mode is not public input",
			body: base(strings.Replace(validData, `"classes": [`, `"mode": "Exclusive", "classes": [`, 1), validChildren),
			want: "data.mode is not supported; omit it because AI classification only supports Exclusive mode",
		},
		{
			name: "parallel mode is not public input",
			body: base(strings.Replace(validData, `"classes": [`, `"mode": "Parallel", "classes": [`, 1), validChildren),
			want: "data.mode is not supported; omit it because AI classification only supports Exclusive mode",
		},
		{
			name: "empty mode is not public input",
			body: base(strings.Replace(validData, `"classes": [`, `"mode": "", "classes": [`, 1), validChildren),
			want: "data.mode is not supported; omit it because AI classification only supports Exclusive mode",
		},
		{
			name: "non string mode is not public input",
			body: base(strings.Replace(validData, `"classes": [`, `"mode": true, "classes": [`, 1), validChildren),
			want: "data.mode is not supported; omit it because AI classification only supports Exclusive mode",
		},
		{
			name: "empty links",
			body: base(validData, `{"links":[]}`),
			want: "children.links must contain one non-empty case link for each class",
		},
		{
			name: "other default label",
			body: base(strings.Replace(validData, `"no_match_action": "fail"`, `"no_match_action": "classifyToOther"`, 1), `{"links":[{"kind":"case","label":"branch_1","desc":"Bug","to":"step_bug"},{"kind":"case","label":"branch_2","desc":"Feature","to":"step_feature"},{"kind":"case","label":"other","desc":"其他","to":"step_other"}]}`),
			want: "label must be default",
		},
		{
			name: "missing no match action still requires default link",
			body: base(strings.Replace(validData, `,
		"no_match_action": "fail"`, "", 1), validChildren),
			want: "children.links must contain exactly one default link when no_match_action is classifyToOther",
		},
		{
			name: "class link count mismatch",
			body: base(validData, `{"links":[{"kind":"case","label":"branch_1","desc":"Bug","to":"step_bug"}]}`),
			want: "children.links must contain one non-empty case link for each class",
		},
		{
			name: "class link desc mismatch",
			body: base(validData, `{"links":[{"kind":"case","label":"branch_1","desc":"Bug","to":"step_bug"},{"kind":"case","label":"branch_2","desc":"Mismatch","to":"step_feature"}]}`),
			want: "desc must equal --json steps data.classes[1].name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			err := runShortcut(t, BaseWorkflowCreate, []string{"+workflow-create", "--base-token", "app_x", "--json", tt.body}, factory, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v want substring %q", err, tt.want)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("err type=%T want *errs.ValidationError", err)
			}
		})
	}
}

func TestBaseWorkflowExecuteValidateAIClassificationOptionalModeAndNoMatchAction(t *testing.T) {
	base := func(data string, children string) string {
		return `{
			"title": "Feedback classify",
			"steps": [
				{"id": "step_trigger", "type": "AddRecordTrigger", "next": "step_classify", "data": {}},
				{"id": "step_classify", "type": "AIClassificationBranch", "children": ` + children + `, "data": ` + data + `},
				{"id": "step_bug", "type": "SetRecordAction", "next": null, "data": {}},
				{"id": "step_feature", "type": "SetRecordAction", "next": null, "data": {}},
				{"id": "step_other", "type": "LarkMessageAction", "next": null, "data": {}}
			]
		}`
	}
	data := `{
		"classes": [
			{"name": "Bug", "desc": "Broken behavior"},
			{"name": "Feature", "desc": "New capability"}
		],
		"content": [{"value_type": "text", "value": "Classify"}],
		"classification_rule": "Use the closest category."
	}`
	children := `{"links":[{"kind":"case","label":"branch_1","desc":"Bug","to":"step_bug"},{"kind":"case","label":"branch_2","desc":"Feature","to":"step_feature"},{"kind":"case","label":"default","desc":"默认分支","to":"step_other"}]}`

	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/workflows",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"workflow_id": "wkf_ai", "title": "Feedback classify"},
		},
	}
	reg.Register(stub)
	if err := runShortcut(t, BaseWorkflowCreate, []string{"+workflow-create", "--base-token", "app_x", "--json", base(data, children)}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := string(stub.CapturedBody)
	if strings.Contains(got, `"mode"`) || strings.Contains(got, `"no_match_action"`) {
		t.Fatalf("AI classification optional fields should not be injected by CLI: %s", got)
	}
}

func TestBaseWorkflowExecuteUpdateRejectsAIClassificationMode(t *testing.T) {
	base := func(mode string) string {
		return `{
			"title": "Feedback classify",
			"steps": [
				{"id": "step_trigger", "type": "AddRecordTrigger", "next": "step_classify", "data": {}},
				{
					"id": "step_classify",
					"type": "AIClassificationBranch",
					"children": {"links":[
						{"kind":"case","label":"branch_1","desc":"Bug","to":"step_bug"},
						{"kind":"case","label":"branch_2","desc":"Feature","to":"step_feature"}
					]},
					"data": {
						"mode": "` + mode + `",
						"classes": [
							{"name": "Bug", "desc": "Broken behavior"},
							{"name": "Feature", "desc": "New capability"}
						],
						"content": [{"value_type": "text", "value": "Classify"}],
						"classification_rule": "Use the closest category.",
						"no_match_action": "fail"
					}
				},
				{"id": "step_bug", "type": "SetRecordAction", "next": null, "data": {}},
				{"id": "step_feature", "type": "SetRecordAction", "next": null, "data": {}}
			]
		}`
	}

	for _, mode := range []string{"Exclusive", "Parallel"} {
		t.Run(mode, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			err := runShortcut(t, BaseWorkflowUpdate, []string{"+workflow-update", "--base-token", "app_x", "--workflow-id", "wkf_1", "--json", base(mode)}, factory, stdout)
			if err == nil || !strings.Contains(err.Error(), "data.mode is not supported; omit it because AI classification only supports Exclusive mode") {
				t.Fatalf("err=%v", err)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("err type=%T want *errs.ValidationError", err)
			}
		})
	}
}

func TestBaseWorkflowExecuteUpdatePreservesAIClassificationWithoutMode(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/base/v3/bases/app_x/workflows/wkf_1",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"workflow_id": "wkf_1", "title": "Feedback classify"},
		},
	}
	reg.Register(stub)

	body := `{
		"title": "Feedback classify",
		"status": "disabled",
		"steps": [
			{"id": "step_trigger", "type": "AddRecordTrigger", "next": "step_classify", "data": {}},
			{
				"id": "step_classify",
				"type": "AIClassificationBranch",
				"children": {"links":[
					{"kind":"case","label":"branch_1","desc":"Bug","to":"step_bug"},
					{"kind":"case","label":"branch_2","desc":"Feature","to":"step_feature"},
					{"kind":"case","label":"default","desc":"默认分支","to":"step_other"}
				]},
				"data": {
					"classes": [
						{"name": "Bug", "desc": "Broken behavior"},
						{"name": "Feature", "desc": "New capability"}
					],
					"content": [{"value_type": "text", "value": "Classify"}],
					"classification_rule": "Use the closest category."
				}
			},
			{"id": "step_bug", "type": "SetRecordAction", "next": null, "data": {}},
			{"id": "step_feature", "type": "SetRecordAction", "next": null, "data": {}},
			{"id": "step_other", "type": "LarkMessageAction", "next": null, "data": {}}
		]
	}`
	if err := runShortcut(t, BaseWorkflowUpdate, []string{"+workflow-update", "--base-token", "app_x", "--workflow-id", "wkf_1", "--json", body}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := string(stub.CapturedBody); strings.Contains(got, `"mode"`) || strings.Contains(got, `"no_match_action"`) || !strings.Contains(got, `"classes":[`) {
		t.Fatalf("AI classification payload should be forwarded without injected optional fields: %s", got)
	}
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

func TestBaseButtonRuleExecuteResolvesFieldReference(t *testing.T) {
	tests := []struct {
		name              string
		shortcut          common.Shortcut
		args              []string
		fieldRef          string
		canonicalFieldID  string
		fieldIdentityKey  string
		buttonRuleMethod  string
		wantWorkflowID    string
		wantWorkflowField bool
	}{
		{
			name: "bind by name", shortcut: BaseButtonRuleBind,
			args:     []string{"+button-rule-bind", "--base-token", "app_x", "--table-id", "tbl_1", "--field-id", "按钮", "--workflow-id", "wkf_1"},
			fieldRef: "按钮", canonicalFieldID: "fld_bind", fieldIdentityKey: "id",
			buttonRuleMethod: "PUT", wantWorkflowID: "wkf_1", wantWorkflowField: true,
		},
		{
			name: "get by name", shortcut: BaseButtonRuleGet,
			args:     []string{"+button-rule-get", "--base-token", "app_x", "--table-id", "tbl_1", "--field-id", "按钮"},
			fieldRef: "按钮", canonicalFieldID: "fld_get", fieldIdentityKey: "id",
			buttonRuleMethod: "GET",
		},
		{
			name: "unbind by name with field_id compatibility", shortcut: BaseButtonRuleUnbind,
			args:     []string{"+button-rule-unbind", "--base-token", "app_x", "--table-id", "tbl_1", "--field-id", "按钮"},
			fieldRef: "按钮", canonicalFieldID: "fld_unbind", fieldIdentityKey: "field_id",
			buttonRuleMethod: "PUT", wantWorkflowID: "", wantWorkflowField: true,
		},
		{
			name: "ID input is still resolved", shortcut: BaseButtonRuleGet,
			args:     []string{"+button-rule-get", "--base-token", "app_x", "--table-id", "tbl_1", "--field-id", "fld_input"},
			fieldRef: "fld_input", canonicalFieldID: "fld_canonical", fieldIdentityKey: "id",
			buttonRuleMethod: "GET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			callOrder := 0
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    baseV3Path("bases", "app_x", "tables", "tbl_1", "fields", tt.fieldRef),
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{tt.fieldIdentityKey: tt.canonicalFieldID, "name": tt.fieldRef},
				},
				OnMatch: func(_ *http.Request) {
					if callOrder != 0 {
						t.Fatalf("field resolution call order=%d want=0", callOrder)
					}
					callOrder++
				},
			})
			buttonRuleStub := &httpmock.Stub{
				Method: tt.buttonRuleMethod,
				URL:    baseV3Path("bases", "app_x", "tables", "tbl_1", "fields", tt.canonicalFieldID, "button_rule"),
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"table_id": "tbl_1", "field_id": tt.canonicalFieldID, "workflow_id": tt.wantWorkflowID, "bound": tt.wantWorkflowID != ""},
				},
				OnMatch: func(_ *http.Request) {
					if callOrder != 1 {
						t.Fatalf("ButtonRule call order=%d want=1", callOrder)
					}
					callOrder++
				},
			}
			reg.Register(buttonRuleStub)

			if err := runShortcut(t, tt.shortcut, tt.args, factory, stdout); err != nil {
				t.Fatalf("err=%v", err)
			}
			if callOrder != 2 {
				t.Fatalf("call order count=%d want=2", callOrder)
			}
			if got := stdout.String(); !strings.Contains(got, `"field_id": "`+tt.canonicalFieldID+`"`) {
				t.Fatalf("stdout=%s", got)
			}
			if tt.wantWorkflowField {
				var body map[string]interface{}
				if err := json.Unmarshal(buttonRuleStub.CapturedBody, &body); err != nil {
					t.Fatalf("decode ButtonRule body: %v", err)
				}
				if got, ok := body["workflow_id"].(string); !ok || got != tt.wantWorkflowID {
					t.Fatalf("workflow_id=%#v want=%q body=%s", body["workflow_id"], tt.wantWorkflowID, buttonRuleStub.CapturedBody)
				}
			}
		})
	}
}

func TestBaseButtonRuleFieldResolutionFailureStopsBeforeButtonRule(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	buttonRuleCalls := 0
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    baseV3Path("bases", "app_x", "tables", "tbl_1", "fields", "missing"),
		Body: map[string]interface{}{
			"code": 1254045,
			"msg":  "field not found",
			"data": map[string]interface{}{"error": map[string]interface{}{"logid": "log_field_resolution"}},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/button_rule", Optional: true,
		OnMatch: func(_ *http.Request) { buttonRuleCalls++ },
	})

	err := runShortcut(t, BaseButtonRuleBind, []string{"+button-rule-bind", "--base-token", "app_x", "--table-id", "tbl_1", "--field-id", "missing", "--workflow-id", "wkf_1"}, factory, stdout)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryAPI || problem.Code != 1254045 || problem.LogID != "log_field_resolution" {
		t.Fatalf("expected preserved typed field resolution error, got %T %#v", err, problem)
	}
	if buttonRuleCalls != 0 {
		t.Fatalf("ButtonRule calls=%d want=0", buttonRuleCalls)
	}
}

func TestBaseButtonRuleFieldResolutionRejectsMissingCanonicalID(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	buttonRuleCalls := 0
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    baseV3Path("bases", "app_x", "tables", "tbl_1", "fields", "按钮"),
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"name": "按钮"}},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/button_rule", Optional: true,
		OnMatch: func(_ *http.Request) { buttonRuleCalls++ },
	})

	err := runShortcut(t, BaseButtonRuleGet, []string{"+button-rule-get", "--base-token", "app_x", "--table-id", "tbl_1", "--field-id", "按钮"}, factory, stdout)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("expected typed invalid-response error, got %T %#v", err, problem)
	}
	if buttonRuleCalls != 0 {
		t.Fatalf("ButtonRule calls=%d want=0", buttonRuleCalls)
	}
}

func TestBaseButtonRuleAPIFailurePreservesTypedCause(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	cause := errors.New("button rule transport failed")
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    baseV3Path("bases", "app_x", "tables", "tbl_1", "fields", "按钮"),
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"id": "fld_1"}},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    baseV3Path("bases", "app_x", "tables", "tbl_1", "fields", "fld_1", "button_rule"),
		Error:  cause,
	})

	err := runShortcut(t, BaseButtonRuleGet, []string{"+button-rule-get", "--base-token", "app_x", "--table-id", "tbl_1", "--field-id", "按钮"}, factory, stdout)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || !errors.Is(err, cause) {
		t.Fatalf("expected typed network error preserving cause, got %T %#v", err, problem)
	}
}

func TestBaseButtonRuleValidateRejectsInternalWorkflowID(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcut(t, BaseButtonRuleBind, []string{"+button-rule-bind", "--base-token", "app_x", "--table-id", "tbl_1", "--field-id", "fld_1", "--workflow-id", "123456"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "public wkf workflow ID") {
		t.Fatalf("err=%v", err)
	}
}
