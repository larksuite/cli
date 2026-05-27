// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDocs_CreateV2RejectsLegacyFlagsDryRun(t *testing.T) {
	setDocsDryRunEnv(t)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name: "markdown",
			args: []string{
				"docs", "+create",
				"--api-version", "v2",
				"--markdown", "## legacy",
				"--dry-run",
			},
			wantErr: "use --content with --doc-format markdown",
		},
		{
			name: "wiki node",
			args: []string{
				"docs", "+create",
				"--api-version", "v2",
				"--content", "<title>内容</title><p>正文</p>",
				"--wiki-node", "wikcn_legacy_node",
				"--dry-run",
			},
			wantErr: "use --parent-token",
		},
		{
			name: "title",
			args: []string{
				"docs", "+create",
				"--api-version", "v2",
				"--content", "<p>正文</p>",
				"--title", "Legacy title",
				"--dry-run",
			},
			wantErr: "include the document title in --content",
		},
		{
			name: "folder token",
			args: []string{
				"docs", "+create",
				"--api-version", "v2",
				"--content", "<title>内容</title><p>正文</p>",
				"--folder-token", "fldcn_legacy_folder",
				"--dry-run",
			},
			wantErr: "use --parent-token",
		},
		{
			name: "wiki space",
			args: []string{
				"docs", "+create",
				"--api-version", "v2",
				"--content", "<title>内容</title><p>正文</p>",
				"--wiki-space", "my_library",
				"--dry-run",
			},
			wantErr: "use --parent-position or --parent-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			assert.Contains(t, docsValidationErrorMessage(result), tt.wantErr)
			assert.Equal(t, int64(0), gjson.Get(result.Stdout, "api.#").Int(),
				"validation failure must not produce dry-run API calls, stdout:\n%s", result.Stdout)
		})
	}
}

func setDocsDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "docs_dryrun_e2e_app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "docs_dryrun_e2e_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

func docsValidationErrorMessage(r *clie2e.Result) string {
	if msg := gjson.Get(r.Stdout, "error.message").String(); msg != "" {
		return msg
	}
	return gjson.Get(r.Stderr, "error.message").String()
}
