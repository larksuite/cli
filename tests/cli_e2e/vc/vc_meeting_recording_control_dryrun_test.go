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

func TestVCRecordingControlDryRun(t *testing.T) {
	setVCDryRunEnv(t)

	tests := []struct {
		command string
		path    string
	}{
		{command: "+meeting-recording-start", path: "/open-apis/vc/v1/meetings/7651377260537433044/recording/start"},
		{command: "+meeting-recording-stop", path: "/open-apis/vc/v1/meetings/7651377260537433044/recording/stop"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"vc", tt.command,
					"--meeting-id", "7651377260537433044",
					"--dry-run",
				},
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			require.Equal(t, int64(1), clie2e.DryRunGet(out, "api.#").Int(), "stdout:\n%s", out)
			require.Equal(t, "PATCH", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
			require.Equal(t, tt.path, clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
			require.False(t, clie2e.DryRunGet(out, "api.0.body").Exists(), "request body must be omitted; stdout:\n%s", out)
		})
	}
}
