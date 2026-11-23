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

func TestVCMeetingScreenshotDryRun(t *testing.T) {
	setVCDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+meeting-screenshot",
			"--meeting-id", "7628568141510692381",
			"--output", "current.jpg",
			"--overwrite",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, int64(1), clie2e.DryRunGet(result.Stdout, "api.#").Int(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/vc/v1/bots/screenshot", clie2e.DryRunGet(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "7628568141510692381", clie2e.DryRunGet(result.Stdout, "api.0.body.meeting_id").String(), "stdout:\n%s", result.Stdout)
}
