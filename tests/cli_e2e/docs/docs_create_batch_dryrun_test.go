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
	"github.com/tidwall/gjson"
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

func TestDocs_CreateContentLimitsFailBeforeDryRunAPIPlan(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name      string
		format    string
		content   string
		limitCode string
		actual    int64
		limit     int64
	}{
		{
			name: "xml block characters", format: "xml",
			content:   `<p>` + strings.Repeat("x", 100_001) + `</p>`,
			limitCode: "DOC_BLOCK_CHAR_LIMIT", actual: 100_001, limit: 100_000,
		},
		{
			name: "markdown block characters", format: "markdown",
			content:   strings.Repeat("x", 100_001),
			limitCode: "DOC_BLOCK_CHAR_LIMIT", actual: 100_001, limit: 100_000,
		},
		{
			name: "xml table cells", format: "xml",
			content:   docsCreateLimitXMLTable(2_001, 1),
			limitCode: "DOC_TABLE_CELL_LIMIT", actual: 2_001, limit: 2_000,
		},
		{
			name: "markdown table columns", format: "markdown",
			content:   docsCreateLimitMarkdownTable(2, 101),
			limitCode: "DOC_TABLE_COLUMN_LIMIT", actual: 101, limit: 100,
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
			result.AssertExitCode(t, 2)
			require.Empty(t, strings.TrimSpace(result.Stdout), "stdout must not contain an API plan:\n%s", result.Stdout)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
			require.Equal(t, tt.limitCode, gjson.Get(result.Stderr, "error.limit_code").String(), "stderr:\n%s", result.Stderr)
			require.Equal(t, "create", gjson.Get(result.Stderr, "error.operation").String(), "stderr:\n%s", result.Stderr)
			require.Equal(t, tt.actual, gjson.Get(result.Stderr, "error.actual").Int(), "stderr:\n%s", result.Stderr)
			require.Equal(t, tt.limit, gjson.Get(result.Stderr, "error.limit").Int(), "stderr:\n%s", result.Stderr)
		})
	}
}

func docsCreateLimitXMLTable(rows, columns int) string {
	var content strings.Builder
	content.WriteString("<table>")
	for row := 0; row < rows; row++ {
		content.WriteString("<tr>")
		for column := 0; column < columns; column++ {
			content.WriteString("<td><p>x</p></td>")
		}
		content.WriteString("</tr>")
	}
	content.WriteString("</table>")
	return content.String()
}

func docsCreateLimitMarkdownTable(rows, columns int) string {
	var content strings.Builder
	writeRow := func(value string) {
		content.WriteByte('|')
		for column := 0; column < columns; column++ {
			content.WriteString(" " + value + " |")
		}
		content.WriteByte('\n')
	}
	writeRow("h")
	writeRow("---")
	for row := 1; row < rows; row++ {
		writeRow("x")
	}
	return content.String()
}
