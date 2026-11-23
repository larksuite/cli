// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAppsFileUploadLiveWorkflow(t *testing.T) {
	if strings.TrimSpace(os.Getenv("LARKSUITE_CLI_CONFIG_DIR")) == "" {
		t.Skip("FIXTURE: Set LARKSUITE_CLI_CONFIG_DIR to an isolated live-test config")
	}
	appID := strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_FILE_APP_ID"))
	if appID == "" {
		t.Skip("FIXTURE: Set LARK_CLI_E2E_APPS_FILE_APP_ID to a dedicated app for upload/delete testing")
	}

	fileName := fmt.Sprintf("lark-cli-host-path-e2e-%d.txt", time.Now().UnixNano())
	absolutePath := filepath.Join(t.TempDir(), fileName)
	content := []byte("host-path-live-e2e")
	require.NoError(t, os.WriteFile(absolutePath, content, 0o600))

	remotePath := ""
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		if remotePath == "" {
			listResult, listErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"apps", "+file-list", "--app-id", appID, "--name", fileName},
				DefaultAs: "user",
			})
			if listErr != nil || listResult.ExitCode != 0 {
				clie2e.ReportCleanupFailure(t, "find uploaded file "+fileName, listResult, listErr)
				return
			}
			for _, item := range gjson.Get(listResult.Stdout, "data.items").Array() {
				if item.Get("file_name").String() == fileName {
					remotePath = item.Get("path").String()
					break
				}
			}
		}
		if remotePath == "" {
			return
		}

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"apps", "+file-delete", "--app-id", appID, "--path", remotePath},
			DefaultAs: "user",
			Yes:       true,
		})
		clie2e.ReportCleanupFailure(t, "delete uploaded file "+remotePath, deleteResult, deleteErr)
		if deleteErr == nil && deleteResult != nil && gjson.Get(deleteResult.Stdout, "data.results.0.status").String() != "ok" {
			t.Errorf("cleanup delete did not report success: %s", deleteResult.Stdout)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	uploadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+file-upload", "--app-id", appID, "--file", absolutePath},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	uploadResult.AssertExitCode(t, 0)
	uploadResult.AssertStdoutStatus(t, true)
	remotePath = gjson.Get(uploadResult.Stdout, "data.path").String()
	require.NotEmpty(t, remotePath, "stdout:\n%s", uploadResult.Stdout)
	assert.Equal(t, fileName, gjson.Get(uploadResult.Stdout, "data.file_name").String(), "stdout:\n%s", uploadResult.Stdout)

	getResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"apps", "+file-get", "--app-id", appID, "--path", remotePath},
		DefaultAs: "user",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 || gjson.Get(result.Stdout, "data.path").String() != remotePath
		},
	})
	require.NoError(t, err)
	getResult.AssertExitCode(t, 0)
	getResult.AssertStdoutStatus(t, true)
	assert.Equal(t, int64(len(content)), gjson.Get(getResult.Stdout, "data.size_bytes").Int(), "stdout:\n%s", getResult.Stdout)
}
