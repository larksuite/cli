// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// SlidesReplacePages rebuilds multiple pages inside an existing presentation.
// It deliberately creates the new page before deleting the old one so a create
// failure cannot remove existing user content. The operation is not atomic.
const replacePagesInitialRevisionID = -1

var SlidesReplacePages = common.Shortcut{
	Service:     "slides",
	Command:     "+replace-pages",
	Description: "Batch rebuild pages inside an existing Slides presentation (create before old page, then delete old page; not atomic)",
	Risk:        "write",
	Scopes:      []string{"slides:presentation:update", "slides:presentation:write_only"},
	// wiki:node:read is required only when --presentation is a wiki URL.
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides", Required: true},
		{Name: "pages", Desc: "JSON array of page replacements (each: {slide_id, content}); supports @file or -", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "continue-on-error", Type: "bool", Desc: "continue with later pages after a create/delete failure; default false"},
		{Name: "validate-only", Type: "bool", Desc: "validate input and build the create/delete plan without write calls"},
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
		pages, err := parseReplacePages(runtime.Str("pages"))
		if err != nil {
			return err
		}
		return validateReplacePagesInput(pages)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		dry := common.NewDryRunAPI()
		resolved, err := prepareReplacePages(runtime)
		if err != nil {
			return dry.Set("error", err.Error())
		}
		appendReplacePagesDryRunCalls(dry, resolved)
		return dry.
			Set("xml_presentation_id", resolved.PresentationID).
			Set("pages_count", len(resolved.Plan)).
			Set("plan", replacePagesPlanOutput(resolved.Plan)).
			Set("note", "dry-run built a create/delete plan from slide_id inputs; no Slides presentation get/create/delete calls were executed")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		resolved, err := prepareReplacePages(runtime)
		if err != nil {
			return err
		}
		if runtime.Bool("validate-only") {
			runtime.Out(map[string]interface{}{
				"xml_presentation_id": resolved.PresentationID,
				"pages_count":         len(resolved.Plan),
				"plan":                replacePagesPlanOutput(resolved.Plan),
				"status":              "validated",
				"note":                "validate-only checked input and built the create/delete plan; no Slides presentation get/create/delete calls were executed",
			}, nil)
			return nil
		}

		revisionID := replacePagesInitialRevisionID
		results := make([]replacePageResult, 0, len(resolved.Plan))
		for i, item := range resolved.Plan {
			result, err := replaceOnePage(runtime, resolved.PresentationID, item, revisionID)
			results = append(results, result)
			if result.RevisionID != nil {
				revisionID = *result.RevisionID
			}
			if err != nil {
				if runtime.Bool("continue-on-error") {
					continue
				}
				return appendSlidesProgressHint(err, fmt.Sprintf("slides +replace-pages stopped at item %d/%d; %d page(s) completed before failure; old page is kept when create failed", i+1, len(resolved.Plan), countReplacedPages(results)))
			}
		}

		out := map[string]interface{}{
			"xml_presentation_id": resolved.PresentationID,
			"pages_count":         len(resolved.Plan),
			"results":             replacePageResultsOutput(results),
			"status":              "completed",
			"summary":             replacePagesSummaryOutput(results),
			"note":                "batch replace is not atomic; each page was created before its old page was deleted",
		}
		if revisionID != replacePagesInitialRevisionID {
			out["revision_id"] = revisionID
		}
		if hasReplacePageFailures(results) {
			out["status"] = "partial_failure"
			return runtime.OutPartialFailure(out, nil)
		}
		runtime.Out(out, nil)
		return nil
	},
}

type replacePageInput struct {
	SlideID string
	Content string
}

type replacePagePlanItem struct {
	OldSlideID string
	Content    string
	Locator    string
}

type replacePagesPrepared struct {
	PresentationID string
	Plan           []replacePagePlanItem
}

type replacePageResult struct {
	OldSlideID string
	NewSlideID string
	Status     string
	Error      string
	Issues     interface{}
	RevisionID *int
}

