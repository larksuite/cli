// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// batchCreateKR represents a key result in the batch create input.
type batchCreateKR struct {
	Text    string   `json:"text"`
	Mention []string `json:"mention,omitempty"`
}

// batchCreateObjective represents an objective in the batch create input.
type batchCreateObjective struct {
	Text    string          `json:"text"`
	Mention []string        `json:"mention,omitempty"`
	KRs     []batchCreateKR `json:"krs,omitempty"`
}

// createdObjective tracks a created objective and its KR IDs for output.
// KRs are automatically deleted by the backend when the objective is deleted (no need to delete them separately during rollback).
type createdObjective struct {
	ObjectiveID string
	KRIDs       []string // for output response only, not used in rollback
}

// parseBatchCreateInput parses and validates the JSON input.
func parseBatchCreateInput(input string) ([]batchCreateObjective, error) {
	var objectives []batchCreateObjective
	if err := json.Unmarshal([]byte(input), &objectives); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--input must be valid JSON array: %s", err).WithParam("--input").WithCause(err)
	}
	if len(objectives) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--input must contain at least one objective").WithParam("--input")
	}
	for i, obj := range objectives {
		if strings.TrimSpace(obj.Text) == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "objective[%d].text is required and cannot be empty", i).WithParam("--input")
		}
		for j, kr := range obj.KRs {
			if strings.TrimSpace(kr.Text) == "" {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "objective[%d].krs[%d].text is required and cannot be empty", i, j).WithParam("--input")
			}
		}
	}
	return objectives, nil
}

// buildContentBlock converts text and mentions to a ContentBlock.
func buildContentBlock(text string, mentions []string) *ContentBlock {
	elements := make([]ContentParagraphElement, 0, len(mentions)+1)

	// Add text element
	textElem := ContentParagraphElement{
		ParagraphElementType: ParagraphElementTypeTextRun.Ptr(),
		TextRun: &ContentTextRun{
			Text: &text,
		},
	}
	elements = append(elements, textElem)

	// Add mention elements
	for _, mention := range mentions {
		mentionElem := ContentParagraphElement{
			ParagraphElementType: ParagraphElementTypeMention.Ptr(),
			Mention: &ContentMention{
				UserID: &mention,
			},
		}
		elements = append(elements, mentionElem)
	}

	return &ContentBlock{
		Blocks: []ContentBlockElement{
			{
				BlockElementType: BlockElementTypeParagraph.Ptr(),
				Paragraph: &ContentParagraph{
					Elements: elements,
				},
			},
		},
	}
}

// createObjective calls the API to create an objective.
func createObjective(ctx context.Context, runtime *common.RuntimeContext, cycleID, userIDType string, obj batchCreateObjective) (string, error) {
	content := buildContentBlock(obj.Text, obj.Mention)
	body := map[string]interface{}{
		"content": content,
	}
	queryParams := map[string]interface{}{
		"cycle_id":     cycleID,
		"user_id_type": userIDType,
	}

	path := fmt.Sprintf("/open-apis/okr/v2/cycles/%s/objectives", cycleID)
	data, err := runtime.CallAPITyped("POST", path, queryParams, body)
	if err != nil {
		return "", wrapOkrNetworkErr(err, "failed to create objective")
	}

	objectiveID, ok := data["objective_id"].(string)
	if !ok {
		return "", errs.NewInternalError(errs.SubtypeUnknown, "create objective response missing objective_id")
	}
	return objectiveID, nil
}

// createKR calls the API to create a key result.
func createKR(ctx context.Context, runtime *common.RuntimeContext, objectiveID, userIDType string, kr batchCreateKR) (string, error) {
	content := buildContentBlock(kr.Text, kr.Mention)
	body := map[string]interface{}{
		"content": content,
	}
	queryParams := map[string]interface{}{
		"objective_id": objectiveID,
		"user_id_type": userIDType,
	}

	path := fmt.Sprintf("/open-apis/okr/v2/objectives/%s/key_results", objectiveID)
	data, err := runtime.CallAPITyped("POST", path, queryParams, body)
	if err != nil {
		return "", wrapOkrNetworkErr(err, "failed to create key result")
	}

	krID, ok := data["key_result_id"].(string)
	if !ok {
		return "", errs.NewInternalError(errs.SubtypeUnknown, "create key result response missing key_result_id")
	}
	return krID, nil
}

