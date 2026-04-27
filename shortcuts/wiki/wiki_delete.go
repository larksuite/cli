// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var (
	wikiDeleteSpacePollAttempts = 30
	wikiDeleteSpacePollInterval = 2 * time.Second
)

const (
	wikiDeleteSpaceStatusSuccess    = "success"
	wikiDeleteSpaceStatusFailure    = "failure"
	wikiDeleteSpaceStatusProcessing = "processing"
)

// WikiDeleteSpace deletes a wiki space. The DELETE endpoint may complete
// synchronously (empty task_id) or return a task_id that must be polled
// against /open-apis/wiki/v2/tasks/:task_id with task_type=delete_space.
var WikiDeleteSpace = common.Shortcut{
	Service:     "wiki",
	Command:     "+delete-space",
	Description: "Delete a wiki space, polling the async delete task when needed",
	Risk:        "high-risk-write",
	Scopes:      []string{"wiki:space:write_only", "wiki:space:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "space-id", Desc: "wiki space ID to delete", Required: true},
	},
	Tips: []string{
		"Deletion is irreversible; double-check --space-id before running.",
		"This is a high-risk-write command; pass --yes to confirm the deletion.",
		"If the API returns a long-running task, this command polls for a bounded window and then prints a follow-up drive +task_result command.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateWikiDeleteSpaceSpec(readWikiDeleteSpaceSpec(runtime))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return buildWikiDeleteSpaceDryRun(readWikiDeleteSpaceSpec(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := readWikiDeleteSpaceSpec(runtime)
		fmt.Fprintf(runtime.IO().ErrOut, "Deleting wiki space %s...\n", spec.SpaceID)

		out, err := runWikiDeleteSpace(ctx, wikiDeleteSpaceAPI{runtime: runtime}, runtime, spec)
		if err != nil {
			return err
		}

		runtime.Out(out, nil)
		return nil
	},
}

type wikiDeleteSpaceSpec struct {
	SpaceID string
}

type wikiDeleteSpaceResponse struct {
	TaskID string
}

type wikiDeleteSpaceTaskStatus struct {
	TaskID    string
	Status    string
	StatusMsg string
}

// normalizedStatus collapses whitespace and case so "  SUCCESS  " is
// classified the same as "success". Ready / Failed / StatusCode all derive
// from this so classification and the output `status` field can't disagree.
func (s wikiDeleteSpaceTaskStatus) normalizedStatus() string {
	return strings.ToLower(strings.TrimSpace(s.Status))
}

func (s wikiDeleteSpaceTaskStatus) Ready() bool {
	return s.normalizedStatus() == wikiDeleteSpaceStatusSuccess
}

func (s wikiDeleteSpaceTaskStatus) Failed() bool {
	// The sample protocol only documents "success" as a terminal OK. Treat any
	// explicit "failure"/"failed" signal as terminal, and unknown non-success
	// values as still-processing so we don't misreport a novel status as a hard
	// failure.
	lowered := s.normalizedStatus()
	return lowered == wikiDeleteSpaceStatusFailure || lowered == "failed"
}

// StatusCode returns a never-empty status value for the output envelope. If
// the backend response omits delete_space_result.status (or sends whitespace),
// fall back to "processing" so the documented timeout-shape stays accurate.
func (s wikiDeleteSpaceTaskStatus) StatusCode() string {
	if status := strings.TrimSpace(s.Status); status != "" {
		return status
	}
	return wikiDeleteSpaceStatusProcessing
}

func (s wikiDeleteSpaceTaskStatus) StatusLabel() string {
	if msg := strings.TrimSpace(s.StatusMsg); msg != "" {
		return msg
	}
	return s.StatusCode()
}

type wikiDeleteSpaceClient interface {
	DeleteSpace(ctx context.Context, spaceID string) (*wikiDeleteSpaceResponse, error)
	GetDeleteSpaceTask(ctx context.Context, taskID string) (wikiDeleteSpaceTaskStatus, error)
}

