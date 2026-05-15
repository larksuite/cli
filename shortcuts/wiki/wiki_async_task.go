// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// Shared async-task polling for wiki delete operations. The wiki delete
// endpoints (DELETE /spaces/{id}, DELETE /spaces/{id}/nodes/{token}) may
// return either an empty task_id (sync completion) or a task_id that must
// be polled against /wiki/v2/tasks/{task_id}?task_type=<...>.
//
// For historical reasons /wiki/v2/tasks/{task_id} stashes the status under a
// different key per task type: delete-space uses `delete_space_result`, while
// delete-node uses the generic `simple_task_result` (the gateway's reusable
// "future async tasks share this" field). move tasks use `move_result` and are
// handled separately in wiki_move.go. Every key still exposes a `status`, so
// the poll loop / classification is factored out here and the caller passes
// the right result key.
//
// Note: `simple_task_result` only carries `status` (no `status_msg`), so for
// delete-node StatusLabel() falls back to the status code — which is fine.

const (
	wikiAsyncStatusSuccess    = "success"
	wikiAsyncStatusFailure    = "failure"
	wikiAsyncStatusProcessing = "processing"

	wikiAsyncTaskTypeDeleteSpace = "delete_space"
	wikiAsyncTaskTypeDeleteNode  = "delete_node"

	wikiAsyncResultDeleteSpace = "delete_space_result"
	// wikiAsyncResultSimpleTask is the generic result key the gateway uses for
	// delete-node (and intends to reuse for future async task types). It is
	// NOT `delete_node_result` — that key does not exist in the response.
	wikiAsyncResultSimpleTask = "simple_task_result"
)

// wikiAsyncTaskStatus is the unified poll-response shape used by every wiki
// delete task. The taskID is captured so error/resume hints can name it.
type wikiAsyncTaskStatus struct {
	TaskID    string
	Status    string
	StatusMsg string
}

// normalizedStatus collapses whitespace and case so "  SUCCESS  " classifies
// the same as "success". Ready()/Failed() (control flow) derive from this;
// StatusCode()/StatusLabel() (display) deliberately surface the raw backend
// value instead. For the real status enums (delete-node: processing/success/
// failed; delete-space's documented set) the two agree. They only diverge for
// an undocumented status string, which is intentional — an unrecognized status
// is shown verbatim rather than masked as a hard failure.
func (s wikiAsyncTaskStatus) normalizedStatus() string {
	return strings.ToLower(strings.TrimSpace(s.Status))
}

func (s wikiAsyncTaskStatus) Ready() bool {
	return s.normalizedStatus() == wikiAsyncStatusSuccess
}

func (s wikiAsyncTaskStatus) Failed() bool {
	// The sample protocol only documents "success" as a terminal OK. Treat any
	// explicit "failure"/"failed" signal as terminal, and unknown non-success
	// values as still-processing so we don't misreport a novel status as a hard
	// failure.
	lowered := s.normalizedStatus()
	return lowered == wikiAsyncStatusFailure || lowered == "failed"
}

// StatusCode returns a never-empty status value for the output envelope. If
// the backend response omits delete_*_result.status (or sends whitespace),
// fall back to "processing" so the documented timeout-shape stays accurate.
func (s wikiAsyncTaskStatus) StatusCode() string {
	if status := strings.TrimSpace(s.Status); status != "" {
		return status
	}
	return wikiAsyncStatusProcessing
}

func (s wikiAsyncTaskStatus) StatusLabel() string {
	if msg := strings.TrimSpace(s.StatusMsg); msg != "" {
		return msg
	}
	return s.StatusCode()
}

// wikiAsyncTaskFetcher returns the latest status for taskID. Implementations
// translate from runtime.CallAPI responses or test fakes.
type wikiAsyncTaskFetcher func(ctx context.Context, taskID string) (wikiAsyncTaskStatus, error)

