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

func TestVCMeetingTransferHostDryRun(t *testing.T) {
	setVCDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"vc", "+meeting-transfer-host",
			"--meeting-id", "7651377260537433044",
			"--participant", "on_target=1",
			"--user-id-type", "union_id",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, int64(1), clie2e.DryRunGet(out, "api.#").Int(), "stdout:\n%s", out)
	require.Equal(t, "PATCH", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/vc/v1/meetings/7651377260537433044/set_host", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, "union_id", clie2e.DryRunGet(out, "api.0.params.user_id_type").String(), "stdout:\n%s", out)
	require.Equal(t, "on_target", clie2e.DryRunGet(out, "api.0.body.host_user.id").String(), "stdout:\n%s", out)
	require.Equal(t, int64(1), clie2e.DryRunGet(out, "api.0.body.host_user.user_type").Int(), "stdout:\n%s", out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.old_host_user").Exists(), "old_host_user must not be sent; stdout:\n%s", out)
}
