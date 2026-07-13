// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	// These are fixed backend wire values for the Wiki-to-Drive task. Keep
	// them unchanged even though the CLI scenario uses wiki_move_to_drive.
	wikiMoveToDriveTaskType  = "move_wiki_to_docs"
	wikiMoveToDriveResultKey = "move_wiki_to_docs_result"
)

// DriveTaskResult exposes a unified read path for the async task types produced
// by Drive import, export, folder move/delete, wiki move, wiki move-to-drive,
// and wiki delete flows.
var DriveTaskResult = common.Shortcut{
	Service:     "drive",
	Command:     "+task_result",
	Description: "Poll async task result for import, export, drive move/delete, wiki move, wiki move-to-drive, or wiki delete operations",
	Risk:        "read",
	// This shortcut multiplexes multiple backend APIs with different scope
	// requirements, so scenario-specific prechecks are handled in Validate.
	Scopes:    []string{},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "ticket", Desc: "async task ticket (for import/export tasks)", Required: false},
		{Name: "task-id", Desc: "async task ID (for drive task_check and all wiki task scenarios)", Required: false},
		{Name: "scenario", Desc: "task scenario: import, export, task_check, wiki_move, wiki_move_to_drive, wiki_delete_space, or wiki_delete_node", Required: true},
		{Name: "file-token", Desc: "source document token used for export task status lookup", Required: false},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		scenario := strings.ToLower(runtime.Str("scenario"))
		validScenarios := map[string]bool{
			"import":             true,
			"export":             true,
			"task_check":         true,
			"wiki_move":          true,
			"wiki_move_to_drive": true,
			"wiki_delete_space":  true,
			"wiki_delete_node":   true,
		}
		if !validScenarios[scenario] {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported scenario: %s. Supported scenarios: import, export, task_check, wiki_move, wiki_move_to_drive, wiki_delete_space, wiki_delete_node", scenario).WithParam("--scenario")
		}

		// Validate required params based on scenario
		switch scenario {
		case "import", "export":
			if runtime.Str("ticket") == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--ticket is required for %s scenario", scenario).WithParam("--ticket")
			}
			if err := validate.ResourceName(runtime.Str("ticket"), "--ticket"); err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--ticket")
			}
		case "task_check", "wiki_move", "wiki_move_to_drive", "wiki_delete_space", "wiki_delete_node":
			if runtime.Str("task-id") == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--task-id is required for %s scenario", scenario).WithParam("--task-id")
			}
			if err := validate.ResourceName(runtime.Str("task-id"), "--task-id"); err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--task-id")
			}
		}

		// For export scenario, file-token is required
		if scenario == "export" && runtime.Str("file-token") == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file-token is required for export scenario").WithParam("--file-token")
		}
		if scenario == "export" {
			if err := validate.ResourceName(runtime.Str("file-token"), "--file-token"); err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--file-token")
			}
		}

		return validateDriveTaskResultScopes(ctx, runtime, scenario)
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
		case "wiki_move":
			dry.GET("/open-apis/wiki/v2/tasks/:task_id").
				Desc("[1] Query wiki move task result").
				Set("task_id", taskID).
				Params(map[string]interface{}{"task_type": "move"})
		case "wiki_move_to_drive":
			dry.GET("/open-apis/wiki/v2/tasks/:task_id").
				Desc("[1] Query wiki move-to-drive task result").
				Set("task_id", taskID).
				Params(map[string]interface{}{"task_type": wikiMoveToDriveTaskType})
		case "wiki_delete_space":
			dry.GET("/open-apis/wiki/v2/tasks/:task_id").
				Desc("[1] Query wiki delete-space task result").
				Set("task_id", taskID).
				Params(map[string]interface{}{"task_type": "delete_space"})
		case "wiki_delete_node":
			dry.GET("/open-apis/wiki/v2/tasks/:task_id").
				Desc("[1] Query wiki delete-node task result").
				Set("task_id", taskID).
				Params(map[string]interface{}{"task_type": "delete_node"})
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
		case "wiki_move":
			result, err = queryWikiMoveTask(runtime, taskID)
		case "wiki_move_to_drive":
			result, err = queryWikiMoveToDriveTask(runtime, taskID)
		case "wiki_delete_space":
			result, err = queryWikiDeleteSpaceTask(runtime, taskID)
		case "wiki_delete_node":
			result, err = queryWikiDeleteNodeTask(runtime, taskID)
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

