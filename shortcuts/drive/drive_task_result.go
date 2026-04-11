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

// DriveTaskResult exposes a unified read path for the async task types produced
// by Drive import, export, and folder move flows.
var DriveTaskResult = common.Shortcut{
	Service:     "drive",
	Command:     "+task_result",
	Description: "Poll async task result for import, export, move, or delete operations",
	Risk:        "read",
	Scopes:      []string{"drive:drive.metadata:readonly"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "ticket", Desc: "async task ticket (for import/export tasks)", Required: false},
		{Name: "task-id", Desc: "async task ID (for move/delete folder tasks)", Required: false},
		{Name: "scenario", Desc: "task scenario: import, export, or task_check", Required: true},
		{Name: "file-token", Desc: "source document token used for export task status lookup", Required: false},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		scenario := strings.ToLower(runtime.Str("scenario"))
		validScenarios := map[string]bool{
			"import":     true,
			"export":     true,
			"task_check": true,
		}
		if !validScenarios[scenario] {
			return output.ErrValidation("unsupported scenario: %s. Supported scenarios: import, export, task_check", scenario)
		}

		// Validate required params based on scenario
		switch scenario {
		case "import", "export":
			if runtime.Str("ticket") == "" {
				return output.ErrValidation("--ticket is required for %s scenario", scenario)
			}
			if err := validate.ResourceName(runtime.Str("ticket"), "--ticket"); err != nil {
				return output.ErrValidation("%s", err)
			}
		case "task_check":
			if runtime.Str("task-id") == "" {
				return output.ErrValidation("--task-id is required for task_check scenario")
			}
			if err := validate.ResourceName(runtime.Str("task-id"), "--task-id"); err != nil {
				return output.ErrValidation("%s", err)
			}
		}

		// For export scenario, file-token is required
		if scenario == "export" && runtime.Str("file-token") == "" {
			return output.ErrValidation("--file-token is required for export scenario")
		}
		if scenario == "export" {
			if err := validate.ResourceName(runtime.Str("file-token"), "--file-token"); err != nil {
				return output.ErrValidation("%s", err)
			}
		}

		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		scenario := strings.ToLower(runtime.Str("scenario"))
		ticket := runtime.Str("ticket")
		taskID := runtime.Str("task-id")
		fileToken := runtime.Str("file-token")

		dry := common.NewDryRunAPI()
		dry.Desc(fmt.Sprintf("Poll async task result for %s scenario", scenario))

		switch scenario {
		case "import":
			dry.GET("/open-apis/drive/v1/import_tasks/:ticket").
				Desc("[1] Query import task result").
				Set("ticket", ticket)
		case "export":
			dry.GET("/open-apis/drive/v1/export_tasks/:ticket").
				Desc("[1] Query export task result").
				Set("ticket", ticket).
				Params(map[string]interface{}{"token": fileToken})
		case "task_check":
			dry.GET("/open-apis/drive/v1/files/task_check").
				Desc("[1] Query move/delete folder task status").
				Params(driveTaskCheckParams(taskID))
		}

		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		scenario := strings.ToLower(runtime.Str("scenario"))
		ticket := runtime.Str("ticket")
		taskID := runtime.Str("task-id")
		fileToken := runtime.Str("file-token")

		fmt.Fprintf(runtime.IO().ErrOut, "Querying %s task result...\n", scenario)

		var result map[string]interface{}
		var err error

		// Each scenario maps to a different backend API, but this shortcut keeps
		// the CLI surface uniform for resume-on-timeout workflows.
		switch scenario {
		case "import":
			result, err = queryImportTaskAndAutoGrantPermission(runtime, ticket)
		case "export":
			result, err = queryExportTask(runtime, ticket, fileToken)
		case "task_check":
			result, err = queryTaskCheck(runtime, taskID)
		}

		if err != nil {
			return err
		}

		runtime.Out(result, nil)
		return nil
	},
}

// queryImportTaskAndAutoGrantPermission returns a stable, shortcut-friendly
// view of the import task and, in bot mode, retries the current-user
// permission grant once the imported cloud document becomes ready.
func queryImportTaskAndAutoGrantPermission(runtime *common.RuntimeContext, ticket string) (map[string]interface{}, error) {
	status, err := getDriveImportStatus(runtime, ticket)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"scenario":         "import",
		"ticket":           status.Ticket,
		"type":             status.DocType,
		"ready":            status.Ready(),
		"failed":           status.Failed(),
		"job_status":       status.JobStatus,
		"job_status_label": status.StatusLabel(),
		"job_error_msg":    status.JobErrorMsg,
		"token":            status.Token,
		"url":              status.URL,
		"extra":            status.Extra,
	}
	if status.Ready() {
		if grant := common.AutoGrantCurrentUserDrivePermission(runtime, status.Token, status.DocType); grant != nil {
			result["permission_grant"] = grant
		}
	}
	return result, nil
}

// queryExportTask returns the export task status together with download metadata
// once the backend has produced the exported file.
func queryExportTask(runtime *common.RuntimeContext, ticket, fileToken string) (map[string]interface{}, error) {
	status, err := getDriveExportStatus(runtime, fileToken, ticket)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"scenario":         "export",
		"ticket":           status.Ticket,
		"ready":            status.Ready(),
		"failed":           status.Failed(),
		"file_extension":   status.FileExtension,
		"type":             status.DocType,
		"file_name":        status.FileName,
		"file_token":       status.FileToken,
		"file_size":        status.FileSize,
		"job_error_msg":    status.JobErrorMsg,
		"job_status":       status.JobStatus,
		"job_status_label": status.StatusLabel(),
	}, nil
}

// queryTaskCheck returns the normalized status of a folder move/delete task.
func queryTaskCheck(runtime *common.RuntimeContext, taskID string) (map[string]interface{}, error) {
	status, err := getDriveTaskCheckStatus(runtime, taskID)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"scenario": "task_check",
		"task_id":  status.TaskID,
		"status":   status.StatusLabel(),
		"ready":    status.Ready(),
		"failed":   status.Failed(),
	}, nil
}
