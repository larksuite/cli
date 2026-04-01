// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"fmt"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var (
	driveMovePollAttempts = 30
	driveMovePollInterval = 2 * time.Second
)

var driveMoveAllowedTypes = map[string]bool{
	"file":     true,
	"docx":     true,
	"bitable":  true,
	"doc":      true,
	"sheet":    true,
	"mindnote": true,
	"folder":   true,
	"slides":   true,
}

type driveMoveSpec struct {
	FileToken   string
	FileType    string
	FolderToken string
}

func (s driveMoveSpec) RequestBody() map[string]interface{} {
	return map[string]interface{}{
		"type":         s.FileType,
		"folder_token": s.FolderToken,
	}
}

func validateDriveMoveSpec(spec driveMoveSpec) error {
	if err := validate.ResourceName(spec.FileToken, "--file-token"); err != nil {
		return output.ErrValidation("%s", err)
	}
	if strings.TrimSpace(spec.FolderToken) != "" {
		if err := validate.ResourceName(spec.FolderToken, "--folder-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
	}
	if !driveMoveAllowedTypes[spec.FileType] {
		return output.ErrValidation("unsupported file type: %s. Supported types: file, docx, bitable, doc, sheet, mindnote, folder, slides", spec.FileType)
	}
	return nil
}

type driveTaskCheckStatus struct {
	TaskID string
	Status string
}

func (s driveTaskCheckStatus) Ready() bool {
	return s.Status == "success"
}

func (s driveTaskCheckStatus) Failed() bool {
	return s.Status == "failed"
}

func (s driveTaskCheckStatus) Pending() bool {
	return !s.Ready() && !s.Failed()
}

func (s driveTaskCheckStatus) StatusLabel() string {
	status := strings.TrimSpace(s.Status)
	if status == "" {
		return "unknown"
	}
	return status
}

func driveTaskCheckResultCommand(taskID string) string {
	return fmt.Sprintf("lark-cli drive +task_result --scenario task_check --task-id %s", taskID)
}

func driveTaskCheckParams(taskID string) map[string]interface{} {
	return map[string]interface{}{"task_id": taskID}
}

func getDriveTaskCheckStatus(runtime *common.RuntimeContext, taskID string) (driveTaskCheckStatus, error) {
	if err := validate.ResourceName(taskID, "--task-id"); err != nil {
		return driveTaskCheckStatus{}, output.ErrValidation("%s", err)
	}

	data, err := runtime.CallAPI("GET", "/open-apis/drive/v1/files/task_check", driveTaskCheckParams(taskID), nil)
	if err != nil {
		return driveTaskCheckStatus{}, err
	}

	return parseDriveTaskCheckStatus(taskID, data), nil
}

func parseDriveTaskCheckStatus(taskID string, data map[string]interface{}) driveTaskCheckStatus {
	result := common.GetMap(data, "result")
	if result == nil {
		result = data
	}

	return driveTaskCheckStatus{
		TaskID: taskID,
		Status: common.GetString(result, "status"),
	}
}

func pollDriveTaskCheck(runtime *common.RuntimeContext, taskID string) (driveTaskCheckStatus, bool, error) {
	lastStatus := driveTaskCheckStatus{TaskID: taskID}
	for attempt := 1; attempt <= driveMovePollAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(driveMovePollInterval)
		}

		status, err := getDriveTaskCheckStatus(runtime, taskID)
		if err != nil {
			return driveTaskCheckStatus{}, false, err
		}
		lastStatus = status
		if status.Ready() {
			fmt.Fprintf(runtime.IO().ErrOut, "Folder move completed successfully.\n")
			return status, true, nil
		}
		if status.Failed() {
			return status, false, output.Errorf(output.ExitAPI, "api_error", "folder move task failed")
		}
	}

	return lastStatus, false, nil
}
