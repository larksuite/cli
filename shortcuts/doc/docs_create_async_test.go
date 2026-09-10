// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestDocsCreateAsyncReadFailureIsNotSuccess(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	registerDocsCreateAPIStub(reg, map[string]interface{}{"task": map[string]interface{}{
		"task_id": "task_denied", "status": "processing",
	}})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/async_tasks/task_denied", Status: 403,
		Headers: http.Header{"X-Tt-Logid": {"poll-denied-log"}},
		Body:    map[string]interface{}{"code": 99991672, "msg": "missing scope"},
	})
	parent := &cobra.Command{Use: "docs", SilenceErrors: true, SilenceUsage: true}
	DocsCreate.Mount(parent, f)
	parent.SetArgs([]string{"+create", "--content", "<title>Async</title><p>Body</p>", "--as", "user"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := parent.ExecuteContext(ctx)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Code != 99991672 || problem.LogID != "poll-denied-log" {
		t.Fatalf("query failure was hidden: err=%v problem=%+v", err, problem)
	}
	assertDocsCreateErrorHasNoTaskRecovery(t, err, "task_denied")
	if stdout.Len() != 0 {
		t.Fatalf("failed command emitted success: %s", stdout)
	}
}

func TestDocsCreateAsyncUnsupportedStatus(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/docs_ai/v1/documents", Status: 200,
		Headers: http.Header{"X-Tt-Logid": {"create-unsupported-log"}},
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"task": map[string]interface{}{
			"task_id": "task_unsupported", "status": "unsupported",
		}}},
	})
	parent := &cobra.Command{Use: "docs", SilenceErrors: true, SilenceUsage: true}
	DocsCreate.Mount(parent, f)
	parent.SetArgs([]string{"+create", "--content", "<title>Async</title><p>Body</p>", "--as", "user"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := parent.ExecuteContext(ctx)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse || problem.LogID != "create-unsupported-log" {
		t.Fatalf("unsupported status was not classified: err=%v problem=%+v", err, problem)
	}
	assertDocsCreateErrorHasNoTaskRecovery(t, err, "task_unsupported")
	if stdout.Len() != 0 {
		t.Fatalf("unsupported status emitted success: %s", stdout)
	}
}

func TestDocsCreateAsyncDeadlineCancelsInflightRead(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := docsCreateTestConfig(t, "")
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), &cobra.Command{Use: "+create"}, cfg)
	runtime.Factory = f
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/async_tasks/task_slow",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"task": map[string]interface{}{
			"task_id": "task_slow", "status": "processing",
		}}},
		OnMatch: func(req *http.Request) {
			if _, ok := req.Context().Deadline(); !ok {
				t.Fatal("poll request has no deadline")
			}
			<-req.Context().Done()
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result, err := pollDocsCreateAsyncTask(ctx, runtime, &docsCreateAsyncTask{TaskID: "task_slow", Status: "processing"}, "create-log", nil)
	problem, ok := errs.ProblemOf(err)
	if result != nil || !errors.Is(err, context.DeadlineExceeded) || !ok || problem.Subtype != errs.SubtypeNetworkTimeout {
		t.Fatalf("deadline result=%v err=%v problem=%+v", result, err, problem)
	}
	if problem.LogID != "create-log" || !strings.Contains(problem.Hint, "--command append") {
		t.Fatalf("timeout recovery missing: %+v", problem)
	}
	assertDocsCreateErrorHasNoTaskRecovery(t, err, "task_slow")
}

func TestDocsCreateAsyncCancellationIsTyped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := pollDocsCreateAsyncTask(ctx, nil, &docsCreateAsyncTask{TaskID: "task_canceled", Status: "processing"}, "", nil)
	if result != nil || !errs.IsTyped(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation result=%v err=%v", result, err)
	}
	problem, _ := errs.ProblemOf(err)
	if problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTransport || problem.Retryable {
		t.Fatalf("incorrect cancellation classification: %+v", problem)
	}
	assertDocsCreateErrorHasNoTaskRecovery(t, err, "task_canceled")
}

