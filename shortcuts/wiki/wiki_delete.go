// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
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

// Back-compat aliases — the shared async-task helper now owns the strings,
// but tests still reference these names.
const (
	wikiDeleteSpaceStatusSuccess    = wikiAsyncStatusSuccess
	wikiDeleteSpaceStatusFailure    = wikiAsyncStatusFailure
	wikiDeleteSpaceStatusProcessing = wikiAsyncStatusProcessing
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

// wikiDeleteSpaceTaskStatus is an alias for the shared wiki async-task shape;
// kept as a named type for the existing test surface. delete-node uses the
// same type directly under its real name (wikiAsyncTaskStatus).
type wikiDeleteSpaceTaskStatus = wikiAsyncTaskStatus

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
	return parseWikiAsyncTaskStatus(taskID, common.GetMap(data, "task"), wikiAsyncResultDeleteSpace)
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
	return pollWikiAsyncTask(
		ctx, runtime, taskID, "delete-space",
		wikiDeleteSpacePollAttempts, wikiDeleteSpacePollInterval,
		func(ctx context.Context, id string) (wikiAsyncTaskStatus, error) {
			return client.GetDeleteSpaceTask(ctx, id)
		},
		wikiDeleteSpaceTaskResultCommand(taskID, runtime.As()),
	)
}

// parseWikiDeleteSpaceTaskStatus is kept as a thin wrapper for the existing
// test surface; new callers should use parseWikiAsyncTaskStatus directly.
func parseWikiDeleteSpaceTaskStatus(taskID string, task map[string]interface{}) (wikiDeleteSpaceTaskStatus, error) {
	return parseWikiAsyncTaskStatus(taskID, task, wikiAsyncResultDeleteSpace)
}
