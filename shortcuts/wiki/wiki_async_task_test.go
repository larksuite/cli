// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// pollWikiAsyncTask is shared infrastructure for every wiki delete shortcut,
// so it gets a dedicated test surface here rather than relying only on the
// transitive coverage from the delete-space / delete-node paths.

func TestPollWikiAsyncTaskSuccessFirstPoll(t *testing.T) {
	t.Parallel()

	runtime, stderr := newWikiNodeDeleteRuntime(t, core.AsUser)
	status, ready, err := pollWikiAsyncTask(
		context.Background(), runtime, "task_ok", "delete-node", 3, 0,
		func(context.Context, string) (wikiAsyncTaskStatus, error) {
			return wikiAsyncTaskStatus{Status: "success"}, nil
		},
		"resume-cmd",
	)
	if err != nil {
		t.Fatalf("pollWikiAsyncTask() error = %v", err)
	}
	if !ready || !status.Ready() {
		t.Fatalf("ready = %v, status = %+v, want ready", ready, status)
	}
	if !strings.Contains(stderr.String(), "delete-node task completed successfully") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPollWikiAsyncTaskFailureIsTerminal(t *testing.T) {
	t.Parallel()

	runtime, _ := newWikiNodeDeleteRuntime(t, core.AsUser)
	_, ready, err := pollWikiAsyncTask(
		context.Background(), runtime, "task_x", "delete-node", 3, 0,
		func(context.Context, string) (wikiAsyncTaskStatus, error) {
			return wikiAsyncTaskStatus{Status: "failure", StatusMsg: "denied"}, nil
		},
		"resume-cmd",
	)
	if ready {
		t.Fatalf("ready = true, want false on failure")
	}
	if err == nil || !strings.Contains(err.Error(), "delete-node task task_x failed: denied") {
		t.Fatalf("err = %v, want terminal failure with reason", err)
	}
}

func TestPollWikiAsyncTaskTimeoutWhenAlwaysProcessing(t *testing.T) {
	t.Parallel()

	runtime, _ := newWikiNodeDeleteRuntime(t, core.AsUser)
	status, ready, err := pollWikiAsyncTask(
		context.Background(), runtime, "task_slow", "delete-space", 2, 0,
		func(context.Context, string) (wikiAsyncTaskStatus, error) {
			return wikiAsyncTaskStatus{Status: "processing"}, nil
		},
		"resume-cmd",
	)
	// A still-processing task after the bounded window is a soft timeout:
	// no error, ready=false, status preserved so the caller can print the
	// follow-up command.
	if err != nil {
		t.Fatalf("pollWikiAsyncTask() error = %v, want nil on timeout", err)
	}
	if ready {
		t.Fatalf("ready = true, want false on timeout")
	}
	if status.StatusCode() != "processing" {
		t.Fatalf("status = %+v, want processing preserved", status)
	}
}

func TestPollWikiAsyncTaskAllPollsFailWrapsWithResumeHint(t *testing.T) {
	t.Parallel()

	runtime, stderr := newWikiNodeDeleteRuntime(t, core.AsUser)
	_, ready, err := pollWikiAsyncTask(
		context.Background(), runtime, "task_lost", "delete-node", 2, 0,
		func(context.Context, string) (wikiAsyncTaskStatus, error) {
			return wikiAsyncTaskStatus{}, errors.New("transport boom")
		},
		"lark-cli drive +task_result --task-id task_lost",
	)
	if ready {
		t.Fatalf("ready = true, want false when every poll failed")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("err = %T %v, want *output.ExitError with detail", err, err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Fatalf("exit code = %d, want ExitAPI", exitErr.Code)
	}
	if !strings.Contains(exitErr.Detail.Hint, "every status poll failed (task_id=task_lost)") ||
		!strings.Contains(exitErr.Detail.Hint, "lark-cli drive +task_result --task-id task_lost") {
		t.Fatalf("hint = %q, want resume guidance naming the task", exitErr.Detail.Hint)
	}
	if !strings.Contains(stderr.String(), "attempt 2/2 failed") {
		t.Fatalf("stderr = %q, want per-attempt progress", stderr.String())
	}
}

func TestPollWikiAsyncTaskPrependsUpstreamExitHint(t *testing.T) {
	t.Parallel()

	runtime, _ := newWikiNodeDeleteRuntime(t, core.AsUser)
	upstream := &output.ExitError{
		Code: output.ExitAPI,
		Detail: &output.ErrDetail{
			Type:    "permission",
			Code:    99991663,
			Message: "permission denied",
			Hint:    "grant the wiki:node:retrieve scope",
		},
	}
	_, _, err := pollWikiAsyncTask(
		context.Background(), runtime, "task_perm", "delete-node", 1, 0,
		func(context.Context, string) (wikiAsyncTaskStatus, error) {
			return wikiAsyncTaskStatus{}, upstream
		},
		"resume-cmd",
	)
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("err = %T %v, want *output.ExitError", err, err)
	}
	// The upstream hint must lead so the actionable cause is read first, with
	// the resume guidance appended. Type and exit code propagate from upstream.
	if !strings.HasPrefix(exitErr.Detail.Hint, "grant the wiki:node:retrieve scope\n") {
		t.Fatalf("hint = %q, want upstream hint prepended", exitErr.Detail.Hint)
	}
	if !strings.Contains(exitErr.Detail.Hint, "resume-cmd") {
		t.Fatalf("hint = %q, want resume command appended", exitErr.Detail.Hint)
	}
	if exitErr.Detail.Type != "permission" || exitErr.Code != output.ExitAPI {
		t.Fatalf("exitErr = %+v, want permission/ExitAPI propagated", exitErr)
	}
	if exitErr.Detail.Message != "permission denied" {
		t.Fatalf("message = %q, want upstream message preserved", exitErr.Detail.Message)
	}
}

func TestPollWikiAsyncTaskHonoursContextCancellation(t *testing.T) {
	t.Parallel()

	runtime, _ := newWikiNodeDeleteRuntime(t, core.AsUser)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, ready, err := pollWikiAsyncTask(
		ctx, runtime, "task_cancel", "delete-node", 5, time.Hour,
		func(context.Context, string) (wikiAsyncTaskStatus, error) {
			calls++
			cancel() // cancel before the next attempt's inter-poll wait
			return wikiAsyncTaskStatus{Status: "processing"}, nil
		},
		"resume-cmd",
	)
	if ready {
		t.Fatalf("ready = true, want false on cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("fetcher calls = %d, want 1 (cancelled before second poll)", calls)
	}
}