func prepareReplacePages(runtime *common.RuntimeContext) (*replacePagesPrepared, error) {
	ref, err := parsePresentationRef(runtime.Str("presentation"))
	if err != nil {
		return nil, err
	}
	presentationID, err := resolvePresentationID(runtime, ref)
	if err != nil {
		return nil, err
	}
	pages, err := parseReplacePages(runtime.Str("pages"))
	if err != nil {
		return nil, err
	}
	if err := validateReplacePagesInput(pages); err != nil {
		return nil, err
	}

	plan, err := buildReplacePagesPlan(pages)
	if err != nil {
		return nil, err
	}
	return &replacePagesPrepared{PresentationID: presentationID, Plan: plan}, nil
}

func parseReplacePages(raw string) ([]replacePageInput, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--pages cannot be empty").WithParam("--pages")
	}
	var decoded []map[string]interface{}
	if err := json.Unmarshal([]byte(s), &decoded); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--pages invalid JSON, must be an array of objects: %v", err).WithParam("--pages").WithCause(err)
	}
	out := make([]replacePageInput, 0, len(decoded))
	for i, m := range decoded {
		p := replacePageInput{}
		if v, ok := m["slide_number"]; ok {
			_ = v
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--pages[%d].slide_number is no longer supported; use slide_id", i).WithParam("--pages").WithHint("read current slide IDs first, then pass slide_id for each page replacement")
		}
		if v, ok := m["slide_id"]; ok {
			s, ok := v.(string)
			if !ok {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--pages[%d].slide_id must be a string", i).WithParam("--pages")
			}
			p.SlideID = s
		}
		if v, ok := m["content"]; ok {
			s, ok := v.(string)
			if !ok {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--pages[%d].content must be a string", i).WithParam("--pages")
			}
			p.Content = s
		}
		out = append(out, p)
	}
	return out, nil
}

func validateReplacePagesInput(pages []replacePageInput) error {
	if len(pages) == 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--pages must contain at least 1 item").WithParam("--pages")
	}
	seenIDs := map[string]bool{}
	for i, p := range pages {
		id := strings.TrimSpace(p.SlideID)
		if id == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--pages[%d].slide_id is required", i).WithParam("--pages")
		}
		if seenIDs[id] {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--pages contains duplicate slide_id %q", id).WithParam("--pages")
		}
		seenIDs[id] = true
		if strings.TrimSpace(p.Content) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--pages[%d].content cannot be empty", i).WithParam("--pages")
		}
		if err := validateCompleteSlideXML(p.Content); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--pages[%d].content must be a complete <slide> XML element: %v", i, err).WithParam("--pages").WithCause(err)
		}
	}
	return nil
}

func validateCompleteSlideXML(content string) error {
	dec := xml.NewDecoder(strings.NewReader(content))
	depth := 0
	seenRoot := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				if seenRoot {
					return invalidSlideXMLStructureError("multiple root elements")
				}
				if t.Name.Local != "slide" {
					return invalidSlideXMLStructureError("root element is <%s>, want <slide>", t.Name.Local)
				}
				seenRoot = true
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return invalidSlideXMLStructureError("non-whitespace text outside root element")
			}
		}
	}
	if !seenRoot {
		return invalidSlideXMLStructureError("missing root element")
	}
	if depth != 0 {
		return invalidSlideXMLStructureError("unclosed XML element")
	}
	return nil
}

func invalidSlideXMLStructureError(format string, args ...interface{}) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

func buildReplacePagesPlan(pages []replacePageInput) ([]replacePagePlanItem, error) {
	plan := make([]replacePagePlanItem, 0, len(pages))
	for _, page := range pages {
		id := strings.TrimSpace(page.SlideID)
		plan = append(plan, replacePagePlanItem{
			OldSlideID: id,
			Content:    page.Content,
			Locator:    "slide_id",
		})
	}
	return plan, nil
}

