// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

type fakeWikiDeleteSpaceClient struct {
	deleteResp *wikiDeleteSpaceResponse
	deleteErr  error

	taskStatuses []wikiDeleteSpaceTaskStatus
	taskErrs     []error

	deleteCalls  []string
	taskCallArgs []string
}

func (fake *fakeWikiDeleteSpaceClient) DeleteSpace(ctx context.Context, spaceID string) (*wikiDeleteSpaceResponse, error) {
	fake.deleteCalls = append(fake.deleteCalls, spaceID)
	if fake.deleteErr != nil {
		return nil, fake.deleteErr
	}
	if fake.deleteResp != nil {
		return fake.deleteResp, nil
	}
	return &wikiDeleteSpaceResponse{}, nil
}

func (fake *fakeWikiDeleteSpaceClient) GetDeleteSpaceTask(ctx context.Context, taskID string) (wikiDeleteSpaceTaskStatus, error) {
	idx := len(fake.taskCallArgs)
	fake.taskCallArgs = append(fake.taskCallArgs, taskID)
	if idx < len(fake.taskErrs) && fake.taskErrs[idx] != nil {
		return wikiDeleteSpaceTaskStatus{TaskID: taskID}, fake.taskErrs[idx]
	}
	if idx < len(fake.taskStatuses) {
		status := fake.taskStatuses[idx]
		if status.TaskID == "" {
			status.TaskID = taskID
		}
		return status, nil
	}
	return wikiDeleteSpaceTaskStatus{TaskID: taskID}, nil
}

var wikiDeleteSpacePollMu sync.Mutex

func withSingleWikiDeleteSpacePoll(t *testing.T) {
	t.Helper()
	wikiDeleteSpacePollMu.Lock()

	prevAttempts, prevInterval := wikiDeleteSpacePollAttempts, wikiDeleteSpacePollInterval
	wikiDeleteSpacePollAttempts, wikiDeleteSpacePollInterval = 1, 0
	t.Cleanup(func() {
		wikiDeleteSpacePollAttempts, wikiDeleteSpacePollInterval = prevAttempts, prevInterval
		wikiDeleteSpacePollMu.Unlock()
	})
}

func newWikiDeleteSpaceRuntimeWithScopes(t *testing.T, as core.Identity, scopes string) (*common.RuntimeContext, *bytes.Buffer) {
	t.Helper()

	cfg := wikiTestConfig()
	factory, _, stderr, _ := cmdutil.TestFactory(t, cfg)
	factory.Credential = credential.NewCredentialProvider(nil, nil, &mockWikiMoveTokenResolver{scopes: scopes}, nil)

	runtime := common.TestNewRuntimeContextWithIdentity(&cobra.Command{Use: "wiki +delete-space"}, cfg, as)
	runtime.Factory = factory
	return runtime, stderr
}

func TestValidateWikiDeleteSpaceSpecRequiresSpaceID(t *testing.T) {
	t.Parallel()

	if err := validateWikiDeleteSpaceSpec(wikiDeleteSpaceSpec{}); err == nil || !strings.Contains(err.Error(), "--space-id is required") {
		t.Fatalf("expected missing space-id error, got %v", err)
	}
	if err := validateWikiDeleteSpaceSpec(wikiDeleteSpaceSpec{SpaceID: "7629741305993170448"}); err != nil {
		t.Fatalf("validateWikiDeleteSpaceSpec(valid) = %v, want nil", err)
	}
}

func TestWikiDeleteSpaceDeclaredScopes(t *testing.T) {
	t.Parallel()

	want := []string{"wiki:space:write_only", "wiki:space:read"}
	if !reflect.DeepEqual(WikiDeleteSpace.Scopes, want) {
		t.Fatalf("WikiDeleteSpace.Scopes = %v, want %v", WikiDeleteSpace.Scopes, want)
	}
}

