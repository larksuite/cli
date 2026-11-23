// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSlidesMediaDownloadDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	tests := []struct {
		name       string
		args       []string
		wantOutput string
	}{
		{
			name: "explicit output path",
			args: []string{
				"slides", "+media-download",
				"--file-token", "media_tok_dryrun",
				"--output", "assets/cover.jpg",
				"--dry-run",
			},
			wantOutput: "assets/cover.jpg",
		},
		{
			name: "default output-dir",
			args: []string{
				"slides", "+media-download",
				"--file-token", "media_tok_dryrun",
				"--dry-run",
			},
			wantOutput: ".lark-slides/media/media_tok_dryrun",
		},
		{
			name: "custom output-dir",
			args: []string{
				"slides", "+media-download",
				"--file-token", "media_tok_dryrun",
				"--output-dir", "downloads",
				"--dry-run",
			},
			wantOutput: "downloads/media_tok_dryrun",
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
			result.AssertExitCode(t, 0)

			stdout := result.Stdout

			require.Equal(t, "media_tok_dryrun", gjson.Get(stdout, "data.file_token").String(), stdout)
			require.Equal(t, tt.wantOutput, gjson.Get(stdout, "data.output").String(), stdout)

			require.Equal(t, "GET", gjson.Get(stdout, "data.api.0.method").String(),
				"first request must be direct download GET; stdout:\n%s", stdout)
			require.Equal(t, "/open-apis/drive/v1/medias/media_tok_dryrun/download",
				gjson.Get(stdout, "data.api.0.url").String(),
				"first request URL must be direct download; stdout:\n%s", stdout)

			require.Equal(t, "GET", gjson.Get(stdout, "data.api.1.method").String(),
				"second request must be preview fallback GET; stdout:\n%s", stdout)
			require.Equal(t, "/open-apis/drive/v1/medias/media_tok_dryrun/preview_download",
				gjson.Get(stdout, "data.api.1.url").String(),
				"second request URL must be preview_download; stdout:\n%s", stdout)
			require.Equal(t, "16", gjson.Get(stdout, "data.api.1.params.preview_type").String(),
				"preview fallback must carry preview_type=16 (source-file preview); stdout:\n%s", stdout)
		})
	}
}

func TestSlidesMediaDownloadValidationDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+media-download",
			"--file-token", "",
			"--output", "out.png",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)

	require.Empty(t, result.Stdout, "rejected command must not print success envelope: %s", result.Stdout)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--file-token", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
}

func TestSlidesMediaDownloadOutputConflictDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+media-download",
			"--file-token", "tok",
			"--output", "a.png",
			"--output-dir", "dir",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)

	require.Empty(t, result.Stdout, "rejected command must not print success envelope: %s", result.Stdout)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "--output", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "cannot be combined", result.Stderr)
}