func TestDocsCreateAsyncRetryClassification(t *testing.T) {
	for _, tt := range []struct {
		err  error
		want bool
	}{
		{errs.NewAPIError(errs.SubtypeRateLimit, "rate limited").WithRetryable(), true},
		{errs.NewNetworkError(errs.SubtypeNetworkServer, "unavailable").WithRetryable(), true},
		{errs.NewNetworkError(errs.SubtypeNetworkTimeout, "timeout"), true},
		{errs.NewNetworkError(errs.SubtypeNetworkTransport, "reset"), true},
		{errs.NewAPIError(errs.SubtypeNotFound, "missing"), false},
		{errs.NewInternalError(errs.SubtypeInvalidResponse, "bad JSON"), false},
		{errs.NewNetworkError(errs.SubtypeNetworkTLS, "invalid certificate"), false},
	} {
		if got := retryableDocsCreateTaskRead(tt.err); got != tt.want {
			t.Fatalf("retry %v = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestDocsCreateAsyncSuccessStillGrantsPermission(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, "ou_current_user"))
	registerDocsCreateAPIStub(reg, map[string]interface{}{"task": map[string]interface{}{
		"task_id": "task_grant", "status": "processing",
	}})
	result, _ := json.Marshal(map[string]interface{}{"document": map[string]interface{}{
		"document_id": "doxcn_async_grant", "revision_id": 1,
	}})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/async_tasks/task_grant",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"task": map[string]interface{}{
			"task_id": "task_grant", "status": "succeeded", "result": map[string]interface{}{"create_document": string(result)},
		}}},
	})
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/permissions/doxcn_async_grant/members",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"member": map[string]interface{}{
			"member_id": "ou_current_user", "member_type": "openid", "perm": "full_access",
		}}},
	})
	err := runDocsCreateShortcut(t, f, stdout, []string{"+create", "--content", "<title>Async</title><p>Body</p>", "--as", "bot"})
	if err != nil {
		t.Fatal(err)
	}
	data := decodeDocsCreateEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantGranted || data["task"] != nil {
		t.Fatalf("async create did not finish synchronous follow-up: %+v", data)
	}
}

func TestDocsCreateAsyncRetriesReadWithoutRepeatingCreate(t *testing.T) {
	t.Setenv(docsCreateDebugEnv, "1")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, stderr, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	registerDocsCreateAPIStub(reg, map[string]interface{}{"task": map[string]interface{}{
		"task_id": "task_retry", "status": "processing",
	}})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/async_tasks/task_retry", Status: 503,
		Body: map[string]interface{}{"code": 233523001, "msg": "unavailable"},
	})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/async_tasks/task_retry",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"task": map[string]interface{}{
			"task_id": "task_retry", "status": "succeeded", "result": map[string]interface{}{
				"create_document": `{"document":{"document_id":"doxcn_retried","revision_id":1}}`,
			},
		}}},
	})
	if err := runDocsCreateShortcut(t, f, stdout, []string{"+create", "--content", "<title>Retry</title><p>Body</p>", "--as", "user"}); err != nil {
		t.Fatal(err)
	}
	data := decodeDocsCreateEnvelope(t, stdout)
	doc, _ := data["document"].(map[string]interface{})
	if doc["document_id"] != "doxcn_retried" {
		t.Fatalf("retry result=%+v", data)
	}
	var creates, polls, retries int
	for _, event := range readCreateDebugEvents(t, stderr.String()) {
		switch event.Event {
		case "create_request.start":
			creates++
		case "poll_request":
			polls++
		case "poll_retry":
			retries++
		}
	}
	if creates != 1 || polls != 2 || retries != 1 {
		t.Fatalf("debug counts: creates=%d polls=%d retries=%d", creates, polls, retries)
	}
}

