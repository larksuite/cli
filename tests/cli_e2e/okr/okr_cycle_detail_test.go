// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOKR_CycleDetailDryRun validates +cycle-detail dry-run output contains the correct method and API path.
func TestOKR_CycleDetailDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+cycle-detail",
			"--cycle-id", "123456",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v2/cycles/123456/objectives"), "dry-run should contain API path with cycle-id, got: %s", output)
	assert.True(t, strings.Contains(output, "123456"), "dry-run should contain cycle-id, got: %s", output)
}

func setDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_APP_ID", "cli_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
