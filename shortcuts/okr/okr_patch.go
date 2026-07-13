// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// patchParams holds the parsed parameters for the patch operation.
type patchParams struct {
	Level      string
	TargetID   string
	Style      string
	Content    *ContentBlock
	Notes      *ContentBlock
	Score      *float64
	Deadline   *string
	UserIDType string
}

// parsePatchParams parses and validates flags from runtime into request-ready parameters.
func parsePatchParams(runtime *common.RuntimeContext) (*patchParams, error) {
	p := &patchParams{
		Level:      runtime.Str("level"),
		TargetID:   runtime.Str("target-id"),
		Style:      runtime.Str("style"),
		UserIDType: runtime.Str("user-id-type"),
	}

	hasField := false

	// Parse content if provided
	if contentStr := runtime.Str("content"); contentStr != "" {
		hasField = true
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
			for i, m := range sp.Mention {
				if strings.TrimSpace(m) == "" {
					return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content mention[%d] cannot be empty", i).WithParam("--content")
				}
			}
			if len(sp.Docs) > 0 || len(sp.Images) > 0 {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content docs and images are not supported in simple style input; use richtext style or remove these fields").WithParam("--content")
			}
			p.Content = sp.ToContentBlock()
		} else {
			var cb ContentBlock
			if err := json.Unmarshal([]byte(contentStr), &cb); err != nil {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content must be valid ContentBlock JSON: %s", err).WithParam("--content").WithCause(err)
			}
			p.Content = &cb
		}
	}

	// Parse notes if provided (only for objective)
	if notesStr := runtime.Str("notes"); notesStr != "" {
		hasField = true
		if err := common.RejectDangerousCharsTyped("--notes", notesStr); err != nil {
			return nil, err
		}
		if p.Level != "objective" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--notes is only supported for level=objective").WithParam("--notes")
		}
		if p.Style == "simple" {
			var sp SemiPlainContent
			if err := json.Unmarshal([]byte(notesStr), &sp); err != nil {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--notes must be valid semi-plain JSON: {\"text\":\"...\",\"mention\":[\"...\"]}: %s", err).WithParam("--notes").WithCause(err)
			}
			if strings.TrimSpace(sp.Text) == "" {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--notes text is required and cannot be empty").WithParam("--notes")
			}
			for i, m := range sp.Mention {
				if strings.TrimSpace(m) == "" {
					return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--notes mention[%d] cannot be empty", i).WithParam("--notes")
				}
			}
			if len(sp.Docs) > 0 || len(sp.Images) > 0 {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--notes docs and images are not supported in simple style input; use richtext style or remove these fields").WithParam("--notes")
			}
			p.Notes = sp.ToContentBlock()
		} else {
			var cb ContentBlock
			if err := json.Unmarshal([]byte(notesStr), &cb); err != nil {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--notes must be valid ContentBlock JSON: %s", err).WithParam("--notes").WithCause(err)
			}
			p.Notes = &cb
		}
	}

	// Parse score if provided
	if scoreStr := runtime.Str("score"); scoreStr != "" {
		hasField = true
		score, err := strconv.ParseFloat(scoreStr, 64)
		if err != nil || math.IsNaN(score) || math.IsInf(score, 0) {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--score must be a valid number").WithParam("--score")
		}
		if score < 0 || score > 1 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--score must be between 0 and 1").WithParam("--score")
		}
		// Check for exactly one decimal place
		scoreStrTrimmed := strings.TrimRight(strings.TrimRight(scoreStr, "0"), ".")
		parts := strings.Split(scoreStrTrimmed, ".")
		if len(parts) == 2 && len(parts[1]) > 1 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--score must have at most one decimal place (e.g., 0.5, not 0.51)").WithParam("--score")
		}
		// Validation ensures at most one decimal place, so score is already correctly formatted
		p.Score = &score
	}

	// Parse deadline if provided
	if deadlineStr := runtime.Str("deadline"); deadlineStr != "" {
		hasField = true
		deadlineMs, err := strconv.ParseInt(deadlineStr, 10, 64)
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--deadline must be a valid millisecond timestamp (integer)").WithParam("--deadline")
		}
		if deadlineMs <= 0 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--deadline must be a positive millisecond timestamp").WithParam("--deadline")
		}
		// Reject non-millisecond timestamps: year 2000 in ms is ~946e9, year 2100 in ms is ~4.1e12
		// Anything less than 1e12 is likely seconds or a wrong unit
		if deadlineMs < 1000000000000 { // 1e12 ms = year ~33658, so use 1e12 as lower bound for reasonable ms timestamps
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--deadline must be a millisecond timestamp (13 digits), not seconds").WithParam("--deadline")
		}
		p.Deadline = &deadlineStr
	}

	// At least one field must be provided
	if !hasField {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "at least one of --content, --notes, --score, or --deadline must be provided")
	}

	return p, nil
}