// deleteObjective deletes an objective (used for rollback).
func deleteObjective(ctx context.Context, runtime *common.RuntimeContext, objectiveID string) error {
	queryParams := map[string]interface{}{
		"objective_id": objectiveID,
		"yes":          true,
	}
	path := fmt.Sprintf("/open-apis/okr/v2/objectives/%s", objectiveID)
	_, err := runtime.CallAPITyped("DELETE", path, queryParams, nil)
	if err != nil {
		return wrapOkrNetworkErr(err, "failed to delete objective %s during rollback", objectiveID)
	}
	return nil
}

// rollback deletes created objectives in reverse order.
// KRs are automatically deleted by the backend when the objective is deleted.
func rollback(ctx context.Context, runtime *common.RuntimeContext, created []createdObjective) []error {
	var errsList []error

	// Iterate in reverse order
	for i := len(created) - 1; i >= 0; i-- {
		obj := created[i]

		// Delete the objective (backend automatically deletes its KRs)
		if err := deleteObjective(ctx, runtime, obj.ObjectiveID); err != nil {
			//nolint:forbidigo // intermediate wrap for rollback error collection; final error is typed via buildRollbackError
			errsList = append(errsList, fmt.Errorf("objective %s: %w", obj.ObjectiveID, err))
		}

		// Rate limiting between deletions
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	return errsList
}

// OKRBatchCreate batch creates objectives and their key results.
var OKRBatchCreate = common.Shortcut{
	Service:     "okr",
	Command:     "+batch-create",
	Description: "Batch create OKR objectives and key results with rollback on failure",
	Risk:        "write",
	Scopes:      []string{"okr:okr.content:writeonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "cycle-id", Desc: "OKR cycle ID (int64)", Required: true},
		{Name: "input", Desc: "JSON array of objectives: [{\"text\":\"...\",\"mention\":[\"...\"],\"krs\":[{\"text\":\"...\",\"mention\":[\"...\"]}]}]", Input: []string{common.File, common.Stdin}, Required: true},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		cycleID := runtime.Str("cycle-id")
		if id, err := strconv.ParseInt(cycleID, 10, 64); err != nil || id <= 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--cycle-id must be a positive int64").WithParam("--cycle-id")
		}

		input := runtime.Str("input")
		if err := common.RejectDangerousCharsTyped("--input", input); err != nil {
			return err
		}
		if _, err := parseBatchCreateInput(input); err != nil {
			return err
		}

		idType := runtime.Str("user-id-type")
		if idType != "open_id" && idType != "union_id" && idType != "user_id" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-id-type must be one of: open_id | union_id | user_id").WithParam("--user-id-type")
		}

		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		cycleID := runtime.Str("cycle-id")
		userIDType := runtime.Str("user-id-type")
		objectives, _ := parseBatchCreateInput(runtime.Str("input"))

		apis := common.NewDryRunAPI()

		for i, obj := range objectives {
			// Objective creation
			objContent := buildContentBlock(obj.Text, obj.Mention)
			objBody := map[string]interface{}{
				"content": objContent,
			}
			objParams := map[string]interface{}{
				"cycle_id":     cycleID,
				"user_id_type": userIDType,
			}
			objPath := fmt.Sprintf("/open-apis/okr/v2/cycles/%s/objectives", cycleID)
			apis = apis.
				POST(objPath).
				Params(objParams).
				Body(objBody).
				Desc(fmt.Sprintf("Create objective[%d]: %s", i, obj.Text))

			// KR creations
			for j, kr := range obj.KRs {
				krContent := buildContentBlock(kr.Text, kr.Mention)
				krBody := map[string]interface{}{
					"content": krContent,
				}
				krParams := map[string]interface{}{
					"objective_id": "<objective_id_from_previous_call>",
					"user_id_type": userIDType,
				}
				krPath := "/open-apis/okr/v2/objectives/<objective_id>/key_results"
				apis = apis.
					POST(krPath).
					Params(krParams).
					Body(krBody).
					Desc(fmt.Sprintf("Create objective[%d].krs[%d]: %s", i, j, kr.Text))
			}
		}

		return apis
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		cycleID := runtime.Str("cycle-id")
		userIDType := runtime.Str("user-id-type")
		objectives, err := parseBatchCreateInput(runtime.Str("input"))
		if err != nil {
			return err
		}

		var created []createdObjective

		for i, obj := range objectives {
			// Rate limiting between objectives
			if i > 0 {
				time.Sleep(500 * time.Millisecond)
			}

			// Create objective
			objectiveID, err := createObjective(ctx, runtime, cycleID, userIDType, obj)
			if err != nil {
				if len(created) == 0 {
					return err
				}
				rollbackErrs := rollback(ctx, runtime, created)
				return buildRollbackError(err, rollbackErrs, created)
			}

			createdObj := createdObjective{
				ObjectiveID: objectiveID,
			}

			// Create KRs
			for j, kr := range obj.KRs {
				// Rate limiting between KRs
				if j > 0 {
					time.Sleep(500 * time.Millisecond)
				}

				krID, err := createKR(ctx, runtime, objectiveID, userIDType, kr)
				if err != nil {
					created = append(created, createdObj)
					rollbackErrs := rollback(ctx, runtime, created)
					return buildRollbackError(err, rollbackErrs, created)
				}

				createdObj.KRIDs = append(createdObj.KRIDs, krID)
			}

			created = append(created, createdObj)
		}

		// Build response
		respCreated := make([]map[string]interface{}, 0, len(created))
		for _, obj := range created {
			respCreated = append(respCreated, map[string]interface{}{
				"objective_id": obj.ObjectiveID,
				"krs":          obj.KRIDs,
			})
		}

		result := map[string]interface{}{
			"ok":   true,
			"data": map[string]interface{}{"created": respCreated},
		}

		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Successfully created %d objective(s)\n", len(created))
			for i, obj := range created {
				fmt.Fprintf(w, "Objective[%d] ID: %s (%d KR(s))\n", i, obj.ObjectiveID, len(obj.KRIDs))
				for j, krID := range obj.KRIDs {
					fmt.Fprintf(w, "  KR[%d] ID: %s\n", j, krID)
				}
			}
		})

		return nil
	},
}