func TestWikiDeleteSpaceTaskStatusClassification(t *testing.T) {
	t.Parallel()

	pending := wikiDeleteSpaceTaskStatus{}
	if pending.Ready() || pending.Failed() || pending.StatusLabel() != wikiDeleteSpaceStatusProcessing {
		t.Fatalf("pending = %+v", pending)
	}

	success := wikiDeleteSpaceTaskStatus{Status: "success"}
	if !success.Ready() || success.Failed() || success.StatusLabel() != "success" {
		t.Fatalf("success = %+v", success)
	}

	failed := wikiDeleteSpaceTaskStatus{Status: "failure", StatusMsg: "permission denied"}
	if failed.Ready() || !failed.Failed() || failed.StatusLabel() != "permission denied" {
		t.Fatalf("failed = %+v", failed)
	}

	// Unknown non-success statuses must not be misreported as hard failures.
	unknown := wikiDeleteSpaceTaskStatus{Status: "running"}
	if unknown.Ready() || unknown.Failed() || unknown.StatusLabel() != "running" {
		t.Fatalf("unknown = %+v", unknown)
	}

	// Whitespace + mixed case must normalize consistently across Ready /
	// Failed / StatusCode — otherwise `" SUCCESS "` would be neither ready
	// nor failed and polling would loop to timeout on a terminal success.
	noisy := wikiDeleteSpaceTaskStatus{Status: "  SUCCESS  "}
	if !noisy.Ready() || noisy.Failed() {
		t.Fatalf("noisy success classification = %+v", noisy)
	}

	// StatusCode must never be empty so the output envelope's `status` field
	// can't surprise users with "" on a timeout branch.
	if got := (wikiDeleteSpaceTaskStatus{}).StatusCode(); got != wikiDeleteSpaceStatusProcessing {
		t.Fatalf("empty StatusCode = %q, want %q", got, wikiDeleteSpaceStatusProcessing)
	}
}

func TestWikiDeleteSpaceDryRunIncludesTaskPoll(t *testing.T) {
	t.Parallel()

	dry := buildWikiDeleteSpaceDryRun(wikiDeleteSpaceSpec{SpaceID: "space_123"})
	if dry == nil {
		t.Fatal("buildWikiDeleteSpaceDryRun returned nil")
	}
	formatted := dry.Format()
	if !strings.Contains(formatted, "DELETE /open-apis/wiki/v2/spaces/space_123") {
		t.Fatalf("dry run missing DELETE line: %s", formatted)
	}
	if !strings.Contains(formatted, "task_type") || !strings.Contains(formatted, "delete_space") {
		t.Fatalf("dry run missing task_type=delete_space: %s", formatted)
	}
}

func TestRunWikiDeleteSpaceSync(t *testing.T) {
	t.Parallel()

	runtime, _ := newWikiDeleteSpaceRuntimeWithScopes(t, core.AsUser, "")
	client := &fakeWikiDeleteSpaceClient{
		deleteResp: &wikiDeleteSpaceResponse{},
	}

	out, err := runWikiDeleteSpace(context.Background(), client, runtime, wikiDeleteSpaceSpec{SpaceID: "space_123"})
	if err != nil {
		t.Fatalf("runWikiDeleteSpace() error = %v", err)
	}
	if out["ready"] != true || out["failed"] != false || out["space_id"] != "space_123" {
		t.Fatalf("unexpected sync output: %#v", out)
	}
	// Sync envelope must mirror the async success shape so downstream scripts
	// can read `status` uniformly regardless of which branch fired.
	if out["status"] != "success" || out["status_msg"] != "success" {
		t.Fatalf("sync status fields = %#v / %#v, want both success", out["status"], out["status_msg"])
	}
	if _, ok := out["task_id"]; ok {
		t.Fatalf("sync output should not include task_id, got %#v", out)
	}
	if len(client.taskCallArgs) != 0 {
		t.Fatalf("sync path should not poll, got calls %v", client.taskCallArgs)
	}
}