// OKRPatch patches an objective or key result.
var OKRPatch = common.Shortcut{
	Service:     "okr",
	Command:     "+patch",
	Description: "Patch an OKR objective or key result (content, notes, score, deadline)",
	Risk:        "write",
	Scopes:      []string{"okr:okr.content:writeonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "level", Desc: "patch level: objective | key-result", Required: true, Enum: []string{"objective", "key-result"}},
		{Name: "target-id", Desc: "target ID (objective or key result ID)", Required: true},
		{Name: "style", Default: "simple", Desc: "input style for content/notes: simple (semi-plain text JSON) | richtext (ContentBlock JSON)", Enum: []string{"simple", "richtext"}},
		{Name: "content", Desc: "content: semi-plain JSON {\"text\":\"...\",\"mention\":[\"...\"]} (simple) or ContentBlock JSON (richtext)", Input: []string{common.File, common.Stdin}},
		{Name: "notes", Desc: "notes (objective only): semi-plain JSON {\"text\":\"...\",\"mention\":[\"...\"]} (simple) or ContentBlock JSON (richtext)", Input: []string{common.File, common.Stdin}},
		{Name: "score", Desc: "score value between 0 and 1, with at most one decimal place (e.g., 0.5)"},
		{Name: "deadline", Desc: "deadline as millisecond timestamp"},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		level := runtime.Str("level")
		if level != "objective" && level != "key-result" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--level must be one of: objective | key-result").WithParam("--level")
		}

		targetID := runtime.Str("target-id")
		if targetID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--target-id is required").WithParam("--target-id")
		}
		if err := common.RejectDangerousCharsTyped("--target-id", targetID); err != nil {
			return err
		}
		if id, err := strconv.ParseInt(targetID, 10, 64); err != nil || id <= 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--target-id must be a positive int64").WithParam("--target-id")
		}

		style := runtime.Str("style")
		if style != "simple" && style != "richtext" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--style must be one of: simple | richtext").WithParam("--style")
		}

		idType := runtime.Str("user-id-type")
		if idType != "open_id" && idType != "union_id" && idType != "user_id" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-id-type must be one of: open_id | union_id | user_id").WithParam("--user-id-type")
		}

		// Delegate content/notes/score/deadline validation to parsePatchParams
		if _, err := parsePatchParams(runtime); err != nil {
			return err
		}

		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		p, err := parsePatchParams(runtime)
		if err != nil {
			return common.NewDryRunAPI().
				PATCH("").
				Desc(fmt.Sprintf("Dry-run skipped: %s", err.Error()))
		}

		body := make(map[string]interface{})
		if p.Content != nil {
			body["content"] = p.Content
		}
		if p.Notes != nil {
			body["notes"] = p.Notes
		}
		if p.Score != nil {
			body["score"] = *p.Score
		}
		if p.Deadline != nil {
			body["deadline"] = *p.Deadline
		}

		params := map[string]interface{}{
			"user_id_type": p.UserIDType,
		}

		api := common.NewDryRunAPI()
		if p.Level == "objective" {
			api = api.PATCH("/open-apis/okr/v2/objectives/:objective_id").
				Set("objective_id", p.TargetID)
		} else {
			api = api.PATCH("/open-apis/okr/v2/key_results/:key_result_id").
				Set("key_result_id", p.TargetID)
		}
		return api.Params(params).Body(body).
			Desc(fmt.Sprintf("Patch OKR %s: content=%v, notes=%v, score=%v, deadline=%v",
				p.Level, p.Content != nil, p.Notes != nil, p.Score != nil, p.Deadline != nil))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		p, err := parsePatchParams(runtime)
		if err != nil {
			return err
		}

		body := make(map[string]interface{})
		if p.Content != nil {
			body["content"] = p.Content
		}
		if p.Notes != nil {
			body["notes"] = p.Notes
		}
		if p.Score != nil {
			body["score"] = *p.Score
		}
		if p.Deadline != nil {
			body["deadline"] = *p.Deadline
		}

		queryParams := map[string]interface{}{
			"user_id_type": p.UserIDType,
		}

		var path string
		if p.Level == "objective" {
			path = fmt.Sprintf("/open-apis/okr/v2/objectives/%s", p.TargetID)
		} else {
			path = fmt.Sprintf("/open-apis/okr/v2/key_results/%s", p.TargetID)
		}

		_, err = runtime.CallAPITyped("PATCH", path, queryParams, body)
		if err != nil {
			return wrapOkrNetworkErr(err, "failed to patch OKR %s", p.Level)
		}

		result := map[string]interface{}{
			"level":     p.Level,
			"target_id": p.TargetID,
			"patched": map[string]bool{
				"content":  p.Content != nil,
				"notes":    p.Notes != nil,
				"score":    p.Score != nil,
				"deadline": p.Deadline != nil,
			},
		}

		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Patched OKR %s [%s]\n", p.Level, p.TargetID)
			if p.Content != nil {
				fmt.Fprintf(w, "  - content: updated\n")
			}
			if p.Notes != nil {
				fmt.Fprintf(w, "  - notes: updated\n")
			}
			if p.Score != nil {
				fmt.Fprintf(w, "  - score: %.1f\n", *p.Score)
			}
			if p.Deadline != nil {
				fmt.Fprintf(w, "  - deadline: %s\n", formatTimestamp(*p.Deadline))
			}
		})

		return nil
	},
}