type wikiDeleteSpaceAPI struct {
	runtime *common.RuntimeContext
}

func (api wikiDeleteSpaceAPI) DeleteSpace(ctx context.Context, spaceID string) (*wikiDeleteSpaceResponse, error) {
	data, err := api.runtime.CallAPI(
		"DELETE",
		fmt.Sprintf("/open-apis/wiki/v2/spaces/%s", validate.EncodePathSegment(spaceID)),
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}
	return &wikiDeleteSpaceResponse{
		TaskID: common.GetString(data, "task_id"),
	}, nil
}

func (api wikiDeleteSpaceAPI) GetDeleteSpaceTask(ctx context.Context, taskID string) (wikiDeleteSpaceTaskStatus, error) {
	data, err := api.runtime.CallAPI(
		"GET",
		fmt.Sprintf("/open-apis/wiki/v2/tasks/%s", validate.EncodePathSegment(taskID)),
		map[string]interface{}{"task_type": "delete_space"},
		nil,
	)
	if err != nil {
		return wikiDeleteSpaceTaskStatus{}, err
	}
	return parseWikiDeleteSpaceTaskStatus(taskID, common.GetMap(data, "task"))
}

func readWikiDeleteSpaceSpec(runtime *common.RuntimeContext) wikiDeleteSpaceSpec {
	return wikiDeleteSpaceSpec{
		SpaceID: strings.TrimSpace(runtime.Str("space-id")),
	}
}

func validateWikiDeleteSpaceSpec(spec wikiDeleteSpaceSpec) error {
	if spec.SpaceID == "" {
		return output.ErrValidation("--space-id is required")
	}
	return validateOptionalResourceName(spec.SpaceID, "--space-id")
}

func buildWikiDeleteSpaceDryRun(spec wikiDeleteSpaceSpec) *common.DryRunAPI {
	dry := common.NewDryRunAPI()
	dry.Desc("2-step orchestration: delete wiki space -> poll wiki delete task when task_id is returned")
	dry.DELETE(fmt.Sprintf("/open-apis/wiki/v2/spaces/%s", dryRunWikiDeleteSpaceID(spec)))
	dry.GET("/open-apis/wiki/v2/tasks/:task_id").
		Desc("[2] Poll wiki delete-space task result when async").
		Set("task_id", "<task_id>").
		Params(map[string]interface{}{"task_type": "delete_space"})
	return dry
}

func dryRunWikiDeleteSpaceID(spec wikiDeleteSpaceSpec) string {
	if spec.SpaceID != "" {
		return validate.EncodePathSegment(spec.SpaceID)
	}
	return "<space_id>"
}

