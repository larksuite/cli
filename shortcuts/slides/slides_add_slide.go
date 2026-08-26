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

// SlidesAddSlide appends (or inserts) a single page into an existing
// presentation. It is the second half of the two-step creation flow: create a
// blank deck with +create, then add pages one at a time.
//
// Value-adds over the raw xml_presentation.slide.create command:
//
//  1. --presentation accepts a token / slides URL / wiki URL, like every other
//     slides shortcut, instead of a hand-built --params JSON blob.
//  2. --slide takes the XML directly (and via @file / stdin), so callers stop
//     nesting a fully escaped XML document inside a JSON string inside a shell
//     argument — the escaping layer that produces most 3350001 reports.
//  3. <img src="@./local.png"> placeholders are uploaded and rewritten to
//     file_tokens, the same as +create --slides. Previously this combination
//     had no CLI support at all: adding an image-bearing page to an existing
//     deck meant calling +media-upload and splicing the token in by hand.
//
// Deliberately single-page: the backend endpoint creates one page per call, so
// a batch flag here would just be a client-side loop with partial-failure
// semantics to explain. Callers who want many pages loop the command, or use
// +create --slides when the deck does not exist yet.
var SlidesAddSlide = common.Shortcut{
	Service:     "slides",
	Command:     "+add-slide",
	Description: "Add one page to an existing presentation (<img src=\"@./local.png\"> placeholders are auto-uploaded and replaced with file_token)",
	Risk:        "write",
	Scopes:      []string{"slides:presentation:update", "slides:presentation:write_only"},
	// Both extras are path-dependent, so they stay conditional rather than
	// gating every call: wiki:node:read only when --presentation is a wiki URL,
	// docs:document.media:upload only when the XML carries @-placeholders.
	// Unlike +create there is no orphan risk to pre-empt here — the
	// presentation already exists, so a late upload 403 leaves nothing behind.
	ConditionalScopes: []string{"wiki:node:read", "docs:document.media:upload"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		requiredPresentationRefFlag(),
		// The "(supports @file, - reads stdin ...)" suffix is appended from Input
		// below, so spelling it out here too produced a doubled parenthetical.
		{Name: "slide", Desc: "one complete <slide> XML document", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "before-slide-id", Desc: "insert before this slide_id (default: append after the last page)"},
		{Name: "revision-id", Type: "int", Default: "-1", Desc: "presentation revision (-1 = latest; pass a specific number for optimistic locking)"},
	},
	Tips: []string{
		"<img src=\"@path\"> placeholders resolve against the current directory, not the directory of the --slide file, and are deduplicated per call: a page-by-page loop re-uploads a shared image once per page, so upload it once with slides +media-upload and reuse the file_token instead.",
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
		slideXML, err := addSlideXML(runtime)
		if err != nil {
			return err
		}
		// validateCompleteSlideXML reports the structural problem alone ("root
		// element is <presentation>, want <slide>"). Re-tag it with the flag it
		// came from so the caller sees which input to fix, and so agents can
		// route on the typed Param.
		if err := validateCompleteSlideXML(slideXML); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide is not a single complete <slide> document: %v", err).WithParam("--slide").WithCause(err)
		}
		// Check placeholder files before any API call so a typo in a path fails
		// locally instead of after the page is half-built.
		placeholders := extractImagePlaceholderPaths([]string{slideXML})
		if len(placeholders) > 0 {
			if err := runtime.EnsureScopes([]string{"docs:document.media:upload"}); err != nil {
				return err
			}
			if err := validateImagePlaceholderFiles(runtime, "--slide", placeholders); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		slideXML, err := addSlideXML(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}

		placeholders := extractImagePlaceholderPaths([]string{slideXML})
		dry := common.NewDryRunAPI()

		presentationID := ref.Token
		step := 1
		total := 1 + len(placeholders)
		if ref.Kind == "wiki" {
			total++
		}

		if ref.Kind == "wiki" {
			presentationID = unresolvedSlidesTokenPlaceholder
			dry.Desc(fmt.Sprintf("%d-step orchestration: resolve wiki → add page", total)).
				GET("/open-apis/wiki/v2/spaces/get_node").
				Desc(fmt.Sprintf("[%d/%d] Resolve wiki node to slides presentation", step, total)).
				Params(map[string]interface{}{"token": ref.Token})
			step++
		} else if len(placeholders) > 0 {
			dry.Desc(fmt.Sprintf("Upload %d image(s) + add 1 page", len(placeholders)))
		} else {
			dry.Desc("Add 1 page")
		}

		for _, path := range placeholders {
			appendSlidesUploadDryRun(dry, path, presentationID, slidesDryRunParentType(ref), step)
			step++
		}

		descSuffix := ""
		if len(placeholders) > 0 {
			descSuffix = " (img placeholders auto-replaced)"
		}
		dry.POST(fmt.Sprintf(
			"/open-apis/slides_ai/v1/xml_presentations/%s/slide",
			validate.EncodePathSegment(presentationID),
		)).
			Desc(fmt.Sprintf("[%d/%d] Add page%s", step, total, descSuffix)).
			Params(addSlideQuery(runtime)).
			Body(addSlideBody(slideXML, runtime.Str("before-slide-id")))

		return dry.Set("images_to_upload", len(placeholders))
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
		slideXML, err := addSlideXML(runtime)
		if err != nil {
			return err
		}

		result := map[string]interface{}{
			"xml_presentation_id": presentationID,
		}

		// Uploads run against the target presentation, so they can only happen
		// after a wiki ref has been resolved to a real presentation id.
		placeholders := extractImagePlaceholderPaths([]string{slideXML})
		if len(placeholders) > 0 {
			tokens, uploaded, err := uploadSlidesPlaceholders(runtime, presentationID, placeholders, "--slide")
			if err != nil {
				return appendSlidesProgressHint(err, fmt.Sprintf("no page was added; %d of %d image(s) uploaded before failure", uploaded, len(placeholders)))
			}
			slideXML = replaceImagePlaceholders(slideXML, tokens)
			result["images_uploaded"] = uploaded
		}

		beforeSlideID := strings.TrimSpace(runtime.Str("before-slide-id"))
		data, err := runtime.CallAPITyped(
			"POST",
			fmt.Sprintf(
				"/open-apis/slides_ai/v1/xml_presentations/%s/slide",
				validate.EncodePathSegment(presentationID),
			),
			addSlideQuery(runtime),
			addSlideBody(slideXML, beforeSlideID),
		)
		if err != nil {
			if len(placeholders) > 0 {
				// The images are already in the deck's media store; say so, or
				// a retry silently uploads a second copy of every file.
				err = appendSlidesProgressHint(err, fmt.Sprintf("%d image(s) were uploaded before the page failed; re-running will upload them again", len(placeholders)))
			}
			return enrichSlidesReplaceError(err)
		}

		slideID := common.GetString(data, "slide_id")
		if slideID == "" {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "slide.create returned no slide_id")
		}
		result["slide_id"] = slideID
		if beforeSlideID != "" {
			result["before_slide_id"] = beforeSlideID
		}
		if rev, ok := revisionFromData(data); ok {
			result["revision_id"] = rev
		}
		// issues carries backend-side schema warnings for content that was
		// accepted but altered; pass it through untouched so the caller can
		// decide whether the page still says what they meant.
		if issues, ok := data["issues"]; ok {
			result["issues"] = issues
		}

		runtime.Out(result, nil)
		return nil
	},
}

// addSlideXML returns the trimmed --slide value, rejecting an empty one.
// --slide is Required, so cobra already blocks a missing flag; this catches
// `--slide ""` and an @file / stdin source that turned out to be blank.
func addSlideXML(runtime *common.RuntimeContext) (string, error) {
	xml := strings.TrimSpace(runtime.Str("slide"))
	if xml == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide cannot be empty").WithParam("--slide")
	}
	return xml, nil
}

// addSlideQuery builds the query params shared by dry-run and execute.
func addSlideQuery(runtime *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{"revision_id": runtime.Int("revision-id")}
}

// addSlideBody builds the request body shared by dry-run and execute.
// before_slide_id is omitted when empty: the backend appends to the end only
// if the key is absent, and an empty string is rejected as an unknown slide.
func addSlideBody(slideXML, beforeSlideID string) map[string]interface{} {
	body := map[string]interface{}{
		"slide": map[string]interface{}{"content": slideXML},
	}
	if id := strings.TrimSpace(beforeSlideID); id != "" {
		body["before_slide_id"] = id
	}
	return body
}
