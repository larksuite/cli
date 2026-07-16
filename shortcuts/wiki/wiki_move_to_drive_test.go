// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

type fakeWikiMoveToDriveClient struct {
	moveTaskID string
	moveErr    error
	taskStatus []wikiMoveToDriveTaskStatus
	taskErrs   []error
	taskHooks  []func()

	moveSpecs []wikiMoveToDriveSpec
	taskCalls []string
}

func (fake *fakeWikiMoveToDriveClient) MoveWikiToDrive(ctx context.Context, spec wikiMoveToDriveSpec) (string, error) {
	fake.moveSpecs = append(fake.moveSpecs, spec)
	if fake.moveErr != nil {
		return "", fake.moveErr
	}
	return fake.moveTaskID, nil
}

func (fake *fakeWikiMoveToDriveClient) GetMoveWikiToDriveTask(ctx context.Context, taskID string) (wikiMoveToDriveTaskStatus, error) {
	idx := len(fake.taskCalls)
	fake.taskCalls = append(fake.taskCalls, taskID)
	if idx < len(fake.taskHooks) && fake.taskHooks[idx] != nil {
		fake.taskHooks[idx]()
	}
	if idx < len(fake.taskErrs) && fake.taskErrs[idx] != nil {
		return wikiMoveToDriveTaskStatus{TaskID: taskID, Status: wikiMoveToDriveStatusProcessing}, fake.taskErrs[idx]
	}
	if idx < len(fake.taskStatus) {
		status := fake.taskStatus[idx]
		if status.TaskID == "" {
			status.TaskID = taskID
		}
		return status, nil
	}
	return wikiMoveToDriveTaskStatus{TaskID: taskID, Status: wikiMoveToDriveStatusProcessing}, nil
}

var wikiMoveToDrivePollMu sync.Mutex

func withSingleWikiMoveToDrivePoll(t *testing.T) {
	withWikiMoveToDrivePoll(t, 1)
}

func withWikiMoveToDrivePoll(t *testing.T, attempts int) {
	t.Helper()
	wikiMoveToDrivePollMu.Lock()

	previousAttempts, previousInterval := wikiMoveToDrivePollAttempts, wikiMoveToDrivePollInterval
	wikiMoveToDrivePollAttempts, wikiMoveToDrivePollInterval = attempts, 0
	t.Cleanup(func() {
		wikiMoveToDrivePollAttempts, wikiMoveToDrivePollInterval = previousAttempts, previousInterval
		wikiMoveToDrivePollMu.Unlock()
	})
}

func newWikiMoveToDriveRuntime(t *testing.T, identity core.Identity) (*common.RuntimeContext, *bytes.Buffer) {
	t.Helper()
	cfg := wikiTestConfig()
	factory, _, stderr, _ := cmdutil.TestFactory(t, cfg)
	runtime := common.TestNewRuntimeContextWithIdentity(nil, cfg, identity)
	runtime.Factory = factory
	return runtime, stderr
}

func TestWikiMoveToDriveDeclaredContract(t *testing.T) {
	t.Parallel()

	wantScopes := []string{"space:document:move", "wiki:space:read"}
	if !reflect.DeepEqual(WikiMoveToDrive.Scopes, wantScopes) {
		t.Fatalf("WikiMoveToDrive.Scopes = %v, want %v", WikiMoveToDrive.Scopes, wantScopes)
	}
	if WikiMoveToDrive.Risk != "write" {
		t.Fatalf("WikiMoveToDrive.Risk = %q, want write", WikiMoveToDrive.Risk)
	}
}

func TestValidateWikiMoveToDriveSpec(t *testing.T) {
	t.Parallel()

	t.Run("requires node token", func(t *testing.T) {
		err := validateWikiMoveToDriveSpec(wikiMoveToDriveSpec{})
		requireWikiValidationParams(t, err, "--node-token")
	})

	t.Run("rejects unsafe folder token", func(t *testing.T) {
		err := validateWikiMoveToDriveSpec(wikiMoveToDriveSpec{
			NodeToken:   "wikcnABC",
			FolderToken: "../folder",
		})
		requireWikiValidationParams(t, err, "--folder-token")
		cause := errors.Unwrap(err)
		if cause == nil || !errors.Is(err, cause) {
			t.Fatal("validation error must preserve its path-validation cause")
		}
	})

	t.Run("accepts optional folder", func(t *testing.T) {
		err := validateWikiMoveToDriveSpec(wikiMoveToDriveSpec{NodeToken: "wikcnABC"})
		if err != nil {
			t.Fatalf("validateWikiMoveToDriveSpec() error = %v", err)
		}
	})
}

