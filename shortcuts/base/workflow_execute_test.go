// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
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

func TestBaseWorkflowExecuteCreate(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/workflows",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"workflow_id": "wkf_new", "title": "My Workflow"},
		},
	}
	reg.Register(createStub)

	body := `{"title":"My Workflow","client_token":"create_1","steps":[{"type":"LarkMessageAction","data":{"receiver":[{"value_type":"user","value":{"id":"ou_x"}}],"content":[{"value_type":"text","value":"Reminder"}]}}]}`
	if err := runShortcut(t, BaseWorkflowCreate, []string{
		"+workflow-create", "--base-token", "app_x", "--json", body,
	}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	reg.Verify(t)
	assertCapturedJSONBody(t, createStub, body)
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

func workflowMessageBody(data string) string {
	return `{"title":"Reminder","steps":[{"type":"LarkMessageAction","data":` + data + `}]}`
}

func assertWorkflowValidation(
	t *testing.T,
	err error,
	wantSubtype errs.Subtype,
	wantMessage string,
) *errs.ValidationError {
	t.Helper()

	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryValidation || p.Subtype != wantSubtype {
		t.Fatalf("expected validation/%s problem, got %T %v", wantSubtype, err, err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--json" {
		t.Fatalf("expected validation error for --json, got %T %v", err, err)
	}
	if !strings.Contains(validationErr.Message, wantMessage) {
		t.Fatalf("message=%q, want %q", validationErr.Message, wantMessage)
	}
	return validationErr
}

func TestValidateWorkflowDefinition(t *testing.T) {
	for _, tt := range []struct {
		name        string
		body        string
		wantSubtype errs.Subtype
		wantMessage string
	}{
		{name: "message data is missing", body: `{"steps":[{"type":"LarkMessageAction"}]}`, wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "data"},
		{name: "message data is not an object", body: `{"steps":[{"type":"LarkMessageAction","data":[]}]}`, wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "data"},
		{name: "missing receiver", body: workflowMessageBody(`{"content":[1]}`), wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "receiver"},
		{name: "mis-cased receiver", body: workflowMessageBody(`{"Receiver":[1],"content":[1]}`), wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "receiver"},
		{name: "empty receiver", body: workflowMessageBody(`{"receiver":[],"content":[1]}`), wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "receiver"},
		{name: "receiver has wrong type", body: workflowMessageBody(`{"receiver":{},"content":[1]}`), wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "receiver"},
		{name: "missing content", body: workflowMessageBody(`{"receiver":[1]}`), wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "content"},
		{name: "mis-cased content", body: workflowMessageBody(`{"receiver":[1],"Content":[1]}`), wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "content"},
		{name: "empty content", body: workflowMessageBody(`{"receiver":[1],"content":[]}`), wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "content"},
		{name: "content has wrong type", body: workflowMessageBody(`{"receiver":[1],"content":{}}`), wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "content"},
		{name: "send to everyone has wrong type", body: workflowMessageBody(`{"receiver":[1],"content":[1],"send_to_everyone":"false"}`), wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "send_to_everyone"},
		{name: "send to everyone is null", body: workflowMessageBody(`{"receiver":[1],"content":[1],"send_to_everyone":null}`), wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "send_to_everyone"},
		{name: "button list has wrong type", body: workflowMessageBody(`{"receiver":[1],"content":[1],"btn_list":{}}`), wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "btn_list"},
		{name: "button list is null", body: workflowMessageBody(`{"receiver":[1],"content":[1],"btn_list":null}`), wantSubtype: errs.SubtypeInvalidArgument, wantMessage: "btn_list"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]interface{}
			if err := json.Unmarshal([]byte(tt.body), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			assertWorkflowValidation(t, validateWorkflowDefinition(body), tt.wantSubtype, tt.wantMessage)
		})
	}
}

func TestBaseWorkflowExecuteValidationWiring(t *testing.T) {
	for _, tt := range []struct {
		name        string
		shortcut    common.Shortcut
		args        []string
		wantSubtype errs.Subtype
		wantMessage string
		wantHint    string
	}{
		{
			name:        "create message action",
			shortcut:    BaseWorkflowCreate,
			args:        []string{"+workflow-create", "--base-token", "app_x", "--json", workflowMessageBody(`{"content":[1]}`)},
			wantSubtype: errs.SubtypeInvalidArgument,
			wantMessage: "receiver",
			wantHint:    "reported field",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			validationErr := assertWorkflowValidation(
				t,
				runShortcut(t, tt.shortcut, tt.args, factory, stdout),
				tt.wantSubtype,
				tt.wantMessage,
			)
			if !strings.Contains(validationErr.Hint, tt.wantHint) {
				t.Fatalf("hint=%q, want %q", validationErr.Hint, tt.wantHint)
			}
		})
	}
}

func assertCapturedJSONBody(t *testing.T, stub *httpmock.Stub, wantJSON string) {
	t.Helper()

	var want map[string]interface{}
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatalf("failed to decode expected request body: %v\nraw=%s", err, wantJSON)
	}
	if got := decodeCapturedJSONBody(t, stub); !reflect.DeepEqual(got, want) {
		t.Fatalf("request body=%#v, want %#v", got, want)
	}
}

func TestBaseWorkflowExecuteUpdatePreservesValidRequests(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
	}{
		{
			name: "valid message action",
			body: `{"title":"My Workflow","steps":[{"type":"LarkMessageAction","data":{"receiver":[{"value_type":"user","value":{"id":"ou_x"}}],"content":[{"value_type":"text","value":"Reminder"}],"send_to_everyone":false,"btn_list":[]}}]}`,
		},
		{name: "unknown step type", body: `{"title":"My Workflow","steps":[{"type":"FutureTrigger","data":{"opaque":true}}]}`},
		{name: "empty steps", body: `{"title":"My Workflow","steps":[]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			updateStub := &httpmock.Stub{
				Method: "PUT",
				URL:    "/open-apis/base/v3/bases/app_x/workflows/wkf_1",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"workflow_id": "wkf_1"},
				},
			}
			reg.Register(updateStub)

			if err := runShortcut(t, BaseWorkflowUpdate, []string{
				"+workflow-update", "--base-token", "app_x", "--workflow-id", "wkf_1", "--json", tt.body,
			}, factory, stdout); err != nil {
				t.Fatalf("err=%v", err)
			}
			reg.Verify(t)
			assertCapturedJSONBody(t, updateStub, tt.body)
		})
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