func TestRunWikiDeleteSpaceAsyncReady(t *testing.T) {
	withSingleWikiDeleteSpacePoll(t)

	runtime, stderr := newWikiDeleteSpaceRuntimeWithScopes(t, core.AsUser, "")
	client := &fakeWikiDeleteSpaceClient{
		deleteResp: &wikiDeleteSpaceResponse{TaskID: "task_123"},
		taskStatuses: []wikiDeleteSpaceTaskStatus{{
			Status: "success",
		}},
	}

	out, err := runWikiDeleteSpace(context.Background(), client, runtime, wikiDeleteSpaceSpec{SpaceID: "space_123"})
	if err != nil {
		t.Fatalf("runWikiDeleteSpace() error = %v", err)
	}
	if out["task_id"] != "task_123" || out["ready"] != true || out["failed"] != false || out["status"] != "success" {
		t.Fatalf("unexpected async-ready output: %#v", out)
	}
	if !strings.Contains(stderr.String(), "async, polling task") || !strings.Contains(stderr.String(), "completed successfully") {
		t.Fatalf("stderr = %q, want async progress logs", stderr.String())
	}
}

func TestRunWikiDeleteSpaceAsyncTimeoutReturnsNextCommand(t *testing.T) {
	withSingleWikiDeleteSpacePoll(t)

	runtime, stderr := newWikiDeleteSpaceRuntimeWithScopes(t, core.AsUser, "")
	client := &fakeWikiDeleteSpaceClient{
		deleteResp: &wikiDeleteSpaceResponse{TaskID: "task_123"},
		taskStatuses: []wikiDeleteSpaceTaskStatus{{
			Status: "processing",
		}},
	}

	out, err := runWikiDeleteSpace(context.Background(), client, runtime, wikiDeleteSpaceSpec{SpaceID: "space_123"})
	if err != nil {
		t.Fatalf("runWikiDeleteSpace() error = %v", err)
	}
	wantNext := wikiDeleteSpaceTaskResultCommand("task_123", core.AsUser)
	if out["ready"] != false || out["timed_out"] != true || out["next_command"] != wantNext {
		t.Fatalf("expected timeout response, got %#v", out)
	}
	// Both `status` and `status_msg` must surface a human-readable value on
	// timeout — never "" — otherwise downstream scripts parsing the envelope
	// will disagree with the reference doc example.
	if out["status"] != "processing" || out["status_msg"] != "processing" {
		t.Fatalf("status fields = %#v / %#v, want both processing", out["status"], out["status_msg"])
	}
	if !strings.Contains(stderr.String(), "Continue with") {
		t.Fatalf("stderr = %q, want continuation hint", stderr.String())
	}
}

func TestRunWikiDeleteSpaceAsyncFailure(t *testing.T) {
	withSingleWikiDeleteSpacePoll(t)

	runtime, _ := newWikiDeleteSpaceRuntimeWithScopes(t, core.AsUser, "")
	client := &fakeWikiDeleteSpaceClient{
		deleteResp: &wikiDeleteSpaceResponse{TaskID: "task_123"},
		taskStatuses: []wikiDeleteSpaceTaskStatus{{
			Status:    "failure",
			StatusMsg: "permission denied",
		}},
	}

	_, err := runWikiDeleteSpace(context.Background(), client, runtime, wikiDeleteSpaceSpec{SpaceID: "space_123"})
	// The error surface must carry both the task_id (for post-mortem) and the
	// backend-reported failure reason.
	if err == nil || !strings.Contains(err.Error(), "wiki delete-space task task_123 failed: permission denied") {
		t.Fatalf("expected async failure error, got %v", err)
	}
}