// buildRollbackError constructs an error that includes both the original failure
// and any rollback failures, with a list of residual IDs that could not be cleaned up.
// KRs are automatically deleted by the backend when the objective is deleted, so we only
// need to track objective IDs for residual cleanup.
func buildRollbackError(originalErr error, rollbackErrs []error, created []createdObjective) error {
	var residualIDs []string

	// Only collect residual IDs when rollback had failures
	// If rollback succeeded (len(rollbackErrs) == 0), all objectives were deleted
	if len(rollbackErrs) > 0 {
		for _, obj := range created {
			residualIDs = append(residualIDs, fmt.Sprintf("objective:%s", obj.ObjectiveID))
		}
	}

	msg := fmt.Sprintf("batch create failed, rolling back: %v", originalErr)
	if len(rollbackErrs) > 0 {
		var rollbackMsgs []string
		for _, e := range rollbackErrs {
			rollbackMsgs = append(rollbackMsgs, e.Error())
		}
		msg += fmt.Sprintf("; rollback also had %d failure(s): %s", len(rollbackErrs), strings.Join(rollbackMsgs, "; "))
	}
	if len(residualIDs) > 0 {
		msg += fmt.Sprintf("; residual objectives that may need manual cleanup (KRs auto-deleted with objective): %s", strings.Join(residualIDs, ", "))
	}

	// Preserve the original error's type information if it's already a typed error
	if prob, ok := errs.ProblemOf(originalErr); ok {
		switch prob.Category {
		case errs.CategoryAPI:
			return errs.NewAPIError(prob.Subtype, "%s", msg).WithCause(originalErr)
		case errs.CategoryNetwork:
			return errs.NewNetworkError(prob.Subtype, "%s", msg).WithCause(originalErr)
		case errs.CategoryValidation:
			return errs.NewValidationError(prob.Subtype, "%s", msg).WithCause(originalErr)
		case errs.CategoryInternal:
			return errs.NewInternalError(prob.Subtype, "%s", msg).WithCause(originalErr)
		default:
			return errs.NewInternalError(prob.Subtype, "%s", msg).WithCause(originalErr)
		}
	}

	return errs.NewInternalError(errs.SubtypeUnknown, "%s", msg).WithCause(originalErr)
}
