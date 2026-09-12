// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	docsCreateAsyncMaxWait             = 10 * time.Minute
	docsCreateAsyncDefaultPollInterval = 3 * time.Second
	docsCreateAsyncMinPollInterval     = 100 * time.Millisecond
	docsCreateAsyncMaxPollInterval     = 10 * time.Second
)

type docsCreateAsyncTask struct {
	TaskID      string                      `json:"task_id"`
	Type        string                      `json:"type"`
	Status      string                      `json:"status"`
	Stage       string                      `json:"stage"`
	PollAfterMS int                         `json:"poll_after_ms"`
	Result      *docsCreateAsyncTaskResult  `json:"result"`
	Failure     *docsCreateAsyncTaskFailure `json:"failure"`
}

type docsCreateAsyncTaskResult struct {
	CreateDocument string `json:"create_document"`
}

type docsCreateAsyncTaskFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type docsCreateAsyncEnvelope struct {
	Task *docsCreateAsyncTask `json:"task"`
}

// waitForDocsCreateAsyncTask preserves the synchronous docs +create contract:
// async success is projected back into the original document/warnings data
// shape before permission and local-resource follow-up work runs. A processing
// task is never a successful command result: timeout/cancellation/read failures
// return a typed error without repeating the write or exposing task recovery.
func waitForDocsCreateAsyncTask(runtime *common.RuntimeContext, initial map[string]interface{}, createLogID string, trace *docsCreateTrace) (map[string]interface{}, error) {
	envelope, err := decodeDocsCreateAsyncEnvelope(initial)
	if err != nil {
		return nil, err
	}
	if envelope.Task == nil {
		trace.event("create_mode", docsCreateDebugDetails{Mode: "direct_response"})
		return initial, nil
	}
	waitCtx, cancel := context.WithTimeout(runtime.Ctx(), docsCreateAsyncMaxWait)
	defer cancel()
	trace.event("create_mode", docsCreateDebugDetails{Mode: "async_task", TaskID: envelope.Task.TaskID})
	return pollDocsCreateAsyncTask(waitCtx, runtime, envelope.Task, createLogID, trace)
}

func pollDocsCreateAsyncTask(ctx context.Context, runtime *common.RuntimeContext, task *docsCreateAsyncTask, logID string, trace *docsCreateTrace) (result map[string]interface{}, err error) {
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"document create response included an async task without task_id")
	}
	defer func() {
		if err != nil {
			err = docsCreateAsyncWaitError(err, logID)
		}
	}()

	// The task endpoint waits on the server, so the first read needs no delay.
	// poll_after_ms applies when that read still returns processing.
	var delay time.Duration
	polls := 0
	trace.event("task_observed", docsCreateDebugDetails{TaskID: taskID, Status: task.Status, Stage: task.Stage, LogID: logID})
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		status := strings.ToLower(strings.TrimSpace(task.Status))
		switch status {
		case "succeeded":
			return decodeDocsCreateTaskResult(task)
		case "failed", "expired":
			return nil, docsCreateAsyncFailure(task, logID)
		case "", "processing":
			// Continue below. An empty status is treated as processing so a
			// temporarily sparse response does not trigger a duplicate create.
		default:
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"document creation returned unsupported status %q", task.Status)
		}

		if delay > 0 {
			trace.event("poll_sleep", docsCreateDebugDetails{WaitMS: delay.Milliseconds()})
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}

		polls++
		trace.event("poll_request", docsCreateDebugDetails{Poll: polls, TaskID: taskID})
		pollStart := time.Now()
		polled, polledLogID, pollErr := getDocsCreateAsyncTask(ctx, runtime, taskID)
		trace.event("poll_response", docsCreateDebugDetails{Poll: polls, LogID: polledLogID, DurationMS: milliseconds(time.Since(pollStart))})
		if polledLogID != "" {
			logID = polledLogID
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if pollErr != nil {
			trace.event("poll_error", docsCreateDebugError(pollErr))
			if !retryableDocsCreateTaskRead(pollErr) {
				return nil, pollErr
			}
			// Only the GET is retried. Keep server-provided minimum delays, and
			// back off repeated failures within the command's wait deadline.
			delay = min(max(min(delay, docsCreateAsyncMaxPollInterval)*2, docsCreateAsyncDefaultPollInterval), docsCreateAsyncMaxPollInterval)
			if retryAfter, ok := errs.RetryAfter(pollErr); ok && retryAfter > delay {
				delay = retryAfter
			}
			trace.event("poll_retry", docsCreateDebugDetails{Poll: polls, WaitMS: delay.Milliseconds()})
			continue
		}
		decoded, decodeErr := decodeDocsCreateAsyncEnvelope(polled)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if decoded.Task == nil {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"document creation status response omitted task")
		}
		if polledID := strings.TrimSpace(decoded.Task.TaskID); polledID != "" && polledID != taskID {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"async-task response returned a mismatched task_id")
		}
		task = decoded.Task
		trace.event("task_observed", docsCreateDebugDetails{Poll: polls, TaskID: taskID, Status: task.Status, Stage: task.Stage, LogID: logID})
		delay = docsCreateAsyncPollInterval(task.PollAfterMS)
	}
}

