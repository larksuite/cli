// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	Notes       *ContentBlock
	CategoryID  string
	UserIDType  string
}

type createContentMultipleJSONValuesError struct{}

func (createContentMultipleJSONValuesError) Error() string {
	return "multiple JSON values"
}

var errCreateContentMultipleJSONValues createContentMultipleJSONValuesError

type okrCreateRequestBody struct {
	Content    *ContentBlock `json:"content"`
	Notes      *ContentBlock `json:"notes,omitempty"`
	CategoryID string        `json:"category_id,omitempty"`
}

type okrCreateObjectiveQuery struct {
	CycleID    string
	UserIDType string
}

type okrCreateKeyResultQuery struct {
	ObjectiveID string
	UserIDType  string
}

type okrCreateObjectiveResponse struct {
	ObjectiveID string
}

type okrCreateKeyResultResponse struct {
	KeyResultID string
}

type okrCreateObjectiveOutput struct {
	Level       string `json:"level"`
	ObjectiveID string `json:"objective_id"`
}

type okrCreateKeyResultOutput struct {
	Level       string `json:"level"`
	ObjectiveID string `json:"objective_id"`
	KeyResultID string `json:"key_result_id"`
}

func decodeCreateContentStrict(inputStr string, target interface{}, param, message string) error {
	dec := json.NewDecoder(bytes.NewReader([]byte(inputStr)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, message, err).
			WithParam(param).
			WithCause(err)
	}
	var trailing interface{}
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errCreateContentMultipleJSONValues
		}
		return errs.NewValidationError(errs.SubtypeInvalidArgument, message, err).
			WithParam(param).
			WithCause(err)
	}
	return nil
}

func parseCreateContentValue(inputStr, param, style string) (*ContentBlock, error) {
	if style == "simple" {
		var sp SemiPlainContent
		if err := decodeCreateContentStrict(inputStr, &sp, param, fmt.Sprintf("%s must be valid semi-plain JSON: {\"text\":\"...\",\"mention\":[\"...\"]}: %%s", param)); err != nil {
			return nil, err
		}
		if strings.TrimSpace(sp.Text) == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s text is required and cannot be empty", param).WithParam(param)
		}
		for i, mention := range sp.Mention {
			if strings.TrimSpace(mention) == "" {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s mention[%d] cannot be empty", param, i).WithParam(param)
			}
		}
		if len(sp.Docs) > 0 || len(sp.Images) > 0 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s docs and images are not supported in simple style input; use richtext style or remove these fields", param).WithParam(param)
		}
		return sp.ToContentBlock(), nil
	}

	var cb ContentBlock
	if err := decodeCreateContentStrict(inputStr, &cb, param, fmt.Sprintf("%s must be valid ContentBlock JSON: %%s", param)); err != nil {
		return nil, err
	}
	if len(cb.Blocks) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s must contain at least one block", param).WithParam(param)
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
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s cannot be empty", param).WithParam(param)
	}
	return &cb, nil
}

func projectCreateRequestBody(body okrCreateRequestBody) map[string]interface{} {
	result := map[string]interface{}{
		"content": body.Content,
	}
	if body.Notes != nil {
		result["notes"] = body.Notes
	}
	if body.CategoryID != "" {
		result["category_id"] = body.CategoryID
	}
	return result
}

func projectCreateObjectiveQuery(query okrCreateObjectiveQuery) map[string]interface{} {
	return map[string]interface{}{
		"cycle_id":     query.CycleID,
		"user_id_type": query.UserIDType,
	}
}

func projectCreateKeyResultQuery(query okrCreateKeyResultQuery) map[string]interface{} {
	return map[string]interface{}{
		"objective_id": query.ObjectiveID,
		"user_id_type": query.UserIDType,
	}
}

func projectCreateObjectiveResponse(data map[string]interface{}) (*okrCreateObjectiveResponse, error) {
	objectiveID, ok := data["objective_id"].(string)
	if !ok || objectiveID == "" {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "create objective response missing objective_id")
	}
	return &okrCreateObjectiveResponse{ObjectiveID: objectiveID}, nil
}

func projectCreateKeyResultResponse(data map[string]interface{}) (*okrCreateKeyResultResponse, error) {
	keyResultID, ok := data["key_result_id"].(string)
	if !ok || keyResultID == "" {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "create key result response missing key_result_id")
	}
	return &okrCreateKeyResultResponse{KeyResultID: keyResultID}, nil
}

