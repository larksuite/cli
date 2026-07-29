// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestBaseURLResolveSelectedBlockDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+url-resolve",
			"--url", "https://example.larkoffice.com/base/app_x?table=blk_selected",
			"--dry-run",
		},
		BinaryPath: "../../../lark-cli",
		DefaultAs:  "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String(), result.Stdout)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/blocks/list", clie2e.DryRunGet(result.Stdout, "api.0.url").String(), result.Stdout)
	require.Equal(t, "blk_selected", clie2e.DryRunGet(result.Stdout, "selected_block_id").String(), result.Stdout)
}
