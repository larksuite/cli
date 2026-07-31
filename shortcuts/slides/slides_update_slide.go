// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// slideRootTag is the only root element +update-slide accepts. Handing over a
// bare element (the frequent mistake, since +replace-slide takes those) would
// otherwise be sent as a page and wipe everything else out.
const slideRootTag = "slide"

// SlidesUpdateSlide applies one page of XML to an existing slide.
//
// It sends a single block_replace part whose block_id is the page's own id, so
// the backend replaces the whole <slide> in one shot: elements the caller kept
// stay, elements they left out are removed, an element they added appears, and
// <style> / <note> follow the XML they handed over. Callers describe the page
// they want instead of enumerating the edits that get them there.
//
// Addressing the page this way needs a backend that accepts the page's own id as
// block_id. Against one that does not, every call fails at the API, not in the
// CLI — there is no client-side fallback here on purpose: splitting a page into
// element-level parts means guessing which of the two XML normalizations, the
// caller's or the server's, counts as "unchanged".
//
// Related: `slides +replace-slide` still takes explicit element-level parts and
// remains the cheaper call when only one element changes.
var SlidesUpdateSlide = common.Shortcut{
	Service:     "slides",
	Command:     "+update-slide",
	Description: "Apply a full <slide> XML to an existing slide, replacing the page in one request (keeps slide_id and page order)",
	Risk:        "write",
	Scopes:      []string{"slides:presentation:update", "slides:presentation:write_only"},
	// wiki:node:read is required only when --presentation is a wiki URL.
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Tips: []string{
		"Read the page first with `slides +xml-get --slide-id <id>`, edit that XML, hand it back whole",
		"Anything left out of --content is removed from the page — pass the full page, not a fragment",
		"Editing one element is cheaper with `slides +replace-slide`",
	},
	Flags:    updateSlideFlags,
	Validate: updateSlideValidate,
	DryRun:   updateSlideDryRun,
	Execute:  updateSlideExecute,
}

// SlidesUpdate registers `slides +update` as a hidden alias.
//
// Agents reach for "slide update" before reading --help, and the command not
// existing cost them a turn on the error plus a help dump. Accepting the shorter
// spelling costs nothing; it stays out of --help so the canonical name is the
// only one advertised.
//
// Derived from the canonical shortcut rather than re-declared, so scopes,
// identities and flags cannot drift between the two spellings.
var SlidesUpdate = func() common.Shortcut {
	sc := SlidesUpdateSlide
	sc.Command = "+update"
	sc.Hidden = true
	return sc
}()

// contentFlagAliases are the spellings agents reach for instead of --content
// when handing a whole page of XML to +update-slide. Declared on the flag, so
// they reach only this command — other slides shortcuts have a --content of
// their own and resolving these there would rewrite a mistyped flag into one the
// caller never meant to use.
//
// Deliberately not "slide": several slides commands take a --slide-id, so
// `--slide <id>` is a likely typo for that, and resolving it to --content would
// turn the typo into a request carrying an id where page XML belongs.
var contentFlagAliases = []string{
	"xml",
	"slide-xml",
	"slide-content",
	"content-xml",
}

var updateSlideFlags = []common.Flag{
	requiredPresentationRefFlag(),
	{Name: "slide-id", Desc: "slide page identifier (slide_id) of the page to replace", Required: true},
	{Name: "content", Aliases: contentFlagAliases, Desc: "the page's full target XML, one <slide> root; elements omitted here are removed from the page", Required: true, Input: []string{common.File, common.Stdin}},
	{Name: "revision-id", Type: "int", Default: "-1", Desc: "revision to apply against; -1 (default) means latest. Pinning an older revision rebuilds the page from that snapshot and discards newer edits to it"},
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
	_, err = updateSlideContent(runtime, slideID)
	return err
}

func updateSlideDryRun(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	fail := func(err error) *common.DryRunAPI {
		return common.NewDryRunAPI().Set("error", err.Error())
	}
	ref, err := parsePresentationRef(runtime.Str("presentation"))
	if err != nil {
		return fail(err)
	}
	slideID, err := updateSlideID(runtime)
	if err != nil {
		return fail(err)
	}
	content, err := updateSlideContent(runtime, slideID)
	if err != nil {
		return fail(err)
	}

	dry := common.NewDryRunAPI()
	presentationID := ref.Token
	if ref.Kind == "wiki" {
		presentationID = "<resolved_slides_token>"
		dry.Desc("2-step orchestration: resolve wiki → replace slide").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to slides presentation").
			Params(map[string]interface{}{"token": ref.Token})
	} else {
		dry.Desc(fmt.Sprintf("Replace slide %s with the supplied page XML", slideID))
	}
	dry.POST(slideReplaceAPIPath(presentationID)).
		Params(updateSlideQuery(runtime, slideID)).
		Body(map[string]interface{}{"parts": updateSlideParts(slideID, content)})
	return dry.Set("slide_id", slideID).Set("content_bytes", len(content))
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
	content, err := updateSlideContent(runtime, slideID)
	if err != nil {
		return err
	}

	data, err := runtime.CallAPITyped("POST", slideReplaceAPIPath(presentationID),
		updateSlideQuery(runtime, slideID),
		map[string]interface{}{"parts": updateSlideParts(slideID, content)})
	if err != nil {
		return enrichUpdateSlideError(err)
	}

	// A single part carries the whole page, so any failed_reason means the page
	// was not written. Reporting that as a success envelope would tell the caller
	// their edit landed when it did not.
	if reason, ok := data["failed_reason"].(string); ok && strings.TrimSpace(reason) != "" {
		return errs.NewAPIError(errs.SubtypeInvalidParameters,
			"slide %s was not updated: %s", slideID, reason).
			WithHint(updateSlideInvalidParamHint)
	}

	result := map[string]interface{}{
		"xml_presentation_id": presentationID,
		"slide_id":            slideID,
	}
	if v, ok := data["revision_id"]; ok {
		result["revision_id"] = v
	}
	runtime.Out(result, nil)
	return nil
}