// parseCreateParams parses and validates flags from runtime into request-ready parameters.
func parseCreateParams(runtime *common.RuntimeContext) (*createParams, error) {
	p := &createParams{
		Level:       runtime.Str("level"),
		CycleID:     runtime.Str("cycle-id"),
		ObjectiveID: runtime.Str("objective-id"),
		Style:       runtime.Str("style"),
		CategoryID:  runtime.Str("category-id"),
		UserIDType:  runtime.Str("user-id-type"),
	}

	contentStr := runtime.Str("content")
	if contentStr == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content is required").WithParam("--content")
	}
	if err := common.RejectDangerousCharsTyped("--content", contentStr); err != nil {
		return nil, err
	}
	content, err := parseCreateContentValue(contentStr, "--content", p.Style)
	if err != nil {
		return nil, err
	}
	p.Content = content

	if notesStr := runtime.Str("notes"); notesStr != "" {
		if p.Level != "objective" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--notes is only supported when --level=objective").WithParam("--notes")
		}
		if err := common.RejectDangerousCharsTyped("--notes", notesStr); err != nil {
			return nil, err
		}
		notes, err := parseCreateContentValue(notesStr, "--notes", p.Style)
		if err != nil {
			return nil, err
		}
		p.Notes = notes
	}
	if p.CategoryID != "" {
		if p.Level != "objective" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--category-id is only supported when --level=objective").WithParam("--category-id")
		}
		if err := common.RejectDangerousCharsTyped("--category-id", p.CategoryID); err != nil {
			return nil, err
		}
		if id, err := strconv.ParseInt(p.CategoryID, 10, 64); err != nil || id <= 0 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--category-id must be a positive int64").WithParam("--category-id")
		}
	}
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
		{Name: "notes", Desc: "objective notes: semi-plain JSON {\"text\":\"...\",\"mention\":[\"...\"]} (simple) or ContentBlock JSON (richtext)", Input: []string{common.File, common.Stdin}},
		{Name: "category-id", Desc: "objective category ID; use only when classification is requested or the tenant requires categories"},
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
			if runtime.Str("objective-id") != "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--objective-id cannot be used when --level=objective").WithParam("--objective-id")
			}
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
			if runtime.Str("cycle-id") != "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--cycle-id cannot be used when --level=key-result").WithParam("--cycle-id")
			}
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

		body := projectCreateRequestBody(okrCreateRequestBody{Content: p.Content, Notes: p.Notes, CategoryID: p.CategoryID})

		if p.Level == "objective" {
			params := projectCreateObjectiveQuery(okrCreateObjectiveQuery{
				CycleID:    p.CycleID,
				UserIDType: p.UserIDType,
			})
			return common.NewDryRunAPI().
				POST("/open-apis/okr/v2/cycles/:cycle_id/objectives").
				Set("cycle_id", p.CycleID).
				Params(params).
				Body(body).
				Desc("Create OKR objective")
		}

		params := projectCreateKeyResultQuery(okrCreateKeyResultQuery{
			ObjectiveID: p.ObjectiveID,
			UserIDType:  p.UserIDType,
		})
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

		body := projectCreateRequestBody(okrCreateRequestBody{Content: p.Content, Notes: p.Notes, CategoryID: p.CategoryID})

		if p.Level == "objective" {
			queryParams := projectCreateObjectiveQuery(okrCreateObjectiveQuery{
				CycleID:    p.CycleID,
				UserIDType: p.UserIDType,
			})
			path := fmt.Sprintf("/open-apis/okr/v2/cycles/%s/objectives", p.CycleID)
			data, err := runtime.CallAPITyped("POST", path, queryParams, body)
			if err != nil {
				return wrapOkrNetworkErr(err, "failed to create objective")
			}
			resp, err := projectCreateObjectiveResponse(data)
			if err != nil {
				return err
			}
			result := okrCreateObjectiveOutput{
				Level:       p.Level,
				ObjectiveID: resp.ObjectiveID,
			}

			runtime.OutFormat(result, nil, func(w io.Writer) {
				fmt.Fprintf(w, "Created OKR objective [%s]\n", resp.ObjectiveID)
			})
			return nil
		}

		queryParams := projectCreateKeyResultQuery(okrCreateKeyResultQuery{
			ObjectiveID: p.ObjectiveID,
			UserIDType:  p.UserIDType,
		})
		path := fmt.Sprintf("/open-apis/okr/v2/objectives/%s/key_results", p.ObjectiveID)
		data, err := runtime.CallAPITyped("POST", path, queryParams, body)
		if err != nil {
			return wrapOkrNetworkErr(err, "failed to create key result")
		}
		resp, err := projectCreateKeyResultResponse(data)
		if err != nil {
			return err
		}
		result := okrCreateKeyResultOutput{
			Level:       p.Level,
			ObjectiveID: p.ObjectiveID,
			KeyResultID: resp.KeyResultID,
		}

		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Created OKR key-result [%s] under objective [%s]\n", resp.KeyResultID, p.ObjectiveID)
		})
		return nil
	},
}
