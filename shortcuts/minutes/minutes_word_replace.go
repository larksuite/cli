// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	minutesWordReplaceNoEditPermission = 40005
	minutesWordReplaceOthersEditing    = 40110
	minutesWordReplaceInvalidParams    = 40001
)

type transcriptWordReplace struct {
	SourceWord string `json:"source_word"`
	TargetWord string `json:"target_word"`
}

// MinutesWordReplace batch-replaces words in a minute's transcript.
var MinutesWordReplace = common.Shortcut{
	Service:     "minutes",
	Command:     "+word-replace",
	Description: "Batch replace words in a minute's transcript",
	Risk:        "write",
	Scopes:      []string{"minutes:minutes:update"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "minute-token", Desc: "minute token", Required: true},
		{
			Name:     "replace-words",
			Desc:     `JSON array of replacements, e.g. [{"source_word":"old","target_word":"new"}]`,
			Required: true,
			Input:    []string{common.File, common.Stdin},
		},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		if minuteToken == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--minute-token is required").WithParam("--minute-token")
		}
		if err := validate.ResourceName(minuteToken, "--minute-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--minute-token")
		}
		if _, err := parseReplaceWords(runtime.Str("replace-words")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		replaceWords, _ := parseReplaceWords(runtime.Str("replace-words"))
		return common.NewDryRunAPI().
			PUT(fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/transcript/word", validate.EncodePathSegment(minuteToken))).
			Body(map[string]interface{}{
				"minute_token":  minuteToken,
				"replace_words": replaceWords,
			})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		replaceWords, err := parseReplaceWords(runtime.Str("replace-words"))
		if err != nil {
			return err
		}

		body := map[string]interface{}{
			"minute_token":  minuteToken,
			"replace_words": replaceWords,
		}

		_, err = runtime.CallAPITyped(http.MethodPut,
			fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/transcript/word", validate.EncodePathSegment(minuteToken)),
			nil, body)
		if err != nil {
			return minutesWordReplaceError(err, minuteToken)
		}

		outData := map[string]interface{}{
			"minute_token":  minuteToken,
			"replace_words": replaceWords,
		}

		runtime.OutFormat(outData, nil, nil)
		return nil
	},
}

func parseReplaceWords(raw string) ([]map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--replace-words: value is required").WithParam("--replace-words")
	}

	var items []transcriptWordReplace
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--replace-words: must be a JSON array of {source_word,target_word} objects: %v", err).WithParam("--replace-words").WithCause(err)
	}
	if len(items) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--replace-words: must include at least one replacement").WithParam("--replace-words")
	}

	replaceWords := make([]map[string]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		sourceWord := strings.TrimSpace(item.SourceWord)
		if sourceWord == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--replace-words: item %d: source_word is required", i).WithParam("--replace-words")
		}
		if _, exists := seen[sourceWord]; exists {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--replace-words: duplicate source_word %q", sourceWord).WithParam("--replace-words")
		}
		seen[sourceWord] = struct{}{}
		replaceWords = append(replaceWords, map[string]string{
			"source_word": sourceWord,
			"target_word": item.TargetWord,
		})
	}
	return replaceWords, nil
}

func minutesWordReplaceError(err error, minuteToken string) error {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}

	switch p.Code {
	case minutesWordReplaceNoEditPermission:
		p.Subtype = errs.SubtypePermissionDenied
		p.Message = fmt.Sprintf("No edit permission for minute %q: cannot replace transcript words.", minuteToken)
		p.Hint = "Ask the minute owner for minute edit permission"
	case minutesWordReplaceOthersEditing:
		p.Subtype = errs.SubtypeConflict
		p.Message = fmt.Sprintf("Minute %q transcript is being edited by someone else.", minuteToken)
		p.Hint = "Wait until the other editor finishes, then retry"
	case minutesWordReplaceInvalidParams:
		if strings.Contains(strings.ToLower(p.Message), "not found in transcript") {
			p.Subtype = errs.SubtypeNotFound
			p.Message = fmt.Sprintf("None of the source words were found in minute %q transcript; nothing was replaced.", minuteToken)
			p.Hint = "Verify each source_word's exact spelling and case against the current transcript (use vc +notes to read it), then retry"
		}
	}

	return err
}