func TestWikiMoveToDriveRequestBodyOmitsEmptyFolder(t *testing.T) {
	t.Parallel()

	withoutFolder := (wikiMoveToDriveSpec{NodeToken: "wikcnABC"}).RequestBody()
	if _, exists := withoutFolder["folder_token"]; exists {
		t.Fatalf("empty folder_token must be omitted, got %#v", withoutFolder)
	}

	withFolder := (wikiMoveToDriveSpec{NodeToken: "wikcnABC", FolderToken: "fldABC"}).RequestBody()
	if withFolder["folder_token"] != "fldABC" {
		t.Fatalf("RequestBody() = %#v, want folder_token=fldABC", withFolder)
	}
}

func TestBuildWikiMoveToDriveDryRun(t *testing.T) {
	t.Parallel()

	steps := decodeDryRunAPIs(t, buildWikiMoveToDriveDryRun(wikiMoveToDriveSpec{
		NodeToken:   "wikcnABC",
		FolderToken: "fldABC",
	}))
	if len(steps) != 2 {
		t.Fatalf("len(api) = %d, want 2", len(steps))
	}
	if steps[0].Method != "POST" || steps[0].URL != "/open-apis/wiki/v2/nodes/wikcnABC/move_wiki_to_docs" {
		t.Fatalf("POST step = %#v", steps[0])
	}
	if steps[0].Body["folder_token"] != "fldABC" {
		t.Fatalf("POST body = %#v", steps[0].Body)
	}
	if steps[1].Method != "GET" || steps[1].URL != "/open-apis/wiki/v2/tasks/%3Ctask_id%3E" {
		t.Fatalf("GET step = %#v", steps[1])
	}
	if steps[1].Params["task_type"] != wikiMoveToDriveTaskType {
		t.Fatalf("task params = %#v", steps[1].Params)
	}
}

func TestParseWikiMoveToDriveTaskStatus(t *testing.T) {
	t.Parallel()

	t.Run("success with task id fallback and result fields", func(t *testing.T) {
		status, err := parseWikiMoveToDriveTaskStatus("signed-task-id", map[string]interface{}{
			"move_wiki_to_docs_result": map[string]interface{}{
				"status":     float64(0),
				"status_msg": "success",
				"obj_token":  "docxABC",
				"obj_type":   "docx",
				"url":        "https://example.feishu.cn/docx/docxABC",
			},
		})
		if err != nil {
			t.Fatalf("parseWikiMoveToDriveTaskStatus() error = %v", err)
		}
		if status.TaskID != "signed-task-id" || !status.Ready() || status.Failed() {
			t.Fatalf("status = %+v", status)
		}
		if status.ObjToken != "docxABC" || status.ObjType != "docx" || status.URL == "" {
			t.Fatalf("result fields = %+v", status)
		}
	})

	t.Run("rejects missing dedicated result", func(t *testing.T) {
		_, err := parseWikiMoveToDriveTaskStatus("task", map[string]interface{}{})
		problem, ok := errs.ProblemOf(err)
		if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
			t.Fatalf("error = %T %v, want internal/invalid_response", err, err)
		}
	})

	t.Run("rejects missing status", func(t *testing.T) {
		_, err := parseWikiMoveToDriveTaskStatus("task", map[string]interface{}{
			"move_wiki_to_docs_result": map[string]interface{}{},
		})
		problem, ok := errs.ProblemOf(err)
		if !ok || problem.Subtype != errs.SubtypeInvalidResponse {
			t.Fatalf("error = %T %v, want invalid_response", err, err)
		}
	})

	for name, rawStatus := range map[string]interface{}{
		"null":          nil,
		"string":        "processing",
		"fractional":    0.5,
		"unknown value": 2,
	} {
		t.Run("rejects "+name+" status", func(t *testing.T) {
			_, err := parseWikiMoveToDriveTaskStatus("task", map[string]interface{}{
				"move_wiki_to_docs_result": map[string]interface{}{"status": rawStatus},
			})
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("error = %T %v, want internal/invalid_response", err, err)
			}
		})
	}
}

