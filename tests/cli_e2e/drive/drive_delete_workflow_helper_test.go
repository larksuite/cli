// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"os"
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

		taskID := deleteAsyncAndVerify(t, context.Background(), "docx_sync", "docx")
		assert.Empty(t, taskID)
	})

	t.Run("transient failure with resource gone is tolerated", func(t *testing.T) {
		fake := mustWriteDriveDeleteWorkflowFakeCLI(t)
		t.Setenv(clie2e.EnvBinaryPath, fake)
		t.Setenv("FAKE_WORKFLOW_DELETE_MODE", "fail")
		t.Setenv("FAKE_WORKFLOW_DELETE_STATE", filepath.Join(t.TempDir(), "delete-attempts"))
		t.Setenv("FAKE_WORKFLOW_META_MODE", "gone")

		taskID := deleteAsyncAndVerify(t, context.Background(), "docx_transient", "docx")
		assert.Empty(t, taskID)

		attempts, err := os.ReadFile(os.Getenv("FAKE_WORKFLOW_DELETE_STATE"))
		require.NoError(t, err)
		assert.Equal(t, "1", string(attempts), "resource already gone must not trigger another delete attempt")
	})

	t.Run("failed delete retries until async success", func(t *testing.T) {
		fake := mustWriteDriveDeleteWorkflowFakeCLI(t)
		t.Setenv(clie2e.EnvBinaryPath, fake)
		t.Setenv("FAKE_WORKFLOW_DELETE_MODE", "fail-then-async")
		t.Setenv("FAKE_WORKFLOW_DELETE_STATE", filepath.Join(t.TempDir(), "delete-attempts"))
		t.Setenv("FAKE_WORKFLOW_META_MODE", "exists-then-gone")
		t.Setenv("FAKE_WORKFLOW_META_STATE", filepath.Join(t.TempDir(), "meta-calls"))
		t.Setenv("FAKE_WORKFLOW_TASK_RESULT_OK", "1")
		withFastDeleteWorkflowBackoff(t)

		taskID := deleteAsyncAndVerify(t, context.Background(), "docx_retry", "docx")
		assert.Equal(t, "task_123", taskID)

		attempts, err := os.ReadFile(os.Getenv("FAKE_WORKFLOW_DELETE_STATE"))
		require.NoError(t, err)
		assert.Equal(t, "2", string(attempts))
	})
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
// three drive delete outcomes exercised by deleteAsyncAndVerify. +task_result
// rejects every call unless FAKE_WORKFLOW_TASK_RESULT_OK=1, which proves the
// sync path never queries task status.
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
  case "$FAKE_WORKFLOW_DELETE_MODE" in
  sync)
    echo '{"ok":true,"identity":"bot","data":{"deleted":true,"file_token":"tok","type":"docx"}}'
    exit 0
    ;;
  fail)
    bump_counter "$FAKE_WORKFLOW_DELETE_STATE" > /dev/null
    echo "Deleting docx tok..." >&2
    echo '{"ok":false,"identity":"bot","error":{"type":"api","subtype":"server_error","message":"drive task failed"}}' >&2
    exit 1
    ;;
  fail-then-async)
    count="$(bump_counter "$FAKE_WORKFLOW_DELETE_STATE")"
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
  if [ "${FAKE_WORKFLOW_TASK_RESULT_OK:-0}" != "1" ]; then
    echo "unexpected +task_result call: $*" >&2
    exit 2
  fi
  echo '{"ok":true,"identity":"bot","data":{"task_id":"task_123","status":"success","failed":false}}'
  exit 0
fi

if [ "$1" = "api" ] && [ "$2" = "post" ] && [ "$3" = "/open-apis/drive/v1/metas/batch_query" ]; then
  case "$FAKE_WORKFLOW_META_MODE" in
  gone)
    echo '{"ok":true,"data":{"metas":[]}}'
    exit 0
    ;;
  exists-then-gone)
    count="$(bump_counter "$FAKE_WORKFLOW_META_STATE")"
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
