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

func TestBuildSetAncestorBody(t *testing.T) {
	tests := []struct {
		name       string
		ancestorID string
		want       map[string]interface{}
	}{
		{name: "empty ancestor", ancestorID: "", want: map[string]interface{}{}},
		{name: "set ancestor", ancestorID: "guid_2", want: map[string]interface{}{"ancestor_guid": "guid_2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSetAncestorBody(tt.ancestorID)
			if len(got) != len(tt.want) {
				t.Fatalf("len(buildSetAncestorBody(%q)) = %d, want %d", tt.ancestorID, len(got), len(tt.want))
			}
			for k, want := range tt.want {
				if got[k] != want {
					t.Fatalf("buildSetAncestorBody(%q)[%q] = %#v, want %#v", tt.ancestorID, k, got[k], want)
				}
			}
		})
	}
}

func TestSetAncestorTask_DryRun(t *testing.T) {
	tests := []struct {
		name      string
		taskID    string
		ancestor  string
		wantParts []string
	}{
		{
			name:      "with ancestor",
			taskID:    "task-123",
			ancestor:  "task-456",
			wantParts: []string{"POST /open-apis/task/v2/tasks/task-123/set_ancestor_task", `"ancestor_guid":"task-456"`},
		},
		{
			name:      "clear ancestor",
			taskID:    "task-123",
			wantParts: []string{"POST /open-apis/task/v2/tasks/task-123/set_ancestor_task"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("task-id", "", "")
			cmd.Flags().String("ancestor-id", "", "")
			_ = cmd.Flags().Set("task-id", tt.taskID)
			if tt.ancestor != "" {
				_ = cmd.Flags().Set("ancestor-id", tt.ancestor)
			}
			runtime := common.TestNewRuntimeContextWithIdentity(cmd, taskTestConfig(t), "bot")
			out := SetAncestorTask.DryRun(nil, runtime).Format()
			for _, want := range tt.wantParts {
				if !strings.Contains(out, want) {
					t.Fatalf("dry run output missing %q: %s", want, out)
				}
			}
		})
	}
}

func TestSetAncestorTask_Execute(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		register  func(*httpmock.Registry)
		wantErr   bool
		wantParts []string
	}{
		{
			name: "json output with ancestor",
			args: []string{"+set-ancestor", "--task-id", "task-123", "--ancestor-id", "task-456", "--as", "bot", "--format", "json"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasks/task-123/set_ancestor_task",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{},
					},
				})
			},
			wantParts: []string{`"guid": "task-123"`},
		},
		{
			name: "pretty output clears ancestor",
			args: []string{"+set-ancestor", "--task-id", "task-123", "--as", "bot", "--format", "pretty"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasks/task-123/set_ancestor_task",
					Body: map[string]interface{}{
						"code": 0,
						"msg":  "success",
						"data": map[string]interface{}{},
					},
				})
			},
			wantParts: []string{"Ancestor cleared", "Task ID: task-123"},
		},
		{
			name: "api-level error (code!=0) returns error",
			args: []string{"+set-ancestor", "--task-id", "task-123", "--ancestor-id", "task-456", "--as", "bot", "--format", "pretty"},
			register: func(reg *httpmock.Registry) {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/task/v2/tasks/task-123/set_ancestor_task",
					Body: map[string]interface{}{
						"code": 10003,
						"msg":  "permission denied",
					},
				})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := taskShortcutTestFactory(t)
			warmTenantToken(t, f, reg)
			tt.register(reg)

			err := runMountedTaskShortcut(t, SetAncestorTask, tt.args, f, stdout)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if out := stdout.String(); out != "" {
					t.Fatalf("expected empty stdout on error, got: %s", out)
				}
				return
			}
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
