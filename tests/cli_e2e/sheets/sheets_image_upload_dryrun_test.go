// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
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
// for tokens prefixed with "fake_office_" (the synthetic token an imported
// office spreadsheet carries) the backend requires "office_sheet_file". The
// three covered entries — sheets +media-upload (backward), sheets
// +cells-set-image, and sheets +create-float-image — are every image-upload
// surface that the office/native split fans out to.
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
			name: "media-upload native",
			args: []string{
				"sheets", "+media-upload",
				"--spreadsheet-token", "shtDryRunNative",
				"--file", "img.png",
				"--dry-run",
			},
			token:          "shtDryRunNative",
			wantParentType: "sheet_image",
		},
		{
			name: "media-upload office",
			args: []string{
				"sheets", "+media-upload",
				"--spreadsheet-token", "fake_office_dryrun",
				"--file", "img.png",
				"--dry-run",
			},
			token:          "fake_office_dryrun",
			wantParentType: "office_sheet_file",
		},
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
				"--spreadsheet-token", "fake_office_dryrun",
				"--sheet-id", "sheet1",
				"--range", "A1",
				"--image", "img.png",
				"--dry-run",
			},
			token:          "fake_office_dryrun",
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
