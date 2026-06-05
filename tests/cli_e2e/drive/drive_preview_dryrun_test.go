// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrivePreviewDryRun_ListOnly verifies preview dry-run request structure
// for list mode.
func TestDrivePreviewDryRun_ListOnly(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+preview",
			"--file-token", "fileDryRunPreview",
			"--list-only",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := gjson.Get(out, "api.0.method").String(); got != "POST" {
		t.Fatalf("method=%q, want POST\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.url").String(); got != "/open-apis/drive/v1/medias/fileDryRunPreview/preview_result" {
		t.Fatalf("url=%q, want preview_result endpoint\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "mode").String(); got != "list" {
		t.Fatalf("mode=%q, want list\nstdout:\n%s", got, out)
	}
}

// TestDrivePreviewDryRun_Download verifies preview dry-run request structure
// for download mode.
func TestDrivePreviewDryRun_Download(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+preview",
			"--file-token", "fileDryRunPreview",
			"--type", "pdf",
			"--version", "12",
			"--output", "./artifacts/report",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := gjson.Get(out, "api.#").Int(); got != 2 {
		t.Fatalf("api count=%d, want 2\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.body.version").String(); got != "12" {
		t.Fatalf("version=%q, want 12\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.1.method").String(); got != "GET" {
		t.Fatalf("download method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.1.url").String(); got != "/open-apis/drive/v1/medias/fileDryRunPreview/preview_download" {
		t.Fatalf("download url=%q, want preview_download endpoint\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.1.params.preview_type").String(); got != "<selected type_code from preview_result>" {
		t.Fatalf("preview_type=%q, want placeholder\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.1.params.version").String(); got != "12" {
		t.Fatalf("download version=%q, want 12\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "requested_type").String(); got != "pdf" {
		t.Fatalf("requested_type=%q, want pdf\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "output").String(); got != "./artifacts/report" {
		t.Fatalf("output=%q, want ./artifacts/report\nstdout:\n%s", got, out)
	}
}

// TestDriveCoverDryRun_Download verifies cover dry-run request structure for
// download mode.
func TestDriveCoverDryRun_Download(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+cover",
			"--file-token", "fileDryRunCover",
			"--spec", "square",
			"--output", "./artifacts/cover",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := gjson.Get(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.url").String(); got != "/open-apis/drive/v1/medias/fileDryRunCover/preview_download" {
		t.Fatalf("url=%q, want preview_download endpoint\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.params.preview_type").String(); got != "1" {
		t.Fatalf("preview_type=%q, want 1\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.params.bus_type").Exists(); got {
		t.Fatalf("bus_type should be omitted for square crop flow\nstdout:\n%s", out)
	}
	if got := gjson.Get(out, "api.0.params.platform").Exists(); got {
		t.Fatalf("platform should be omitted when using default platform\nstdout:\n%s", out)
	}
	if got := gjson.Get(out, "api.0.params.width").String(); got != "360" {
		t.Fatalf("width=%q, want 360\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.params.height").String(); got != "360" {
		t.Fatalf("height=%q, want 360\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.params.policy").String(); got != "near" {
		t.Fatalf("policy=%q, want near\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "selected_spec").String(); got != "square" {
		t.Fatalf("selected_spec=%q, want square\nstdout:\n%s", got, out)
	}
}
