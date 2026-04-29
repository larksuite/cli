// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDriveVersionWorkflow(t *testing.T) {
	if os.Getenv("LARK_DRIVE_VERSION_E2E") == "" {
		t.Skip("set LARK_DRIVE_VERSION_E2E=1 to run drive version live workflow")
	}

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	fileName := "lark-cli-version-workflow-" + suffix + ".md"

	createResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+create",
			"--name", fileName,
			"--content", "# v1\n",
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	createResult.AssertStdoutStatus(t, true)

	fileToken := gjson.Get(createResult.Stdout, "data.file_token").String()
	require.NotEmpty(t, fileToken, "stdout:\n%s", createResult.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"drive", "+delete",
				"--file-token", fileToken,
				"--type", "file",
				"--yes",
			},
			DefaultAs:  "bot",
			BinaryPath: "../../../lark-cli",
		})
		clie2e.ReportCleanupFailure(parentT, "delete version workflow file "+fileToken, deleteResult, deleteErr)
	})

	overwriteResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+overwrite",
			"--file-token", fileToken,
			"--content", "# v2\n",
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	overwriteResult.AssertExitCode(t, 0)
	overwriteResult.AssertStdoutStatus(t, true)

	historyResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+version-history",
			"--file-token", fileToken,
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	historyResult.AssertExitCode(t, 0)
	historyResult.AssertStdoutStatus(t, true)
}