func validateDriveTaskResultScopes(ctx context.Context, runtime *common.RuntimeContext, scenario string) error {
	result, err := runtime.Factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(runtime.As(), runtime.Config.AppID))
	if err != nil {
		// Propagate cancellation/timeout so callers stop instead of falling through
		// to the API call. Other token errors are non-fatal here: the API call will
		// surface a clearer permission error.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return nil
	}
	if result == nil || result.Scopes == "" {
		return nil
	}

	var required []string
	switch scenario {
	case "import", "export", "task_check":
		required = []string{"drive:drive.metadata:readonly"}
	case "wiki_move", "wiki_move_to_drive", "wiki_delete_space", "wiki_delete_node":
		required = []string{"wiki:space:read"}
	}

	return requireDriveScopes(result.Scopes, required)
}

func requireDriveScopes(storedScopes string, required []string) error {
	if len(required) == 0 {
		return nil
	}

	missing := missingDriveScopes(storedScopes, required)
	if len(missing) == 0 {
		return nil
	}

	return errs.NewPermissionError(errs.SubtypeMissingScope,
		"missing required scope(s): %s", strings.Join(missing, ", ")).
		WithMissingScopes(missing...).
		WithHint("run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", strings.Join(missing, " "))
}

func missingDriveScopes(storedScopes string, required []string) []string {
	granted := make(map[string]bool)
	for _, scope := range strings.Fields(storedScopes) {
		granted[scope] = true
	}

	missing := make([]string, 0, len(required))
	for _, scope := range required {
		if !granted[scope] {
			missing = append(missing, scope)
		}
	}
	return missing
}

type wikiMoveTaskResultStatus struct {
	Node      map[string]interface{}
	Status    int
	StatusMsg string
}

type wikiMoveTaskQueryStatus struct {
	TaskID      string
	MoveResults []wikiMoveTaskResultStatus
}

func (s wikiMoveTaskQueryStatus) Ready() bool {
	if len(s.MoveResults) == 0 {
		return false
	}
	for _, result := range s.MoveResults {
		if result.Status != 0 {
			return false
		}
	}
	return true
}

func (s wikiMoveTaskQueryStatus) Failed() bool {
	for _, result := range s.MoveResults {
		if result.Status < 0 {
			return true
		}
	}
	return false
}

func (s wikiMoveTaskQueryStatus) FirstResult() *wikiMoveTaskResultStatus {
	if len(s.MoveResults) == 0 {
		return nil
	}
	return &s.MoveResults[0]
}

// primaryResult picks the most informative move_result for top-level status
// surfacing: prefer a failing entry so multi-doc tasks don't mask failures
// behind an earlier success, then a still-processing entry, and finally fall
// back to the first entry.
func (s wikiMoveTaskQueryStatus) primaryResult() *wikiMoveTaskResultStatus {
	for i := range s.MoveResults {
		if s.MoveResults[i].Status < 0 {
			return &s.MoveResults[i]
		}
	}
	for i := range s.MoveResults {
		if s.MoveResults[i].Status > 0 {
			return &s.MoveResults[i]
		}
	}
	return s.FirstResult()
}

func (s wikiMoveTaskQueryStatus) PrimaryStatusCode() int {
	if r := s.primaryResult(); r != nil {
		return r.Status
	}
	return 1
}

func (s wikiMoveTaskQueryStatus) PrimaryStatusLabel() string {
	if r := s.primaryResult(); r != nil {
		if msg := strings.TrimSpace(r.StatusMsg); msg != "" {
			return msg
		}
	}
	switch {
	case s.Ready():
		return "success"
	case s.Failed():
		return "failure"
	default:
		return "processing"
	}
}

func queryWikiMoveTask(runtime *common.RuntimeContext, taskID string) (map[string]interface{}, error) {
	status, err := getWikiMoveTaskStatus(runtime, taskID)
	if err != nil {
		return nil, err
	}

	out := map[string]interface{}{
		"scenario":   "wiki_move",
		"task_id":    status.TaskID,
		"ready":      status.Ready(),
		"failed":     status.Failed(),
		"status":     status.PrimaryStatusCode(),
		"status_msg": status.PrimaryStatusLabel(),
	}

	moveResults := make([]map[string]interface{}, 0, len(status.MoveResults))
	for _, result := range status.MoveResults {
		item := map[string]interface{}{
			"status":     result.Status,
			"status_msg": result.StatusMsg,
		}
		if result.Node != nil {
			item["node"] = result.Node
		}
		moveResults = append(moveResults, item)
	}
	if len(moveResults) > 0 {
		out["move_results"] = moveResults
	}

	if first := status.FirstResult(); first != nil {
		// Mirror the first moved node at the top level so follow-up commands can
		// reuse a stable field set without digging into move_results[0].node.
		if first.Node != nil {
			out["node"] = first.Node
			appendWikiMoveNodeFields(out, first.Node)
			if token := common.GetString(first.Node, "node_token"); token != "" {
				out["wiki_token"] = token
			}
		}
	}

	return out, nil
}

