// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"strings"
	"testing"

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
