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

// SlidesUpdateSlide applies a whole page of XML to an existing slide, keeping
// its slide_id and its position in the deck.
//
// The caller hands over the page they want; the CLI reads the page that is
// there, diffs the two, and sends one element-level part per difference. That
// indirection is not a stylistic choice — a single part covering the whole page
// is impossible. ReplacePart.block_id is validated as a short ELEMENT id (it
// must start with "b"), so the page's own id ("p"-prefixed) and the background
// fill's id ("f"-prefixed) are both rejected with 3350001. Element ids are the
// only handles this endpoint offers.
//
// What that buys the caller is the part they actually found painful: they no
// longer enumerate parts or hand-write each element's full XML (coordinates,
// size, font size included) to restyle a page. What it costs is one capability
// the endpoint cannot express at all:
//
//   - The page background lives in <style>, which has no id of its own and
//     whose <fill> id starts with "f". A changed <style> is therefore an error,
//     not a silent no-op.
//
// Two more limits fall out of having no move operation and no way to invent
// ids: reordering existing elements is rejected, and an id in --content that
// does not exist on the page is rejected rather than created.
var SlidesUpdateSlide = common.Shortcut{
	Service:     "slides",
	Command:     "+update-slide",
	Description: "Apply a full <slide> XML to an existing slide by diffing it against the current page (keeps slide_id and page order; background changes are not supported)",
	Risk:        "write",
	// slides:presentation:read is unconditional: every execution reads the
	// page before writing it, so it belongs in the enforced pre-flight set —
	// ConditionalScopes is metadata only and would let a write-only token
	// reach the GET before failing.
	Scopes: []string{"slides:presentation:read", "slides:presentation:update", "slides:presentation:write_only"},
	// wiki:node:read is required only when --presentation is a wiki URL.
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Tips: []string{
		"Read-modify-write: `slides +xml-get --presentation <id> --slide-id <sid> --output page.xml` → edit page.xml → `slides +update-slide --content @page.xml`",
		"--content is the page's target state: an element you drop is deleted, an element without an id is created",
		"Keep the <style> block from the read unchanged — the page background cannot be changed through this command",
		"Elements cannot be reordered and an unknown id cannot be created; both are rejected up front",
		"Editing one shape / image is cheaper with `slides +replace-slide`",
	},
	Flags:    updateSlideFlags,
	Validate: updateSlideValidate,
	DryRun:   updateSlideDryRun,
	Execute:  updateSlideExecute,
}

// SlidesUpdate registers `slides +update` as a hidden alias of +update-slide.
//
// Agents reach for "slide update" before reading --help (the command did not
// exist, so they burned turns on the error plus a help dump). Accepting the
// shorter spelling costs nothing and removes those round trips; it stays out
// of --help so the canonical name is the only one advertised.
//
// Derived from the canonical shortcut rather than re-declared, so scopes,
// identities and flags cannot drift between the two spellings.
var SlidesUpdate = func() common.Shortcut {
	sc := SlidesUpdateSlide
	sc.Command = "+update"
	sc.Hidden = true
	return sc
}()

var updateSlideFlags = []common.Flag{
	{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides", Required: true},
	{Name: "slide-id", Desc: "slide page identifier (slide_id) of the page to update", Required: true},
	{Name: "content", Desc: "full page XML with a single <slide> root; it is diffed against the current page", Required: true, Input: []string{common.File, common.Stdin}},
	{Name: "revision-id", Type: "int", Default: "-1", Desc: "revision to read and apply against; -1 (default) means latest. Pinning an older revision rebuilds the page from that snapshot and discards newer edits to it"},
	{Name: "tid", Desc: "transaction id for concurrent-edit locking (usually empty)"},
}

func updateSlideValidate(_ context.Context, runtime *common.RuntimeContext) error {
	ref, err := parsePresentationRef(runtime.Str("presentation"))
	if err != nil {
		return err
	}
	if ref.Kind == "wiki" {
		if err := runtime.EnsureScopes([]string{"wiki:node:read"}); err != nil {
			return err
		}
	}
	slideID, err := updateSlideID(runtime)
	if err != nil {
		return err
	}
	// Only the shape of --content can be checked without a network call; the
	// diff itself needs the current page.
	_, err = parseWantedPageFor(runtime.Str("content"), slideID)
	return err
}

// updateSlideDryRun reports what would be read and how, without calling the
// API. The parts cannot be shown: they are derived from the page's current
// state, which is exactly what dry-run must not fetch.
func updateSlideDryRun(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	ref, err := parsePresentationRef(runtime.Str("presentation"))
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}
	slideID, err := updateSlideID(runtime)
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}
	wanted, err := parseWantedPageFor(runtime.Str("content"), slideID)
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}

	dry := common.NewDryRunAPI()
	presentationID := ref.Token
	if ref.Kind == "wiki" {
		presentationID = "<resolved_slides_token>"
		dry.Desc("3-step orchestration: resolve wiki → read page → replace changed elements").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to slides presentation").
			Params(map[string]interface{}{"token": ref.Token})
	} else {
		dry.Desc(fmt.Sprintf("2-step orchestration: read slide %s, then replace the elements that differ", slideID))
	}
	dry.GET(slideReadAPIPath(presentationID)).
		Desc("[1] Read the current page to diff against").
		Params(updateSlideQuery(runtime, slideID))
	dry.POST(slideReplaceAPIPath(presentationID)).
		Desc("[2] One element-level part per difference; the parts depend on the page's current state").
		Params(updateSlideQuery(runtime, slideID))
	return dry.Set("wanted_element_count", len(wanted.Elements))
}

