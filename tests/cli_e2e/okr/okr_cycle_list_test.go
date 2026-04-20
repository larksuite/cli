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

// --- Dry-run E2E tests (no real API calls, no secrets needed) ---

// TestOKR_CycleListDryRun validates +cycle-list dry-run output contains the correct method and API path.
func TestOKR_CycleListDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+cycle-list",
			"--user-id", "ou_dryrun_test",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v2/cycles"), "dry-run should contain API path, got: %s", output)
	assert.True(t, strings.Contains(output, "ou_dryrun_test"), "dry-run should contain user-id, got: %s", output)
}

// TestOKR_CycleListDryRun_WithTimeRange validates +cycle-list dry-run with --time-range flag.
func TestOKR_CycleListDryRun_WithTimeRange(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+cycle-list",
			"--user-id", "ou_dryrun_test",
			"--time-range", "2025-01--2025-06",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v2/cycles"), "dry-run should contain API path, got: %s", output)
}
