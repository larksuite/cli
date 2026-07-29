// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAppsFileUploadDryRun_AcceptsAbsoluteHostPath(t *testing.T) {
	setAppsDryRunEnv(t)
	absolutePath := filepath.Join(t.TempDir(), "report.pdf")
	require.NoError(t, os.WriteFile(absolutePath, []byte("dry-run-input"), 0o600))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"apps", "+file-upload",
			"--app-id", "app_x",
			"--file", absolutePath,
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	assert.Equal(t, "/open-apis/spark/v1/apps/app_x/storage/file_pre_upload", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	assert.Equal(t, "report.pdf", clie2e.DryRunGet(result.Stdout, "api.0.body.file_name").String())
}

func TestAppsFileUploadDryRun_RejectsMissingHostPath(t *testing.T) {
	setAppsDryRunEnv(t)
	missingAbsolutePath := filepath.Join(t.TempDir(), "does-not-exist", "report.pdf")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"apps", "+file-upload",
			"--app-id", "app_x",
			"--file", missingAbsolutePath,
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--file", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
}