func runWikiDeleteSpace(ctx context.Context, client wikiDeleteSpaceClient, runtime *common.RuntimeContext, spec wikiDeleteSpaceSpec) (map[string]interface{}, error) {
	response, err := client.DeleteSpace(ctx, spec.SpaceID)
	if err != nil {
		return nil, err
	}

	out := map[string]interface{}{
		"space_id": spec.SpaceID,
	}

	// Empty task_id means the delete completed synchronously. A non-empty
	// task_id means the backend queued an async deletion; poll until it
	// resolves or the bounded window elapses.
	if response.TaskID == "" {
		// Sync and async success envelopes keep the same shape so downstream
		// scripts can read `status` uniformly regardless of which branch fired.
		out["ready"] = true
		out["failed"] = false
		out["status"] = wikiDeleteSpaceStatusSuccess
		out["status_msg"] = wikiDeleteSpaceStatusSuccess
		return out, nil
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Wiki space delete is async, polling task %s...\n", response.TaskID)
	status, ready, err := pollWikiDeleteSpaceTask(ctx, client, runtime, response.TaskID)
	if err != nil {
		return nil, err
	}

	out["task_id"] = response.TaskID
	out["ready"] = ready
	out["failed"] = status.Failed()
	out["status"] = status.StatusCode()
	out["status_msg"] = status.StatusLabel()

	if !ready {
		nextCommand := wikiDeleteSpaceTaskResultCommand(response.TaskID, runtime.As())
		fmt.Fprintf(runtime.IO().ErrOut, "Wiki delete-space task is still in progress. Continue with: %s\n", nextCommand)
		out["timed_out"] = true
		out["next_command"] = nextCommand
	}
	return out, nil
}

func wikiDeleteSpaceTaskResultCommand(taskID string, identity core.Identity) string {
	asFlag := string(identity)
	if asFlag == "" {
		asFlag = "user"
	}
	return fmt.Sprintf("lark-cli drive +task_result --scenario wiki_delete_space --task-id %s --as %s", taskID, asFlag)
}

func pollWikiDeleteSpaceTask(ctx context.Context, client wikiDeleteSpaceClient, runtime *common.RuntimeContext, taskID string) (wikiDeleteSpaceTaskStatus, bool, error) {
	lastStatus := wikiDeleteSpaceTaskStatus{TaskID: taskID}
	var lastErr error
	hadSuccessfulPoll := false

	// The delete request already succeeded. Treat poll failures as transient
	// until every attempt fails, then return a resume hint instead of discarding
	// the task identifier.
	for attempt := 1; attempt <= wikiDeleteSpacePollAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return lastStatus, false, ctx.Err()
			case <-time.After(wikiDeleteSpacePollInterval):
			}
		}

		status, err := client.GetDeleteSpaceTask(ctx, taskID)
		if err != nil {
			lastErr = err
			fmt.Fprintf(runtime.IO().ErrOut, "Wiki delete-space status attempt %d/%d failed: %v\n", attempt, wikiDeleteSpacePollAttempts, err)
			continue
		}
		lastStatus = status
		hadSuccessfulPoll = true

		if status.Ready() {
			fmt.Fprintf(runtime.IO().ErrOut, "Wiki delete-space task completed successfully.\n")
			return status, true, nil
		}
		if status.Failed() {
			return status, false, output.Errorf(output.ExitAPI, "api_error", "wiki delete-space task %s failed: %s", taskID, status.StatusLabel())
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Wiki delete-space status %d/%d: %s\n", attempt, wikiDeleteSpacePollAttempts, status.StatusLabel())
	}

	if !hadSuccessfulPoll && lastErr != nil {
		nextCommand := wikiDeleteSpaceTaskResultCommand(taskID, runtime.As())
		hint := fmt.Sprintf(
			"the wiki delete-space task was created but every status poll failed (task_id=%s)\nretry status lookup with: %s",
			taskID,
			nextCommand,
		)
		var exitErr *output.ExitError
		if errors.As(lastErr, &exitErr) && exitErr.Detail != nil {
			if strings.TrimSpace(exitErr.Detail.Hint) != "" {
				hint = exitErr.Detail.Hint + "\n" + hint
			}
			return lastStatus, false, output.ErrWithHint(exitErr.Code, exitErr.Detail.Type, exitErr.Detail.Message, hint)
		}
		return lastStatus, false, output.ErrWithHint(output.ExitAPI, "api_error", lastErr.Error(), hint)
	}

	return lastStatus, false, nil
}

func parseWikiDeleteSpaceTaskStatus(taskID string, task map[string]interface{}) (wikiDeleteSpaceTaskStatus, error) {
	if task == nil {
		return wikiDeleteSpaceTaskStatus{}, output.Errorf(output.ExitAPI, "api_error", "wiki task response missing task")
	}

	result := common.GetMap(task, "delete_space_result")
	status := wikiDeleteSpaceTaskStatus{
		TaskID: common.GetString(task, "task_id"),
	}
	if status.TaskID == "" {
		status.TaskID = taskID
	}
	if result != nil {
		status.Status = common.GetString(result, "status")
		status.StatusMsg = common.GetString(result, "status_msg")
	}
	return status, nil
}
