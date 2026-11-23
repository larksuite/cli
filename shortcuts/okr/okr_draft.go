// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

func validateDraftObjectiveID(ctx context.Context, runtime *common.RuntimeContext) error {
	objectiveID := runtime.Str("objective-id")
	if err := common.RejectDangerousCharsTyped("--objective-id", objectiveID); err != nil {
		return err
	}
	if id, err := strconv.ParseInt(objectiveID, 10, 64); err != nil || id <= 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--objective-id must be a positive int64").WithParam("--objective-id")
	}
	return nil
}

func executeObjectiveDraftAction(runtime *common.RuntimeContext, action, verb, successVerb string) error {
	objectiveID := runtime.Str("objective-id")
	path := fmt.Sprintf("/open-apis/okr/v2/objectives/%s/%s", objectiveID, action)
	data, err := runtime.CallAPITyped("POST", path, nil, nil)
	if err != nil {
		return wrapOkrNetworkErr(err, "failed to %s objective draft", verb)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid objective draft response: %s", err).WithCause(err)
	}
	var response struct {
		ObjectiveID string `json:"objective_id"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid objective draft response: %s", err).WithCause(err)
	}
	if response.ObjectiveID != "" {
		objectiveID = response.ObjectiveID
	}

	result := map[string]interface{}{
		"objective_id": objectiveID,
		"action":       action,
	}
	runtime.OutFormat(result, nil, func(w io.Writer) {
		fmt.Fprintf(w, "%s objective draft [%s]\n", successVerb, objectiveID)
	})
	return nil
}

// OKRPublishDraft publishes the entire draft aggregate for an objective.
var OKRPublishDraft = common.Shortcut{
	Service:     "okr",
	Command:     "+publish-draft",
	Description: "Publish an objective draft",
	Risk:        "write",
	Scopes:      []string{"okr:okr.content:writeonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "objective-id", Desc: "objective ID (int64)", Required: true},
	},
	Validate: validateDraftObjectiveID,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/okr/v2/objectives/:objective_id/draft_publish").
			Set("objective_id", runtime.Str("objective-id")).
			Desc("Publish the entire objective draft")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeObjectiveDraftAction(runtime, "draft_publish", "publish", "Published")
	},
}

// OKRDropDraft drops the entire draft aggregate for an objective.
var OKRDropDraft = common.Shortcut{
	Service:     "okr",
	Command:     "+drop-draft",
	Description: "Drop an objective draft",
	Risk:        "high-risk-write",
	Scopes:      []string{"okr:okr.content:writeonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "objective-id", Desc: "objective ID (int64)", Required: true},
	},
	Validate: validateDraftObjectiveID,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/okr/v2/objectives/:objective_id/draft_drop").
			Set("objective_id", runtime.Str("objective-id")).
			Desc("Permanently drop the entire objective draft")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeObjectiveDraftAction(runtime, "draft_drop", "drop", "Dropped")
	},
}
