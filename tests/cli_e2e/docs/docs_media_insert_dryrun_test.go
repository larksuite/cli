// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestDocsMediaInsertDryRun_AppendsWithoutMCP(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+media-insert",
			"--doc", "doxcnMediaInsertDryRun",
			"--file", "fixture.png",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, int64(4), clie2e.DryRunGet(out, "api.#").Int(), out)
	require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Contains(t, clie2e.DryRunGet(out, "api.0.url").String(), "/open-apis/docx/v1/documents/", out)
	require.Equal(t, "<children_len>", clie2e.DryRunGet(out, "api.1.body.index").String(), out)
	require.Equal(t, "/open-apis/drive/v1/medias/upload_all", clie2e.DryRunGet(out, "api.2.url").String(), out)
	require.Equal(t, "PATCH", clie2e.DryRunGet(out, "api.3.method").String(), out)
	require.False(t, strings.Contains(out, "/mcp") || strings.Contains(out, "locate-doc"), out)
}

func TestDocsMediaInsertRemovedLocationFlagsRejected(t *testing.T) {
	setDocsDryRunEnv(t)

	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "selection", args: []string{"--selection-with-ellipsis", "target text"}},
		{name: "before", args: []string{"--before"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			args := []string{
				"docs", "+media-insert",
				"--doc", "doxcnMediaInsertDryRun",
				"--file", "fixture.png",
				"--dry-run",
			}
			args = append(args, test.args...)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Contains(t, result.Stderr, "unknown flag", result.Stderr)
		})
	}
}
