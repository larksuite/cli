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

var DriveTaskResult = common.Shortcut{
	Service:     "drive",
	Command:     "+task_result",
	Description: "Poll async task result for import, export, move, or delete operations",
	Risk:        "readonly",
	Scopes:      []string{"drive:drive.metadata:readonly"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "ticket", Desc: "async task ticket (for import/export tasks)", Required: false},
		{Name: "task-id", Desc: "async task ID (for move/delete folder tasks)", Required: false},
		{Name: "scenario", Desc: "task scenario: import, export, or task_check", Required: true},
		{Name: "file-token", Desc: "file token (required for export task)", Required: false},
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
		case "task_check":
			if runtime.Str("task-id") == "" {
				return output.ErrValidation("--task-id is required for task_check scenario")
			}
		}

		// For export scenario, file-token is required
		if scenario == "export" && runtime.Str("file-token") == "" {
			return output.ErrValidation("--file-token is required for export scenario")
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
				Set("token", fileToken)
		case "task_check":
			dry.GET("/open-apis/drive/v1/files/task_check").
				Desc("[1] Query move/delete folder task status").
				Set("task_id", taskID)
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

		switch scenario {
		case "import":
			result, err = queryImportTask(ctx, runtime, ticket)
		case "export":
			result, err = queryExportTask(ctx, runtime, ticket, fileToken)
		case "task_check":
			result, err = queryTaskCheck(ctx, runtime, taskID)
		}

		if err != nil {
			return err
		}

		runtime.Out(result, nil)
		return nil
	},
}

// queryImportTask queries import task result
func queryImportTask(ctx context.Context, runtime *common.RuntimeContext, ticket string) (map[string]interface{}, error) {
	if err := validate.ResourceName(ticket, "ticket"); err != nil {
		return nil, output.ErrValidation("invalid ticket: %s", err)
	}

	data, err := runtime.CallAPI(
		"GET",
		fmt.Sprintf("/open-apis/drive/v1/import_tasks/%s", validate.EncodePathSegment(ticket)),
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}

	result := common.GetMap(data, "result")
	if result == nil {
		result = data
	}

	return map[string]interface{}{
		"scenario":      "import",
		"ticket":        common.GetString(result, "ticket"),
		"type":          common.GetString(result, "type"),
		"job_status":    int(common.GetFloat(result, "job_status")),
		"job_error_msg": common.GetString(result, "job_error_msg"),
		"token":         common.GetString(result, "token"),
		"url":           common.GetString(result, "url"),
		"extra":         result["extra"],
	}, nil
}

// queryExportTask queries export task result
func queryExportTask(ctx context.Context, runtime *common.RuntimeContext, ticket, fileToken string) (map[string]interface{}, error) {
	if err := validate.ResourceName(ticket, "ticket"); err != nil {
		return nil, output.ErrValidation("invalid ticket: %s", err)
	}
	if err := validate.ResourceName(fileToken, "file-token"); err != nil {
		return nil, output.ErrValidation("invalid file-token: %s", err)
	}

	params := map[string]interface{}{
		"token": fileToken,
	}

	data, err := runtime.CallAPI(
		"GET",
		fmt.Sprintf("/open-apis/drive/v1/export_tasks/%s", validate.EncodePathSegment(ticket)),
		params,
		nil,
	)
	if err != nil {
		return nil, err
	}

	result := common.GetMap(data, "result")
	if result == nil {
		result = data
	}

	return map[string]interface{}{
		"scenario":       "export",
		"ticket":         ticket,
		"file_extension": common.GetString(result, "file_extension"),
		"type":           common.GetString(result, "type"),
		"file_name":      common.GetString(result, "file_name"),
		"file_token":     common.GetString(result, "file_token"),
		"file_size":      int64(common.GetFloat(result, "file_size")),
		"job_error_msg":  common.GetString(result, "job_error_msg"),
		"job_status":     int(common.GetFloat(result, "job_status")),
	}, nil
}

// queryTaskCheck queries move/delete folder task status
func queryTaskCheck(ctx context.Context, runtime *common.RuntimeContext, taskID string) (map[string]interface{}, error) {
	if err := validate.ResourceName(taskID, "task-id"); err != nil {
		return nil, output.ErrValidation("invalid task-id: %s", err)
	}

	params := map[string]interface{}{
		"task_id": taskID,
	}

	data, err := runtime.CallAPI("GET", "/open-apis/drive/v1/files/task_check", params, nil)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"scenario": "task_check",
		"task_id":  taskID,
		"status":   common.GetString(data, "status"),
	}, nil
}
