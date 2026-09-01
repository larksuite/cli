// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
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
// shape before permission and local-resource follow-up work runs. If the
// bounded wait expires, the latest processing task remains a successful,
// resumable result instead of being mistaken for a completed document.
func waitForDocsCreateAsyncTask(runtime *common.RuntimeContext, initial map[string]interface{}) (map[string]interface{}, bool, error) {
	envelope, err := decodeDocsCreateAsyncEnvelope(initial)
	if err != nil {
		return nil, false, err
	}
	if envelope.Task == nil {
		return initial, false, nil
	}

	taskID := strings.TrimSpace(envelope.Task.TaskID)
	if taskID == "" {
		return nil, false, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"document create response included an async task without task_id")
	}

	deadline := time.Now().Add(docsCreateAsyncMaxWait)
	latest := initial
	task := envelope.Task
	polledOnce := false
	for {
		status := strings.ToLower(strings.TrimSpace(task.Status))
		switch status {
		case "succeeded":
			result, decodeErr := decodeDocsCreateTaskResult(task)
			return result, false, decodeErr
		case "failed", "expired":
			return nil, false, docsCreateAsyncFailure(task)
		case "", "processing":
			// Continue below. An empty status is treated as processing so a
			// temporarily sparse response does not trigger a duplicate create.
		default:
			return nil, false, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"document creation task %s returned unsupported status %q", taskID, task.Status)
		}

		if !time.Now().Before(deadline) {
			return latest, true, nil
		}
		interval := docsCreateAsyncPollInterval(task.PollAfterMS)
		if polledOnce {
			if remaining := time.Until(deadline); interval > remaining {
				interval = remaining
			}
			timer := time.NewTimer(interval)
			select {
			case <-runtime.Ctx().Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return nil, false, runtime.Ctx().Err()
			case <-timer.C:
			}
		}

		polledOnce = true
		polled, pollErr := doDocAPI(runtime, "GET",
			fmt.Sprintf("/open-apis/docs_ai/v1/async_tasks/%s", url.PathEscape(taskID)), nil)
		if pollErr != nil {
			// The create was already accepted. Keep transient status-read failures
			// inside the bounded polling budget so callers do not retry the write.
			continue
		}
		decoded, decodeErr := decodeDocsCreateAsyncEnvelope(polled)
		if decodeErr != nil {
			return nil, false, decodeErr
		}
		if decoded.Task == nil {
			return nil, false, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"async-task response for document creation task %s omitted task", taskID)
		}
		if polledID := strings.TrimSpace(decoded.Task.TaskID); polledID != "" && polledID != taskID {
			return nil, false, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"async-task response returned a mismatched task_id")
		}
		latest = polled
		task = decoded.Task
	}
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

func docsCreateAsyncFailure(task *docsCreateAsyncTask) error {
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
	return errs.NewAPIError(errs.SubtypeServerError,
		"document creation task %s %s: %s", taskID, status, message).
		WithHint("query GET /open-apis/docs_ai/v1/async_tasks/%s for the terminal task record", url.PathEscape(taskID))
}
