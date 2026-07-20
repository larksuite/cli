// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestVCMeetingEventsDryRun(t *testing.T) {
	setVCDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+meeting-events",
			"--meeting-id", "7628568141510692381",
			"--page-token", "1710000000000000000",
			"--page-size", "40",
			"--start", "1710000000",
			"--end", "1710003600",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, int64(1), clie2e.DryRunGet(out, "api.#").Int(), "stdout:\n%s", out)
	require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/vc/v1/bots/events", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, "7628568141510692381", clie2e.DryRunGet(out, "api.0.params.meeting_id").String(), "stdout:\n%s", out)
	require.Equal(t, "1710000000000000000", clie2e.DryRunGet(out, "api.0.params.page_token").String(), "stdout:\n%s", out)
	require.Equal(t, "40", clie2e.DryRunGet(out, "api.0.params.page_size").String(), "stdout:\n%s", out)
	require.Equal(t, "1710000000", clie2e.DryRunGet(out, "api.0.params.start_time").String(), "stdout:\n%s", out)
	require.Equal(t, "1710003600", clie2e.DryRunGet(out, "api.0.params.end_time").String(), "stdout:\n%s", out)
}
