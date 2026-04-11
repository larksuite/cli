// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	defaultPresentationWidth  = 960
	defaultPresentationHeight = 540
	maxSlidesPerCreate        = 10
)

// SlidesCreate creates a new Lark Slides presentation with bot auto-grant.
var SlidesCreate = common.Shortcut{
	Service:     "slides",
	Command:     "+create",
	Description: "Create a Lark Slides presentation",
	Risk:        "write",
	AuthTypes:   []string{"user", "bot"},
	Scopes:      []string{"slides:presentation:create", "slides:presentation:write_only"},
	Flags: []common.Flag{
		{Name: "title", Desc: "presentation title"},
		{Name: "slides", Desc: "slide content JSON array (each element is a <slide> XML string, max 10; for more pages, create first then add via xml_presentation.slide.create)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if slidesStr := runtime.Str("slides"); slidesStr != "" {
			var slides []string
			if err := json.Unmarshal([]byte(slidesStr), &slides); err != nil {
				return common.FlagErrorf("--slides invalid JSON, must be an array of XML strings")
			}
			if len(slides) > maxSlidesPerCreate {
				return common.FlagErrorf("--slides array exceeds maximum of %d slides; create the presentation first, then add slides via xml_presentation.slide.create", maxSlidesPerCreate)
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		title := effectiveTitle(runtime.Str("title"))
		slidesStr := runtime.Str("slides")
		createBody := map[string]interface{}{
			"xml_presentation": map[string]interface{}{"content": buildPresentationXML(title)},
		}

		dry := common.NewDryRunAPI()

		if slidesStr == "" {
			dry.Desc("Create empty presentation").
				POST("/open-apis/slides_ai/v1/xml_presentations").
				Body(createBody)
		} else {
			var slides []string
			_ = json.Unmarshal([]byte(slidesStr), &slides)
			n := len(slides)
			total := n + 1

			dry.Desc(fmt.Sprintf("Create presentation + add %d slide(s)", n)).
				POST("/open-apis/slides_ai/v1/xml_presentations").
				Desc(fmt.Sprintf("[1/%d] Create presentation", total)).
				Body(createBody)

			for i, slideXML := range slides {
				dry.POST("/open-apis/slides_ai/v1/xml_presentations/<xml_presentation_id>/slide").
					Desc(fmt.Sprintf("[%d/%d] Add slide %d", i+2, total, i+1)).
					Body(map[string]interface{}{
						"slide": map[string]interface{}{"content": slideXML},
					})
			}
		}

		if runtime.IsBot() {
			dry.Desc("After creation succeeds in bot mode, the CLI will also try to grant the current CLI user full_access (可管理权限) on the new presentation.")
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		title := effectiveTitle(runtime.Str("title"))
		content := buildPresentationXML(title)
		slidesStr := runtime.Str("slides")

		// Step 1: Create presentation
		data, err := runtime.CallAPI(
			"POST",
			"/open-apis/slides_ai/v1/xml_presentations",
			nil,
			map[string]interface{}{
				"xml_presentation": map[string]interface{}{
					"content": content,
				},
			},
		)
		if err != nil {
			return err
		}

		presentationID := common.GetString(data, "xml_presentation_id")
		if presentationID == "" {
			return output.Errorf(output.ExitAPI, "api_error", "slides create returned no xml_presentation_id")
		}

		result := map[string]interface{}{
			"xml_presentation_id": presentationID,
			"title":               title,
		}
		if revisionID := common.GetFloat(data, "revision_id"); revisionID > 0 {
			result["revision_id"] = int(revisionID)
		}

		// Step 2: Add slides if provided
		if slidesStr != "" {
			var slides []string
			_ = json.Unmarshal([]byte(slidesStr), &slides) // already validated

			if len(slides) > 0 {
				slideURL := fmt.Sprintf(
					"/open-apis/slides_ai/v1/xml_presentations/%s/slide",
					validate.EncodePathSegment(presentationID),
				)

				var slideIDs []string
				for i, slideXML := range slides {
					slideData, err := runtime.CallAPI(
						"POST",
						slideURL,
						map[string]interface{}{"revision_id": -1},
						map[string]interface{}{
							"slide": map[string]interface{}{"content": slideXML},
						},
					)
					if err != nil {
						return output.Errorf(output.ExitAPI, "api_error",
							"slide %d/%d failed: %v (presentation %s was created; %d slide(s) added before failure)",
							i+1, len(slides), err, presentationID, i)
					}
					if sid := common.GetString(slideData, "slide_id"); sid != "" {
						slideIDs = append(slideIDs, sid)
					}
				}

				result["slide_ids"] = slideIDs
				result["slides_added"] = len(slideIDs)
			}
		}

		// Fetch presentation URL via drive meta (best-effort)
		if metaData, err := runtime.CallAPI(
			"POST",
			"/open-apis/drive/v1/metas/batch_query",
			nil,
			map[string]interface{}{
				"request_docs": []map[string]interface{}{
					{
						"doc_token": presentationID,
						"doc_type":  "slides",
					},
				},
				"with_url": true,
			},
		); err == nil {
			metas := common.GetSlice(metaData, "metas")
			if len(metas) > 0 {
				if meta, ok := metas[0].(map[string]interface{}); ok {
					if url := common.GetString(meta, "url"); url != "" {
						result["url"] = url
					}
				}
			}
		}

		if grant := common.AutoGrantCurrentUserDrivePermission(runtime, presentationID, "slides"); grant != nil {
			result["permission_grant"] = grant
		}

		runtime.Out(result, nil)
		return nil
	},
}

// effectiveTitle returns the title to use, falling back to "Untitled".
func effectiveTitle(title string) string {
	if title == "" {
		return "Untitled"
	}
	return title
}

// buildPresentationXML builds the minimal XML for a new empty presentation.
func buildPresentationXML(title string) string {
	escapedTitle := xmlEscape(title)
	if escapedTitle == "" {
		escapedTitle = "Untitled"
	}
	return fmt.Sprintf(
		`<presentation xmlns="http://www.larkoffice.com/sml/2.0" width="%d" height="%d"><title>%s</title></presentation>`,
		defaultPresentationWidth, defaultPresentationHeight, escapedTitle,
	)
}

// xmlEscape escapes special XML characters in text content.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