func getWikiMoveTaskStatus(runtime *common.RuntimeContext, taskID string) (wikiMoveTaskQueryStatus, error) {
	if err := validate.ResourceName(taskID, "--task-id"); err != nil {
		return wikiMoveTaskQueryStatus{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--task-id")
	}

	data, err := runtime.CallAPITyped(
		"GET",
		fmt.Sprintf("/open-apis/wiki/v2/tasks/%s", validate.EncodePathSegment(taskID)),
		map[string]interface{}{"task_type": "move"},
		nil,
	)
	if err != nil {
		return wikiMoveTaskQueryStatus{}, err
	}

	return parseWikiMoveTaskQueryStatus(taskID, common.GetMap(data, "task"))
}

func parseWikiMoveTaskQueryStatus(taskID string, task map[string]interface{}) (wikiMoveTaskQueryStatus, error) {
	if task == nil {
		return wikiMoveTaskQueryStatus{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki task response missing task")
	}

	status := wikiMoveTaskQueryStatus{
		TaskID: common.GetString(task, "task_id"),
	}
	if status.TaskID == "" {
		status.TaskID = taskID
	}

	for _, item := range common.GetSlice(task, "move_result") {
		resultMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		status.MoveResults = append(status.MoveResults, wikiMoveTaskResultStatus{
			Node:      parseWikiMoveTaskNode(common.GetMap(resultMap, "node")),
			Status:    int(common.GetFloat(resultMap, "status")),
			StatusMsg: common.GetString(resultMap, "status_msg"),
		})
	}

	return status, nil
}

func parseWikiMoveTaskNode(node map[string]interface{}) map[string]interface{} {
	if node == nil {
		return nil
	}

	return map[string]interface{}{
		"space_id":          common.GetString(node, "space_id"),
		"node_token":        common.GetString(node, "node_token"),
		"obj_token":         common.GetString(node, "obj_token"),
		"obj_type":          common.GetString(node, "obj_type"),
		"parent_node_token": common.GetString(node, "parent_node_token"),
		"node_type":         common.GetString(node, "node_type"),
		"origin_node_token": common.GetString(node, "origin_node_token"),
		"title":             common.GetString(node, "title"),
		"has_child":         common.GetBool(node, "has_child"),
	}
}

func appendWikiMoveNodeFields(out, node map[string]interface{}) {
	if out == nil || node == nil {
		return
	}
	out["space_id"] = common.GetString(node, "space_id")
	out["node_token"] = common.GetString(node, "node_token")
	out["obj_token"] = common.GetString(node, "obj_token")
	out["obj_type"] = common.GetString(node, "obj_type")
	out["parent_node_token"] = common.GetString(node, "parent_node_token")
	out["node_type"] = common.GetString(node, "node_type")
	out["origin_node_token"] = common.GetString(node, "origin_node_token")
	out["title"] = common.GetString(node, "title")
	out["has_child"] = common.GetBool(node, "has_child")
}

// queryWikiMoveToDriveTask returns the normalized status and final Drive
// resource fields for wiki +move-to-drive. The task endpoint uses a dedicated
// result object with numeric status codes: 0 success, 1 processing, -1 failure.
func queryWikiMoveToDriveTask(runtime *common.RuntimeContext, taskID string) (map[string]interface{}, error) {
	if err := validate.ResourceName(taskID, "--task-id"); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--task-id").WithCause(err)
	}

	data, err := runtime.CallAPITyped(
		"GET",
		fmt.Sprintf("/open-apis/wiki/v2/tasks/%s", validate.EncodePathSegment(taskID)),
		map[string]interface{}{"task_type": wikiMoveToDriveTaskType},
		nil,
	)
	if err != nil {
		return nil, err
	}

	task := common.GetMap(data, "task")
	if task == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki task response missing task")
	}
	result := common.GetMap(task, wikiMoveToDriveResultKey)
	if result == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki task response missing %s", wikiMoveToDriveResultKey)
	}
	statusCode, ok := common.GetFloatOK(result, "status")
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki task response has missing or non-numeric %s.status", wikiMoveToDriveResultKey)
	}
	if statusCode != -1 && statusCode != 0 && statusCode != 1 {
		return nil, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"wiki task response has unsupported %s.status: %v",
			wikiMoveToDriveResultKey,
			statusCode,
		)
	}

	resolvedTaskID := common.GetString(task, "task_id")
	if resolvedTaskID == "" {
		resolvedTaskID = taskID
	}
	status := int(statusCode)
	statusMsg := strings.TrimSpace(common.GetString(result, "status_msg"))
	if statusMsg == "" {
		switch {
		case status == 0:
			statusMsg = "success"
		case status < 0:
			statusMsg = "failure"
		default:
			statusMsg = "processing"
		}
	}

	return map[string]interface{}{
		"scenario":   "wiki_move_to_drive",
		"task_id":    resolvedTaskID,
		"ready":      status == 0,
		"failed":     status < 0,
		"status":     status,
		"status_msg": statusMsg,
		"obj_token":  common.GetString(result, "obj_token"),
		"obj_type":   common.GetString(result, "obj_type"),
		"url":        common.GetString(result, "url"),
	}, nil
}