// parseWikiAsyncTaskStatus normalizes an /wiki/v2/tasks/{task_id} payload.
// resultKey selects the right shape ("delete_space_result" for delete-space,
// "simple_task_result" for delete-node).
func parseWikiAsyncTaskStatus(taskID string, task map[string]interface{}, resultKey string) (wikiAsyncTaskStatus, error) {
	if task == nil {
		return wikiAsyncTaskStatus{}, output.Errorf(output.ExitAPI, "api_error", "wiki task response missing task")
	}

	result := common.GetMap(task, resultKey)
	status := wikiAsyncTaskStatus{
		TaskID: common.GetString(task, "task_id"),
	}
	if status.TaskID == "" {
		status.TaskID = taskID
	}
	if result != nil {
		status.Status = common.GetString(result, "status")
		status.StatusMsg = common.GetString(result, "status_msg")
	}
	return status, nil
}

// pollWikiAsyncTask runs the bounded polling loop shared by every wiki delete
// shortcut. label is the human-readable operation name surfaced in stderr
// progress lines ("delete-space" / "delete-node"). nextCommand is the resume
// hint embedded into the wrapped error when every poll fails.
//
// attempts/interval are taken as parameters (instead of consts) so callers
// can keep their per-operation tunable constants for back-compat with the
// existing test hooks.
func pollWikiAsyncTask(
	ctx context.Context,
	runtime *common.RuntimeContext,
	taskID, label string,
	attempts int,
	interval time.Duration,
	fetcher wikiAsyncTaskFetcher,
	nextCommand string,
) (wikiAsyncTaskStatus, bool, error) {
	lastStatus := wikiAsyncTaskStatus{TaskID: taskID}
	var lastErr error
	hadSuccessfulPoll := false

	// The delete request already succeeded. Treat poll failures as transient
	// until every attempt fails, then return a resume hint instead of
	// discarding the task identifier.
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return lastStatus, false, ctx.Err()
			case <-time.After(interval):
			}
		}

		status, err := fetcher(ctx, taskID)
		if err != nil {
			lastErr = err
			fmt.Fprintf(runtime.IO().ErrOut, "Wiki %s status attempt %d/%d failed: %v\n", label, attempt, attempts, err)
			continue
		}
		lastStatus = status
		hadSuccessfulPoll = true

		if status.Ready() {
			fmt.Fprintf(runtime.IO().ErrOut, "Wiki %s task completed successfully.\n", label)
			return status, true, nil
		}
		if status.Failed() {
			return status, false, output.Errorf(output.ExitAPI, "api_error", "wiki %s task %s failed: %s", label, taskID, status.StatusLabel())
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Wiki %s status %d/%d: %s\n", label, attempt, attempts, status.StatusLabel())
	}

	if !hadSuccessfulPoll && lastErr != nil {
		hint := fmt.Sprintf(
			"the wiki %s task was created but every status poll failed (task_id=%s)\nretry status lookup with: %s",
			label, taskID, nextCommand,
		)
		var exitErr *output.ExitError
		if errors.As(lastErr, &exitErr) && exitErr.Detail != nil {
			if strings.TrimSpace(exitErr.Detail.Hint) != "" {
				hint = exitErr.Detail.Hint + "\n" + hint
			}
			// ErrWithHint rebuilds the error and drops the upstream Lark
			// Detail.Code / ConsoleURL / Risk / nested Detail. Build the
			// ExitError by hand so the original API code survives a fully
			// failed poll, matching wrapWikiNodeDeleteAPIError.
			return lastStatus, false, &output.ExitError{
				Code: exitErr.Code,
				Detail: &output.ErrDetail{
					Type:       exitErr.Detail.Type,
					Code:       exitErr.Detail.Code,
					Message:    exitErr.Detail.Message,
					Hint:       hint,
					ConsoleURL: exitErr.Detail.ConsoleURL,
					Risk:       exitErr.Detail.Risk,
					Detail:     exitErr.Detail.Detail,
				},
			}
		}
		return lastStatus, false, output.ErrWithHint(output.ExitAPI, "api_error", lastErr.Error(), hint)
	}

	return lastStatus, false, nil
}
