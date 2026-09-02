// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestSheets_ImageUploadDryRunParentType pins the parent_type the sheets
// image-upload shortcuts emit in --dry-run output for native vs. imported
// "office" spreadsheets. For native tokens parent_type must be "sheet_image";
// for tokens carrying the interleaved "OFL0X" marker the backend requires
// "office_sheet_file". The covered entries — sheets +cells-set-image and
// sheets +float-image-create — are every image-upload surface that the
// office/native split fans out to.
//
// The wiki rows carry an office-shaped node_token on purpose. A preview cannot
// know what a wiki node resolves to — that needs the get_node call a dry-run
// must not make — so it must not read the office/native split out of the
// node_token it happens to be holding. Those rows show the same token as a
// /wiki/ URL and as a raw spreadsheet token and expect different answers, which
// only holds if the preview reads the ref's kind.
func TestSheets_ImageUploadDryRunParentType(t *testing.T) {
	setSheetsDryRunEnv(t)

	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "img.png"), []byte("png-bytes"), 0o600))

	type tc struct {
		name           string
		args           []string
		token          string
		wantParentType string
	}
	tests := []tc{
		{
			name: "cells-set-image native",
			args: []string{
				"sheets", "+cells-set-image",
				"--spreadsheet-token", "shtDryRunNative",
				"--sheet-id", "sheet1",
				"--range", "A1",
				"--image", "img.png",
				"--dry-run",
			},
			token:          "shtDryRunNative",
			wantParentType: "sheet_image",
		},
		{
			name: "cells-set-image office",
			args: []string{
				"sheets", "+cells-set-image",
				"--spreadsheet-token", "aaaaOaaaaFaaaaLaaaa0aaaaXaaa",
				"--sheet-id", "sheet1",
				"--range", "A1",
				"--image", "img.png",
				"--dry-run",
			},
			token:          "aaaaOaaaaFaaaaLaaaa0aaaaXaaa",
			wantParentType: "office_sheet_file",
		},
		{
			name: "cells-set-image wiki ref stays native",
			args: []string{
				"sheets", "+cells-set-image",
				"--url", "https://example.feishu.cn/wiki/aaaaOaaaaFaaaaLaaaa0aaaaXaaa",
				"--sheet-id", "sheet1",
				"--range", "A1",
				"--image", "img.png",
				"--dry-run",
			},
			// parent_node previews the still-unresolved node_token: sheets
			// dry-runs show the input token as given. parent_type is the field
			// that must not be derived from it.
			token:          "aaaaOaaaaFaaaaLaaaa0aaaaXaaa",
			wantParentType: "sheet_image",
		},
		{
			name: "float-image-create wiki ref stays native",
			args: []string{
				"sheets", "+float-image-create",
				"--url", "https://example.feishu.cn/wiki/aaaaOaaaaFaaaaLaaaa0aaaaXaaa",
				"--sheet-id", "sheet1",
				"--image-name", "img.png",
				"--image", "img.png",
				"--position-row", "0",
				"--position-col", "A",
				"--size-width", "100",
				"--size-height", "100",
				"--dry-run",
			},
			token:          "aaaaOaaaaFaaaaLaaaa0aaaaXaaa",
			wantParentType: "sheet_image",
		},
		{
			name: "float-image-create office",
			args: []string{
				"sheets", "+float-image-create",
				"--spreadsheet-token", "aaaaOaaaaFaaaaLaaaa0aaaaXaaa",
				"--sheet-id", "sheet1",
				"--image-name", "img.png",
				"--image", "img.png",
				"--position-row", "0",
				"--position-col", "A",
				"--size-width", "100",
				"--size-height", "100",
				"--dry-run",
			},
			token:          "aaaaOaaaaFaaaaLaaaa0aaaaXaaa",
			wantParentType: "office_sheet_file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "user",
				WorkDir:   workDir,
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "data.api.0 must be the drive upload; stdout:\n%s", out)
			require.Equal(t, "/open-apis/drive/v1/medias/upload_all",
				clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
			require.Equal(t, tt.wantParentType, clie2e.DryRunGet(out, "api.0.body.parent_type").String(),
				"parent_type for token %q must be %q; stdout:\n%s", tt.token, tt.wantParentType, out)
			require.Equal(t, tt.token, clie2e.DryRunGet(out, "api.0.body.parent_node").String(),
				"parent_node must equal the spreadsheet token; stdout:\n%s", out)
		})
	}
}

// TestSheets_ImageUploadDryRunChunked pins that the preview follows the same
// 20 MB branch Execute takes. Under the ceiling the CLI previews one
// upload_all; past it, the upload_prepare / upload_part / upload_finish trio.
// A preview that promised a single-part upload for a file the CLI will send in
// chunks is a preview of a different request.
//
// The oversized fixture is sparse: only its stat'd size decides the branch, so
// writing 20 MB of real bytes would cost every CI run the same for no extra
// signal.
func TestSheets_ImageUploadDryRunChunked(t *testing.T) {
	setSheetsDryRunEnv(t)

	const singlePartCeiling = 20 * 1024 * 1024

	workDir := t.TempDir()
	small, err := os.Create(filepath.Join(workDir, "small.png"))
	require.NoError(t, err)
	require.NoError(t, small.Truncate(singlePartCeiling))
	require.NoError(t, small.Close())
	big, err := os.Create(filepath.Join(workDir, "big.png"))
	require.NoError(t, err)
	require.NoError(t, big.Truncate(singlePartCeiling+1))
	require.NoError(t, big.Close())

	tests := []struct {
		name      string
		file      string
		wantSteps []string
	}{
		{
			name:      "at the ceiling stays single-part",
			file:      "small.png",
			wantSteps: []string{"/open-apis/drive/v1/medias/upload_all"},
		},
		{
			name: "one byte past it goes chunked",
			file: "big.png",
			wantSteps: []string{
				"/open-apis/drive/v1/medias/upload_prepare",
				"/open-apis/drive/v1/medias/upload_part",
				"/open-apis/drive/v1/medias/upload_finish",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"sheets", "+cells-set-image",
					"--spreadsheet-token", "shtDryRunNative",
					"--sheet-id", "sheet1",
					"--range", "A1",
					"--image", tt.file,
					"--dry-run",
				},
				DefaultAs: "user",
				WorkDir:   workDir,
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			for i, want := range tt.wantSteps {
				require.Equal(t, want, clie2e.DryRunGet(out, fmt.Sprintf("api.%d.url", i)).String(),
					"step %d; stdout:\n%s", i, out)
			}
			// The tool call follows the upload steps and nothing else does.
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRunNative/tools/invoke_write",
				clie2e.DryRunGet(out, fmt.Sprintf("api.%d.url", len(tt.wantSteps))).String(), "stdout:\n%s", out)
			require.False(t, clie2e.DryRunGet(out, fmt.Sprintf("api.%d", len(tt.wantSteps)+1)).Exists(),
				"no step should follow the tool call; stdout:\n%s", out)
		})
	}
}
