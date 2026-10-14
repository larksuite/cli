// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTaskDownloadAttachmentWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	taskGUID := createTask(t, parentT, ctx, clie2e.Request{
		Args:      []string{"task", "+create"},
		DefaultAs: "bot",
		Data: map[string]any{
			"summary":     "lark-cli-e2e-attachment-download-" + suffix,
			"description": "created by the Task attachment download live workflow",
		},
	})

	workDir := t.TempDir()
	const fixtureContent = "task attachment download live fixture"
	fixtureName := "attachment-" + suffix + ".txt"
	if err := os.WriteFile(filepath.Join(workDir, fixtureName), []byte(fixtureContent), 0o600); err != nil {
		t.Fatal(err)
	}

	uploadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"task", "+upload-attachment",
			"--resource-id", taskGUID,
			"--file", "./" + fixtureName,
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	uploadResult.AssertExitCode(t, 0)
	uploadResult.AssertStdoutStatus(t, true)
	attachmentGUID := gjson.Get(uploadResult.Stdout, "data.guid").String()
	require.NotEmpty(t, attachmentGUID, "stdout:\n%s", uploadResult.Stdout)

	downloadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"task", "+download-attachment",
			"--attachment-guid", attachmentGUID,
			"--output", "./downloaded.txt",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	downloadResult.AssertExitCode(t, 0)
	downloadResult.AssertStdoutStatus(t, true)

	content, err := os.ReadFile(filepath.Join(workDir, "downloaded.txt"))
	require.NoError(t, err)
	require.Equal(t, fixtureContent, string(content))
	require.Equal(t, attachmentGUID, gjson.Get(downloadResult.Stdout, "data.attachment_guid").String())
	require.False(t, strings.Contains(downloadResult.Stdout, "authcode"), "temporary URL leaked in stdout:\n%s", downloadResult.Stdout)
}