func TestDocsCreateAsyncSuccessUploadsAndBindsLocalImage(t *testing.T) {
	t.Setenv(docsCreateDebugEnv, "1")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cmdutil.TestChdir(t, t.TempDir())
	if err := os.WriteFile("image.png", []byte(localDocResourcePNG(t, 4, 4)), 0o600); err != nil {
		t.Fatal(err)
	}
	f, stdout, stderr, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	taskResult := map[string]interface{}{}
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/docs_ai/v1/documents",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"task": map[string]interface{}{
			"task_id": "task_image", "status": "processing",
		}}},
		BodyFilter: func(raw []byte) bool {
			var body struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatal(err)
			}
			marker := regexp.MustCompile(`@lcli_img_[0-9a-f]{32}`).FindString(body.Content)
			if marker == "" {
				t.Fatal("image was not prepared before create")
			}
			encoded, err := json.Marshal(map[string]interface{}{"document": map[string]interface{}{
				"document_id": "doxcn_image", "revision_id": 1,
				"new_blocks": []interface{}{map[string]interface{}{"block_type": "image", "block_id": "block_image", "block_token": marker}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			taskResult["create_document"] = string(encoded)
			return true
		},
	})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/async_tasks/task_image",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"task": map[string]interface{}{
			"task_id": "task_image", "status": "succeeded", "result": taskResult,
		}}},
	})
	upload := &httpmock.Stub{Method: "POST", URL: "/medias/upload_all",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "uploaded_image"}},
	}
	reg.Register(upload)
	bind := &httpmock.Stub{Method: "PATCH", URL: "/documents/doxcn_image/blocks/batch_update",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"document_revision_id": 2}},
	}
	reg.Register(bind)
	if err := runDocsCreateShortcut(t, f, stdout, []string{"+create", "--content", `<title>Image</title><img path="@image.png"/>`, "--as", "user"}); err != nil {
		t.Fatal(err)
	}
	if len(upload.CapturedBodies) != 1 || !strings.Contains(string(bind.CapturedBody), "uploaded_image") {
		t.Fatalf("upload/bind follow-up missing: upload=%d bind=%s", len(upload.CapturedBodies), bind.CapturedBody)
	}
	data := decodeDocsCreateEnvelope(t, stdout)
	doc, _ := data["document"].(map[string]interface{})
	if doc["revision_id"] != float64(2) || strings.Contains(stdout.String(), "@lcli_img_") {
		t.Fatalf("output was returned before image finalization: %s", stdout)
	}
	var resourceSteps []string
	for _, event := range readCreateDebugEvents(t, stderr.String()) {
		if strings.HasPrefix(event.Event, "resource_") {
			resourceSteps = append(resourceSteps, event.Event)
		}
	}
	if strings.Join(resourceSteps, ",") != "resource_upload.start,resource_upload.end,resource_bind.start,resource_bind.end,resource_cleanup.start,resource_cleanup.end" {
		t.Fatalf("resource trace=%v", resourceSteps)
	}
}

func TestDocsCreatePreservesLogIDWithoutCommonAPIChanges(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response map[string]interface{}
		wantCode int
	}{
		{name: "API failure", response: map[string]interface{}{"code": 99991672, "msg": "missing scope"}, wantCode: 99991672},
		{name: "accepted task failure", response: map[string]interface{}{"code": 0, "data": map[string]interface{}{"task": map[string]interface{}{
			"task_id": "task_failed", "status": "failed", "failure": map[string]interface{}{"code": "execution_interrupted", "message": "worker stopped"},
		}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
			reg.Register(&httpmock.Stub{Method: "POST", URL: "/docs_ai/v1/documents", Status: 200,
				Headers: http.Header{"X-Tt-Logid": {"creation-response-log"}}, Body: tc.response,
			})
			err := runDocsCreateShortcut(t, f, stdout, []string{"+create", "--content", "<title>Log ID</title><p>Body</p>", "--as", "user"})
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.LogID != "creation-response-log" || problem.Code != tc.wantCode {
				t.Fatalf("err=%v problem=%+v", err, problem)
			}
			assertDocsCreateErrorHasNoTaskRecovery(t, err, "task_failed")
			if stdout.Len() != 0 {
				t.Fatalf("failure emitted success: %s", stdout)
			}
		})
	}
}

