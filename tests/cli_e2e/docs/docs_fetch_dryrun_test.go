// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDocsFetchDryRunXMLIncludesCommentsForUserAndBot(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	type scopeCase struct {
		name string
		args []string
	}
	scopes := []scopeCase{
		{name: "full"},
		{name: "outline", args: []string{"--scope", "outline"}},
		{name: "partial", args: []string{"--scope", "keyword", "--keyword", "commented"}},
	}
	bodies := make(map[string]map[string]map[string]interface{}, len(scopes))
	for _, scope := range scopes {
		bodies[scope.name] = make(map[string]map[string]interface{}, 2)
		for _, identity := range []string{"user", "bot"} {
			t.Run(identity+"/"+scope.name, func(t *testing.T) {
				args := []string{
					"docs", "+fetch",
					"--doc", "doxcnDryRunComments",
					"--doc-format", "xml",
					"--dry-run",
				}
				args = append(args, scope.args...)
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: identity})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)

				raw := clie2e.DryRunGet(result.Stdout, "api.0.body.extra_param").String()
				var extra map[string]bool
				require.NoError(t, json.Unmarshal([]byte(raw), &extra))
				require.Equal(t, map[string]bool{
					"enable_user_cite_reference_map": true,
					"include_comments":               true,
					"return_html5_block_data":        true,
				}, extra, "stdout:\n%s", result.Stdout)

				var body map[string]interface{}
				require.NoError(t, json.Unmarshal([]byte(clie2e.DryRunGet(result.Stdout, "api.0.body").Raw), &body))
				bodies[scope.name][identity] = body
			})
		}
		require.Equal(t, bodies[scope.name]["user"], bodies[scope.name]["bot"], "user and bot request bodies must match for %s", scope.name)
	}
}

func TestDocsFetchDryRunMarkdownFormatsIncludeCommentSidecar(t *testing.T) {
	setDocsDryRunEnv(t)

	for _, identity := range []string{"user", "bot"} {
		for _, docFormat := range []string{"markdown", "im-markdown"} {
			t.Run(identity+"/"+docFormat, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				t.Cleanup(cancel)
				result, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args: []string{
						"docs", "+fetch",
						"--doc", "doxcnDryRunComments",
						"--doc-format", docFormat,
						"--dry-run",
					},
					DefaultAs: identity,
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				require.Equal(t, "markdown", clie2e.DryRunGet(result.Stdout, "api.0.body.format").String(), "stdout:\n%s", result.Stdout)

				raw := clie2e.DryRunGet(result.Stdout, "api.0.body.extra_param").String()
				var extra map[string]bool
				require.NoError(t, json.Unmarshal([]byte(raw), &extra))
				require.Equal(t, map[string]bool{
					"enable_user_cite_reference_map": true,
					"include_comments":               true,
					"return_html5_block_data":        true,
				}, extra, "stdout:\n%s", result.Stdout)
			})
		}
	}
}

func TestDocsFetchCommentsFlagIsRemovedFromHelpAndRejected(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	help, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"docs", "+fetch", "--help"}, DefaultAs: "bot"})
	require.NoError(t, err)
	help.AssertExitCode(t, 0)
	require.NotContains(t, help.Stdout, "--comments")
	require.NotContains(t, help.Stdout, "docs:document.comment:read")

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"docs", "+fetch", "--doc", "doxcnDryRunComments", "--comments", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Empty(t, result.Stdout)

	errJSON := gjson.Get(result.Stderr, "error")
	require.True(t, errJSON.Exists(), "stderr missing typed error envelope:\n%s", result.Stderr)
	require.Equal(t, "validation", errJSON.Get("type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", errJSON.Get("subtype").String(), result.Stderr)
	require.Contains(t, errJSON.Get("message").String(), `unknown flag "--comments"`, result.Stderr)
	require.Equal(t, int64(1), errJSON.Get("params.#").Int(), result.Stderr)
	require.Equal(t, "--comments", errJSON.Get("params.0.name").String(), result.Stderr)
	require.Equal(t, "unknown flag", errJSON.Get("params.0.reason").String(), result.Stderr)
	require.NotEmpty(t, errJSON.Get("hint").String(), "unknown flag error must include recovery guidance: %s", result.Stderr)
}

func TestDocsCommandsHideFormatHelpButKeepCompatibility(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	for _, command := range []string{"+fetch", "+create", "+update"} {
		t.Run(command+" help", func(t *testing.T) {
			help, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"docs", command, "--help"}, DefaultAs: "bot"})
			require.NoError(t, err)
			help.AssertExitCode(t, 0)
			require.NotContains(t, help.Stdout, "--format ", "help must not advertise the compatibility output flag:\n%s", help.Stdout)
			require.NotContains(t, help.Stdout, "--json ", "help must not advertise the redundant JSON shorthand:\n%s", help.Stdout)
		})
	}

	tests := []struct {
		name string
		args []string
	}{
		{name: "fetch", args: []string{"docs", "+fetch", "--doc", "doxcnFormatCompat", "--format", "pretty", "--dry-run"}},
		{name: "create", args: []string{"docs", "+create", "--content", "<p>format compat</p>", "--format", "pretty", "--dry-run"}},
		{name: "update", args: []string{"docs", "+update", "--doc", "doxcnFormatCompat", "--command", "append", "--content", "<p>format compat</p>", "--format", "pretty", "--dry-run"}},
	}
	for _, test := range tests {
		t.Run(test.name+" accepts explicit format", func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: test.args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
		})
	}
}

func TestDocsFetchDryRunIgnoresAPIVersionCompatFlag(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+fetch",
			"--doc", "doxcnDryRunCompat",
			"--api-version", "v1",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "POST" {
		t.Fatalf("method=%q, want POST\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/docs_ai/v1/documents/doxcnDryRunCompat/fetch" {
		t.Fatalf("url=%q, want docs fetch endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.format").String(); got != "xml" {
		t.Fatalf("format=%q, want xml\nstdout:\n%s", got, out)
	}
}

func TestDocsFetchDryRunSelectionAnchorFragmentBecomesRangeStart(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+fetch",
			"--doc", "https://example.larksuite.com/wiki/wikcnDryRun#share-CUE3d6Ykno2fkexEvt8cGF8Wnse",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/docs_ai/v1/documents/wikcnDryRun/fetch" {
		t.Fatalf("url=%q, want docs fetch endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.read_option.read_mode").String(); got != "range" {
		t.Fatalf("read_mode=%q, want range\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.read_option.start_block_id").String(); got != "share-CUE3d6Ykno2fkexEvt8cGF8Wnse" {
		t.Fatalf("start_block_id=%q, want selection anchor\nstdout:\n%s", got, out)
	}
}

func TestDocsFetchDryRunUnsupportedSelectionAnchorFragmentStaysFull(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+fetch",
			"--doc", "https://example.larksuite.com/wiki/wikcnDryRun#part-CUE3d6Ykno2fkexEvt8cGF8Wnse",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.body.read_option").Raw; got != "" {
		t.Fatalf("read_option=%s, want omitted for unsupported selection anchor\nstdout:\n%s", got, out)
	}
}

func setDocsDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "docs_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "docs_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
