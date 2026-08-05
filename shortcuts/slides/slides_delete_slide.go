// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// SlidesDeleteSlide removes a single page from a presentation.
//
// Value-adds over the raw xml_presentation.slide.delete command are the same
// two every slides shortcut provides: --presentation accepts a token / slides
// URL / wiki URL, and the identifiers are ordinary flags instead of a
// hand-escaped --params JSON blob.
//
// Deliberately single-page: deletion is destructive and slide_id lists invite
// a partial-failure story ("3 of 5 deleted, which 3?"). One page per call
// keeps the outcome unambiguous.
var SlidesDeleteSlide = common.Shortcut{
	Service:     "slides",
	Command:     "+delete-slide",
	Description: "Delete one page from a presentation by slide_id",
	Risk:        "write",
	Scopes:      []string{"slides:presentation:update", "slides:presentation:write_only"},
	// wiki:node:read is required only when --presentation is a wiki URL.
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		requiredPresentationRefFlag(),
		{Name: "slide-id", Desc: "slide page identifier (slide_id) to delete", Required: true},
		{Name: "revision-id", Type: "int", Default: "-1", Desc: "presentation revision (-1 = latest; pass a specific number for optimistic locking)"},
	},
	Tips: []string{
		"Deletion is not undoable in place; recover a wrongly deleted page with slides +history-list then +history-revert.",
		"Use --dry-run to confirm which presentation and slide_id will be hit before running.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		if ref.Kind == "wiki" {
			if err := runtime.EnsureScopes([]string{"wiki:node:read"}); err != nil {
				return err
			}
		}
		if _, err := deleteSlideID(runtime); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		slideID, err := deleteSlideID(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}

		dry := common.NewDryRunAPI()
		presentationID := ref.Token
		step := 1
		total := 1
		if ref.Kind == "wiki" {
			total = 2
			presentationID = "<resolved_slides_token>"
			dry.Desc("2-step orchestration: resolve wiki → delete page").
				GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1/2] Resolve wiki node to slides presentation").
				Params(map[string]interface{}{"token": ref.Token})
			step = 2
		} else {
			dry.Desc(fmt.Sprintf("Delete page %s", slideID))
		}

		dry.DELETE(fmt.Sprintf(
			"/open-apis/slides_ai/v1/xml_presentations/%s/slide",
			validate.EncodePathSegment(presentationID),
		)).
			Desc(fmt.Sprintf("[%d/%d] Delete page %s", step, total, slideID)).
			Params(deleteSlideQuery(runtime, slideID))

		return dry.Set("slide_id", slideID)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		presentationID, err := resolvePresentationID(runtime, ref)
		if err != nil {
			return err
		}
		slideID, err := deleteSlideID(runtime)
		if err != nil {
			return err
		}

		data, err := runtime.CallAPITyped(
			"DELETE",
			fmt.Sprintf(
				"/open-apis/slides_ai/v1/xml_presentations/%s/slide",
				validate.EncodePathSegment(presentationID),
			),
			deleteSlideQuery(runtime, slideID),
			nil,
		)
		if err != nil {
			return err
		}

		result := map[string]interface{}{
			"xml_presentation_id": presentationID,
			"slide_id":            slideID,
			"deleted":             true,
		}
		if rev, ok := revisionFromData(data); ok {
			result["revision_id"] = rev
		}

		runtime.Out(result, nil)
		return nil
	},
}

// deleteSlideID returns the trimmed --slide-id, rejecting an empty one.
// --slide-id is Required, so cobra already blocks a missing flag; this catches
// `--slide-id ""`, which the backend would otherwise reject as a 404-ish
// "slide not found" after a pointless round trip.
func deleteSlideID(runtime *common.RuntimeContext) (string, error) {
	id := strings.TrimSpace(runtime.Str("slide-id"))
	if id == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide-id cannot be empty").WithParam("--slide-id")
	}
	return id, nil
}

// deleteSlideQuery builds the query params shared by dry-run and execute.
func deleteSlideQuery(runtime *common.RuntimeContext, slideID string) map[string]interface{} {
	return map[string]interface{}{
		"slide_id":    slideID,
		"revision_id": runtime.Int("revision-id"),
	}
}
