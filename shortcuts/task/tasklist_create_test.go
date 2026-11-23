// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"bytes"
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

func TestCreateTasklist_UserMissingScopeProjectsInlineHint(t *testing.T) {
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
				URL:    "/open-apis/task/v2/tasklists",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "success",
					"data": map[string]interface{}{
						"tasklist": map[string]interface{}{"guid": "tl-new", "name": "My List"},
					},
				},
			})
			reg.Register(&httpmock.Stub{
				Method:     "POST",
				URL:        "/open-apis/task/v2/tasks",
				BodyFilter: func(body []byte) bool { return bytes.Contains(body, []byte("bad-task")) },
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

			s := CreateTasklist
			s.AuthTypes = []string{"bot", "user"}
			err := runMountedTaskShortcut(t, s, []string{
				"+tasklist-create", "--name", "My List", "--data", `[{"summary":"bad-task"}]`,
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

// TestCreateTasklist_PartialFailure exercises the batch sub-task path: the
// tasklist is created (code 0), then two sub-tasks are created concurrently —
// one succeeds, one fails with a typed API error. The command returns the typed
// partial-failure exit signal (*output.PartialFailureError, ExitAPI) via
// runtime.OutPartialFailure, and stdout carries both created_tasks (the
// success) and failed_tasks (the failure) so the partial result is inspectable.
// Sub-tasks are routed by summary via BodyFilter because both POST the same
// /tasks URL and run on separate goroutines.
func TestCreateTasklist_PartialFailure(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)
	createdTask := fullTaskOutputFixture()
	createdTask["guid"] = "task-ok"
	createdTask["url"] = "https://example.feishu.cn/task-ok"

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/task/v2/tasklists",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"tasklist": map[string]interface{}{
					"guid": "tl-new",
					"name": "My List",
					"url":  "https://example.feishu.cn/tl-new",
				},
			},
		},
	})

	// Succeeding sub-task (summary "ok-task").
	reg.Register(&httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/task/v2/tasks",
		BodyFilter: func(b []byte) bool { return bytes.Contains(b, []byte("ok-task")) },
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"task": createdTask},
		},
	})

	// Failing sub-task (summary "bad-task") → typed permission_denied.
	reg.Register(&httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/task/v2/tasks",
		BodyFilter: func(b []byte) bool { return bytes.Contains(b, []byte("bad-task")) },
		Body: map[string]interface{}{
			"code": ErrCodeTaskPermissionDenied, "msg": "no permission",
		},
	})

	s := CreateTasklist
	s.AuthTypes = []string{"bot", "user"}

	data := `[{"summary":"ok-task"},{"summary":"bad-task"}]`
	args := []string{"+tasklist-create", "--name", "My List", "--data", data, "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("err = %T, want *output.PartialFailureError; err = %v", err, err)
	}
	if pfErr.Code != output.ExitAPI {
		t.Errorf("exit code = %d, want %d (ExitAPI)", pfErr.Code, output.ExitAPI)
	}

	out := stdout.String()

	// The tasklist itself is created and stays in the payload.
	if !strings.Contains(out, "tl-new") {
		t.Errorf("expected created tasklist guid tl-new in output, got: %s", out)
	}
	// Success lands in created_tasks.
	if !strings.Contains(out, "task-ok") {
		t.Errorf("expected created sub-task task-ok in output, got: %s", out)
	}
	// Failure lands in failed_tasks (keyed by index + summary).
	if !strings.Contains(out, "bad-task") {
		t.Errorf("expected failed sub-task bad-task in output, got: %s", out)
	}
	if !strings.Contains(out, string(errs.SubtypePermissionDenied)) {
		t.Errorf("expected typed subtype %q in failed_tasks, got: %s", errs.SubtypePermissionDenied, out)
	}
	if !strings.Contains(out, `"code": 1470403`) && !strings.Contains(out, `"code":1470403`) {
		t.Errorf("expected task permission code in failed_tasks, got: %s", out)
	}
	if strings.Contains(out, "permission_error") {
		t.Errorf("legacy type \"permission_error\" leaked into output: %s", out)
	}
	var envelope map[string]interface{}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &envelope); decodeErr != nil {
		t.Fatalf("decode output: %v\n%s", decodeErr, out)
	}
	resultData, _ := envelope["data"].(map[string]interface{})
	createdTasks, _ := resultData["created_tasks"].([]interface{})
	if len(createdTasks) != 1 {
		t.Fatalf("created_tasks = %#v, want one item", resultData["created_tasks"])
	}
	assertStandardTaskFields(t, createdTasks[0].(map[string]interface{}))
}

func TestCreateTasklist_PartialFailurePrettyOutput(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/task/v2/tasklists",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"tasklist": map[string]interface{}{
					"guid": "tl-new",
					"name": "My List",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/task/v2/tasks",
		BodyFilter: func(b []byte) bool { return bytes.Contains(b, []byte("ok-task")) },
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"task": map[string]interface{}{"guid": "task-ok"},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/task/v2/tasks",
		BodyFilter: func(b []byte) bool { return bytes.Contains(b, []byte("bad-task")) },
		Body:       map[string]interface{}{"code": ErrCodeTaskPermissionDenied, "msg": "no permission"},
	})

	s := CreateTasklist
	s.AuthTypes = []string{"bot", "user"}

	err := runMountedTaskShortcut(t, s, []string{
		"+tasklist-create",
		"--name", "My List",
		"--data", `[{"summary":"ok-task"},{"summary":"bad-task"}]`,
		"--as", "bot",
		"--format", "pretty",
	}, f, stdout)

	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("err = %T, want *output.PartialFailureError; err = %v", err, err)
	}
	out := stdout.String()
	for _, want := range []string{
		"Tasklist created successfully",
		"Tasks created: 1/2",
		"Failed tasks:",
		"Index",
		"bad-task",
		"bot lacks permission",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("pretty output missing %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, `"ok":`) {
		t.Errorf("pretty partial failure should use text output, got JSON envelope:\n%s", out)
	}
}

// TestCreateTasklist_InvalidDataJSON covers the --data validation arm: a string
// that is not a JSON array must surface a typed *errs.ValidationError
// (invalid_argument, exit 2) after the tasklist create succeeds.
func TestCreateTasklist_InvalidDataJSON(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	// No POST /tasklists stub is registered on purpose: invalid --data must be
	// rejected before any remote write, leaving no orphan tasklist. If the
	// ordering regressed (create first), the POST would hit no stub and surface
	// as a non-validation transport error, failing the assertion below.
	s := CreateTasklist
	s.AuthTypes = []string{"bot", "user"}

	args := []string{"+tasklist-create", "--name", "My List", "--data", "{not-an-array", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T, want *errs.ValidationError; err = %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Errorf("exit code = %d, want %d (ExitValidation)", got, output.ExitValidation)
	}
}

// TestCreateTasklist_MalformedResponse covers the create-tasklist parse arm: a
// 200 with a non-JSON body must surface a typed
// *errs.InternalError(invalid_response) (exit 5) from the json.Unmarshal guard.
func TestCreateTasklist_MalformedResponse(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/task/v2/tasklists",
		RawBody: []byte("not json"),
	})

	s := CreateTasklist
	s.AuthTypes = []string{"bot", "user"}

	args := []string{"+tasklist-create", "--name", "My List", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("err = %T, want *errs.InternalError; err = %v", err, err)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("subtype = %q, want %q", ie.Subtype, errs.SubtypeInvalidResponse)
	}
	if got := output.ExitCodeOf(err); got != output.ExitInternal {
		t.Errorf("exit code = %d, want %d (ExitInternal)", got, output.ExitInternal)
	}
}
