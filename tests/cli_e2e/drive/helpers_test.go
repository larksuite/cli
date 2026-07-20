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

func createDriveFolder(t *testing.T, parentT *testing.T, ctx context.Context, name string, parentFolderToken string) string {
	t.Helper()
	folderToken := CreateDriveFolder(t, parentT, ctx, name, "bot", parentFolderToken)
	require.NotEmpty(t, folderToken)
	return folderToken
}

func TestDeleteDriveResourceAndVerify(t *testing.T) {
	t.Run("retries retryable delete contention", func(t *testing.T) {
		fake := mustWriteDriveCleanupFakeCLI(t)
		t.Setenv(clie2e.EnvBinaryPath, fake)
		t.Setenv("FAKE_DRIVE_DELETE_RETRYABLE_ATTEMPTS", "2")
		t.Setenv("FAKE_DRIVE_DELETE_STATE", filepath.Join(t.TempDir(), "delete-attempts"))
		t.Setenv("FAKE_DRIVE_META_EMPTY", "1")
		withFastDriveDeleteRetry(t)

		result, err := DeleteDriveResourceAndVerify(context.Background(), "fld_retry", "folder", "bot")
		require.NotNil(t, result)
		require.NoError(t, err)
		assert.Equal(t, 0, result.ExitCode)
	})

	t.Run("successful delete with stale meta returns cleanup warning", func(t *testing.T) {
		fake := mustWriteDriveCleanupFakeCLI(t)
		t.Setenv(clie2e.EnvBinaryPath, fake)

		result, err := deleteDriveResourceAndVerify(context.Background(), "fld_stale", "folder", "bot", clie2e.WaitOptions{
			Timeout:  10 * time.Millisecond,
			Interval: time.Millisecond,
		})
		require.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)
		require.Error(t, err)
		assert.True(t, clie2e.IsCleanupWarning(err), "err: %v", err)
	})

	t.Run("failed delete with existing meta remains fatal", func(t *testing.T) {
		fake := mustWriteDriveCleanupFakeCLI(t)
		t.Setenv(clie2e.EnvBinaryPath, fake)
		t.Setenv("FAKE_DRIVE_DELETE_EXIT", "1")

		result, err := DeleteDriveResourceAndVerify(context.Background(), "fld_existing", "folder", "bot")
		require.NotNil(t, result)
		assert.Equal(t, 1, result.ExitCode)
		require.Error(t, err)
		assert.False(t, clie2e.IsCleanupWarning(err), "err: %v", err)
		assert.Contains(t, err.Error(), "still exists after delete failed")
	})
}

func withFastDriveDeleteRetry(t *testing.T) {
	t.Helper()

	original := driveDeleteRetry
	driveDeleteRetry = clie2e.RetryOptions{
		Attempts:        3,
		InitialDelay:    time.Millisecond,
		MaxDelay:        time.Millisecond,
		BackoffMultiple: 2,
		ShouldRetry:     clie2e.ResultHasRetryableError,
	}
	t.Cleanup(func() {
		driveDeleteRetry = original
	})
}

func mustWriteDriveCleanupFakeCLI(t *testing.T) string {
	t.Helper()

	script := `#!/bin/sh
if [ "$1" = "drive" ] && [ "$2" = "+delete" ]; then
  if [ -n "$FAKE_DRIVE_DELETE_RETRYABLE_ATTEMPTS" ]; then
    state="$FAKE_DRIVE_DELETE_STATE"
    count=0
    if [ -f "$state" ]; then
      count="$(cat "$state")"
    fi
    next=$((count + 1))
    echo "$next" > "$state"
    if [ "$count" -lt "$FAKE_DRIVE_DELETE_RETRYABLE_ATTEMPTS" ]; then
      echo "Deleting folder fake..." >&2
      echo '{"ok":false,"error":{"type":"api","code":1061045,"message":"resource contention occurred, please retry.","retryable":true}}' >&2
      exit 1
    fi
  fi
  if [ "${FAKE_DRIVE_DELETE_EXIT:-0}" != "0" ]; then
    echo '{"ok":false,"error":{"type":"api","message":"delete failed"}}' >&2
    exit "$FAKE_DRIVE_DELETE_EXIT"
  fi
  echo '{"ok":true}'
  exit 0
fi

if [ "$1" = "api" ] && [ "$2" = "post" ] && [ "$3" = "/open-apis/drive/v1/metas/batch_query" ]; then
  if [ "${FAKE_DRIVE_META_EMPTY:-0}" = "1" ]; then
    echo '{"ok":true,"data":{"metas":[]}}'
    exit 0
  fi
  echo '{"ok":true,"data":{"metas":[{"url":"https://example.com/still-visible"}]}}'
  exit 0
fi

echo "unexpected fake CLI args: $*" >&2
exit 2
`

	binaryPath := filepath.Join(t.TempDir(), "fake-lark-cli")
	require.NoError(t, os.WriteFile(binaryPath, []byte(script), 0o755))
	return binaryPath
}
