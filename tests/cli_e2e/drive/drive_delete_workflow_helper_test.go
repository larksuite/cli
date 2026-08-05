// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteAsyncAndVerify(t *testing.T) {
	t.Run("sync delete without task_id skips task_result", func(t *testing.T) {
		fake := mustWriteDriveDeleteWorkflowFakeCLI(t)
		t.Setenv(clie2e.EnvBinaryPath, fake)
		t.Setenv("FAKE_WORKFLOW_DELETE_MODE", "sync")
		t.Setenv("FAKE_WORKFLOW_META_MODE", "gone")
		counters := setupFakeWorkflowCounters(t)

		taskID := deleteAsyncAndVerify(t, context.Background(), "docx_sync", "docx")
		assert.Empty(t, taskID)

		assert.Equal(t, "1", readFakeCounter(t, counters.deletes), "sync path must delete exactly once")
		assert.Equal(t, "1", readFakeCounter(t, counters.metas), "sync path must still verify the resource is gone")
		assert.Equal(t, "0", readFakeCounter(t, counters.taskResults), "sync path must not query task status")
	})

	t.Run("transient failure with resource gone is tolerated", func(t *testing.T) {
		fake := mustWriteDriveDeleteWorkflowFakeCLI(t)
		t.Setenv(clie2e.EnvBinaryPath, fake)
		t.Setenv("FAKE_WORKFLOW_DELETE_MODE", "fail")
		t.Setenv("FAKE_WORKFLOW_META_MODE", "gone")
		counters := setupFakeWorkflowCounters(t)

		taskID := deleteAsyncAndVerify(t, context.Background(), "docx_transient", "docx")
		assert.Empty(t, taskID)

		assert.Equal(t, "1", readFakeCounter(t, counters.deletes), "resource already gone must not trigger another delete attempt")
		assert.Equal(t, "1", readFakeCounter(t, counters.metas), "transient failure must verify the terminal state")
		assert.Equal(t, "0", readFakeCounter(t, counters.taskResults))
	})

	t.Run("failed delete retries until async success", func(t *testing.T) {
		fake := mustWriteDriveDeleteWorkflowFakeCLI(t)
		t.Setenv(clie2e.EnvBinaryPath, fake)
		t.Setenv("FAKE_WORKFLOW_DELETE_MODE", "fail-then-async")
		t.Setenv("FAKE_WORKFLOW_META_MODE", "exists-then-gone")
		t.Setenv("FAKE_WORKFLOW_TASK_RESULT_OK", "1")
		counters := setupFakeWorkflowCounters(t)
		withFastDeleteWorkflowBackoff(t)

		taskID := deleteAsyncAndVerify(t, context.Background(), "docx_retry", "docx")
		assert.Equal(t, "task_123", taskID)

		assert.Equal(t, "2", readFakeCounter(t, counters.deletes))
		assert.Equal(t, "2", readFakeCounter(t, counters.metas), "one terminal-state check after the failure plus the final visibility wait")
		assert.Equal(t, "1", readFakeCounter(t, counters.taskResults), "async success must verify the task result")
	})
}

