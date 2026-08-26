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

func TestDocs_CreateLargeContentDryRunPlansCreateThenAppend(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name    string
		format  string
		content string
	}{
		{
			name:    "xml",
			format:  "xml",
			content: "<title>Large XML</title>\n" + strings.Repeat("<p>x</p>\n", 5_000),
		},
		{
			name:    "markdown",
			format:  "markdown",
			content: "# Large Markdown\n\n" + strings.Repeat("paragraph\n\n", 5_000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"docs", "+create",
					"--doc-format", tt.format,
					"--content", "-",
					"--dry-run",
				},
				DefaultAs: "bot",
				Stdin:     []byte(tt.content),
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			apis := clie2e.DryRunGet(result.Stdout, "api").Array()
			require.Len(t, apis, 3, "stdout:\n%s", result.Stdout)
			require.Equal(t, "POST", apis[0].Get("method").String())
			require.Equal(t, "/open-apis/docs_ai/v1/documents", apis[0].Get("url").String())
			require.Equal(t, "PUT", apis[1].Get("method").String())
			require.Equal(t, "/open-apis/docs_ai/v1/documents/<created_document_id>", apis[1].Get("url").String())
			require.Equal(t, "PUT", apis[2].Get("method").String())
			require.Equal(t, "/open-apis/docs_ai/v1/documents/<created_document_id>", apis[2].Get("url").String())
			require.Equal(t, "block_insert_after", apis[1].Get("body.command").String())
			require.Equal(t, "-1", apis[1].Get("body.block_id").String())
			require.Equal(t, tt.content, apis[0].Get("body.content").String()+apis[1].Get("body.content").String()+apis[2].Get("body.content").String())
			require.Equal(t, int64(3), clie2e.DryRunGet(result.Stdout, "create_batch_count").Int())
		})
	}
}
