// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestProjectTaskFields(t *testing.T) {
	task := map[string]interface{}{
		"summary": "Ship compact fields",
		"members": []interface{}{map[string]interface{}{
			"id": "ou_owner", "name": "Owner", "type": "user", "role": "assignee",
		}},
		"start":  map[string]interface{}{"timestamp": "1000", "is_all_day": false},
		"due":    map[string]interface{}{"timestamp": "2000", "is_all_day": false},
		"status": "todo",
	}
	out := map[string]interface{}{"guid": "task-1"}

	projectTaskFields(out, task, standardTaskOutputFields...)

	for _, field := range standardTaskOutputFields {
		key := string(field)
		if !reflect.DeepEqual(out[key], task[key]) {
			t.Fatalf("%s = %#v, want %#v", key, out[key], task[key])
		}
	}
	if out["guid"] != "task-1" {
		t.Fatalf("existing guid changed: %#v", out)
	}
}

func TestProjectTaskFieldsOmitsAbsentFields(t *testing.T) {
	out := map[string]interface{}{}
	projectTaskFields(out, map[string]interface{}{"summary": "Only title"}, standardTaskOutputFields...)

	if out["summary"] != "Only title" {
		t.Fatalf("summary = %#v, want Only title", out["summary"])
	}
	for _, key := range []string{"members", "start", "due", "status"} {
		if _, ok := out[key]; ok {
			t.Fatalf("%s unexpectedly projected: %#v", key, out)
		}
	}
}

func TestTaskRootOutputFields(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		method   string
		url      string
		args     []string
	}{
		{
			name: "create", shortcut: CreateTask, method: http.MethodPost,
			url:  "/open-apis/task/v2/tasks",
			args: []string{"+create", "--summary", "Requested title", "--as", "bot", "--format", "json"},
		},
		{
			name: "reopen", shortcut: ReopenTask, method: http.MethodPatch,
			url:  "/open-apis/task/v2/tasks/task-1",
			args: []string{"+reopen", "--task-id", "task-1", "--as", "bot", "--format", "json"},
		},
		{
			name: "assign", shortcut: AssignTask, method: http.MethodPost,
			url:  "/open-apis/task/v2/tasks/task-1/add_members",
			args: []string{"+assign", "--task-id", "task-1", "--add", "ou_owner", "--as", "bot", "--format", "json"},
		},
		{
			name: "followers", shortcut: FollowersTask, method: http.MethodPost,
			url:  "/open-apis/task/v2/tasks/task-1/add_members",
			args: []string{"+followers", "--task-id", "task-1", "--add", "ou_follower", "--as", "bot", "--format", "json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := taskShortcutTestFactory(t)
			warmTenantToken(t, f, reg)
			reg.Register(&httpmock.Stub{
				Method: tt.method,
				URL:    tt.url,
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "success",
					"data": map[string]interface{}{"task": fullTaskOutputFixture()},
				},
			})

			shortcut := tt.shortcut
			shortcut.AuthTypes = []string{"bot", "user"}
			if err := runMountedTaskShortcut(t, shortcut, tt.args, f, stdout); err != nil {
				t.Fatalf("runMountedTaskShortcut() error = %v", err)
			}

			assertStandardTaskFields(t, decodeTaskOutputData(t, stdout.Bytes()))
		})
	}
}

func fullTaskOutputFixture() map[string]interface{} {
	return map[string]interface{}{
		"guid":    "task-1",
		"url":     "https://example.com/task-1&suite_entity_num=t1",
		"summary": "Server title",
		"members": []interface{}{
			map[string]interface{}{"id": "ou_owner", "name": "Owner", "type": "user", "role": "assignee"},
		},
		"start":  map[string]interface{}{"timestamp": "1000", "is_all_day": false},
		"due":    map[string]interface{}{"timestamp": "2000", "is_all_day": false},
		"status": "todo",
	}
}

func decodeTaskOutputData(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, raw)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data = %#v, want object", envelope["data"])
	}
	return data
}

func assertStandardTaskFields(t *testing.T, data map[string]interface{}) {
	t.Helper()
	for _, key := range []string{"summary", "members", "start", "due", "status"} {
		if _, ok := data[key]; !ok {
			t.Errorf("data missing %q: %#v", key, data)
		}
	}
}