func TestDocsCreateWrapsClientInitializationError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := docsCreateTestConfig(t, "")
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	cause := errors.New("client initialization failed")
	f.HttpClient = func() (*http.Client, error) { return nil, cause }
	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), &cobra.Command{Use: "+create"}, cfg)
	runtime.Factory = f
	data, logID, err := createDocsDocumentWithLogID(runtime, map[string]interface{}{"content": "<p>Body</p>"})
	problem, ok := errs.ProblemOf(err)
	if data != nil || logID != "" || !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeUnknown || !errors.Is(err, cause) {
		t.Fatalf("data=%v logID=%q err=%v problem=%+v", data, logID, err, problem)
	}
}

func TestDocsCreateEmptyDataPreservesResponseLogID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/docs_ai/v1/documents", Status: 200,
		Headers: http.Header{"X-Tt-Logid": {"empty-response-log"}}, Body: map[string]interface{}{"code": 0},
	})
	err := runDocsCreateShortcut(t, f, stdout, []string{"+create", "--content", "<title>Empty response</title><p>Body</p>", "--as", "user"})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse || problem.LogID != "empty-response-log" {
		t.Fatalf("err=%v problem=%+v", err, problem)
	}
	if stdout.Len() != 0 {
		t.Fatalf("empty response emitted success: %s", stdout)
	}
}

// Assert the actual error envelope consumed by agents, including the absence of
// internal task recovery instructions on every unsuccessful polling outcome.
func assertDocsCreateErrorHasNoTaskRecovery(t *testing.T, err error, taskID string) {
	t.Helper()
	var stderr bytes.Buffer
	if !output.WriteTypedErrorEnvelope(&stderr, err, "user") {
		t.Fatal("error did not render as a typed envelope")
	}
	for _, forbidden := range []string{taskID, "async_tasks", "task_id", "Do not repeat"} {
		if strings.Contains(stderr.String(), forbidden) {
			t.Fatalf("error exposed task recovery %q: %s", forbidden, stderr.String())
		}
	}
}

func TestDocsCreateAsyncTerminalFailureGuidance(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   string
		failure  *docsCreateAsyncTaskFailure
		category errs.Category
		subtype  errs.Subtype
		batch    bool
	}{
		{name: "expired", status: "expired", category: errs.CategoryNetwork, subtype: errs.SubtypeNetworkTimeout, batch: true},
		{name: "interrupted", status: "failed", failure: &docsCreateAsyncTaskFailure{Code: "execution_interrupted", Message: "worker stopped"}, category: errs.CategoryNetwork, subtype: errs.SubtypeNetworkTimeout, batch: true},
		{name: "business failure", status: "failed", failure: &docsCreateAsyncTaskFailure{Code: "status_123", Message: "invalid document content"}, category: errs.CategoryAPI, subtype: errs.SubtypeServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := pollDocsCreateAsyncTask(context.Background(), nil, &docsCreateAsyncTask{
				TaskID: "task_terminal", Status: tc.status, Failure: tc.failure,
			}, "terminal-log", nil)
			problem, ok := errs.ProblemOf(err)
			if result != nil || !ok || problem.Category != tc.category || problem.Subtype != tc.subtype || problem.LogID != "terminal-log" || problem.Retryable {
				t.Fatalf("result=%v err=%v problem=%+v", result, err, problem)
			}
			if tc.batch {
				if !strings.Contains(problem.Message, "took too long") || !strings.Contains(problem.Hint, "--command append") {
					t.Fatalf("batch recovery missing: %+v", problem)
				}
			} else if !strings.Contains(problem.Message, tc.failure.Message) || strings.Contains(problem.Message, "took too long") {
				t.Fatalf("business failure was misreported: %+v", problem)
			}
			assertDocsCreateErrorHasNoTaskRecovery(t, err, "task_terminal")
		})
	}
}

