// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	// These are fixed backend wire values. Keep them unchanged even though the
	// shortcut and its continuation scenario use the user-facing Drive name.
	wikiMoveToDriveTaskType = "move_wiki_to_docs"
	wikiMoveToDriveResult   = "move_wiki_to_docs_result"

	wikiMoveToDriveStatusSuccess    = 0
	wikiMoveToDriveStatusProcessing = 1
	wikiMoveToDriveStatusFailure    = -1
)

var (
	wikiMoveToDrivePollAttempts = 30
	wikiMoveToDrivePollInterval = 2 * time.Second
)

// WikiMoveToDrive moves a Wiki node out of its knowledge space and into a
// Drive folder. The API always creates an async task, so the shortcut polls the
// Wiki task endpoint and returns a resumable command when the bounded window
// expires.
var WikiMoveToDrive = common.Shortcut{
	Service:     "wiki",
	Command:     "+move-to-drive",
	Description: "Move a wiki node to a Drive folder, polling the async task until it finishes",
	Risk:        "write",
	// The move endpoint's wiki:wiki / wiki:node:move /
	// space:document:move list is an OR-set, while Shortcut.Scopes is an
	// ALL-required preflight. Use the registry's highest-priority candidate
	// plus the read scope required by the task-status endpoint.
	Scopes:    []string{"space:document:move", "wiki:space:read"},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "node-token", Desc: "wiki node_token to move out of the knowledge space", Required: true},
		{Name: "folder-token", Desc: "target Drive folder token; omit to move to the calling identity's personal-space root"},
	},
	Tips: []string{
		"The source must be a wiki node_token, not the backing document's obj_token; use wiki +node-get when unsure.",
		"Omit --folder-token to move the document to the calling identity's personal-space root.",
		"Moving out of Wiki removes the node from the Wiki tree and replaces inherited Wiki permissions with the target Drive folder's permission model.",
		"The move is asynchronous; if the bounded poll times out, continue with the next_command returned in the output.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateWikiMoveToDriveSpec(readWikiMoveToDriveSpec(runtime))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return buildWikiMoveToDriveDryRun(readWikiMoveToDriveSpec(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := readWikiMoveToDriveSpec(runtime)
		out, err := runWikiMoveToDrive(ctx, wikiMoveToDriveAPI{runtime: runtime}, runtime, spec)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

type wikiMoveToDriveSpec struct {
	NodeToken   string
	FolderToken string
}

func (spec wikiMoveToDriveSpec) RequestBody() map[string]interface{} {
	body := map[string]interface{}{}
	if spec.FolderToken != "" {
		body["folder_token"] = spec.FolderToken
	}
	return body
}

type wikiMoveToDriveTaskStatus struct {
	TaskID    string
	Status    int
	StatusMsg string
	ObjToken  string
	ObjType   string
	URL       string
}

func (s wikiMoveToDriveTaskStatus) Ready() bool {
	return s.Status == wikiMoveToDriveStatusSuccess
}

func (s wikiMoveToDriveTaskStatus) Failed() bool {
	return s.Status < wikiMoveToDriveStatusSuccess
}

func (s wikiMoveToDriveTaskStatus) StatusLabel() string {
	if label := strings.TrimSpace(s.StatusMsg); label != "" {
		return label
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

type wikiMoveToDriveClient interface {
	MoveWikiToDrive(ctx context.Context, spec wikiMoveToDriveSpec) (string, error)
	GetMoveWikiToDriveTask(ctx context.Context, taskID string) (wikiMoveToDriveTaskStatus, error)
}

type wikiMoveToDriveAPI struct {
	runtime *common.RuntimeContext
}

func (api wikiMoveToDriveAPI) MoveWikiToDrive(ctx context.Context, spec wikiMoveToDriveSpec) (string, error) {
	data, err := api.runtime.CallAPITyped(
		"POST",
		fmt.Sprintf(
			"/open-apis/wiki/v2/nodes/%s/move_wiki_to_docs",
			validate.EncodePathSegment(spec.NodeToken),
		),
		nil,
		spec.RequestBody(),
	)
	if err != nil {
		return "", err
	}

	taskID := common.GetString(data, "task_id")
	if taskID == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki move-to-drive response missing task_id")
	}
	return taskID, nil
}

func (api wikiMoveToDriveAPI) GetMoveWikiToDriveTask(ctx context.Context, taskID string) (wikiMoveToDriveTaskStatus, error) {
	data, err := api.runtime.CallAPITyped(
		"GET",
		fmt.Sprintf("/open-apis/wiki/v2/tasks/%s", validate.EncodePathSegment(taskID)),
		map[string]interface{}{"task_type": wikiMoveToDriveTaskType},
		nil,
	)
	if err != nil {
		return wikiMoveToDriveTaskStatus{}, err
	}
	return parseWikiMoveToDriveTaskStatus(taskID, common.GetMap(data, "task"))
}

func readWikiMoveToDriveSpec(runtime *common.RuntimeContext) wikiMoveToDriveSpec {
	return wikiMoveToDriveSpec{
		NodeToken:   strings.TrimSpace(runtime.Str("node-token")),
		FolderToken: strings.TrimSpace(runtime.Str("folder-token")),
	}
}

func validateWikiMoveToDriveSpec(spec wikiMoveToDriveSpec) error {
	if spec.NodeToken == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--node-token is required").WithParam("--node-token")
	}
	if err := validateOptionalResourceName(spec.NodeToken, "--node-token"); err != nil {
		return err
	}
	return validateOptionalResourceName(spec.FolderToken, "--folder-token")
}

func buildWikiMoveToDriveDryRun(spec wikiMoveToDriveSpec) *common.DryRunAPI {
	dry := common.NewDryRunAPI().Desc(
		"2-step orchestration: move wiki node to Drive -> poll wiki move-to-drive task result",
	)
	dry.POST(fmt.Sprintf(
		"/open-apis/wiki/v2/nodes/%s/move_wiki_to_docs",
		validate.EncodePathSegment(spec.NodeToken),
	)).
		Desc("[1] Move wiki node to Drive").
		Body(spec.RequestBody())
	dry.GET("/open-apis/wiki/v2/tasks/:task_id").
		Desc("[2] Poll wiki move-to-drive task result").
		Set("task_id", "<task_id>").
		Params(map[string]interface{}{"task_type": wikiMoveToDriveTaskType})
	return dry
}

func runWikiMoveToDrive(
	ctx context.Context,
	client wikiMoveToDriveClient,
	runtime *common.RuntimeContext,
	spec wikiMoveToDriveSpec,
) (map[string]interface{}, error) {
	folderLabel := "personal-space root"
	if spec.FolderToken != "" {
		folderLabel = common.MaskToken(spec.FolderToken)
	}
	fmt.Fprintf(
		runtime.IO().ErrOut,
		"Moving wiki node %s to Drive folder %s...\n",
		common.MaskToken(spec.NodeToken),
		folderLabel,
	)

	taskID, err := client.MoveWikiToDrive(ctx, spec)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Wiki move-to-drive is async, polling task %s...\n", taskID)
	status, ready, err := pollWikiMoveToDriveTask(ctx, client, runtime, taskID)
	if err != nil {
		return nil, err
	}

	out := map[string]interface{}{
		"node_token":   spec.NodeToken,
		"folder_token": spec.FolderToken,
		"task_id":      taskID,
		"ready":        ready,
		"failed":       status.Failed(),
		"status":       status.Status,
		"status_msg":   status.StatusLabel(),
		"obj_token":    status.ObjToken,
		"obj_type":     status.ObjType,
		"url":          status.URL,
	}
	if !ready {
		nextCommand := wikiMoveToDriveTaskResultCommand(taskID, runtime.As(), wikiMoveToDriveProfileName(runtime))
		fmt.Fprintf(runtime.IO().ErrOut, "Wiki move-to-drive task is still in progress. Continue with: %s\n", nextCommand)
		out["timed_out"] = true
		out["next_command"] = nextCommand
	}
	return out, nil
}

func pollWikiMoveToDriveTask(
	ctx context.Context,
	client wikiMoveToDriveClient,
	runtime *common.RuntimeContext,
	taskID string,
) (wikiMoveToDriveTaskStatus, bool, error) {
	lastStatus := wikiMoveToDriveTaskStatus{
		TaskID: taskID,
		Status: wikiMoveToDriveStatusProcessing,
	}
	var lastErr error
	hadSuccessfulPoll := false

	for attempt := 1; attempt <= wikiMoveToDrivePollAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return lastStatus, false, wrapWikiMoveToDrivePollContextError(
					ctx.Err(), taskID, runtime.As(), wikiMoveToDriveProfileName(runtime),
				)
			case <-time.After(wikiMoveToDrivePollInterval):
			}
		}

		status, err := client.GetMoveWikiToDriveTask(ctx, taskID)
		if err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return lastStatus, false, wrapWikiMoveToDrivePollContextError(
					contextErr, taskID, runtime.As(), wikiMoveToDriveProfileName(runtime),
				)
			}
			lastErr = err
			fmt.Fprintf(runtime.IO().ErrOut, "Wiki move-to-drive status attempt %d/%d failed: %v\n", attempt, wikiMoveToDrivePollAttempts, err)
			continue
		}
		lastStatus = status
		hadSuccessfulPoll = true

		if status.Ready() {
			fmt.Fprintln(runtime.IO().ErrOut, "Wiki move-to-drive task completed successfully.")
			return status, true, nil
		}
		if status.Failed() {
			return status, false, errs.NewAPIError(
				errs.SubtypeServerError,
				"wiki move-to-drive task %s failed: %s",
				taskID,
				status.StatusLabel(),
			)
		}

		fmt.Fprintf(
			runtime.IO().ErrOut,
			"Wiki move-to-drive status %d/%d: %s\n",
			attempt,
			wikiMoveToDrivePollAttempts,
			status.StatusLabel(),
		)
	}

	if err := ctx.Err(); err != nil {
		return lastStatus, false, wrapWikiMoveToDrivePollContextError(
			err, taskID, runtime.As(), wikiMoveToDriveProfileName(runtime),
		)
	}

	if !hadSuccessfulPoll && lastErr != nil {
		hint := fmt.Sprintf(
			"the wiki move-to-drive task was created but every status poll failed (task_id=%s)\nretry status lookup with: %s",
			taskID,
			wikiMoveToDriveTaskResultCommand(taskID, runtime.As(), wikiMoveToDriveProfileName(runtime)),
		)
		if _, ok := errs.ProblemOf(lastErr); ok {
			return lastStatus, false, appendWikiProblemHint(lastErr, hint)
		}
		return lastStatus, false, errs.NewInternalError(errs.SubtypeUnknown, "%s", lastErr.Error()).
			WithHint("%s", hint).
			WithCause(lastErr)
	}

	return lastStatus, false, nil
}