func updateSlideExecute(_ context.Context, runtime *common.RuntimeContext) error {
	ref, err := parsePresentationRef(runtime.Str("presentation"))
	if err != nil {
		return err
	}
	presentationID, err := resolvePresentationID(runtime, ref)
	if err != nil {
		return err
	}
	slideID, err := updateSlideID(runtime)
	if err != nil {
		return err
	}
	wanted, err := parseWantedPageFor(runtime.Str("content"), slideID)
	if err != nil {
		return err
	}

	current, err := readCurrentPage(runtime, presentationID, slideID)
	if err != nil {
		return err
	}

	// diffPage already reports the edits it cannot express as typed validation
	// errors against --content.
	diff, err := diffPage(current, wanted)
	if err != nil {
		return err
	}

	result := map[string]interface{}{
		"xml_presentation_id": presentationID,
		"slide_id":            slideID,
		"parts_count":         len(diff.Parts),
		"replaced":            diff.Replaced,
		"inserted":            diff.Inserted,
		"deleted":             diff.Deleted,
	}
	if diff.NoteCleared {
		result["note_cleared"] = true
	}
	if diff.NoteReplaced {
		result["note_replaced"] = true
	}
	// Nothing differs: report it instead of sending an empty batch, which the
	// backend rejects, and instead of claiming a write that never happened.
	if len(diff.Parts) == 0 {
		result["unchanged"] = true
		runtime.Out(result, nil)
		return nil
	}
	if len(diff.Parts) > maxReplaceParts {
		return errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"the page differs in %d elements, which needs %d parts and exceeds the maximum of %d; split the edit across several calls",
			len(diff.Parts), len(diff.Parts), maxReplaceParts,
		).WithParam("--content")
	}

	parts := make([]map[string]interface{}, 0, len(diff.Parts))
	for _, part := range diff.Parts {
		parts = append(parts, part.toMap())
	}
	data, err := runtime.CallAPITyped(
		"POST",
		slideReplaceAPIPath(presentationID),
		updateSlideQuery(runtime, slideID),
		map[string]interface{}{"parts": parts},
	)
	if err != nil {
		return enrichSlidesReplaceError(err)
	}

	// A failure reason means the batch was rejected and nothing was written, so
	// it cannot be reported inside a success envelope. The backend currently
	// pairs one with a non-zero code, which CallAPITyped already turns into an
	// error, so this guards an inconsistent response rather than a reachable
	// path.
	if reason := strings.TrimSpace(common.GetString(data, "failed_reason")); reason != "" {
		return errs.NewAPIError(
			errs.SubtypeInvalidParameters,
			"slide %s was not updated: %s", slideID, reason,
		).WithHint(slides3350001Hint)
	}
	if _, ok := data["revision_id"]; ok {
		result["revision_id"] = int(common.GetFloat(data, "revision_id"))
	}

	runtime.Out(result, nil)
	return nil
}

// readCurrentPage fetches the page the diff is computed against.
func readCurrentPage(runtime *common.RuntimeContext, presentationID, slideID string) (pageDoc, error) {
	data, err := runtime.CallAPITyped("GET", slideReadAPIPath(presentationID), updateSlideQuery(runtime, slideID), nil)
	if err != nil {
		return pageDoc{}, err
	}
	content := common.GetString(common.GetMap(data, "slide"), "content")
	if strings.TrimSpace(content) == "" {
		return pageDoc{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "reading slide %s returned empty content", slideID)
	}
	current, err := parsePageDoc(content)
	if err != nil {
		return pageDoc{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "slide %s returned XML the CLI cannot parse: %v", slideID, err).WithCause(err)
	}
	// A page whose structure the diff cannot see cannot be safely edited: the
	// unrecognized part would be invisible to the comparison, so the command
	// could neither preserve it deliberately nor notice the caller changing it.
	if current.Unsupported != "" {
		return pageDoc{}, errs.NewValidationError(
			errs.SubtypeFailedPrecondition,
			"slide %s contains %s, which this command cannot represent; use `slides +replace-slide` for element-level edits on this page",
			slideID, current.Unsupported,
		)
	}
	// A page carrying an <undefined> placeholder is refused outright. The
	// placeholder stands for an object the server could not export (a
	// whiteboard, unexported media); whether the whole-page rewrite behind
	// slide.replace preserves an untouched one is a server-owned behavior that
	// no self-contained test can pin down — boards cannot be created
	// programmatically. Editing on top of an unverifiable assumption risks
	// silently destroying the one object the caller cannot see, so the page is
	// off-limits to this command until preservation is provable.
	for _, el := range current.Elements {
		if el.Tag == placeholderTag {
			return pageDoc{}, errs.NewValidationError(
				errs.SubtypeFailedPrecondition,
				"slide %s contains an <undefined> placeholder (element %s) for an object the server could not export, such as a whiteboard; this command refuses to edit the page because it cannot prove a rewrite would preserve that object — use `slides +replace-slide` for element-level edits here",
				slideID, el.ID,
			)
		}
	}
	return current, nil
}