// TestIsTransientDriveDeleteFailure locks the tolerance boundary: only the one
// verified backend transient may fall through to terminal-state checking, so a
// crash, a protocol regression, or any other error keeps failing the workflow
// even when the resource happens to be gone.
func TestIsTransientDriveDeleteFailure(t *testing.T) {
	t.Run("matches compact envelope", func(t *testing.T) {
		result := &clie2e.Result{
			ExitCode: 1,
			Stderr:   "Deleting docx tok...\n{\"ok\":false,\"identity\":\"bot\",\"error\":{\"type\":\"api\",\"subtype\":\"server_error\",\"message\":\"drive task failed\"}}",
		}
		assert.True(t, isTransientDriveDeleteFailure(result))
	})

	t.Run("matches pretty-printed envelope from CI", func(t *testing.T) {
		result := &clie2e.Result{
			ExitCode: 1,
			Stderr: "Deleting docx NTw0...Rngb...\nDelete is async, polling task schedule|7663369798226545963...\n" +
				"{\n  \"ok\": false,\n  \"identity\": \"bot\",\n  \"error\": {\n    \"type\": \"api\",\n    \"subtype\": \"server_error\",\n    \"message\": \"drive task failed\"\n  }\n}",
		}
		assert.True(t, isTransientDriveDeleteFailure(result))
	})

	t.Run("rejects other server errors", func(t *testing.T) {
		result := &clie2e.Result{
			ExitCode: 1,
			Stderr:   "{\"ok\":false,\"identity\":\"bot\",\"error\":{\"type\":\"api\",\"subtype\":\"server_error\",\"message\":\"internal error\"}}",
		}
		assert.False(t, isTransientDriveDeleteFailure(result))
	})

	t.Run("rejects non-server-error subtypes", func(t *testing.T) {
		result := &clie2e.Result{
			ExitCode: 1,
			Stderr:   "{\"ok\":false,\"identity\":\"bot\",\"error\":{\"type\":\"api\",\"subtype\":\"permission_denied\",\"message\":\"drive task failed\"}}",
		}
		assert.False(t, isTransientDriveDeleteFailure(result))
	})

	t.Run("rejects non-JSON output", func(t *testing.T) {
		result := &clie2e.Result{ExitCode: 2, Stderr: "panic: runtime error"}
		assert.False(t, isTransientDriveDeleteFailure(result))
	})

	t.Run("rejects nil result", func(t *testing.T) {
		assert.False(t, isTransientDriveDeleteFailure(nil))
	})
}

// TestDeleteAsyncAndVerifyRejectsUnexpectedFailure locks the P1 boundary
// end-to-end by re-running this test binary as a subprocess that really calls
// deleteAsyncAndVerify: an unrelated non-zero exit must fail the helper
// immediately — no terminal-state check may rescue it even though meta reports
// the resource gone. Fatalf cannot be observed on the parent *testing.T, so
// the boundary is proven by the child process exiting non-zero AND the meta
// endpoint never being reached. Removing the isTransientDriveDeleteFailure
// guard from the main loop turns this test red.
func TestDeleteAsyncAndVerifyRejectsUnexpectedFailure(t *testing.T) {
	fake := mustWriteDriveDeleteWorkflowFakeCLI(t)
	counters := newFakeWorkflowCounterPaths(t)

	output, err := runDeleteWorkflowSubprocess(t, fake, counters, map[string]string{
		"FAKE_WORKFLOW_TOKEN":       "docx_unexpected",
		"FAKE_WORKFLOW_DELETE_MODE": "fail-unexpected",
		"FAKE_WORKFLOW_META_MODE":   "gone",
	})
	require.Error(t, err, "deleteAsyncAndVerify must fail the test process on an unexpected delete error\noutput:\n%s", output)
	assert.Contains(t, output, "drive +delete failed with an unexpected error", "output:\n%s", output)

	assert.Equal(t, "1", readFakeCounter(t, counters.deletes))
	assert.Equal(t, "0", readFakeCounter(t, counters.metas), "unexpected failures must not fall through to terminal-state checking")
	assert.Equal(t, "0", readFakeCounter(t, counters.taskResults))
}

// TestDeleteAsyncAndVerifyFailsOnTaskResultFailure proves a non-zero
// drive +task_result exit fails the workflow before the final visibility
// polling: the task-result endpoint is reached once and the meta endpoint
// never.
func TestDeleteAsyncAndVerifyFailsOnTaskResultFailure(t *testing.T) {
	fake := mustWriteDriveDeleteWorkflowFakeCLI(t)
	counters := newFakeWorkflowCounterPaths(t)

	output, err := runDeleteWorkflowSubprocess(t, fake, counters, map[string]string{
		"FAKE_WORKFLOW_TOKEN":       "docx_taskresult",
		"FAKE_WORKFLOW_DELETE_MODE": "async",
		"FAKE_WORKFLOW_META_MODE":   "gone",
		// FAKE_WORKFLOW_TASK_RESULT_OK stays unset: +task_result exits 2.
	})
	require.Error(t, err, "deleteAsyncAndVerify must fail the test process when +task_result fails\noutput:\n%s", output)
	assert.Contains(t, output, "drive +task_result failed", "output:\n%s", output)

	assert.Equal(t, "1", readFakeCounter(t, counters.deletes))
	assert.Equal(t, "1", readFakeCounter(t, counters.taskResults))
	assert.Equal(t, "0", readFakeCounter(t, counters.metas), "task-result failure must abort before visibility polling")
}

