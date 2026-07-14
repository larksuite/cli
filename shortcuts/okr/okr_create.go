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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// createParams holds the parsed parameters for single-object create operations.
type createParams struct {
	Level       string
	CycleID     string
	ObjectiveID string
	Style       string
	Content     *ContentBlock
	UserIDType  string
}

// parseCreateParams parses and validates flags from runtime into request-ready parameters.
func parseCreateParams(runtime *common.RuntimeContext) (*createParams, error) {
	p := &createParams{
		Level:       runtime.Str("level"),
		CycleID:     runtime.Str("cycle-id"),
		ObjectiveID: runtime.Str("objective-id"),
		Style:       runtime.Str("style"),
		UserIDType:  runtime.Str("user-id-type"),
	}

	contentStr := runtime.Str("content")
	if contentStr == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content is required").WithParam("--content")
	}
	if err := common.RejectDangerousCharsTyped("--content", contentStr); err != nil {
		return nil, err
	}

	if p.Style == "simple" {
		var sp SemiPlainContent
		if err := json.Unmarshal([]byte(contentStr), &sp); err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content must be valid semi-plain JSON: {\"text\":\"...\",\"mention\":[\"...\"]}: %s", err).WithParam("--content").WithCause(err)
		}
		if strings.TrimSpace(sp.Text) == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content text is required and cannot be empty").WithParam("--content")
		}
		for i, mention := range sp.Mention {
			if strings.TrimSpace(mention) == "" {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content mention[%d] cannot be empty", i).WithParam("--content")
			}
		}
		if len(sp.Docs) > 0 || len(sp.Images) > 0 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content docs and images are not supported in simple style input; use richtext style or remove these fields").WithParam("--content")
		}
		p.Content = sp.ToContentBlock()
		return p, nil
	}

	var cb ContentBlock
	if err := json.Unmarshal([]byte(contentStr), &cb); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content must be valid ContentBlock JSON: %s", err).WithParam("--content").WithCause(err)
	}
	if len(cb.Blocks) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content must contain at least one block").WithParam("--content")
	}

	hasNonEmptyParagraph := false
	for _, block := range cb.Blocks {
		if block.Paragraph != nil && len(block.Paragraph.Elements) > 0 {
			hasNonEmptyParagraph = true
			break
		}
		if block.Gallery != nil && len(block.Gallery.Images) > 0 {
			hasNonEmptyParagraph = true
			break
		}
	}
	if !hasNonEmptyParagraph {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content cannot be empty").WithParam("--content")
	}

	p.Content = &cb
	return p, nil
}

