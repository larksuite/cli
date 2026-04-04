// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// DriveExportDownload downloads an already-generated export artifact when the
// caller has a file token from a previous export task.
var DriveExportDownload = common.Shortcut{
	Service:     "drive",
	Command:     "+export-download",
	Description: "Download an exported file by file_token",
	Risk:        "read",
	Scopes: []string{
		"docs:document:export",
	},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "exported file token", Required: true},
		{Name: "file-name", Desc: "preferred output filename (optional)"},
		{Name: "output-dir", Default: ".", Desc: "local output directory (default: current directory)"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validate.ResourceName(runtime.Str("file-token"), "--file-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET("/open-apis/drive/v1/export_tasks/file/:file_token/download").
			Set("file_token", runtime.Str("file-token")).
			Set("output_dir", runtime.Str("output-dir"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		// Reuse the shared export download helper so overwrite checks, filename
		// resolution, and output metadata stay consistent with drive +export.
		out, err := downloadDriveExportFile(
			ctx,
			runtime,
			runtime.Str("file-token"),
			runtime.Str("output-dir"),
			runtime.Str("file-name"),
			runtime.Bool("overwrite"),
		)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}
