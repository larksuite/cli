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

func TestDocsMediaDownloadDryRun_PlansExportAuthBeforeMediaDownload(t *testing.T) {
	setDocsDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+media-download",
			"--token", "mediaDryRunDownload",
			"--output", "./artifacts/media.bin",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.#").Int(); got != 2 {
		t.Fatalf("api count=%d, want 2\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/permissions/mediaDryRunDownload/members/auth" {
		t.Fatalf("api.0.url=%q, want permission auth\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.type").String(); got != "file" {
		t.Fatalf("api.0.params.type=%q, want file\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.action").String(); got != "export" {
		t.Fatalf("api.0.params.action=%q, want export\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/medias/mediaDryRunDownload/download" {
		t.Fatalf("api.1.url=%q, want media download\nstdout:\n%s", got, out)
	}
}

func TestDocsMediaDownloadDryRun_WhiteboardSkipsExportAuth(t *testing.T) {
	setDocsDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+media-download",
			"--token", "boardDryRunDownload",
			"--type", "whiteboard",
			"--output", "./artifacts/board.png",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.#").Int(); got != 1 {
		t.Fatalf("api count=%d, want 1\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/board/v1/whiteboards/boardDryRunDownload/download_as_image" {
		t.Fatalf("api.0.url=%q, want whiteboard download only\nstdout:\n%s", got, out)
	}
}

func TestDocsMediaPreviewDryRun_PlansPreviewDownload(t *testing.T) {
	setDocsDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+media-preview",
			"--token", "mediaDryRunPreview",
			"--output", "./artifacts/preview.bin",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.#").Int(); got != 1 {
		t.Fatalf("api count=%d, want 1\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/medias/mediaDryRunPreview/preview_download" {
		t.Fatalf("api.0.url=%q, want media preview download\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.preview_type").String(); got != "16" {
		t.Fatalf("api.0.params.preview_type=%q, want source-file preview type 16\nstdout:\n%s", got, out)
	}
}