func TestPollWikiDeleteSpaceTaskWrapsPollFailuresWithHint(t *testing.T) {
	withSingleWikiDeleteSpacePoll(t)

	runtime, stderr := newWikiDeleteSpaceRuntimeWithScopes(t, core.AsUser, "")
	client := &fakeWikiDeleteSpaceClient{
		taskErrs: []error{output.ErrWithHint(output.ExitAPI, "api_error", "poll failed", "retry original")},
	}

	status, ready, err := pollWikiDeleteSpaceTask(context.Background(), client, runtime, "task_123")
	if err == nil {
		t.Fatal("expected pollWikiDeleteSpaceTask() error, got nil")
	}
	if ready {
		t.Fatal("expected ready=false when every poll fails")
	}
	if status.TaskID != "task_123" {
		t.Fatalf("status.TaskID = %q, want %q", status.TaskID, "task_123")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected structured exit error, got %T %v", err, err)
	}
	if !strings.Contains(exitErr.Detail.Hint, "retry original") || !strings.Contains(exitErr.Detail.Hint, wikiDeleteSpaceTaskResultCommand("task_123", core.AsUser)) {
		t.Fatalf("hint = %q, want original hint and resume command", exitErr.Detail.Hint)
	}
	if !strings.Contains(stderr.String(), "Wiki delete-space status attempt 1/1 failed") {
		t.Fatalf("stderr = %q, want poll failure log", stderr.String())
	}
}

func TestParseWikiDeleteSpaceTaskStatusFallbackTaskID(t *testing.T) {
	t.Parallel()

	status, err := parseWikiDeleteSpaceTaskStatus("task_fallback", map[string]interface{}{
		"delete_space_result": map[string]interface{}{
			"status": "success",
		},
	})
	if err != nil {
		t.Fatalf("parseWikiDeleteSpaceTaskStatus() error = %v", err)
	}
	if status.TaskID != "task_fallback" {
		t.Fatalf("TaskID = %q, want %q", status.TaskID, "task_fallback")
	}
	if !status.Ready() || status.StatusLabel() != "success" {
		t.Fatalf("unexpected parsed status: %+v", status)
	}
}

func TestParseWikiDeleteSpaceTaskStatusRejectsMissingTask(t *testing.T) {
	t.Parallel()

	_, err := parseWikiDeleteSpaceTaskStatus("task_123", nil)
	if err == nil || !strings.Contains(err.Error(), "missing task") {
		t.Fatalf("expected missing task error, got %v", err)
	}
}

func TestWikiDeleteSpaceExecuteSync(t *testing.T) {
	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
	deleteStub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/wiki/v2/spaces/space_123",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"task_id": "",
			},
		},
	}
	reg.Register(deleteStub)

	err := mountAndRunWiki(t, WikiDeleteSpace, []string{
		"+delete-space",
		"--space-id", "space_123",
		"--yes",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	if data["ready"] != true || data["failed"] != false || data["space_id"] != "space_123" {
		t.Fatalf("unexpected sync execute output: %#v", data)
	}
}

func TestWikiDeleteSpaceExecuteRequiresYesConfirmation(t *testing.T) {
	factory, stdout, _, _ := cmdutil.TestFactory(t, wikiTestConfig())

	err := mountAndRunWiki(t, WikiDeleteSpace, []string{
		"+delete-space",
		"--space-id", "space_123",
		"--as", "user",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "requires confirmation") {
		t.Fatalf("expected high-risk confirmation error, got %v", err)
	}
}

func TestWikiDeleteSpaceExecuteAsyncSuccess(t *testing.T) {
	withSingleWikiDeleteSpacePoll(t)

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/wiki/v2/spaces/space_123",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"task_id": "task_async_1",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/tasks/task_async_1",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"task": map[string]interface{}{
					"delete_space_result": map[string]interface{}{
						"status": "success",
					},
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiDeleteSpace, []string{
		"+delete-space",
		"--space-id", "space_123",
		"--yes",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	if data["task_id"] != "task_async_1" || data["ready"] != true || data["failed"] != false {
		t.Fatalf("unexpected async execute output: %#v", data)
	}
}
