// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

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
		validTypes := map[string]bool{
			"file":     true,
			"docx":     true,
			"bitable":  true,
			"doc":      true,
			"sheet":    true,
			"mindnote": true,
			"folder":   true,
			"slides":   true,
		}
		fileType := strings.ToLower(runtime.Str("type"))
		if !validTypes[fileType] {
			return output.ErrValidation("unsupported file type: %s. Supported types: file, docx, bitable, doc, sheet, mindnote, folder, slides", fileType)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		fileToken := runtime.Str("file-token")
		fileType := runtime.Str("type")
		folderToken := runtime.Str("folder-token")

		dry := common.NewDryRunAPI().
			Desc("Move file or folder in Drive")

		dry.POST("/open-apis/drive/v1/files/:file_token/move").
			Desc("[1] Move file/folder").
			Set("file_token", fileToken).
			Body(map[string]interface{}{
				"type":         fileType,
				"folder_token": folderToken,
			})

		// If moving a folder, show the async task check step
		if fileType == "folder" {
			dry.GET("/open-apis/drive/v1/files/task_check").
				Desc("[2] Poll async task status (for folder move)").
				Set("task_id", "<task_id>")
		}

		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := runtime.Str("file-token")
		fileType := strings.ToLower(runtime.Str("type"))
		folderToken := runtime.Str("folder-token")

		if err := validate.ResourceName(fileToken, "--file-token"); err != nil {
			return output.ErrValidation("%s", err)
		}

		// If folder-token is empty, get the root folder token
		if folderToken == "" {
			fmt.Fprintf(runtime.IO().ErrOut, "No target folder specified, getting root folder...\n")
			rootToken, err := getRootFolderToken(ctx, runtime)
			if err != nil {
				return err
			}
			folderToken = rootToken
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Moving %s %s to folder %s...\n", fileType, common.MaskToken(fileToken), common.MaskToken(folderToken))

		requestBody := map[string]interface{}{
			"type":         fileType,
			"folder_token": folderToken,
		}

		data, err := runtime.CallAPI(
			"POST",
			fmt.Sprintf("/open-apis/drive/v1/files/%s/move", validate.EncodePathSegment(fileToken)),
			nil,
			requestBody,
		)
		if err != nil {
			return err
		}

		// If moving a folder, need to poll async task
		if fileType == "folder" {
			taskID := common.GetString(data, "task_id")
			if taskID == "" {
				return output.Errorf(output.ExitAPI, "api_error", "move folder returned no task_id")
			}

			fmt.Fprintf(runtime.IO().ErrOut, "Folder move is async, polling task %s...\n", taskID)

			status, err := pollMoveTask(ctx, runtime, taskID)
			if err != nil {
				return err
			}

			runtime.Out(map[string]interface{}{
				"task_id":      taskID,
				"status":       status,
				"file_token":   fileToken,
				"folder_token": folderToken,
			}, nil)
		} else {
			runtime.Out(map[string]interface{}{
				"file_token":   fileToken,
				"folder_token": folderToken,
				"type":         fileType,
			}, nil)
		}

		return nil
	},
}

// getRootFolderToken gets the user's root folder token
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

// pollMoveTask polls the async task status for folder move
func pollMoveTask(ctx context.Context, runtime *common.RuntimeContext, taskID string) (string, error) {
	maxRetries := 30
	delay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		data, err := runtime.CallAPI("GET", "/open-apis/drive/v1/files/task_check", map[string]interface{}{"task_id": taskID}, nil)
		if err != nil {
			return "", err
		}

		status := common.GetString(data, "status")
		if status == "success" {
			fmt.Fprintf(runtime.IO().ErrOut, "Folder move completed successfully.\n")
			return status, nil
		}
		if status == "failed" {
			return "", output.Errorf(output.ExitAPI, "api_error", "folder move task failed")
		}

		time.Sleep(delay)
	}

	return "", output.Errorf(output.ExitAPI, "timeout", "folder move task did not complete in time")
}
