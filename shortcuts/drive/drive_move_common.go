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
	driveTaskCheckPollAttempts = 30
	driveTaskCheckPollInterval = 2 * time.Second
)

// driveMoveAllowedTypes mirrors the document kinds accepted by the Drive move
// endpoint that this shortcut wraps.
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

// driveMoveSpec contains the normalized input needed to issue a move request.
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

// driveTaskCheckStatus represents the status payload returned by
// /drive/v1/files/task_check for async folder move/delete operations.
type driveTaskCheckStatus struct {
	TaskID string
	Status string
}

func (s driveTaskCheckStatus) Ready() bool {
	return strings.EqualFold(strings.TrimSpace(s.Status), "success")
}

func (s driveTaskCheckStatus) Failed() bool {
	status := strings.TrimSpace(s.Status)
	// The shared task_check endpoint is reused by multiple async flows. Some
	// backends return "failed", while folder delete can return the shorter
	// terminal state "fail".
	return strings.EqualFold(status, "failed") || strings.EqualFold(status, "fail")
}

func (s driveTaskCheckStatus) Pending() bool {
	return !s.Ready() && !s.Failed()
}

func (s driveTaskCheckStatus) StatusLabel() string {
	status := strings.TrimSpace(s.Status)
	if status == "" {
		// Empty status is treated as unknown so callers can still render a
		// meaningful label instead of an empty string.
		return "unknown"
	}
	return status
}

// driveTaskCheckResultCommand prints the resume command shown when bounded
// polling ends before the backend task completes.
func driveTaskCheckResultCommand(taskID, as string) string {
	return fmt.Sprintf("lark-cli drive +task_result --scenario task_check --task-id %s --as %s", taskID, as)
}

// driveTaskCheckParams keeps the task_check query parameter shape in one place
// for both dry-run and execution paths.
func driveTaskCheckParams(taskID string) map[string]interface{} {
	return map[string]interface{}{"task_id": taskID}
}

// getDriveTaskCheckStatus fetches and validates the current state of an async
// folder move or delete task.
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

// parseDriveTaskCheckStatus tolerates both wrapped and already-unwrapped
// response shapes used in tests and helpers.
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

// pollDriveTaskCheck polls the shared task_check endpoint for a bounded period
// and returns the last seen status so callers can emit a follow-up command
// when needed.
func pollDriveTaskCheck(runtime *common.RuntimeContext, taskID string) (driveTaskCheckStatus, bool, error) {
	lastStatus := driveTaskCheckStatus{TaskID: taskID}
	var (
		seenStatus bool
		lastErr    error
	)
	for attempt := 1; attempt <= driveTaskCheckPollAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(driveTaskCheckPollInterval)
		}

		status, err := getDriveTaskCheckStatus(runtime, taskID)
		if err != nil {
			lastErr = err
			fmt.Fprintf(runtime.IO().ErrOut, "Error polling task %s: %s\n", taskID, err)
			continue
		}
		seenStatus = true
		lastStatus = status
		// Success and failure are terminal backend states. Any other value is kept
		// as pending so the caller can decide whether to continue or resume later.
		if status.Ready() {
			fmt.Fprintf(runtime.IO().ErrOut, "Folder task completed successfully.\n")
			return status, true, nil
		}
		if status.Failed() {
			return status, false, output.Errorf(output.ExitAPI, "api_error", "folder task failed")
		}
	}

	if !seenStatus && lastErr != nil {
		return driveTaskCheckStatus{}, false, lastErr
	}

	return lastStatus, false, nil
}
