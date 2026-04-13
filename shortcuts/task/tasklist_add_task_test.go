// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAddTaskToTasklist_Success(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/task/v2/tasks/task-1/add_tasklist",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"task": map[string]interface{}{
					"guid": "task-1",
				},
			},
		},
	})

	s := AddTaskToTasklist
	s.AuthTypes = []string{"bot", "user"}

	args := []string{"+tasklist-task-add", "--tasklist-id", "tl-123", "--task-id", "task-1", "--section-guid", "sec-456", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"tasklist_guid":"tl-123"`) && !strings.Contains(out, `"tasklist_guid": "tl-123"`) {
		t.Errorf("expected tasklist_guid in output, got: %s", out)
	}
}
