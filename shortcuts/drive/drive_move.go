// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// DriveMove moves a Drive file or folder and handles the async task polling
// required by folder moves.
var DriveMove = common.Shortcut{
	Service:     "drive",
	Command:     "+move",
	Description: "Move a file or folder to another location in Drive",
	Risk:        "write",
	Scopes:      []string{"space:document:move"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file-token", Desc: "file or folder token to move", Required: true},
		{Name: "type", Desc: "file type (file, docx, bitable, doc, sheet, mindnote, folder, slides)", Required: true},
		{Name: "folder-token", Desc: "target folder token (default: root folder)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveMoveSpec(driveMoveSpec{
			FileToken:   runtime.Str("file-token"),
			FileType:    strings.ToLower(runtime.Str("type")),
			FolderToken: runtime.Str("folder-token"),
		})
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := driveMoveSpec{
			FileToken:   runtime.Str("file-token"),
			FileType:    strings.ToLower(runtime.Str("type")),
			FolderToken: runtime.Str("folder-token"),
		}

		dry := common.NewDryRunAPI().
			Desc("Move file or folder in Drive")

		dry.POST("/open-apis/drive/v1/files/:file_token/move").
			Desc("[1] Move file/folder").
			Set("file_token", spec.FileToken).
			Body(spec.RequestBody())

		// If moving a folder, show the async task check step
		if spec.FileType == "folder" {
			dry.GET("/open-apis/drive/v1/files/task_check").
				Desc("[2] Poll async task status (for folder move)").
				Params(driveTaskCheckParams("<task_id>"))
		}

		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := driveMoveSpec{
			FileToken:   runtime.Str("file-token"),
			FileType:    strings.ToLower(runtime.Str("type")),
			FolderToken: runtime.Str("folder-token"),
		}

		// Default to the caller's root folder so the command can move items
		// without requiring an explicit destination in common cases.
		if spec.FolderToken == "" {
			fmt.Fprintf(runtime.IO().ErrOut, "No target folder specified, getting root folder...\n")
			rootToken, err := getRootFolderToken(ctx, runtime)
			if err != nil {
				return err
			}
			if rootToken == "" {
				return output.Errorf(output.ExitAPI, "api_error", "get root folder token failed, root folder is empty")
			}
			spec.FolderToken = rootToken
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Moving %s %s to folder %s...\n", spec.FileType, common.MaskToken(spec.FileToken), common.MaskToken(spec.FolderToken))

		data, err := runtime.CallAPI(
			"POST",
			fmt.Sprintf("/open-apis/drive/v1/files/%s/move", validate.EncodePathSegment(spec.FileToken)),
			nil,
			spec.RequestBody(),
		)
		if err != nil {
			return err
		}

		// Folder moves are asynchronous; file moves complete in the initial call.
		if spec.FileType == "folder" {
			taskID := common.GetString(data, "task_id")
			if taskID == "" {
				return output.Errorf(output.ExitAPI, "api_error", "move folder returned no task_id")
			}

			fmt.Fprintf(runtime.IO().ErrOut, "Folder move is async, polling task %s...\n", taskID)

			status, ready, err := pollDriveTaskCheck(runtime, taskID)
			if err != nil {
				return err
			}

			// Include both the source and destination identifiers so a timed-out
			// folder move can be resumed or inspected without reconstructing inputs.
			out := map[string]interface{}{
				"task_id":      taskID,
				"status":       status.StatusLabel(),
				"file_token":   spec.FileToken,
				"folder_token": spec.FolderToken,
				"ready":        ready,
			}
			if !ready {
				nextCommand := driveTaskCheckResultCommand(taskID, string(runtime.As()))
				fmt.Fprintf(runtime.IO().ErrOut, "Folder move task is still in progress. Continue with: %s\n", nextCommand)
				out["timed_out"] = true
				out["next_command"] = nextCommand
			}

			runtime.Out(out, nil)
		} else {
			// Non-folder moves are synchronous, so the initial request is the final
			// outcome and no follow-up task metadata is needed.
			runtime.Out(map[string]interface{}{
				"file_token":   spec.FileToken,
				"folder_token": spec.FolderToken,
				"type":         spec.FileType,
			}, nil)
		}

		return nil
	},
}

// getRootFolderToken resolves the caller's Drive root folder token so other
// commands can safely use it as a default destination.
func getRootFolderToken(ctx context.Context, runtime *common.RuntimeContext) (string, error) {
	data, err := runtime.CallAPI("GET", "/open-apis/drive/explorer/v2/root_folder/meta", nil, nil)
	if err != nil {
		return "", err
	}

	token := common.GetString(data, "token")
	if token == "" {
		return "", output.Errorf(output.ExitAPI, "api_error", "root_folder/meta returned no token")
	}

	return token, nil
}
