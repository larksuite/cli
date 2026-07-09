// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
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
	transportErr := errors.New("transport boom")
	_, ready, err := pollWikiAsyncTask(
		context.Background(), runtime, "task_lost", "delete-node", 2, 0,
		func(context.Context, string) (wikiAsyncTaskStatus, error) {
			return wikiAsyncTaskStatus{}, transportErr
		},
		"lark-cli drive +task_result --task-id task_lost",
	)
	if ready {
		t.Fatalf("ready = true, want false when every poll failed")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %T %v, want a typed errs.* error", err, err)
	}
	if p.Subtype != errs.SubtypeUnknown {
		t.Fatalf("subtype = %q, want unknown for an untyped poll failure", p.Subtype)
	}
	if !errors.Is(err, transportErr) {
		t.Fatalf("err does not preserve the transport cause: %v", err)
	}
	if !strings.Contains(p.Hint, "every status poll failed (task_id=task_lost)") ||
		!strings.Contains(p.Hint, "lark-cli drive +task_result --task-id task_lost") {
		t.Fatalf("hint = %q, want resume guidance naming the task", p.Hint)
	}
	if !strings.Contains(stderr.String(), "attempt 2/2 failed") {
		t.Fatalf("stderr = %q, want per-attempt progress", stderr.String())
	}
}

func TestParseWikiAsyncTaskStatusRejectsNilTask(t *testing.T) {
	t.Parallel()

	_, err := parseWikiAsyncTaskStatus("task_x", nil, "delete_space_result")
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("expected internal/invalid_response, got %v", err)
	}
}

func TestPollWikiAsyncTaskPrependsUpstreamExitHint(t *testing.T) {
	t.Parallel()

	runtime, _ := newWikiNodeDeleteRuntime(t, core.AsUser)
	// The upstream poll error is a typed error carrying its own hint, mirroring
	// what runtime.CallAPITyped produces for a permission failure.
	upstream := errs.NewPermissionError(errs.SubtypePermissionDenied, "permission denied").
		WithHint("grant the wiki:node:retrieve scope")
	_, _, err := pollWikiAsyncTask(
		context.Background(), runtime, "task_perm", "delete-node", 1, 0,
		func(context.Context, string) (wikiAsyncTaskStatus, error) {
			return wikiAsyncTaskStatus{}, upstream
		},
		"resume-cmd",
	)
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %T %v, want a typed errs.* error", err, err)
	}
	// The upstream hint must lead so the actionable cause is read first, with
	// the resume guidance appended. The original typed error propagates in place.
	if !strings.HasPrefix(p.Hint, "grant the wiki:node:retrieve scope\n") {
		t.Fatalf("hint = %q, want upstream hint prepended", p.Hint)
	}
	if !strings.Contains(p.Hint, "resume-cmd") {
		t.Fatalf("hint = %q, want resume command appended", p.Hint)
	}
	if p.Subtype != errs.SubtypePermissionDenied {
		t.Fatalf("subtype = %q, want permission_denied propagated", p.Subtype)
	}
	if p.Message != "permission denied" {
		t.Fatalf("message = %q, want upstream message preserved", p.Message)
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
