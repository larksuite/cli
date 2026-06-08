// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDriveListCommentsDryRun_RequestsRelationAndBlockIDs(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+list-comments",
			"--doc", "https://example.larksuite.com/docx/doxDryRunComments",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/drive/v1/files/doxDryRunComments/comments", gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)
	require.True(t, gjson.Get(out, "api.0.params.need_relation").Bool(), "stdout:\n%s", out)
	require.True(t, gjson.Get(out, "api.0.params.need_reaction").Bool(), "stdout:\n%s", out)
	require.False(t, gjson.Get(out, "api.0.params.is_solved").Bool(), "stdout:\n%s", out)

	require.Equal(t, "/open-apis/docs_ai/v1/documents/doxDryRunComments/fetch", gjson.Get(out, "api.1.url").String(), "stdout:\n%s", out)
	require.Equal(t, "xml", gjson.Get(out, "api.1.body.format").String(), "stdout:\n%s", out)
	require.True(t, gjson.Get(out, "api.1.body.export_option.export_block_id").Bool(), "stdout:\n%s", out)
}