// TestDeleteAsyncAndVerifyStopsAfterExhaustedRetries proves the transient
// tolerance is bounded: with the resource still present, exactly
// deleteWorkflowMaxAttempts delete attempts (each followed by one terminal
// state check) run before the workflow fails for good.
func TestDeleteAsyncAndVerifyStopsAfterExhaustedRetries(t *testing.T) {
	fake := mustWriteDriveDeleteWorkflowFakeCLI(t)
	counters := newFakeWorkflowCounterPaths(t)

	output, err := runDeleteWorkflowSubprocess(t, fake, counters, map[string]string{
		"FAKE_WORKFLOW_TOKEN":        "docx_exhausted",
		"FAKE_WORKFLOW_DELETE_MODE":  "fail",
		"FAKE_WORKFLOW_META_MODE":    "exists",
		"FAKE_WORKFLOW_FAST_BACKOFF": "1",
	})
	require.Error(t, err, "deleteAsyncAndVerify must fail the test process after exhausting retries\noutput:\n%s", output)
	assert.Contains(t, output, "drive +delete failed 3 times", "output:\n%s", output)

	assert.Equal(t, "3", readFakeCounter(t, counters.deletes))
	assert.Equal(t, "3", readFakeCounter(t, counters.metas))
	assert.Equal(t, "0", readFakeCounter(t, counters.taskResults))
}

// runDeleteWorkflowSubprocess re-runs this test binary anchored to the child
// entry point below with the fake CLI and counter files wired in via env.
func runDeleteWorkflowSubprocess(t *testing.T, fake string, counters fakeWorkflowCounters, env map[string]string) (string, error) {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=TestDeleteAsyncAndVerifySubprocess$", "-test.v")
	cmd.Env = append(os.Environ(),
		"FAKE_WORKFLOW_SUBPROCESS=1",
		clie2e.EnvBinaryPath+"="+fake,
		"FAKE_WORKFLOW_DELETE_STATE="+counters.deletes,
		"FAKE_WORKFLOW_META_STATE="+counters.metas,
		"FAKE_WORKFLOW_TASK_RESULT_STATE="+counters.taskResults,
	)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// TestDeleteAsyncAndVerifySubprocess is the child entry point driven by
// runDeleteWorkflowSubprocess. It does nothing in a normal test run.
func TestDeleteAsyncAndVerifySubprocess(t *testing.T) {
	if os.Getenv("FAKE_WORKFLOW_SUBPROCESS") != "1" {
		return
	}
	if os.Getenv("FAKE_WORKFLOW_FAST_BACKOFF") == "1" {
		deleteWorkflowRetryBackoff = time.Millisecond
	}
	deleteAsyncAndVerify(t, context.Background(), os.Getenv("FAKE_WORKFLOW_TOKEN"), "docx")
}

type fakeWorkflowCounters struct {
	deletes     string
	metas       string
	taskResults string
}

func newFakeWorkflowCounterPaths(t *testing.T) fakeWorkflowCounters {
	t.Helper()

	dir := t.TempDir()
	return fakeWorkflowCounters{
		deletes:     filepath.Join(dir, "delete-attempts"),
		metas:       filepath.Join(dir, "meta-calls"),
		taskResults: filepath.Join(dir, "task-result-calls"),
	}
}

// setupFakeWorkflowCounters wires per-endpoint call counters into the fake CLI
// so tests can assert exactly which commands ran.
func setupFakeWorkflowCounters(t *testing.T) fakeWorkflowCounters {
	t.Helper()

	counters := newFakeWorkflowCounterPaths(t)
	t.Setenv("FAKE_WORKFLOW_DELETE_STATE", counters.deletes)
	t.Setenv("FAKE_WORKFLOW_META_STATE", counters.metas)
	t.Setenv("FAKE_WORKFLOW_TASK_RESULT_STATE", counters.taskResults)
	return counters
}

func readFakeCounter(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "0"
	}
	require.NoError(t, err)
	return string(data)
}

func withFastDeleteWorkflowBackoff(t *testing.T) {
	t.Helper()

	original := deleteWorkflowRetryBackoff
	deleteWorkflowRetryBackoff = time.Millisecond
	t.Cleanup(func() {
		deleteWorkflowRetryBackoff = original
	})
}