// queryWikiDeleteSpaceTask returns the normalized status of an async wiki
// delete-space task. The backend reports a single delete_space_result object
// rather than the per-node array used by wiki move.
func queryWikiDeleteSpaceTask(runtime *common.RuntimeContext, taskID string) (map[string]interface{}, error) {
	if err := validate.ResourceName(taskID, "--task-id"); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--task-id")
	}

	data, err := runtime.CallAPITyped(
		"GET",
		fmt.Sprintf("/open-apis/wiki/v2/tasks/%s", validate.EncodePathSegment(taskID)),
		map[string]interface{}{"task_type": "delete_space"},
		nil,
	)
	if err != nil {
		return nil, err
	}

	task := common.GetMap(data, "task")
	if task == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki task response missing task")
	}

	resolvedTaskID := common.GetString(task, "task_id")
	if resolvedTaskID == "" {
		resolvedTaskID = taskID
	}

	result := common.GetMap(task, "delete_space_result")
	var status, statusMsg string
	if result != nil {
		status = common.GetString(result, "status")
		statusMsg = common.GetString(result, "status_msg")
	}

	lowered := strings.ToLower(strings.TrimSpace(status))
	ready := lowered == "success"
	failed := lowered == "failure" || lowered == "failed"

	// Fall back to "processing" when the backend omits delete_space_result.status
	// so the output "status" field is never an empty string on timeout. This
	// mirrors the same fallback in wiki_delete.go's StatusCode() (intentionally
	// duplicated — the two call sites stay in lockstep via the shared literal
	// "processing" rather than a cross-package import).
	resolvedStatus := strings.TrimSpace(status)
	if resolvedStatus == "" {
		resolvedStatus = "processing"
	}

	label := strings.TrimSpace(statusMsg)
	if label == "" {
		label = resolvedStatus
	}

	return map[string]interface{}{
		"scenario":   "wiki_delete_space",
		"task_id":    resolvedTaskID,
		"ready":      ready,
		"failed":     failed,
		"status":     resolvedStatus,
		"status_msg": label,
	}, nil
}

// queryWikiDeleteNodeTask returns the normalized status of an async wiki
// delete-node task. For historical reasons the gateway stashes delete-node
// status under the generic `simple_task_result` key (NOT `delete_node_result`),
// and that object only carries `status` — there is no `status_msg`, so the
// label falls back to the status code. Mirrors queryWikiDeleteSpaceTask;
// intentionally duplicated here (rather than importing the wiki package) to
// keep drive from depending on shortcuts/wiki.
func queryWikiDeleteNodeTask(runtime *common.RuntimeContext, taskID string) (map[string]interface{}, error) {
	if err := validate.ResourceName(taskID, "--task-id"); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--task-id")
	}

	data, err := runtime.CallAPITyped(
		"GET",
		fmt.Sprintf("/open-apis/wiki/v2/tasks/%s", validate.EncodePathSegment(taskID)),
		map[string]interface{}{"task_type": "delete_node"},
		nil,
	)
	if err != nil {
		return nil, err
	}

	task := common.GetMap(data, "task")
	if task == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki task response missing task")
	}

	resolvedTaskID := common.GetString(task, "task_id")
	if resolvedTaskID == "" {
		resolvedTaskID = taskID
	}

	result := common.GetMap(task, "simple_task_result")
	var status string
	if result != nil {
		status = common.GetString(result, "status")
	}

	// Keep in sync with wiki.parseWikiAsyncTaskStatus / wikiAsyncTaskStatus
	// classification (intentionally duplicated to avoid a drive→wiki import —
	// see the doc comment above). If the success/failed/processing rules change
	// there, mirror the change here.
	lowered := strings.ToLower(strings.TrimSpace(status))
	ready := lowered == "success"
	failed := lowered == "failure" || lowered == "failed"

	resolvedStatus := strings.TrimSpace(status)
	if resolvedStatus == "" {
		resolvedStatus = "processing"
	}

	return map[string]interface{}{
		"scenario":   "wiki_delete_node",
		"task_id":    resolvedTaskID,
		"ready":      ready,
		"failed":     failed,
		"status":     resolvedStatus,
		"status_msg": resolvedStatus,
	}, nil
}
