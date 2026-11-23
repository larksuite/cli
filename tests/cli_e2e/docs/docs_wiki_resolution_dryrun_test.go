// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestDocsWikiResourceDryRunsUseNodeByToken(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	workDir := t.TempDir()
	writeLocalResourceFixture(t, workDir, "fixture.png", onePixelPNG)
	const wikiToken = "wikcnDocsNodeByToken"
	const wikiURL = "https://example.larksuite.com/wiki/" + wikiToken

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "media insert",
			args: []string{"docs", "+media-insert", "--doc", wikiURL, "--file", "fixture.png", "--dry-run"},
		},
		{
			name: "resource download",
			args: []string{"docs", "+resource-download", "--doc", wikiURL, "--output", "cover", "--dry-run"},
		},
		{
			name: "resource update",
			args: []string{"docs", "+resource-update", "--doc", wikiURL, "--url", "https://93.184.216.34/cover.png", "--dry-run"},
		},
		{
			name: "resource delete",
			args: []string{"docs", "+resource-delete", "--doc", wikiURL, "--dry-run"},
		},
		{
			name: "update with local image",
			args: []string{"docs", "+update", "--doc", wikiURL, "--command", "append", "--content", `<img path="@fixture.png"/>`, "--dry-run"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "bot",
				WorkDir:   workDir,
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, "/open-apis/wiki/v2/spaces/node_by_token", clie2e.DryRunGet(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, wikiToken, clie2e.DryRunGet(result.Stdout, "api.0.params.token").String(), "stdout:\n%s", result.Stdout)
			require.NotContains(t, result.Stdout, "/open-apis/wiki/v2/spaces/get_node", "stdout:\n%s", result.Stdout)
		})
	}
}
