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
// return an error that identifies the accepted task without repeating the write.
func waitForDocsCreateAsyncTask(runtime *common.RuntimeContext, initial map[string]interface{}, createLogID string) (map[string]interface{}, error) {
	envelope, err := decodeDocsCreateAsyncEnvelope(initial)
	if err != nil {
		return nil, err
	}
	if envelope.Task == nil {
		return initial, nil
	}
	waitCtx, cancel := context.WithTimeout(runtime.Ctx(), docsCreateAsyncMaxWait)
	defer cancel()
	return pollDocsCreateAsyncTask(waitCtx, runtime, envelope.Task, createLogID)
}

func pollDocsCreateAsyncTask(ctx context.Context, runtime *common.RuntimeContext, task *docsCreateAsyncTask, logID string) (result map[string]interface{}, err error) {
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"document create response included an async task without task_id")
	}
	defer func() {
		if err != nil {
			err = docsCreateAsyncWaitError(err, taskID, logID)
		}
	}()

	var delay time.Duration
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
				"document creation task %s returned unsupported status %q", taskID, task.Status)
		}

		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}

		polled, polledLogID, pollErr := getDocsCreateAsyncTask(ctx, runtime, taskID)
		if polledLogID != "" {
			logID = polledLogID
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if pollErr != nil {
			if !retryableDocsCreateTaskRead(pollErr) {
				return nil, pollErr
			}
			// Only the GET is retried. Keep server-provided minimum delays, and
			// back off repeated failures within the command's wait deadline.
			delay = min(max(min(delay, docsCreateAsyncMaxPollInterval)*2, docsCreateAsyncDefaultPollInterval), docsCreateAsyncMaxPollInterval)
			if retryAfter, ok := errs.RetryAfter(pollErr); ok && retryAfter > delay {
				delay = retryAfter
			}
			continue
		}
		decoded, decodeErr := decodeDocsCreateAsyncEnvelope(polled)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if decoded.Task == nil {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"async-task response for document creation task %s omitted task", taskID)
		}
		if polledID := strings.TrimSpace(decoded.Task.TaskID); polledID != "" && polledID != taskID {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"async-task response returned a mismatched task_id")
		}
		task = decoded.Task
		delay = docsCreateAsyncPollInterval(task.PollAfterMS)
	}
}

func getDocsCreateAsyncTask(ctx context.Context, runtime *common.RuntimeContext, taskID string) (map[string]interface{}, string, error) {
	resp, err := runtime.DoAPIWithContext(ctx, &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/docs_ai/v1/async_tasks/%s", url.PathEscape(taskID)),
	})
	if err != nil {
		return nil, "", err
	}
	data, err := runtime.ClassifyAPIResponse(resp)
	logID := ""
	if resp != nil {
		logID = resp.Header.Get("X-Tt-Logid")
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

func docsCreateAsyncWaitError(err error, taskID, logID string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		err = errs.NewNetworkError(errs.SubtypeNetworkTimeout, "timed out waiting for document creation task %s", taskID).WithCause(err)
	} else if !errs.IsTyped(err) {
		err = errs.NewInternalError(errs.SubtypeUnknown, "stopped waiting for document creation task %s: %v", taskID, err).WithCause(err)
	}
	if problem, ok := errs.ProblemOf(err); ok {
		if problem.LogID == "" {
			problem.LogID = logID
		}
		hint := fmt.Sprintf("Creation was already accepted as task %s. Do not repeat docs +create; query GET /open-apis/docs_ai/v1/async_tasks/%s to check its outcome. CLI permission and resource-upload follow-up has not run.", taskID, url.PathEscape(taskID))
		problem.Hint = strings.TrimSpace(problem.Hint + " " + hint)
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
	taskID := strings.TrimSpace(task.TaskID)
	status := strings.ToLower(strings.TrimSpace(task.Status))
	message := status
	code := ""
	if task.Failure != nil {
		code = strings.TrimSpace(task.Failure.Code)
		if failureMessage := strings.TrimSpace(task.Failure.Message); failureMessage != "" {
			message = failureMessage
		}
	}
	if message == "" {
		message = "unknown failure"
	}
	if code != "" {
		message += " (code: " + code + ")"
	}
	err := errs.NewAPIError(errs.SubtypeServerError,
		"document creation task %s %s: %s", taskID, status, message).
		WithHint("query GET /open-apis/docs_ai/v1/async_tasks/%s for the terminal task record", url.PathEscape(taskID))
	if logID != "" {
		err = err.WithLogID(logID)
	}
	return err
}
