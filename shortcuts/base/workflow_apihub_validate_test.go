// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

var apiHubFixtureParams = map[string]string{
	"Automation_Create_Task":         `[{"key":"rich_summary","value":[{"value_type":"text","value":"Follow up"}]},{"key":"endTime","value":[{"value_type":"date","value":"2026-09-30 18:00"}]}]`,
	"Automation_Complete_Task":       `[{"key":"task_id","value":[{"value_type":"ref","value":"$.create.taskId"}]}]`,
	"Automation_Uncomplete_Task":     `[{"key":"task_id","value":[{"value_type":"text","value":"task-id"}]}]`,
	"Automation_Delete_Task":         `[{"key":"task_id","value":[{"value_type":"text","value":"task-id"}]}]`,
	"Automation_Create_Schedule":     `[{"key":"startTime","value":[{"value_type":"date","value":"2026-09-30 17:00"}]},{"key":"endTime","value":[{"value_type":"date","value":"2026-09-30 18:00"}]},{"key":"calendarId","value":[{"value_type":"option","value":{"id":"calendar-id","name":"Work"}}]},{"key":"needNotification","value":[{"value_type":"boolean","value":false}]}]`,
	"Automation_Delete_Schedule":     `[{"key":"calendarId","value":[{"value_type":"ref","value":"$.create.calendarId"}]},{"key":"eventId","value":[{"value_type":"ref","value":"$.create.scheduleId"}]}]`,
	"Automation_Create_Group":        `[{"key":"name","value":[{"value_type":"text","value":"Project"}]},{"key":"userList","value":[{"value_type":"user","value":{"id":"ou_member","name":"Member"}}]},{"key":"owner","value":[{"value_type":"user","value":{"id":"ou_owner","name":"Owner"}}]}]`,
	"Automation_Add_Group_Member":    `[{"key":"group","value":[{"value_type":"group","value":{"id":"oc_group","name":"Project"}}]},{"key":"userList","value":[{"value_type":"user","value":{"id":"ou_member","name":"Member"}}]}]`,
	"Automation_Delete_Group_Member": `[{"key":"group","value":[{"value_type":"ref","value":"$.create.group"}]},{"key":"userList","value":[{"value_type":"user","value":{"id":"ou_member","name":"Member"}}]}]`,
}

func apiHubFixture(component, params string) string {
	return `{"title":"APIHub test","client_token":"test-request","steps":[{"id":"action","type":"APIHubAction","meta":{"component_id":"` + component + `"},"data":{"params":` + params + `,"future_field":true}}]}`
}

func TestWorkflowAPIHubCreateUpdatePreservesPayload(t *testing.T) {
	for component, params := range apiHubFixtureParams {
		for _, update := range []bool{false, true} {
			for _, dry := range []bool{false, true} {
				name := component
				if update {
					name += "/update"
				} else {
					name += "/create"
				}
				if dry {
					name += "/dry-run"
				}
				t.Run(name, func(t *testing.T) {
					factory, stdout, reg := newExecuteFactory(t)
					shortcut, method, url := BaseWorkflowCreate, "POST", "/open-apis/base/v3/bases/app_x/workflows"
					args := []string{"+workflow-create", "--base-token", "app_x"}
					if update {
						shortcut = BaseWorkflowUpdate
						method = "PUT"
						url += "/wkf_1"
						args = []string{"+workflow-update", "--base-token", "app_x", "--workflow-id", "wkf_1"}
					}
					body := apiHubFixture(component, params)
					args = append(args, "--json", body)
					if dry {
						args = append(args, "--dry-run", "--format", "pretty")
					}
					var stub *httpmock.Stub
					if !dry {
						stub = &httpmock.Stub{Method: method, URL: url, Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"workflow_id": "wkf_1"}}}
						reg.Register(stub)
					}
					if err := runShortcut(t, shortcut, args, factory, stdout); err != nil {
						t.Fatal(err)
					}
					if dry {
						if !strings.Contains(stdout.String(), method+" "+url) || !strings.Contains(stdout.String(), component) {
							t.Fatalf("dry-run=%s", stdout.String())
						}
						return
					}
					var got, want interface{}
					if err := json.Unmarshal(stub.CapturedBody, &got); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal([]byte(body), &want); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("wire payload changed: got=%s", stub.CapturedBody)
					}
				})
			}
		}
	}
}

