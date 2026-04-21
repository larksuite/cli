// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinutesUpload_DryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"minutes", "+upload",
			"--file-token", "boxcn123456",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "POST"), "dry-run should contain POST method, got: %s", output)
	assert.True(t, strings.Contains(output, "/open-apis/minutes/v1/minutes/upload"), "dry-run should contain API path, got: %s", output)
	assert.True(t, strings.Contains(output, "boxcn123456"), "dry-run should contain file_token, got: %s", output)
}

func setDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_APP_ID", "cli_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
