// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"os"
	"path/filepath"
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
		wantContains   []string
		wantURL        string
		wantParams     map[string]any
		wantBody       map[string]any
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
			wantContains: []string{"/open-apis/docs_ai/v1/documents"},
		},
		{
			name: "create api-version v1 compatibility",
			args: []string{
				"docs", "+create",
				"--api-version", "v1",
				"--content", "<title>Dry Run</title><p>hello</p>",
				"--dry-run",
			},
			wantContains: []string{"/open-apis/docs_ai/v1/documents"},
		},
		{
			name: "fetch",
			args: []string{
				"docs", "+fetch",
				"--doc", "doxcnDryRunE2E",
				"--dry-run",
			},
			wantContains:   []string{"/open-apis/docs_ai/v1/documents/doxcnDryRunE2E/fetch"},
			wantExtraParam: `{"enable_user_cite_reference_map":true,"include_comments":true,"return_html5_block_data":true}`,
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
			wantContains: []string{"/open-apis/docs_ai/v1/documents/doxcnDryRunE2E"},
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
			wantContains: []string{"/open-apis/docs_ai/v1/documents/doxcnDryRunE2E"},
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
			wantContains: []string{"/open-apis/docs_ai/v1/documents/doxcnDryRunE2E"},
		},
		{
			name: "block_replace inclusive range",
			args: []string{
				"docs", "+update",
				"--doc", "doxcnDryRunE2E",
				"--command", "block_replace",
				"--start-block-id", "li1",
				"--end-block-id", "li3",
				"--content", "<li>combined</li>",
				"--dry-run",
			},
			wantContains: []string{"/open-apis/docs_ai/v1/documents/doxcnDryRunE2E"},
			wantBody: map[string]any{
				"command":        "block_replace",
				"start_block_id": "li1",
				"end_block_id":   "li3",
				"content":        "<li>combined</li>",
			},
		},
		{
			name: "block_delete inclusive range",
			args: []string{
				"docs", "+update",
				"--doc", "doxcnDryRunE2E",
				"--command", "block_delete",
				"--start-block-id", "p1",
				"--end-block-id", "p3",
				"--dry-run",
			},
			wantContains: []string{"/open-apis/docs_ai/v1/documents/doxcnDryRunE2E"},
			wantBody: map[string]any{
				"command":        "block_delete",
				"start_block_id": "p1",
				"end_block_id":   "p3",
			},
		},
		{
			name: "history list",
			args: []string{
				"docs", "+history-list",
				"--doc", "doxcnDryRunE2E",
				"--page-size", "5",
				"--page-token", "page_token_1",
				"--dry-run",
			},
			wantURL: "/open-apis/docs_ai/v1/documents/doxcnDryRunE2E/histories",
			wantParams: map[string]any{
				"page_size":  5,
				"page_token": "page_token_1",
			},
		},
		{
			name: "history revert",
			args: []string{
				"docs", "+history-revert",
				"--doc", "doxcnDryRunE2E",
				"--history-version-id", "42",
				"--wait-timeout-ms", "0",
				"--dry-run",
			},
			wantURL: "/open-apis/docs_ai/v1/documents/doxcnDryRunE2E/history/revert",
			wantBody: map[string]any{
				"history_version_id": "42",
				"wait_timeout_ms":    0,
			},
		},
		{
			name: "history revert status",
			args: []string{
				"docs", "+history-revert-status",
				"--doc", "doxcnDryRunE2E",
				"--task-id", "task_1",
				"--dry-run",
			},
			wantURL: "/open-apis/docs_ai/v1/documents/doxcnDryRunE2E/history/revert_status",
			wantParams: map[string]any{
				"task_id": "task_1",
			},
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
			for _, want := range append(tt.wantContains, "docs_ai/v1") {
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
			if tt.wantURL != "" {
				require.Equal(t, tt.wantURL, clie2e.DryRunGet(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
			}
			for key, want := range tt.wantParams {
				assertDryRunField(t, result.Stdout, "api.0.params."+key, want)
			}
			for key, want := range tt.wantBody {
				assertDryRunField(t, result.Stdout, "api.0.body."+key, want)
			}
			if tt.wantExtraParam != "" {
				extraParam := clie2e.DryRunGet(result.Stdout, "api.0.body.extra_param").String()
				require.JSONEq(t, tt.wantExtraParam, extraParam, "stdout:\n%s", result.Stdout)
			}
			if tt.wantRefLabel != "" {
				got := clie2e.DryRunGet(result.Stdout, "api.0.body.reference_map.widget.r1.label").String()
				require.Equal(t, tt.wantRefLabel, got, "stdout:\n%s", result.Stdout)
			}
		})
	}
}

func assertDryRunField(t *testing.T, stdout, path string, want any) {
	t.Helper()

	got := clie2e.DryRunGet(stdout, path)
	require.True(t, got.Exists(), "%s missing in stdout:\n%s", path, stdout)
	switch want := want.(type) {
	case int:
		require.Equal(t, int64(want), got.Int(), "%s in stdout:\n%s", path, stdout)
	case string:
		require.Equal(t, want, got.String(), "%s in stdout:\n%s", path, stdout)
	default:
		t.Fatalf("unsupported dry-run assertion type %T for %s", want, path)
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
	require.Equal(t, "/open-apis/docs_ai/v1/documents", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, "markdown", clie2e.DryRunGet(out, "api.0.body.format").String(), "stdout:\n%s", out)
	require.Equal(t, "<title>Dry Run &amp; Title</title>\n## Body", clie2e.DryRunGet(out, "api.0.body.content").String(), "stdout:\n%s", out)
}

func TestDocsUpdateDryRunLegacyFlagReturnsCurrentEmbeddedGuidance(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+update",
			"--doc", "doxcnDryRunE2E",
			"--mode", "overwrite",
			"--content", "<p>hello</p>",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Empty(t, result.Stdout, "validate-stage failure must not write to stdout")

	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--mode", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Equal(t,
		"run `lark-cli docs +update --help` for the latest command flags; read the version-matched embedded guidance before retrying: `lark-cli skills read lark-doc`, `lark-cli skills read lark-doc/references/lark-doc-update.md`, `lark-cli skills read lark-doc/references/lark-doc-xml.md`, `lark-cli skills read lark-doc/references/lark-doc-md.md`; do not inspect another local SKILL.md copy",
		gjson.Get(result.Stderr, "error.hint").String(),
		result.Stderr,
	)
}

func TestDocs_CreateEmptyContentFileReportsActionableError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "draft.xml"), nil, 0o600))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+create",
			"--doc-format", "xml",
			"--content", "@draft.xml",
			"--dry-run",
		},
		DefaultAs: "user",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Empty(t, result.Stdout, "stdout must stay reserved for program data")
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
	require.Equal(t, "--content", gjson.Get(result.Stderr, "error.param").String())
	require.Equal(t, `--content file "draft.xml" is empty`, gjson.Get(result.Stderr, "error.message").String())
	require.Contains(t, gjson.Get(result.Stderr, "error.hint").String(), "exact data.draft_path returned by that command")
}