func TestRunWikiMoveToDriveSuccess(t *testing.T) {
	withSingleWikiMoveToDrivePoll(t)
	runtime, stderr := newWikiMoveToDriveRuntime(t, core.AsUser)
	client := &fakeWikiMoveToDriveClient{
		moveTaskID: "raw-task-signature",
		taskStatus: []wikiMoveToDriveTaskStatus{{
			Status:    wikiMoveToDriveStatusSuccess,
			StatusMsg: "success",
			ObjToken:  "docxABC",
			ObjType:   "docx",
			URL:       "https://example.feishu.cn/docx/docxABC",
		}},
	}

	out, err := runWikiMoveToDrive(context.Background(), client, runtime, wikiMoveToDriveSpec{
		NodeToken:   "wikcnABC",
		FolderToken: "fldABC",
	})
	if err != nil {
		t.Fatalf("runWikiMoveToDrive() error = %v", err)
	}
	if out["task_id"] != "raw-task-signature" || out["ready"] != true || out["failed"] != false {
		t.Fatalf("output = %#v", out)
	}
	if out["obj_token"] != "docxABC" || out["obj_type"] != "docx" || out["url"] == "" {
		t.Fatalf("output result fields = %#v", out)
	}
	if len(client.moveSpecs) != 1 || client.moveSpecs[0].FolderToken != "fldABC" {
		t.Fatalf("move specs = %#v", client.moveSpecs)
	}
	if !strings.Contains(stderr.String(), "completed successfully") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunWikiMoveToDriveTimeoutReturnsResumeCommand(t *testing.T) {
	withSingleWikiMoveToDrivePoll(t)
	runtime, _ := newWikiMoveToDriveRuntime(t, core.AsBot)
	runtime.Config.ProfileName = "secondary"
	client := &fakeWikiMoveToDriveClient{
		moveTaskID: "raw-task-signature",
		taskStatus: []wikiMoveToDriveTaskStatus{{Status: wikiMoveToDriveStatusProcessing}},
	}

	out, err := runWikiMoveToDrive(context.Background(), client, runtime, wikiMoveToDriveSpec{NodeToken: "wikcnABC"})
	if err != nil {
		t.Fatalf("runWikiMoveToDrive() error = %v", err)
	}
	if out["ready"] != false || out["failed"] != false || out["timed_out"] != true {
		t.Fatalf("timeout output = %#v", out)
	}
	nextCommand, _ := out["next_command"].(string)
	if !strings.HasPrefix(nextCommand, "lark-cli --profile secondary drive +task_result") ||
		!strings.Contains(nextCommand, "--scenario wiki_move_to_drive") ||
		!strings.Contains(nextCommand, "--as bot") {
		t.Fatalf("next_command = %q", nextCommand)
	}
}

func TestPollWikiMoveToDriveContinuesFromProcessingToSuccess(t *testing.T) {
	withWikiMoveToDrivePoll(t, 2)
	runtime, _ := newWikiMoveToDriveRuntime(t, core.AsUser)
	client := &fakeWikiMoveToDriveClient{
		taskStatus: []wikiMoveToDriveTaskStatus{
			{Status: wikiMoveToDriveStatusProcessing},
			{Status: wikiMoveToDriveStatusSuccess, ObjToken: "docxABC"},
		},
	}

	status, ready, err := pollWikiMoveToDriveTask(context.Background(), client, runtime, "signed-task-id")
	if err != nil || !ready || !status.Ready() || status.ObjToken != "docxABC" {
		t.Fatalf("status=%+v ready=%t err=%v", status, ready, err)
	}
	if len(client.taskCalls) != 2 {
		t.Fatalf("task calls = %v, want two attempts", client.taskCalls)
	}
}

func TestPollWikiMoveToDriveRecoversFromTransientError(t *testing.T) {
	withWikiMoveToDrivePoll(t, 2)
	runtime, _ := newWikiMoveToDriveRuntime(t, core.AsUser)
	requestTimeout := errs.NewNetworkError(errs.SubtypeNetworkTimeout, "temporary request timeout").
		WithCause(context.DeadlineExceeded)
	client := &fakeWikiMoveToDriveClient{
		taskErrs: []error{requestTimeout},
		taskStatus: []wikiMoveToDriveTaskStatus{
			{},
			{Status: wikiMoveToDriveStatusSuccess, ObjToken: "docxABC"},
		},
	}

	status, ready, err := pollWikiMoveToDriveTask(context.Background(), client, runtime, "signed-task-id")
	if err != nil || !ready || !status.Ready() || status.ObjToken != "docxABC" {
		t.Fatalf("status=%+v ready=%t err=%v", status, ready, err)
	}
	if len(client.taskCalls) != 2 {
		t.Fatalf("task calls = %v, want two attempts", client.taskCalls)
	}
}

func TestPollWikiMoveToDriveDoesNotSwallowFinalCancellation(t *testing.T) {
	withWikiMoveToDrivePoll(t, 2)
	runtime, _ := newWikiMoveToDriveRuntime(t, core.AsUser)
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeWikiMoveToDriveClient{
		taskStatus: []wikiMoveToDriveTaskStatus{{Status: wikiMoveToDriveStatusProcessing}},
		taskErrs:   []error{nil, context.Canceled},
		taskHooks:  []func(){nil, cancel},
	}

	_, ready, err := pollWikiMoveToDriveTask(ctx, client, runtime, "signed-task-id")
	if ready || !errors.Is(err, context.Canceled) {
		t.Fatalf("ready=%t err=%T %v, want preserved context cancellation", ready, err, err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("error = %T %v, want network/transport", err, err)
	}
}

func TestRunWikiMoveToDriveFailureIsTypedAPIError(t *testing.T) {
	withSingleWikiMoveToDrivePoll(t)
	runtime, _ := newWikiMoveToDriveRuntime(t, core.AsUser)
	client := &fakeWikiMoveToDriveClient{
		moveTaskID: "raw-task-signature",
		taskStatus: []wikiMoveToDriveTaskStatus{{Status: wikiMoveToDriveStatusFailure, StatusMsg: "failure"}},
	}

	_, err := runWikiMoveToDrive(context.Background(), client, runtime, wikiMoveToDriveSpec{NodeToken: "wikcnABC"})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeServerError {
		t.Fatalf("error = %T %v, want api/server_error", err, err)
	}
}

func TestPollWikiMoveToDrivePreservesTypedPollError(t *testing.T) {
	withSingleWikiMoveToDrivePoll(t)
	runtime, _ := newWikiMoveToDriveRuntime(t, core.AsUser)
	cause := errors.New("connection reset")
	upstream := errs.NewNetworkError(errs.SubtypeNetworkTransport, "poll failed").
		WithCode(503).
		WithHint("retry upstream").
		WithCause(cause)
	client := &fakeWikiMoveToDriveClient{taskErrs: []error{upstream}}

	_, ready, err := pollWikiMoveToDriveTask(context.Background(), client, runtime, "raw-task-signature")
	if ready || err != upstream {
		t.Fatalf("ready=%t err=%T %v, want original typed error", ready, err, err)
	}
	if !errors.Is(err, cause) {
		t.Fatal("typed poll error must preserve its cause")
	}
	problem, _ := errs.ProblemOf(err)
	if problem.Code != 503 || !strings.Contains(problem.Hint, "retry upstream") || !strings.Contains(problem.Hint, "wiki_move_to_drive") {
		t.Fatalf("problem = %+v", problem)
	}
}

func TestWrapWikiMoveToDrivePollContextError(t *testing.T) {
	t.Parallel()

	err := wrapWikiMoveToDrivePollContextError(context.DeadlineExceeded, "task-id", core.AsUser, "secondary")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("wrapped deadline must preserve context.DeadlineExceeded")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTimeout {
		t.Fatalf("error = %T %v, want network/timeout", err, err)
	}
	if !strings.Contains(problem.Hint, "wiki_move_to_drive") || !strings.Contains(problem.Hint, "--profile secondary") {
		t.Fatalf("hint = %q", problem.Hint)
	}
}

func TestWikiMoveToDriveExecuteCallsPostAndTaskEndpoint(t *testing.T) {
	withSingleWikiMoveToDrivePoll(t)
	factory, stdout, _, registry := cmdutil.TestFactory(t, wikiTestConfig())

	moveStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/nodes/wikcnABC/move_wiki_to_docs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"task_id": "raw-task-signature"},
		},
	}
	registry.Register(moveStub)

	var taskQuery string
	taskStub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/tasks/raw-task-signature",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"task": map[string]interface{}{
					// The external handler currently omits task.task_id for this
					// task type, so the CLI must preserve the signed request ID.
					"move_wiki_to_docs_result": map[string]interface{}{
						"status":     0,
						"status_msg": "success",
						"obj_token":  "docxABC",
						"obj_type":   "docx",
						"url":        "https://example.feishu.cn/docx/docxABC",
					},
				},
			},
		},
	}
	taskStub.OnMatch = func(req *http.Request) { taskQuery = req.URL.RawQuery }
	registry.Register(taskStub)

	err := mountAndRunWiki(t, WikiMoveToDrive, []string{
		"+move-to-drive",
		"--node-token", "wikcnABC",
		"--folder-token", "fldABC",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	body := decodeWikiCapturedJSONBody(t, moveStub)
	if body["folder_token"] != "fldABC" {
		t.Fatalf("captured POST body = %#v", body)
	}
	if !strings.Contains(taskQuery, "task_type=move_wiki_to_docs") {
		t.Fatalf("task query = %q", taskQuery)
	}
	data := decodeWikiEnvelope(t, stdout)
	if data["task_id"] != "raw-task-signature" || data["ready"] != true || data["obj_token"] != "docxABC" {
		t.Fatalf("output = %#v", data)
	}
}

func TestWikiMoveToDriveExecuteRejectsMissingTaskID(t *testing.T) {
	factory, stdout, _, registry := cmdutil.TestFactory(t, wikiTestConfig())
	registry.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/nodes/wikcnABC/move_wiki_to_docs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunWiki(t, WikiMoveToDrive, []string{
		"+move-to-drive",
		"--node-token", "wikcnABC",
		"--as", "user",
	}, factory, stdout)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("error = %T %v, want internal/invalid_response", err, err)
	}
}
