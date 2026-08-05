// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var driveDeleteAllowedTypes = map[string]bool{
	"file":     true,
	"docx":     true,
	"bitable":  true,
	"doc":      true,
	"sheet":    true,
	"mindnote": true,
	"folder":   true,
	"shortcut": true,
	"slides":   true,
}

// driveDeleteSpec contains the normalized input needed to issue a delete
// request against the Drive files endpoint.
type driveDeleteSpec struct {
	FileToken string
	FileType  string
}

// DriveDelete deletes a Drive file or folder with async=true. When the response
// includes a task_id, it performs a bounded task_check poll before returning a
// resume command for unfinished tasks.
var DriveDelete = common.Shortcut{
	Service:     "drive",
	Command:     "+delete",
	Description: "Delete a file or folder in Drive",
	Risk:        "high-risk-write",
	Scopes:      []string{"space:document:delete", "drive:drive.metadata:readonly"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "file or folder token to delete", Required: true},
		{Name: "type", Desc: "file type (file, docx, bitable, doc, sheet, mindnote, folder, shortcut, slides)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveDeleteSpec(driveDeleteSpec{
			FileToken: runtime.Str("file-token"),
			FileType:  strings.ToLower(runtime.Str("type")),
		})
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := driveDeleteSpec{
			FileToken: runtime.Str("file-token"),
			FileType:  strings.ToLower(runtime.Str("type")),
		}

		dry := common.NewDryRunAPI().
			Desc("Delete file or folder in Drive")

		dry.DELETE("/open-apis/drive/v1/files/:file_token").
			Desc("[1] Delete file/folder").
			Set("file_token", spec.FileToken).
			Params(driveDeleteParams(spec))

		dry.GET("/open-apis/drive/v1/files/task_check").
			Desc("[2] Poll async delete task status when task_id is returned").
			Params(driveTaskCheckParams("<task_id>"))

		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := driveDeleteSpec{
			FileToken: runtime.Str("file-token"),
			FileType:  strings.ToLower(runtime.Str("type")),
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Deleting %s %s...\n", spec.FileType, common.MaskToken(spec.FileToken))

		data, err := runtime.CallAPITyped(
			"DELETE",
			fmt.Sprintf("/open-apis/drive/v1/files/%s", validate.EncodePathSegment(spec.FileToken)),
			driveDeleteParams(spec),
			nil,
		)
		if err != nil {
			return err
		}

		taskID := common.GetString(data, "task_id")
		if taskID == "" {
			runtime.Out(map[string]interface{}{
				"deleted":    true,
				"file_token": spec.FileToken,
				"type":       spec.FileType,
			}, nil)
			return nil
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Delete is async, polling task %s...\n", taskID)

		status, ready, err := pollDriveTaskCheck(runtime, taskID)
		if err != nil {
			return err
		}

		out := map[string]interface{}{
			"task_id":    taskID,
			"status":     status.StatusLabel(),
			"file_token": spec.FileToken,
			"type":       spec.FileType,
			"ready":      ready,
		}
		if ready {
			out["deleted"] = true
		}
		if !ready {
			nextCommand := driveTaskCheckResultCommand(taskID, string(runtime.As()))
			fmt.Fprintf(runtime.IO().ErrOut, "Delete task is still in progress. Continue with: %s\n", nextCommand)
			out["timed_out"] = true
			out["next_command"] = nextCommand
		}

		runtime.Out(out, nil)
		return nil
	},
}

func driveDeleteParams(spec driveDeleteSpec) map[string]interface{} {
	return map[string]interface{}{
		"type":  spec.FileType,
		"async": true,
	}
}

func validateDriveDeleteSpec(spec driveDeleteSpec) error {
	if err := validate.ResourceName(spec.FileToken, "--file-token"); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--file-token")
	}
	if spec.FileType == "wiki" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported file type: wiki. This shortcut only supports Drive files and folders; wiki documents are not supported").WithParam("--type")
	}
	if !driveDeleteAllowedTypes[spec.FileType] {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported file type: %s. Supported types: file, docx, bitable, doc, sheet, mindnote, folder, shortcut, slides", spec.FileType).WithParam("--type")
	}
	return nil
}