func parseWikiMoveToDriveTaskStatus(taskID string, task map[string]interface{}) (wikiMoveToDriveTaskStatus, error) {
	if task == nil {
		return wikiMoveToDriveTaskStatus{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki task response missing task")
	}

	result := common.GetMap(task, wikiMoveToDriveResult)
	if result == nil {
		return wikiMoveToDriveTaskStatus{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"wiki task response missing %s",
			wikiMoveToDriveResult,
		)
	}
	statusCode, ok := common.GetFloatOK(result, "status")
	if !ok {
		return wikiMoveToDriveTaskStatus{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"wiki task response has missing or non-numeric %s.status",
			wikiMoveToDriveResult,
		)
	}
	if statusCode != wikiMoveToDriveStatusFailure &&
		statusCode != wikiMoveToDriveStatusSuccess &&
		statusCode != wikiMoveToDriveStatusProcessing {
		return wikiMoveToDriveTaskStatus{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"wiki task response has unsupported %s.status: %v",
			wikiMoveToDriveResult,
			statusCode,
		)
	}

	status := wikiMoveToDriveTaskStatus{
		TaskID:    common.GetString(task, "task_id"),
		Status:    int(statusCode),
		StatusMsg: common.GetString(result, "status_msg"),
		ObjToken:  common.GetString(result, "obj_token"),
		ObjType:   common.GetString(result, "obj_type"),
		URL:       common.GetString(result, "url"),
	}
	if status.TaskID == "" {
		status.TaskID = taskID
	}
	return status, nil
}

// Preserve the originating identity and profile so a resumed status lookup
// uses the same credential context that created the async task.
func wikiMoveToDriveTaskResultCommand(taskID string, identity core.Identity, profileName string) string {
	asFlag := string(identity)
	if asFlag == "" {
		asFlag = "user"
	}
	profileFlag := ""
	if profileName != "" {
		profileFlag = fmt.Sprintf(" --profile %s", profileName)
	}
	return fmt.Sprintf(
		"lark-cli%s drive +task_result --scenario wiki_move_to_drive --task-id %s --as %s",
		profileFlag,
		taskID,
		asFlag,
	)
}

func wikiMoveToDriveProfileName(runtime *common.RuntimeContext) string {
	if runtime == nil || runtime.Config == nil {
		return ""
	}
	return runtime.Config.ProfileName
}

func wrapWikiMoveToDrivePollContextError(err error, taskID string, identity core.Identity, profileName string) error {
	if err == nil {
		return nil
	}
	subtype := errs.SubtypeNetworkTransport
	message := "wiki move-to-drive task polling cancelled: %s"
	if errors.Is(err, context.DeadlineExceeded) {
		subtype = errs.SubtypeNetworkTimeout
		message = "wiki move-to-drive task polling deadline exceeded: %s"
	}
	return errs.NewNetworkError(subtype, message, err).
		WithHint("the task may still be running; retry status lookup with: %s", wikiMoveToDriveTaskResultCommand(taskID, identity, profileName)).
		WithCause(err)
}
