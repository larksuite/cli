// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestDriveDownloadDryRun_EntityLookup pins the lookup, conditional legacy
// fallback, and downstream requests for every accepted input form.
func TestDriveDownloadDryRun_EntityLookup(t *testing.T) {
	for _, tc := range []struct {
		name, flag, input string
		wiki, explicit    bool
	}{
		{name: "file default name", flag: "--file-token", input: "inputToken"},
		{name: "file explicit output", flag: "--file-token", input: "inputToken", explicit: true},
		{name: "file URL", flag: "--url", input: "https://example.com/file/inputToken", explicit: true},
		{name: "wiki URL", flag: "--url", input: "https://example.com/wiki/inputToken", wiki: true},
		{name: "wiki token", flag: "--wiki-token", input: "inputToken", wiki: true, explicit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setDriveDryRunConfigEnv(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			args := []string{"drive", "+download", tc.flag, tc.input, "--dry-run"}
			if tc.explicit {
				args = append(args, "--output", "./artifacts/report.bin")
			}
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			out := result.Stdout
			require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String())
			require.Equal(t, "/open-apis/drive/v2/files/query_by_token", clie2e.DryRunGet(out, "api.0.url").String())
			require.Equal(t, "inputToken", clie2e.DryRunGet(out, "api.0.params.token").String())
			require.Contains(t, clie2e.DryRunGet(out, "api.0.desc").String(), "Best-effort")
			index := 1
			if tc.wiki {
				require.Equal(t, "/open-apis/wiki/v2/spaces/get_node", clie2e.DryRunGet(out, "api.1.url").String())
				require.Equal(t, "inputToken", clie2e.DryRunGet(out, "api.1.params.token").String())
				require.Contains(t, clie2e.DryRunGet(out, "api.1.desc").String(), "Only if entity lookup fails")
				require.Equal(t, "inputToken", clie2e.DryRunGet(out, "wiki_token").String())
				index++
			} else {
				require.Equal(t, "inputToken", clie2e.DryRunGet(out, "fallback_file_token").String())
			}
			field := func(suffix string) string { return fmt.Sprintf("api.%d.%s", index, suffix) }
			require.Equal(t, "GET", clie2e.DryRunGet(out, field("method")).String())
			require.Equal(t, "/open-apis/drive/v1/permissions/resolved_file_token/members/auth", clie2e.DryRunGet(out, field("url")).String())
			require.Equal(t, "file", clie2e.DryRunGet(out, field("params.type")).String())
			require.Equal(t, "export", clie2e.DryRunGet(out, field("params.action")).String())
			index++
			if !tc.explicit {
				require.Equal(t, "POST", clie2e.DryRunGet(out, field("method")).String())
				require.Equal(t, "/open-apis/drive/v1/metas/batch_query", clie2e.DryRunGet(out, field("url")).String())
				require.Equal(t, "resolved_file_token", clie2e.DryRunGet(out, field("body.request_docs.0.doc_token")).String())
				require.Equal(t, "file", clie2e.DryRunGet(out, field("body.request_docs.0.doc_type")).String())
				require.Equal(t, "<Content-Disposition filename | metadata title | token>", clie2e.DryRunGet(out, "output").String())
				index++
			} else {
				require.Equal(t, "./artifacts/report.bin", clie2e.DryRunGet(out, "output").String())
			}
			require.Equal(t, "GET", clie2e.DryRunGet(out, field("method")).String())
			require.Equal(t, "/open-apis/drive/v1/files/resolved_file_token/download", clie2e.DryRunGet(out, field("url")).String())
			require.Equal(t, int64(index+1), clie2e.DryRunGet(out, "api.#").Int())
		})
	}
}

// TestDriveDownloadDryRun_RejectsMutuallyExclusiveInputs verifies passing more
// than one source flag fails validation instead of silently picking one.
func TestDriveDownloadDryRun_RejectsMutuallyExclusiveInputs(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+download",
			"--file-token", "fileDryRunDownload",
			"--wiki-token", "wikiDryRunDownload",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	if result.Stdout != "" {
		t.Fatalf("stdout must stay empty on validation failure, got:\n%s", result.Stdout)
	}
	if got := clie2e.DryRunGet(result.Stderr, "error.type").String(); got != "validation" {
		t.Fatalf("error.type=%q, want validation\nstderr:\n%s", got, result.Stderr)
	}
	if got := clie2e.DryRunGet(result.Stderr, "error.subtype").String(); got != "invalid_argument" {
		t.Fatalf("error.subtype=%q, want invalid_argument\nstderr:\n%s", got, result.Stderr)
	}
	if got := clie2e.DryRunGet(result.Stderr, "error.param").String(); got != "--file-token" {
		t.Fatalf("error.param=%q, want --file-token\nstderr:\n%s", got, result.Stderr)
	}
	if got := clie2e.DryRunGet(result.Stderr, "error.message").String(); got == "" {
		t.Fatalf("error.message must be non-empty\nstderr:\n%s", result.Stderr)
	}
}
