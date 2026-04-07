// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func createWikiNode(t *testing.T, ctx context.Context, req clie2e.Request) gjson.Result {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, req)
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, 0)

	node := gjson.Get(result.Stdout, "data.node")
	require.True(t, node.Exists(), "stdout:\n%s", result.Stdout)

	return node
}
