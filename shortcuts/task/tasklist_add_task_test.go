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
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
)

func TestAddTaskToTasklist_UserMissingScopeProjectsInlineHint(t *testing.T) {
	tests := []struct {
		name string
		plan *surface.Plan
	}{
		{name: "visible"},
		{
			name: "concealed",
			plan: surface.NewPlan(map[surface.CommandID]surface.CommandState{
				surface.CommandAuthLogin: surface.CommandConcealed,
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := taskShortcutTestFactory(t)
			f.Recovery = recovery.NewProjector(func() *surface.Plan { return tt.plan })
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/task/v2/tasks/task-scope/add_tasklist",
				Body: map[string]interface{}{
					"code": 99991679,
					"msg":  "missing scope",
					"error": map[string]interface{}{
						"permission_violations": []interface{}{
							map[string]interface{}{"subject": "task:task:write"},
						},
					},
				},
			})

			s := AddTaskToTasklist
			s.AuthTypes = []string{"bot", "user"}
			err := runMountedTaskShortcut(t, s, []string{
				"+tasklist-task-add", "--tasklist-id", "tl-123", "--task-id", "task-scope",
				"--as", "user", "--format", "json",
			}, f, stdout)
			var partial *output.PartialFailureError
			if !errors.As(err, &partial) {
				t.Fatalf("err = %T, want *output.PartialFailureError: %v", err, err)
			}

			var envelope struct {
				OK   bool `json:"ok"`
				Data struct {
					Failed []map[string]interface{} `json:"failed_tasks"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("unmarshal stdout: %v\n%s", err, stdout.String())
			}
			if envelope.OK || len(envelope.Data.Failed) != 1 {
				t.Fatalf("envelope = %#v, want ok:false with one failure", envelope)
			}
			failed := envelope.Data.Failed[0]
			if got, want := failed["type"], string(errs.SubtypeMissingScope); got != want {
				t.Errorf("failed type = %v, want %v", got, want)
			}
			if got, want := failed["hint"], recovery.UserAuthorization("task:task:write").Render(tt.plan); got != want {
				t.Errorf("failed hint = %q, want %q", got, want)
			}
			if tt.plan != nil && strings.Contains(failed["hint"].(string), "auth login") {
				t.Errorf("concealed hint leaked auth command: %q", failed["hint"])
			}
		})
	}
}

func TestAddTaskToTasklist_Success(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)
	task := fullTaskOutputFixture()

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/task/v2/tasks/task-1/add_tasklist",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"task": task},
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
	var envelope map[string]interface{}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &envelope); decodeErr != nil {
		t.Fatalf("decode output: %v\n%s", decodeErr, out)
	}
	data, _ := envelope["data"].(map[string]interface{})
	successful, _ := data["successful_tasks"].([]interface{})
	if len(successful) != 1 {
		t.Fatalf("successful_tasks = %#v, want one item", data["successful_tasks"])
	}
	assertStandardTaskFields(t, successful[0].(map[string]interface{}))
}

// TestAddTaskToTasklist_PartialFailure exercises the batch path: some tasks
// succeed, others fail with typed API errors. Successful and failed tasks both
// land in stdout as an ok:false envelope, and the command returns the typed
// partial-failure exit signal (exit 1) via runtime.OutPartialFailure. The
// failed_tasks[].type carries the typed subtype (e.g. "permission_denied",
// "not_found") read off errs.ProblemOf.
func TestAddTaskToTasklist_PartialFailure(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/task/v2/tasks/task-ok/add_tasklist",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"task": map[string]interface{}{
					"guid": "task-ok",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/task/v2/tasks/task-perm/add_tasklist",
		Body: map[string]interface{}{
			"code": ErrCodeTaskPermissionDenied, "msg": "no permission",
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/task/v2/tasks/task-missing/add_tasklist",
		Body: map[string]interface{}{
			"code": ErrCodeTaskNotFound, "msg": "task not found",
		},
	})

	s := AddTaskToTasklist
	s.AuthTypes = []string{"bot", "user"}

	args := []string{"+tasklist-task-add", "--tasklist-id", "tl-123", "--task-id", "task-ok,task-perm,task-missing", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)
	// Partial failure now surfaces as a non-zero exit (ok:false), not nil.
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected *output.PartialFailureError on partial failure, got %T: %v", err, err)
	}
	if pfErr.Code != output.ExitAPI {
		t.Errorf("exit code = %d, want %d (ExitAPI)", pfErr.Code, output.ExitAPI)
	}

	out := stdout.String()

	// Successful task is in stdout.
	if !strings.Contains(out, "task-ok") {
		t.Errorf("expected successful task-ok in output, got: %s", out)
	}

	// Failed tasks carry the typed subtype, not the legacy Detail.Type.
	if !strings.Contains(out, string(errs.SubtypePermissionDenied)) {
		t.Errorf("expected typed subtype %q in failed_tasks, got: %s", errs.SubtypePermissionDenied, out)
	}
	if !strings.Contains(out, string(errs.SubtypeNotFound)) {
		t.Errorf("expected typed subtype %q in failed_tasks, got: %s", errs.SubtypeNotFound, out)
	}

	// The legacy shapes must not leak.
	if strings.Contains(out, "permission_error") {
		t.Errorf("legacy type \"permission_error\" leaked into output: %s", out)
	}
}