// Keep successful-response log IDs local to document creation: the API may
// return code=0 with a failed task that the command classifies afterward.
func createDocsDocumentWithLogID(runtime *common.RuntimeContext, body interface{}) (map[string]interface{}, string, error) {
	return callDocsCreateAPIWithLogID(runtime.Ctx(), runtime, &larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/docs_ai/v1/documents",
		Body:       body,
	})
}

func getDocsCreateAsyncTask(ctx context.Context, runtime *common.RuntimeContext, taskID string) (map[string]interface{}, string, error) {
	return callDocsCreateAPIWithLogID(ctx, runtime, &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/docs_ai/v1/async_tasks/%s", url.PathEscape(taskID)),
	})
}

func callDocsCreateAPIWithLogID(ctx context.Context, runtime *common.RuntimeContext, req *larkcore.ApiReq) (map[string]interface{}, string, error) {
	resp, err := runtime.DoAPIWithContext(ctx, req)
	if err != nil {
		if !errs.IsTyped(err) {
			err = errs.NewInternalError(errs.SubtypeUnknown, "%v", err).WithCause(err)
		}
		return nil, "", err
	}
	data, err := runtime.ClassifyAPIResponse(resp)
	logID := ""
	if resp != nil {
		logID = resp.Header.Get("X-Tt-Logid")
	}
	if err == nil && data == nil {
		err = errs.NewInternalError(errs.SubtypeInvalidResponse, "document API returned an empty data object").WithLogID(logID)
	}
	return data, logID, err
}

func retryableDocsCreateTaskRead(err error) bool {
	if errs.IsRetryable(err) {
		return true
	}
	// The SDK's transport classifier does not mark requests replay-safe. These
	// transient transport subtypes are safe to retry for this read-only GET.
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork {
		return false
	}
	switch problem.Subtype {
	case errs.SubtypeNetworkTimeout, errs.SubtypeNetworkDNS, errs.SubtypeNetworkTransport:
		return true
	default:
		return false
	}
}

const docsCreateBatchHint = "first use `lark-cli docs +create` to create part of the content and obtain the document token (document_id), then use `lark-cli docs +update --doc \"<document_id>\" --command append` to append the remaining content."

func docsCreateAsyncWaitError(err error, logID string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		err = errs.NewNetworkError(errs.SubtypeNetworkTimeout, "document processing took too long").WithCause(err).WithHint(docsCreateBatchHint)
	} else if errors.Is(err, context.Canceled) {
		err = errs.NewNetworkError(errs.SubtypeNetworkTransport, "document creation was canceled").WithCause(err)
	} else if !errs.IsTyped(err) {
		err = errs.NewInternalError(errs.SubtypeUnknown, "stopped waiting for document creation").WithCause(err)
	}
	if problem, ok := errs.ProblemOf(err); ok && problem.LogID == "" {
		problem.LogID = logID
	}
	return err
}

func decodeDocsCreateAsyncEnvelope(data map[string]interface{}) (*docsCreateAsyncEnvelope, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"failed to inspect document create response").WithCause(err)
	}
	var envelope docsCreateAsyncEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"failed to decode document create response").WithCause(err)
	}
	return &envelope, nil
}

func decodeDocsCreateTaskResult(task *docsCreateAsyncTask) (map[string]interface{}, error) {
	if task == nil || task.Result == nil || strings.TrimSpace(task.Result.CreateDocument) == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"successful document creation task omitted create_document result")
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(task.Result.CreateDocument), &result); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"successful document creation task returned invalid create_document JSON").WithCause(err)
	}
	if result == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"successful document creation task returned an empty create_document result")
	}
	return result, nil
}

func docsCreateAsyncPollInterval(pollAfterMS int) time.Duration {
	interval := time.Duration(pollAfterMS) * time.Millisecond
	if interval <= 0 {
		return docsCreateAsyncDefaultPollInterval
	}
	if interval < docsCreateAsyncMinPollInterval {
		return docsCreateAsyncMinPollInterval
	}
	if interval > docsCreateAsyncMaxPollInterval {
		return docsCreateAsyncMaxPollInterval
	}
	return interval
}

func docsCreateAsyncFailure(task *docsCreateAsyncTask, logID string) error {
	status := strings.ToLower(strings.TrimSpace(task.Status))
	message := status
	code := ""
	if task.Failure != nil {
		code = strings.TrimSpace(task.Failure.Code)
		if failureMessage := strings.TrimSpace(task.Failure.Message); failureMessage != "" {
			message = failureMessage
		}
	}
	if status == "expired" || code == "execution_interrupted" {
		return errs.NewNetworkError(errs.SubtypeNetworkTimeout,
			"document processing took too long").
			WithHint(docsCreateBatchHint).WithLogID(logID)
	}
	if message == "" {
		message = "unknown failure"
	}
	if code != "" {
		message += " (code: " + code + ")"
	}
	return errs.NewAPIError(errs.SubtypeServerError,
		"document creation failed: %s", message).WithLogID(logID)
}
