// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestParseTaskGUIDs(t *testing.T) {
	got, err := parseTaskGUIDs(" task-guid-1, https://applink.larksuite.com/client/todo/detail?guid=task-guid-2 ")
	if err != nil {
		t.Fatalf("parseTaskGUIDs() error = %v", err)
	}
	want := []string{"task-guid-1", "task-guid-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseTaskGUIDs() = %v, want %v", got, want)
	}

	_, err = parseTaskGUIDs("task-guid-1,t12345")
	if err == nil {
		t.Fatal("parseTaskGUIDs() error = nil, want invalid display-number error")
	}
}

func TestTaskUpdateDryRunPreviewsEveryTaskID(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("task-id", "task-guid-1,https://applink.larksuite.com/client/todo/detail?guid=task-guid-2", "")
	cmd.Flags().String("summary", "updated", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("due", "", "")
	cmd.Flags().String("data", "", "")

	preview := UpdateTask.DryRun(context.Background(), &common.RuntimeContext{Cmd: cmd})
	payload, err := json.Marshal(preview)
	if err != nil {
		t.Fatalf("marshal dry-run preview: %v", err)
	}

	var got struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode dry-run preview: %v", err)
	}
	if len(got.API) != 2 {
		t.Fatalf("dry-run API calls = %d, want 2; payload: %s", len(got.API), payload)
	}

	wantURLs := []string{
		"/open-apis/task/v2/tasks/task-guid-1",
		"/open-apis/task/v2/tasks/task-guid-2",
	}
	for i, call := range got.API {
		if call.Method != "PATCH" {
			t.Errorf("api[%d].method = %q, want PATCH", i, call.Method)
		}
		if call.URL != wantURLs[i] {
			t.Errorf("api[%d].url = %q, want %q", i, call.URL, wantURLs[i])
		}
		if !reflect.DeepEqual(call.Params, map[string]interface{}{"user_id_type": "open_id"}) {
			t.Errorf("api[%d].params = %#v", i, call.Params)
		}
		if !reflect.DeepEqual(call.Body, got.API[0].Body) {
			t.Errorf("api[%d].body = %#v, want same body as first call %#v", i, call.Body, got.API[0].Body)
		}
	}
}

func TestTaskUpdateNormalizesAllIDsAndReturnsConfirmedFields(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	first := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/task/v2/tasks/task-guid-1",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"task": map[string]interface{}{
					"guid":        "task-guid-1",
					"url":         "https://example.com/task-guid-1",
					"summary":     "server summary one",
					"description": "server description one",
				},
			},
		},
	}
	second := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/task/v2/tasks/task-guid-2",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"task": map[string]interface{}{
					"guid":    "task-guid-2",
					"url":     "https://example.com/task-guid-2",
					"summary": "server summary two",
				},
			},
		},
	}
	reg.Register(first)
	reg.Register(second)

	err := runMountedTaskShortcut(t, UpdateTask, []string{
		"+update",
		"--task-id", "task-guid-1,https://applink.larksuite.com/client/todo/detail?guid=task-guid-2",
		"--summary", "requested summary",
		"--description", "requested description",
		"--format", "json",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("UpdateTask error = %v", err)
	}
	reg.Verify(t)

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data = %#v, want object", envelope["data"])
	}
	if got := stringSlice(data["updated_fields"]); !reflect.DeepEqual(got, []string{"summary", "description"}) {
		t.Fatalf("updated_fields = %v, want [summary description]", got)
	}

	tasks, ok := data["tasks"].([]interface{})
	if !ok || len(tasks) != 2 {
		t.Fatalf("tasks = %#v, want two tasks", data["tasks"])
	}
	firstTask := tasks[0].(map[string]interface{})
	if firstTask["guid"] != "task-guid-1" || firstTask["url"] != "https://example.com/task-guid-1" {
		t.Fatalf("first task identifiers = %#v", firstTask)
	}
	if got := firstTask["confirmed"]; !reflect.DeepEqual(got, map[string]interface{}{
		"summary": "server summary one", "description": "server description one",
	}) {
		t.Fatalf("first confirmed = %#v", got)
	}

	secondTask := tasks[1].(map[string]interface{})
	if got := secondTask["confirmed"]; !reflect.DeepEqual(got, map[string]interface{}{
		"summary": "server summary two",
	}) {
		t.Fatalf("second confirmed = %#v; omitted server fields must not be echoed from the request", got)
	}
}

func TestTaskUpdateValidatesEveryIDBeforeFirstWrite(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	err := runMountedTaskShortcut(t, UpdateTask, []string{
		"+update",
		"--task-id", "task-guid-1,t12345",
		"--summary", "must not be written",
		"--format", "json",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("UpdateTask error = nil, want invalid task ID error")
	}

	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("error = %T %v, want typed invalid-argument error", err, err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--task-id" {
		t.Fatalf("error param = %#v, want --task-id", validationErr)
	}
}

func stringSlice(value interface{}) []string {
	items, _ := value.([]interface{})
	result := make([]string, 0, len(items))
	for _, item := range items {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}
	return result
}