// updateSlideInvalidParamHint replaces the generic +replace-slide checklist for
// this command. That checklist opens with "block_id not found in current slide —
// re-run slide.get", which is the wrong move here: block_id is the page's own id,
// so it is never missing, and re-fetching would just loop.
//
// The hint names what the caller can act on and does not claim which cause it
// was: 3350001 covers both a rejected page id and bad XML, and a hint asserting
// the former goes stale the moment whole-page update is generally available.
const updateSlideInvalidParamHint = "check --content first: an unsupported element, a <shape> without" +
	" <content/>, or coordinates outside 960x540. Re-fetching the page will not help — block_id is the" +
	" page's own id, so it is never missing. If --content is known good, this backend may not accept a" +
	" whole-page update yet; edit the elements individually with `slides +replace-slide` instead"

// enrichUpdateSlideError attaches updateSlideInvalidParamHint on 3350001, leaving
// any more specific upstream hint in place. Mirrors enrichSlidesReplaceError.
func enrichUpdateSlideError(err error) error {
	p, ok := errs.ProblemOf(err)
	if !ok || p.Code != larkCodeSlidesInvalidParam {
		return err
	}
	if p.Hint == "" {
		p.Hint = updateSlideInvalidParamHint
	}
	return err
}

func updateSlideID(runtime *common.RuntimeContext) (string, error) {
	slideID := strings.TrimSpace(runtime.Str("slide-id"))
	if slideID == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide-id cannot be empty").WithParam("--slide-id")
	}
	return slideID, nil
}

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

// updateSlideParts builds the one part the command ever sends. block_id is the
// page id, which is what makes the backend swap the whole <slide> out.
func updateSlideParts(slideID, content string) []map[string]interface{} {
	return []map[string]interface{}{{
		"action":      "block_replace",
		"block_id":    slideID,
		"replacement": content,
	}}
}

// slideReplaceAPIPath is shared with +replace-slide: both commands drive the same
// endpoint, and a single helper keeps them from drifting apart.
func slideReplaceAPIPath(presentationID string) string {
	return fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s/slide/replace",
		validate.EncodePathSegment(presentationID))
}

// updateSlideContent validates --content and returns it with the root id set to
// slideID. The caller's own bytes are preserved apart from that one attribute:
// their formatting ends up in the page.
func updateSlideContent(runtime *common.RuntimeContext, slideID string) (string, error) {
	content := strings.TrimSpace(runtime.Str("content"))
	if content == "" {
		return "", contentError("--content cannot be empty")
	}
	rootID, err := checkSlideRoot(content)
	if err != nil {
		return "", err
	}
	// A root id naming a different page is the signature of XML read from page A
	// about to be written over page B. Overriding it silently would destroy the
	// wrong page, so refuse instead.
	if rootID != "" && rootID != slideID {
		return "", contentError(
			"--content root is <slide id=%q> but --slide-id is %q; pass the page you mean to replace, or drop the id",
			rootID, slideID)
	}
	stamped, err := ensureXMLRootID(content, slideID)
	if err != nil {
		// checkSlideRoot already proved there is a single <slide> root, so the only
		// way the stamper can fail is a root spelling it cannot rewrite. Say which
		// one rather than passing on "no root element found in XML fragment".
		return "", contentError("--content root <slide> is written in a form the page id cannot be attached to" +
			" (a namespace prefix such as <sml:slide> does this); write it as <slide> and declare the namespace" +
			" with a default xmlns if you need one").WithCause(err)
	}
	return stamped, nil
}

// checkSlideRoot walks the tokens of content and returns the root element's id
// attribute (empty when absent). It rejects anything that is not exactly one
// <slide> element: a bare element root, trailing elements or trailing text after
// the root close, which would otherwise be dropped without a word by the
// server's parser.
func checkSlideRoot(content string) (string, error) {
	decoder := xml.NewDecoder(strings.NewReader(content))
	depth := 0
	seenRoot := false
	rootID := ""
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", contentError("--content is not well-formed XML: %s", err).WithCause(err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				if seenRoot {
					return "", contentError("--content must hold a single <%s> element; found a second root <%s>", slideRootTag, t.Name.Local)
				}
				if t.Name.Local != slideRootTag {
					return "", contentError("--content root must be <%s>, got <%s>. To edit one element use `slides +replace-slide`", slideRootTag, t.Name.Local)
				}
				seenRoot = true
				for _, attr := range t.Attr {
					if attr.Name.Local == "id" && attr.Name.Space == "" {
						rootID = attr.Value
					}
				}
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return "", contentError("--content has text outside the <%s> element: %q", slideRootTag, strings.TrimSpace(string(t)))
			}
		}
	}
	if !seenRoot {
		return "", contentError("--content has no <%s> element", slideRootTag)
	}
	return rootID, nil
}

// contentError returns the concrete type so callers can chain .WithCause when
// they have an underlying error to preserve.
func contentError(format string, args ...interface{}) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...).WithParam("--content")
}