// mustWriteDriveDeleteWorkflowFakeCLI writes a fake lark-cli that emulates the
// drive delete outcomes exercised by deleteAsyncAndVerify. Every endpoint
// bumps a per-endpoint counter when its FAKE_WORKFLOW_*_STATE env is set, so
// tests can assert call contracts. +task_result rejects every call unless
// FAKE_WORKFLOW_TASK_RESULT_OK=1, which proves the sync path never queries
// task status.
func mustWriteDriveDeleteWorkflowFakeCLI(t *testing.T) string {
	t.Helper()

	script := `#!/bin/sh
bump_counter() {
  state="$1"
  count=0
  if [ -f "$state" ]; then
    count="$(cat "$state")"
  fi
  next=$((count + 1))
  printf '%s' "$next" > "$state"
  echo "$count"
}

if [ "$1" = "drive" ] && [ "$2" = "+delete" ]; then
  count=0
  if [ -n "$FAKE_WORKFLOW_DELETE_STATE" ]; then
    count="$(bump_counter "$FAKE_WORKFLOW_DELETE_STATE")"
  fi
  case "$FAKE_WORKFLOW_DELETE_MODE" in
  sync)
    echo '{"ok":true,"identity":"bot","data":{"deleted":true,"file_token":"tok","type":"docx"}}'
    exit 0
    ;;
  fail)
    echo "Deleting docx tok..." >&2
    echo '{"ok":false,"identity":"bot","error":{"type":"api","subtype":"server_error","message":"drive task failed"}}' >&2
    exit 1
    ;;
  fail-unexpected)
    echo '{"ok":false,"identity":"bot","error":{"type":"api","subtype":"invalid_request","message":"file token not found"}}' >&2
    exit 1
    ;;
  async)
    echo '{"ok":true,"identity":"bot","data":{"task_id":"task_123","status":"success","file_token":"tok","type":"docx"}}'
    exit 0
    ;;
  fail-then-async)
    if [ "$count" -lt 1 ]; then
      echo '{"ok":false,"identity":"bot","error":{"type":"api","subtype":"server_error","message":"drive task failed"}}' >&2
      exit 1
    fi
    echo '{"ok":true,"identity":"bot","data":{"task_id":"task_123","status":"success","file_token":"tok","type":"docx"}}'
    exit 0
    ;;
  esac
  echo "unexpected FAKE_WORKFLOW_DELETE_MODE: $FAKE_WORKFLOW_DELETE_MODE" >&2
  exit 2
fi

if [ "$1" = "drive" ] && [ "$2" = "+task_result" ]; then
  if [ -n "$FAKE_WORKFLOW_TASK_RESULT_STATE" ]; then
    bump_counter "$FAKE_WORKFLOW_TASK_RESULT_STATE" > /dev/null
  fi
  if [ "${FAKE_WORKFLOW_TASK_RESULT_OK:-0}" != "1" ]; then
    echo "unexpected +task_result call: $*" >&2
    exit 2
  fi
  echo '{"ok":true,"identity":"bot","data":{"task_id":"task_123","status":"success","failed":false}}'
  exit 0
fi

if [ "$1" = "api" ] && [ "$2" = "post" ] && [ "$3" = "/open-apis/drive/v1/metas/batch_query" ]; then
  count=0
  if [ -n "$FAKE_WORKFLOW_META_STATE" ]; then
    count="$(bump_counter "$FAKE_WORKFLOW_META_STATE")"
  fi
  case "$FAKE_WORKFLOW_META_MODE" in
  gone)
    echo '{"ok":true,"data":{"metas":[]}}'
    exit 0
    ;;
  exists)
    echo '{"ok":true,"data":{"metas":[{"url":"https://example.com/still-visible"}]}}'
    exit 0
    ;;
  exists-then-gone)
    if [ "$count" -lt 1 ]; then
      echo '{"ok":true,"data":{"metas":[{"url":"https://example.com/still-visible"}]}}'
      exit 0
    fi
    echo '{"ok":true,"data":{"metas":[]}}'
    exit 0
    ;;
  esac
  echo "unexpected FAKE_WORKFLOW_META_MODE: $FAKE_WORKFLOW_META_MODE" >&2
  exit 2
fi

echo "unexpected fake CLI args: $*" >&2
exit 2
`

	binaryPath := filepath.Join(t.TempDir(), "fake-lark-cli")
	require.NoError(t, os.WriteFile(binaryPath, []byte(script), 0o755))
	return binaryPath
}
