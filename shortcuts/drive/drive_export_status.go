// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var DriveExportStatus = common.Shortcut{
	Service:     "drive",
	Command:     "+export-status",
	Description: "Query an export task result by ticket",
	Risk:        "read",
	Scopes: []string{
		"docs:document:export",
		//"drive:export:readonly",
	},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "token", Desc: "source document token", Required: true},
		{Name: "ticket", Desc: "export task ticket", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validate.ResourceName(runtime.Str("token"), "--token"); err != nil {
			return output.ErrValidation("%s", err)
		}
		if err := validate.ResourceName(runtime.Str("ticket"), "--ticket"); err != nil {
			return output.ErrValidation("%s", err)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET("/open-apis/drive/v1/export_tasks/:ticket").
			Set("ticket", runtime.Str("ticket")).
			Params(map[string]interface{}{"token": runtime.Str("token")})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		status, err := getDriveExportStatus(runtime, runtime.Str("token"), runtime.Str("ticket"))
		if err != nil {
			return err
		}

		out := map[string]interface{}{
			"ticket":          status.Ticket,
			"ready":           status.Ready(),
			"failed":          status.Failed(),
			"job_status":      status.StatusLabel(),
			"job_status_code": status.JobStatus,
		}
		if status.FileExtension != "" {
			out["file_extension"] = status.FileExtension
		}
		if status.DocType != "" {
			out["doc_type"] = status.DocType
		}
		if status.FileName != "" {
			out["file_name"] = ensureExportFileExtension(sanitizeExportFileName(status.FileName, status.Ticket), status.FileExtension)
		}
		if status.FileToken != "" {
			out["file_token"] = status.FileToken
		}
		if status.FileSize > 0 {
			out["file_size"] = status.FileSize
		}
		if status.JobErrorMsg != "" {
			out["job_error_msg"] = status.JobErrorMsg
		}

		runtime.Out(out, nil)
		return nil
	},
}
