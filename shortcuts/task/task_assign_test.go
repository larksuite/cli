// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/smartystreets/goconvey/convey"
)

// TestAssignTask_RequiresAddOrRemove covers the Validate guard: neither --add
// nor --remove yields a typed validation error (exit 2) before any API call.
func TestAssignTask_RequiresAddOrRemove(t *testing.T) {
	f, stdout, _, _ := taskShortcutTestFactory(t)

	s := AssignTask
	args := []string{"+assign", "--task-id", "task-1", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T, want *errs.ValidationError; err = %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", got, output.ExitValidation)
	}
}

// TestAssignTask_MalformedResponse covers the Execute parse-response arm: a
// 200 with an unparseable body surfaces a typed internal invalid_response
// error (exit 5).
func TestAssignTask_MalformedResponse(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/task/v2/tasks/task-1/add_members",
		Status:  200,
		RawBody: []byte("{not-json"),
	})

	s := AssignTask
	args := []string{"+assign", "--task-id", "task-1", "--add", "ou_user_1", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("err = %T, want *errs.InternalError; err = %v", err, err)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("subtype = %q, want %q", ie.Subtype, errs.SubtypeInvalidResponse)
	}
	if got := output.ExitCodeOf(err); got != output.ExitInternal {
		t.Errorf("exit code = %d, want %d", got, output.ExitInternal)
	}
}

// TestAssignTask_MalformedResponse_RemoveArm covers the Execute remove-members
// parse arm: with only --remove set, the add arm is skipped and the
// remove_members POST returns a 200 with an unparseable body, which must
// surface a typed internal invalid_response error (exit 5).
func TestAssignTask_MalformedResponse_RemoveArm(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/task/v2/tasks/task-1/remove_members",
		Status:  200,
		RawBody: []byte("{not-json"),
	})

	s := AssignTask
	args := []string{"+assign", "--task-id", "task-1", "--remove", "ou_user_1", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("err = %T, want *errs.InternalError; err = %v", err, err)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("subtype = %q, want %q", ie.Subtype, errs.SubtypeInvalidResponse)
	}
	if got := output.ExitCodeOf(err); got != output.ExitInternal {
		t.Errorf("exit code = %d, want %d", got, output.ExitInternal)
	}
}

func TestBuildMembersBody(t *testing.T) {
	convey.Convey("Build with ids and token", t, func() {
		body := buildMembersBody("u1, u2 , ", "assignee", "token1")
		members := body["members"].([]map[string]interface{})
		convey.So(len(members), convey.ShouldEqual, 2)
		convey.So(body["client_token"], convey.ShouldEqual, "token1")
		convey.So(members[0]["role"], convey.ShouldEqual, "assignee")
		convey.So(members[0]["type"], convey.ShouldEqual, "user")
	})

	convey.Convey("Build infers app assignee members from cli prefix", t, func() {
		body := buildMembersBody("cli_bot_1", "assignee", "")
		members := body["members"].([]map[string]interface{})
		convey.So(len(members), convey.ShouldEqual, 1)
		convey.So(members[0]["id"], convey.ShouldEqual, "cli_bot_1")
		convey.So(members[0]["role"], convey.ShouldEqual, "assignee")
		convey.So(members[0]["type"], convey.ShouldEqual, "app")
	})

	convey.Convey("Build infers mixed member types in one list", t, func() {
		body := buildMembersBody("ou_user_1, cli_bot_1", "assignee", "")
		members := body["members"].([]map[string]interface{})
		convey.So(len(members), convey.ShouldEqual, 2)
		convey.So(members[0]["type"], convey.ShouldEqual, "user")
		convey.So(members[1]["type"], convey.ShouldEqual, "app")
	})
}

func TestBuildTaskCreateBodySupportsAssigneeAndFollower(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("summary", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("assignee", "", "")
	cmd.Flags().String("follower", "", "")
	cmd.Flags().String("due", "", "")
	cmd.Flags().String("tasklist-id", "", "")
	cmd.Flags().String("idempotency-key", "", "")
	cmd.Flags().String("data", "", "")
	_ = cmd.Flags().Set("summary", "bot task")
	_ = cmd.Flags().Set("assignee", "cli_bot_xxx")
	_ = cmd.Flags().Set("follower", "ou_follower_xxx")

	runtime := &common.RuntimeContext{Cmd: cmd}
	body, err := buildTaskCreateBody(runtime)
	if err != nil {
		t.Fatalf("buildTaskCreateBody() error = %v", err)
	}

	members := body["members"].([]map[string]interface{})
	if len(members) != 2 {
		t.Fatalf("members len = %d, want 2", len(members))
	}
	if got := members[0]["type"]; got != "app" {
		t.Fatalf("member[0] type = %v, want app", got)
	}
	if got := members[0]["role"]; got != "assignee" {
		t.Fatalf("member[0] role = %v, want assignee", got)
	}
	if got := members[1]["type"]; got != "user" {
		t.Fatalf("member[1] type = %v, want user", got)
	}
	if got := members[1]["role"]; got != "follower" {
		t.Fatalf("member[1] role = %v, want follower", got)
	}
}
