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

func TestDocs_DryRunDefaultsToV2OpenAPI(t *testing.T) {
	// Fake creds are enough — dry-run short-circuits before any real API call.
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name           string
		args           []string
		wantURL        string
		wantExtraParam string
		wantRefLabel   string
	}{
		{
			name: "create",
			args: []string{
				"docs", "+create",
				"--content", "<title>Dry Run</title><p>hello</p>",
				"--dry-run",
			},
			wantURL: "/open-apis/docs_ai/v1/documents",
		},
		{
			name: "create api-version v1 compatibility",
			args: []string{
				"docs", "+create",
				"--api-version", "v1",
				"--content", "<title>Dry Run</title><p>hello</p>",
				"--dry-run",
			},
			wantURL: "/open-apis/docs_ai/v1/documents",
		},
		{
			name: "fetch",
			args: []string{
				"docs", "+fetch",
				"--doc", "doxcnDryRunE2E",
				"--dry-run",
			},
			wantURL:        "/open-apis/docs_ai/v1/documents/doxcnDryRunE2E/fetch",
			wantExtraParam: `{"enable_user_cite_reference_map":true}`,
		},
		{
			name: "update",
			args: []string{
				"docs", "+update",
				"--doc", "doxcnDryRunE2E",
				"--command", "append",
				"--content", "<p>hello</p>",
				"--dry-run",
			},
			wantURL: "/open-apis/docs_ai/v1/documents/doxcnDryRunE2E",
		},
		{
			name: "update reference-map",
			args: []string{
				"docs", "+update",
				"--doc", "doxcnDryRunE2E",
				"--command", "append",
				"--content", `<p><widget data-ref="r1"></widget></p>`,
				"--reference-map", `{"widget":{"r1":{"label":"widget-ref-value"}}}`,
				"--dry-run",
			},
			wantURL:      "/open-apis/docs_ai/v1/documents/doxcnDryRunE2E",
			wantRefLabel: "widget-ref-value",
		},
		{
			name: "block_delete batch",
			args: []string{
				"docs", "+update",
				"--doc", "doxcnDryRunE2E",
				"--command", "block_delete",
				"--block-id", "blkA,blkB,blkC",
				"--dry-run",
			},
			wantURL: "/open-apis/docs_ai/v1/documents/doxcnDryRunE2E",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			combined := result.Stdout + "\n" + result.Stderr
			for _, want := range []string{
				tt.wantURL,
				"docs_ai/v1",
			} {
				if !strings.Contains(combined, want) {
					t.Fatalf("dry-run output missing %q\nstdout:\n%s\nstderr:\n%s", want, result.Stdout, result.Stderr)
				}
			}
			if strings.Contains(combined, "/mcp") || strings.Contains(combined, "MCP tool") {
				t.Fatalf("dry-run output should not use MCP\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			}
			if strings.Contains(combined, "--api-version") {
				t.Fatalf("dry-run output should not ask for --api-version\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			}
			if tt.wantExtraParam != "" {
				extraParam := gjson.Get(result.Stdout, "api.0.body.extra_param").String()
				require.JSONEq(t, tt.wantExtraParam, extraParam, "stdout:\n%s", result.Stdout)
			}
			if tt.wantRefLabel != "" {
				got := gjson.Get(result.Stdout, "api.0.body.reference_map.widget.r1.label").String()
				require.Equal(t, tt.wantRefLabel, got, "stdout:\n%s", result.Stdout)
			}
		})
	}
}

func TestDocs_CreateTitleDryRunPrependsContent(t *testing.T) {
	// Fake creds are enough — dry-run short-circuits before any real API call.
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+create",
			"--title", "Dry Run & Title",
			"--doc-format", "markdown",
			"--content", "## Body",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/docs_ai/v1/documents", gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, "markdown", gjson.Get(out, "api.0.body.format").String(), "stdout:\n%s", out)
	require.Equal(t, "<title>Dry Run &amp; Title</title>\n## Body", gjson.Get(out, "api.0.body.content").String(), "stdout:\n%s", out)
}