func appendReplacePagesDryRunCalls(dry *common.DryRunAPI, resolved *replacePagesPrepared) {
	dry.Desc("Batch replace pages in-place: create each new page before old page, then delete old page (not atomic)")
	for i, item := range resolved.Plan {
		dry.POST(fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s/slide", validate.EncodePathSegment(resolved.PresentationID))).
			Desc(fmt.Sprintf("[%d/%d] Create replacement before old slide %s", i*2+1, len(resolved.Plan)*2, item.OldSlideID)).
			Params(map[string]interface{}{"revision_id": "<latest_or_revision_returned_by_previous_step>"}).
			Body(map[string]interface{}{
				"slide":           map[string]interface{}{"content": item.Content},
				"before_slide_id": item.OldSlideID,
			})
		dry.DELETE(fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s/slide", validate.EncodePathSegment(resolved.PresentationID))).
			Desc(fmt.Sprintf("[%d/%d] Delete old slide %s after create succeeds", i*2+2, len(resolved.Plan)*2, item.OldSlideID)).
			Params(map[string]interface{}{
				"slide_id":    item.OldSlideID,
				"revision_id": "<revision_returned_by_create>",
			})
	}
}

func replaceOnePage(runtime *common.RuntimeContext, presentationID string, item replacePagePlanItem, revisionID int) (replacePageResult, error) {
	result := replacePageResult{
		OldSlideID: item.OldSlideID,
		Status:     "pending",
	}
	slideURL := fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s/slide", validate.EncodePathSegment(presentationID))
	createData, err := runtime.CallAPITyped(
		"POST",
		slideURL,
		map[string]interface{}{"revision_id": revisionID},
		map[string]interface{}{
			"slide":           map[string]interface{}{"content": item.Content},
			"before_slide_id": item.OldSlideID,
		},
	)
	if err != nil {
		result.Status = "create_failed"
		result.Error = err.Error()
		return result, err
	}
	newSlideID := common.GetString(createData, "slide_id")
	if newSlideID == "" {
		err := errs.NewInternalError(errs.SubtypeInvalidResponse, "slide.create returned no slide_id for replacement of slide_id %q", item.OldSlideID)
		result.Status = "create_failed"
		result.Error = err.Error()
		return result, err
	}
	result.NewSlideID = newSlideID
	if issues, ok := createData["issues"]; ok {
		result.Issues = issues
	}
	if rev, ok := revisionFromData(createData); ok {
		revisionID = rev
		result.RevisionID = &rev
	}

	deleteData, err := runtime.CallAPITyped(
		"DELETE",
		slideURL,
		map[string]interface{}{
			"slide_id":    item.OldSlideID,
			"revision_id": revisionID,
		},
		nil,
	)
	if err != nil {
		result.Status = "delete_failed"
		result.Error = err.Error()
		return result, err
	}
	if rev, ok := revisionFromData(deleteData); ok {
		result.RevisionID = &rev
	}
	result.Status = "replaced"
	return result, nil
}

func revisionFromData(data map[string]interface{}) (int, bool) {
	if _, ok := data["revision_id"]; !ok {
		return 0, false
	}
	return int(common.GetFloat(data, "revision_id")), true
}

func replacePagesPlanOutput(plan []replacePagePlanItem) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(plan))
	for _, item := range plan {
		out = append(out, map[string]interface{}{
			"old_slide_id":           item.OldSlideID,
			"insert_before_slide_id": item.OldSlideID,
			"locator":                item.Locator,
			"action":                 "create_before_then_delete_old",
		})
	}
	return out
}

func replacePageResultsOutput(results []replacePageResult) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(results))
	for _, result := range results {
		m := map[string]interface{}{
			"old_slide_id": result.OldSlideID,
			"status":       result.Status,
		}
		if result.NewSlideID != "" {
			m["new_slide_id"] = result.NewSlideID
		}
		if result.Error != "" {
			m["error"] = result.Error
		}
		if result.Issues != nil {
			m["issues"] = result.Issues
		}
		if result.RevisionID != nil {
			m["revision_id"] = *result.RevisionID
		}
		out = append(out, m)
	}
	return out
}

func replacePagesSummaryOutput(results []replacePageResult) map[string]interface{} {
	replaced := countReplacedPages(results)
	return map[string]interface{}{
		"replaced": replaced,
		"failed":   len(results) - replaced,
		"total":    len(results),
	}
}

func countReplacedPages(results []replacePageResult) int {
	n := 0
	for _, result := range results {
		if result.Status == "replaced" {
			n++
		}
	}
	return n
}

func hasReplacePageFailures(results []replacePageResult) bool {
	for _, result := range results {
		if result.Status == "create_failed" || result.Status == "delete_failed" {
			return true
		}
	}
	return false
}
