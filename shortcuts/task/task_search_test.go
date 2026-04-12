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

func TestBuildTaskSearchBody(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*cobra.Command)
		wantErr bool
		check   func(*testing.T, map[string]interface{})
	}{
		{
			name: "query creator due and page token",
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("query", "release")
				_ = cmd.Flags().Set("creator", "ou_a,ou_b")
				_ = cmd.Flags().Set("completed", "true")
				_ = cmd.Flags().Set("due", "-1d,+1d")
				_ = cmd.Flags().Set("page-token", "pt_123")
			},
			check: func(t *testing.T, body map[string]interface{}) {
				filter := body["filter"].(map[string]interface{})
				dueTime := filter["due_time"].(map[string]interface{})
				if body["query"] != "release" || body["page_token"] != "pt_123" {
					t.Fatalf("unexpected body: %#v", body)
				}
				if len(filter["creator_ids"].([]string)) != 2 || filter["is_completed"] != true {
					t.Fatalf("unexpected filter: %#v", filter)
				}
				startTime, _ := dueTime["start_time"].(string)
				endTime, _ := dueTime["end_time"].(string)
				if startTime == "" || endTime == "" || !strings.Contains(startTime, "T") || !strings.Contains(endTime, "T") {
					t.Fatalf("unexpected due_time: %#v", dueTime)
				}
			},
		},
		{
			name:    "requires query or filter",
			setup:   func(cmd *cobra.Command) {},
			wantErr: true,
		},
		{
			name: "assignee follower and incomplete",
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("assignee", "ou_assignee")
				_ = cmd.Flags().Set("follower", "ou_follower")
				_ = cmd.Flags().Set("completed", "false")
			},
			check: func(t *testing.T, body map[string]interface{}) {
				filter := body["filter"].(map[string]interface{})
				if filter["assignee_ids"].([]string)[0] != "ou_assignee" || filter["follower_ids"].([]string)[0] != "ou_follower" {
					t.Fatalf("unexpected filter: %#v", filter)
				}
				if filter["is_completed"] != false {
					t.Fatalf("expected is_completed false, got %#v", filter["is_completed"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("query", "", "")
			cmd.Flags().String("creator", "", "")
			cmd.Flags().String("assignee", "", "")
			cmd.Flags().String("follower", "", "")
			cmd.Flags().Bool("completed", false, "")
			cmd.Flags().String("due", "", "")
			cmd.Flags().String("page-token", "", "")
			tt.setup(cmd)

			runtime := common.TestNewRuntimeContextWithIdentity(cmd, taskTestConfig(t), "user")
			body, err := buildTaskSearchBody(runtime)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildTaskSearchBody() error = %v", err)
			}
			tt.check(t, body)
		})
	}
}

func TestSearchTask_DryRun(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*cobra.Command)
		wantParts []string
	}{
		{
			name: "valid dry run",
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("query", "demo")
				_ = cmd.Flags().Set("page-token", "pt_demo")
			},
			wantParts: []string{"POST /open-apis/task/v2/tasks/search", `"query":"demo"`},
		},
		{
			name: "dry run error on invalid due",
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("due", "bad-time")
			},
			wantParts: []string{"error:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("query", "", "")
			cmd.Flags().String("creator", "", "")
			cmd.Flags().String("assignee", "", "")
			cmd.Flags().String("follower", "", "")
			cmd.Flags().Bool("completed", false, "")
			cmd.Flags().String("due", "", "")
			cmd.Flags().String("page-token", "", "")
			tt.setup(cmd)

			runtime := common.TestNewRuntimeContextWithIdentity(cmd, taskTestConfig(t), "user")
			if !strings.Contains(tt.name, "error") {
				if err := SearchTask.Validate(nil, runtime); err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
			}
			out := SearchTask.DryRun(nil, runtime).Format()
			for _, want := range tt.wantParts {
				if !strings.Contains(out, want) {
					t.Fatalf("dry run output missing %q: %s", want, out)
				}
			}
		})
	}
}

func TestSearchTask_Execute(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		register  func(*httpmock.Registry)
		wantParts []string
	}{
		{
			name: "json success",
			args: []string{"+search", "--query", "release", "--as", "bot", "--format", "json"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasks/search",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{
							"has_more":   false,
							"page_token": "",
							"items": []interface{}{
								map[string]interface{}{"id": "task-123", "meta_data": map[string]interface{}{"app_link": "https://example.com/task-123"}},
							},
						},
					},
				})
				reg.Register(&httpmock.Stub{
					Method: "GET",
					URL:    "/open-apis/task/v2/tasks/task-123",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{
							"task": map[string]interface{}{"guid": "task-123", "summary": "Search Result", "created_at": "1775174400000", "url": "https://example.com/task-123"},
						},
					},
				})
			},
			wantParts: []string{`"guid": "task-123"`, `"summary": "Search Result"`},
		},
		{
			name: "fallback to app link",
			args: []string{"+search", "--query", "fallback", "--as", "bot", "--format", "json"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasks/search",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{
							"has_more":   false,
							"page_token": "",
							"items": []interface{}{
								map[string]interface{}{"id": "task-999", "meta_data": map[string]interface{}{"app_link": "https://example.com/task-999&suite_entity_num=t999"}},
							},
						},
					},
				})
				reg.Register(&httpmock.Stub{
					Method: "GET",
					URL:    "/open-apis/task/v2/tasks/task-999",
					Body:   map[string]interface{}{"code": 99991663, "msg": "not found"},
				})
			},
			wantParts: []string{`"guid": "task-999"`, `"url": "https://example.com/task-999"`},
		},
		{
			name: "empty pretty with pagination",
			args: []string{"+search", "--query", "none", "--as", "bot", "--format", "pretty", "--page-limit", "2"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasks/search",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{"has_more": true, "page_token": "pt_2", "items": []interface{}{}},
					},
				})
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasks/search",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{"has_more": false, "page_token": "", "items": []interface{}{}},
					},
				})
			},
			wantParts: []string{"No tasks found."},
		},
		{
			name: "pretty with next page token",
			args: []string{"+search", "--query", "pretty", "--as", "bot", "--format", "pretty", "--page-limit", "1"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasks/search",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{
							"has_more":   true,
							"page_token": "pt_next",
							"items": []interface{}{
								map[string]interface{}{"id": "task-321", "meta_data": map[string]interface{}{"app_link": "https://example.com/task-321"}},
							},
						},
					},
				})
				reg.Register(&httpmock.Stub{
					Method: "GET",
					URL:    "/open-apis/task/v2/tasks/task-321",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{
							"task": map[string]interface{}{"guid": "task-321", "summary": "Pretty Search", "url": "https://example.com/task-321"},
						},
					},
				})
			},
			wantParts: []string{"Pretty Search", "Next page token: pt_next"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := taskShortcutTestFactory(t)
			warmTenantToken(t, f, reg)
			tt.register(reg)

			s := SearchTask
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