func TestDocsCreateAsyncTimeoutEnvelope(t *testing.T) {
	err := docsCreateAsyncWaitError(context.DeadlineExceeded, "timeout-log")
	var stderr bytes.Buffer
	if !output.WriteTypedErrorEnvelope(&stderr, err, "user") {
		t.Fatal("timeout did not render as a typed envelope")
	}
	var envelope struct {
		OK       bool         `json:"ok"`
		Identity string       `json:"identity"`
		Error    errs.Problem `json:"error"`
	}
	if decodeErr := json.Unmarshal(stderr.Bytes(), &envelope); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if envelope.OK || envelope.Identity != "user" || envelope.Error.Category != errs.CategoryNetwork || envelope.Error.Subtype != errs.SubtypeNetworkTimeout || envelope.Error.LogID != "timeout-log" || output.ExitCodeOf(err) != 4 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("invalid timeout envelope: %s", stderr.String())
	}
	if envelope.Error.Message != "document processing took too long" || !strings.Contains(envelope.Error.Hint, "--command append") {
		t.Fatalf("incorrect timeout recovery: %s", stderr.String())
	}
	assertDocsCreateErrorHasNoTaskRecovery(t, err, "task_timeout")
	t.Log(stderr.String())
}

func TestDocsCreateBatchHintCommandsDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cmdutil.TestChdir(t, t.TempDir())
	if err := os.WriteFile("batch.xml", []byte("<p>Batch</p>"), 0o600); err != nil {
		t.Fatal(err)
	}
	commands := regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(docsCreateBatchHint, -1)
	if len(commands) != 2 {
		t.Fatalf("expected create and append commands in hint, got %q", docsCreateBatchHint)
	}
	for i, command := range commands {
		t.Run([]string{"create", "append"}[i], func(t *testing.T) {
			// Supply a content batch for each abbreviated command in the hint.
			example := strings.ReplaceAll(command[1], "<document_id>", "doxcnBatchHint")
			args := strings.Fields(example)
			if len(args) < 3 || args[0] != "lark-cli" || args[1] != "docs" {
				t.Fatalf("invalid command example: %q", example)
			}
			args = args[2:]
			for j := range args {
				args[j] = strings.Trim(args[j], "\"")
			}
			f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
			parent := &cobra.Command{Use: "docs", SilenceErrors: true, SilenceUsage: true}
			DocsCreate.Mount(parent, f)
			DocsUpdate.Mount(parent, f)
			parent.SetArgs(append(args, "--doc-format", "xml", "--content", "@./batch.xml", "--dry-run", "--as", "user"))
			if err := parent.Execute(); err != nil {
				t.Fatalf("hint command %q is invalid: %v", example, err)
			}
			var envelope struct {
				Data struct {
					API []struct {
						Method string `json:"method"`
						URL    string `json:"url"`
						Body   struct {
							Content string `json:"content"`
							Format  string `json:"format"`
							Command string `json:"command"`
							BlockID string `json:"block_id"`
						} `json:"body"`
					} `json:"api"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if len(envelope.Data.API) == 0 {
				t.Fatalf("hint command emitted no API call: %s", stdout)
			}
			api := envelope.Data.API[0]
			if i == 0 {
				if api.Method != "POST" || api.URL != "/open-apis/docs_ai/v1/documents" || api.Body.Content != "<p>Batch</p>" {
					t.Fatalf("create example did not create the first content batch: %s", stdout)
				}
			} else if api.Method != "PUT" || api.URL != "/open-apis/docs_ai/v1/documents/doxcnBatchHint" || api.Body.Command != "block_insert_after" || api.Body.BlockID != "-1" || api.Body.Format != "xml" || api.Body.Content != "<p>Batch</p>" {
				t.Fatalf("append example did not append the XML batch to the returned document: %s", stdout)
			}
		})
	}
}