func TestWorkflowAPIHubRejectsInvalidPayload(t *testing.T) {
	task := apiHubFixture("Automation_Create_Task", apiHubFixtureParams["Automation_Create_Task"])
	schedule := apiHubFixture("Automation_Create_Schedule", apiHubFixtureParams["Automation_Create_Schedule"])
	cases := []struct{ name, body string }{
		{"missing component", strings.Replace(task, `"component_id":"Automation_Create_Task"`, `"other":"x"`, 1)},
		{"unknown component", strings.Replace(task, "Automation_Create_Task", "Automation_Unknown", 1)},
		{"params object", apiHubFixture("Automation_Create_Task", `{}`)},
		{"missing title", apiHubFixture("Automation_Create_Task", `[]`)},
		{"wrong key case", strings.Replace(task, "endTime", "end_time", 1)},
		{"empty required value", apiHubFixture("Automation_Create_Task", `[{"key":"rich_summary","value":[]}]`)},
		{"null value", apiHubFixture("Automation_Create_Task", `[{"key":"rich_summary","value":[{"value_type":"text","value":null}]}]`)},
		{"duplicate key", apiHubFixture("Automation_Create_Task", `[{"key":"rich_summary","value":[{"value_type":"text","value":"A"}]},{"key":"rich_summary","value":[{"value_type":"text","value":"B"}]}]`)},
		{"calendar text", strings.Replace(schedule, `"value_type":"option"`, `"value_type":"text"`, 1)},
		{"calendar ref", strings.Replace(schedule, `"value_type":"option"`, `"value_type":"ref"`, 1)},
		{"string boolean", strings.Replace(schedule, `"value":false`, `"value":"false"`, 1)},
		{"combined type names", strings.Replace(task, `"value_type":"text"`, `"value_type":"text ref"`, 1)},
		{"conflicting type", strings.Replace(task, `"value_type":"text"`, `"value_type":"text","valueType":"user"`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, shortcut := range []common.Shortcut{BaseWorkflowCreate, BaseWorkflowUpdate} {
				factory, stdout, _ := newExecuteFactory(t)
				args := []string{shortcut.Command, "--base-token", "app_x", "--json", tc.body}
				if shortcut.Command == BaseWorkflowUpdate.Command {
					args = append(args, "--workflow-id", "wkf_1")
				}
				err := runShortcut(t, shortcut, args, factory, stdout)
				var cliErr *errs.ValidationError
				if !errors.As(err, &cliErr) {
					t.Fatalf("expected typed validation error, got %v", err)
				}
			}
		})
	}
}

func TestWorkflowAPIHubUnknownComponentListsSupportedIDs(t *testing.T) {
	step := map[string]interface{}{"meta": map[string]interface{}{"component_id": "Automation_Unknown"}}
	err := validateWorkflowAPIHubAction(0, step)
	var cliErr *errs.ValidationError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected typed validation error, got %v", err)
	}
	components := make([]string, 0, len(workflowAPIHubRules))
	for component := range workflowAPIHubRules {
		components = append(components, component)
	}
	slices.Sort(components)
	want := "supported components: " + strings.Join(components, ", ")
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error does not list supported components in stable order: %v", err)
	}
}

func TestWorkflowAPIHubCamelValueTypeCompatible(t *testing.T) {
	body := strings.ReplaceAll(apiHubFixture("Automation_Create_Task", apiHubFixtureParams["Automation_Create_Task"]), "value_type", "valueType")
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatal(err)
	}
	if err := validateWorkflowBodyForCLI(parsed); err != nil {
		t.Fatal(err)
	}
}
