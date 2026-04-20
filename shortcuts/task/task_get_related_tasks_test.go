// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestTaskFollowedBy(t *testing.T) {
	tests := []struct {
		name       string
		task       map[string]interface{}
		userOpenID string
		want       bool
	}{
		{
			name: "contains follower",
			task: map[string]interface{}{
				"members": []interface{}{
					map[string]interface{}{"id": "ou_1", "role": "assignee"},
					map[string]interface{}{"id": "ou_2", "role": "follower"},
				},
			},
			userOpenID: "ou_2",
			want:       true,
		},
		{
			name: "missing follower",
			task: map[string]interface{}{
				"members": []interface{}{
					map[string]interface{}{"id": "ou_1", "role": "assignee"},
				},
			},
			userOpenID: "ou_3",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := taskFollowedBy(tt.task, tt.userOpenID)
			if got != tt.want {
				t.Fatalf("taskFollowedBy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetRelatedTasks_DryRun(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*cobra.Command)
		wantParts []string
	}{
		{
			name: "with page token and incomplete filter",
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("include-complete", "false")
				_ = cmd.Flags().Set("page-token", "pt_001")
			},
			wantParts: []string{"GET /open-apis/task/v2/task_v2/list_related_task", "page_token=pt_001", "completed=false"},
		},
		{
			name:      "default query params",
			setup:     func(cmd *cobra.Command) {},
			wantParts: []string{"GET /open-apis/task/v2/task_v2/list_related_task", "page_size=100", "user_id_type=open_id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().Bool("include-complete", true, "")
			cmd.Flags().String("page-token", "", "")
			tt.setup(cmd)
			runtime := common.TestNewRuntimeContextWithIdentity(cmd, taskTestConfig(t), "user")
			out := GetRelatedTasks.DryRun(nil, runtime).Format()
			for _, want := range tt.wantParts {
				if !strings.Contains(out, want) {
					t.Fatalf("dry run output missing %q: %s", want, out)
				}
			}
		})
	}
}

func TestGetRelatedTasks_Execute(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		register  func(*httpmock.Registry)
		wantParts []string
	}{
		{
			name: "json created by me",
			args: []string{"+get-related-tasks", "--as", "bot", "--format", "json", "--created-by-me"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "GET",
					URL:    "/open-apis/task/v2/task_v2/list_related_task",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{
							"has_more":   false,
							"page_token": "",
							"items": []interface{}{
								map[string]interface{}{
									"guid":          "task-123",
									"summary":       "Related Task",
									"description":   "desc",
									"status":        "done",
									"source":        1,
									"mode":          2,
									"subtask_count": 0,
									"tasklists":     []interface{}{},
									"url":           "https://example.com/task-123",
									"creator":       map[string]interface{}{"id": "ou_testuser", "type": "user"},
								},
							},
						},
					},
				})
			},
			wantParts: []string{`"guid": "task-123"`, `"summary": "Related Task"`},
		},
		{
			name: "pretty pagination followed by me",
			args: []string{"+get-related-tasks", "--as", "bot", "--format", "pretty", "--followed-by-me", "--page-limit", "2"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "GET",
					URL:    "/open-apis/task/v2/task_v2/list_related_task",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{
							"has_more":   true,
							"page_token": "pt_2",
							"items": []interface{}{
								map[string]interface{}{
									"guid":    "task-1",
									"summary": "Task One",
									"url":     "https://example.com/task-1",
									"creator": map[string]interface{}{"id": "ou_other", "type": "user"},
									"members": []interface{}{map[string]interface{}{"id": "ou_testuser", "role": "follower"}},
								},
							},
						},
					},
				})
				reg.Register(&httpmock.Stub{
					Method: "GET",
					URL:    "page_token=pt_2",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{
							"has_more":   false,
							"page_token": "",
							"items": []interface{}{
								map[string]interface{}{
									"guid":    "task-2",
									"summary": "Task Two",
									"url":     "https://example.com/task-2",
									"creator": map[string]interface{}{"id": "ou_other", "type": "user"},
									"members": []interface{}{map[string]interface{}{"id": "ou_testuser", "role": "follower"}},
								},
							},
						},
					},
				})
			},
			wantParts: []string{"Task One", "Task Two"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := taskShortcutTestFactory(t)
			warmTenantToken(t, f, reg)
			tt.register(reg)

			s := GetRelatedTasks
			s.AuthTypes = []string{"bot", "user"}
			err := runMountedTaskShortcut(t, s, tt.args, f, stdout)
			if err != nil {
				t.Fatalf("runMountedTaskShortcut() error = %v", err)
			}

			out := stdout.String()
			outNorm := strings.ReplaceAll(out, `":"`, `": "`)
			for _, want := range tt.wantParts {
				if !strings.Contains(out, want) && !strings.Contains(outNorm, want) {
					t.Fatalf("output missing %q: %s", want, out)
				}
			}
		})
	}
}
