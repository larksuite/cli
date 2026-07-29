// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestCompleteTask(t *testing.T) {
	tests := []struct {
		name           string
		taskId         string
		isCompleted    bool
		formatFlag     string
		expectedOutput []string
	}{
		{
			name:        "task already completed",
			taskId:      "task-123",
			isCompleted: true,
			formatFlag:  "pretty",
			expectedOutput: []string{
				"✅ Task completed successfully!",
				"Task ID: task-123",
			},
		},
		{
			name:        "task not completed",
			taskId:      "task-456",
			isCompleted: false,
			formatFlag:  "pretty",
			expectedOutput: []string{
				"✅ Task completed successfully!",
				"Task ID: task-456",
			},
		},
		{
			name:        "task not completed json format",
			taskId:      "task-789",
			isCompleted: false,
			formatFlag:  "json",
			expectedOutput: []string{
				`"guid": "task-789"`,
				`"status": "done"`,
				`"completed_at": "1775174400000"`,
				`"already_completed": false`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := taskShortcutTestFactory(t)
			warmTenantToken(t, f, reg)

			completedAt := "0"
			if tt.isCompleted {
				completedAt = "1775174400000"
			}

			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/task/v2/tasks/" + tt.taskId,
				Body: map[string]interface{}{
					"code": 0, "msg": "success",
					"data": map[string]interface{}{
						"task": map[string]interface{}{
							"guid":         tt.taskId,
							"summary":      "Test Task " + tt.taskId,
							"completed_at": completedAt,
							"url":          "https://example.com/" + tt.taskId,
						},
					},
				},
			})

			if !tt.isCompleted {
				reg.Register(&httpmock.Stub{
					Method: "PATCH",
					URL:    "/open-apis/task/v2/tasks/" + tt.taskId,
					Body: map[string]interface{}{
						"code": 0, "msg": "success",
						"data": map[string]interface{}{
							"task": map[string]interface{}{
								"guid":         tt.taskId,
								"summary":      "Test Task " + tt.taskId,
								"completed_at": "1775174400000",
								"url":          "https://example.com/" + tt.taskId,
							},
						},
					},
				})
			}

			err := runMountedTaskShortcut(t, CompleteTask, []string{"+complete", "--task-id", tt.taskId, "--format", tt.formatFlag, "--as", "bot"}, f, stdout)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			out := stdout.String()
			outNorm := strings.ReplaceAll(out, `":"`, `": "`)

			for _, expected := range tt.expectedOutput {
				if !strings.Contains(outNorm, expected) && !strings.Contains(out, expected) {
					t.Errorf("output missing expected string (%s), got: %s", expected, out)
				}
			}
		})
	}
}

func TestTaskCompleteAcceptsTaskApplink(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	for _, method := range []string{"GET", "PATCH"} {
		reg.Register(&httpmock.Stub{
			Method: method,
			URL:    "/open-apis/task/v2/tasks/task-guid-applink",
			Body: map[string]interface{}{
				"code": 0, "msg": "success",
				"data": map[string]interface{}{
					"task": map[string]interface{}{
						"guid":         "task-guid-applink",
						"summary":      "Applink task",
						"completed_at": map[string]string{"GET": "0", "PATCH": "1775174400000"}[method],
						"url":          "https://example.com/task-guid-applink",
					},
				},
			},
		})
	}

	err := runMountedTaskShortcut(t, CompleteTask, []string{
		"+complete",
		"--task-id", "https://applink.larksuite.com/client/todo/detail?guid=task-guid-applink",
		"--format", "json",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("CompleteTask error = %v", err)
	}
	reg.Verify(t)
	if !strings.Contains(stdout.String(), `"guid": "task-guid-applink"`) {
		t.Fatalf("output = %s, want normalized task GUID", stdout.String())
	}
}

func TestTaskCompleteAlreadyCompletedReturnsServerState(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/task/v2/tasks/task-guid-done",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"task": map[string]interface{}{
					"guid":         "task-guid-done",
					"summary":      "Already done",
					"completed_at": "1775174400000",
					"url":          "https://example.com/task-guid-done",
				},
			},
		},
	})

	err := runMountedTaskShortcut(t, CompleteTask, []string{
		"+complete", "--task-id", "task-guid-done", "--format", "json", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("CompleteTask error = %v", err)
	}
	reg.Verify(t)

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data["status"] != "done" || data["completed_at"] != "1775174400000" || data["already_completed"] != true {
		t.Fatalf("completion state = %#v, want done/already_completed server state", data)
	}
}

func TestTaskCompleteRejectsDisplayNumberBeforeRead(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	err := runMountedTaskShortcut(t, CompleteTask, []string{
		"+complete", "--task-id", "t12345", "--format", "json", "--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("CompleteTask error = nil, want invalid task ID error")
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