// parseWantedPage validates --content and decomposes it.
//
// The root-tag check is a safety gate, not politeness: --content describes the
// whole page, so an element-level fragment would be read as "the page should
// contain only this", and every other element on it would be deleted.
func parseWantedPage(content string) (pageDoc, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return pageDoc{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content cannot be empty").WithParam("--content")
	}
	doc, err := parsePageDoc(trimmed)
	if err != nil {
		return pageDoc{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content is not well-formed XML: %v", err).WithParam("--content").WithCause(err)
	}
	if doc.RootTag == "" {
		return pageDoc{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content has no root element; pass one full <slide>…</slide> fragment").WithParam("--content")
	}
	if doc.RootTag != "slide" {
		return pageDoc{}, updateSlideRootTagError(doc.RootTag)
	}
	if doc.TrailingTag != "" {
		return pageDoc{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--content has a <%s> element after the </slide> root; pass exactly one page per call",
			doc.TrailingTag,
		).WithParam("--content")
	}
	if doc.TrailingText {
		return pageDoc{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--content has text after the </slide> root; pass exactly one <slide>…</slide> fragment",
		).WithParam("--content")
	}
	// Anything the decomposition could not place has no part it could become.
	// Accepting it and diffing only the recognized parts would drop the edit —
	// and could even answer `unchanged` for a change the caller asked for.
	if doc.Unsupported != "" {
		return pageDoc{}, contentError(
			"--content contains %s, which this command cannot represent; a <slide> carries exactly one <style>, one <data> and one <note>",
			doc.Unsupported,
		)
	}
	return doc, nil
}

// parseWantedPageFor additionally pins the root id, when present, to the page
// being updated. XML fetched for page A and posted against --slide-id for page
// B is the classic wrong-target mistake; the element-id checks catch it only
// incidentally (an empty page, or one whose element ids were stripped, would
// sail through and rebuild B with A's content).
func parseWantedPageFor(content, slideID string) (pageDoc, error) {
	doc, err := parseWantedPage(content)
	if err != nil {
		return doc, err
	}
	if doc.RootID != "" && doc.RootID != slideID {
		return doc, contentError(
			"--content root carries id %q but --slide-id is %q; this XML looks like it was read from a different page — re-run `slides +xml-get` for this page, or drop the root id to apply the content here",
			doc.RootID, slideID,
		)
	}
	return doc, nil
}

// updateSlideRootTagError explains what to use instead, picked by what the
// caller actually passed: a whole presentation is a multi-page job, anything
// else is an element-level edit.
func updateSlideRootTagError(rootTag string) error {
	remedy := "use `slides +replace-slide` to edit individual elements"
	if rootTag == "presentation" {
		remedy = "pass a single page's <slide> XML, or use `slides +replace-pages` to rebuild several pages at once"
	}
	return errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"--content root element is <%s>, but +update-slide takes a whole page and requires a single <slide> root; %s",
		rootTag, remedy,
	).WithParam("--content")
}

// updateSlideID reads and validates --slide-id.
func updateSlideID(runtime *common.RuntimeContext) (string, error) {
	slideID := strings.TrimSpace(runtime.Str("slide-id"))
	if slideID == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide-id cannot be empty").WithParam("--slide-id")
	}
	return slideID, nil
}

// updateSlideQuery builds the query params shared by the read, the write and
// dry-run. The same revision is used for both calls so the parts are applied to
// the snapshot they were computed from.
func updateSlideQuery(runtime *common.RuntimeContext, slideID string) map[string]interface{} {
	query := map[string]interface{}{
		"slide_id":    slideID,
		"revision_id": runtime.Int("revision-id"),
	}
	if tid := strings.TrimSpace(runtime.Str("tid")); tid != "" {
		query["tid"] = tid
	}
	return query
}

// slideReadAPIPath is the single-slide read endpoint.
func slideReadAPIPath(presentationID string) string {
	return fmt.Sprintf(
		"/open-apis/slides_ai/v1/xml_presentations/%s/slide",
		validate.EncodePathSegment(presentationID),
	)
}