// OKRCreate creates a single objective or key result.
var OKRCreate = common.Shortcut{
	Service:     "okr",
	Command:     "+create",
	Description: "Create a single OKR objective or key result",
	Risk:        "write",
	Scopes:      []string{"okr:okr.content:writeonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "level", Desc: "create level: objective | key-result", Required: true, Enum: []string{"objective", "key-result"}},
		{Name: "cycle-id", Desc: "OKR cycle ID (required for level=objective)"},
		{Name: "objective-id", Desc: "objective ID (required for level=key-result)"},
		{Name: "style", Default: "simple", Desc: "input style for content: simple (semi-plain text JSON) | richtext (ContentBlock JSON)", Enum: []string{"simple", "richtext"}},
		{Name: "content", Desc: "content: semi-plain JSON {\"text\":\"...\",\"mention\":[\"...\"]} (simple) or ContentBlock JSON (richtext)", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		level := runtime.Str("level")
		if level != "objective" && level != "key-result" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--level must be one of: objective | key-result").WithParam("--level")
		}

		style := runtime.Str("style")
		if style != "simple" && style != "richtext" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--style must be one of: simple | richtext").WithParam("--style")
		}

		idType := runtime.Str("user-id-type")
		if idType != "open_id" && idType != "union_id" && idType != "user_id" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-id-type must be one of: open_id | union_id | user_id").WithParam("--user-id-type")
		}

		switch level {
		case "objective":
			cycleID := runtime.Str("cycle-id")
			if cycleID == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--cycle-id is required when --level=objective").WithParam("--cycle-id")
			}
			if err := common.RejectDangerousCharsTyped("--cycle-id", cycleID); err != nil {
				return err
			}
			if id, err := strconv.ParseInt(cycleID, 10, 64); err != nil || id <= 0 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--cycle-id must be a positive int64").WithParam("--cycle-id")
			}
		case "key-result":
			objectiveID := runtime.Str("objective-id")
			if objectiveID == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--objective-id is required when --level=key-result").WithParam("--objective-id")
			}
			if err := common.RejectDangerousCharsTyped("--objective-id", objectiveID); err != nil {
				return err
			}
			if id, err := strconv.ParseInt(objectiveID, 10, 64); err != nil || id <= 0 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--objective-id must be a positive int64").WithParam("--objective-id")
			}
		}

		_, err := parseCreateParams(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		p, err := parseCreateParams(runtime)
		if err != nil {
			return common.NewDryRunAPI().
				POST("").
				Desc(fmt.Sprintf("Dry-run skipped: %s", err.Error()))
		}

		body := map[string]interface{}{
			"content": p.Content,
		}
		params := map[string]interface{}{
			"user_id_type": p.UserIDType,
		}

		if p.Level == "objective" {
			params["cycle_id"] = p.CycleID
			return common.NewDryRunAPI().
				POST("/open-apis/okr/v2/cycles/:cycle_id/objectives").
				Set("cycle_id", p.CycleID).
				Params(params).
				Body(body).
				Desc("Create OKR objective")
		}

		params["objective_id"] = p.ObjectiveID
		return common.NewDryRunAPI().
			POST("/open-apis/okr/v2/objectives/:objective_id/key_results").
			Set("objective_id", p.ObjectiveID).
			Params(params).
			Body(body).
			Desc("Create OKR key result")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		p, err := parseCreateParams(runtime)
		if err != nil {
			return err
		}

		body := map[string]interface{}{
			"content": p.Content,
		}
		queryParams := map[string]interface{}{
			"user_id_type": p.UserIDType,
		}

		result := map[string]interface{}{
			"level": p.Level,
		}

		if p.Level == "objective" {
			queryParams["cycle_id"] = p.CycleID
			path := fmt.Sprintf("/open-apis/okr/v2/cycles/%s/objectives", p.CycleID)
			data, err := runtime.CallAPITyped("POST", path, queryParams, body)
			if err != nil {
				return wrapOkrNetworkErr(err, "failed to create objective")
			}
			objectiveID, ok := data["objective_id"].(string)
			if !ok || objectiveID == "" {
				return errs.NewInternalError(errs.SubtypeUnknown, "create objective response missing objective_id")
			}
			result["objective_id"] = objectiveID

			runtime.OutFormat(result, nil, func(w io.Writer) {
				fmt.Fprintf(w, "Created OKR objective [%s]\n", objectiveID)
			})
			return nil
		}

		queryParams["objective_id"] = p.ObjectiveID
		path := fmt.Sprintf("/open-apis/okr/v2/objectives/%s/key_results", p.ObjectiveID)
		data, err := runtime.CallAPITyped("POST", path, queryParams, body)
		if err != nil {
			return wrapOkrNetworkErr(err, "failed to create key result")
		}
		keyResultID, ok := data["key_result_id"].(string)
		if !ok || keyResultID == "" {
			return errs.NewInternalError(errs.SubtypeUnknown, "create key result response missing key_result_id")
		}
		result["key_result_id"] = keyResultID
		result["objective_id"] = p.ObjectiveID

		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Created OKR key-result [%s] under objective [%s]\n", keyResultID, p.ObjectiveID)
		})
		return nil
	},
}
