// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"html"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// listCommentsPageSize is the page size used when listing file comments.
const listCommentsPageSize = 100

// listCommentsMaxPages caps pagination depth to prevent runaway loops on
// documents with very large comment histories.
const listCommentsMaxPages = 100

// stickyNoteQuote is the placeholder quote text the comments API returns when
// a comment is anchored to a sticky-note element (XML element type
// <readonly-block>). The actual sticky-note content is not exposed via the
// comments list API, so orphan detection falls back to "does the document
// still contain any readonly-block element?".
const stickyNoteQuote = "[Sticky note]"

// structuralAnchorTag is the XML prefix used to detect the existence of any
// sticky-note element when validating a [Sticky note] comment.
const structuralAnchorTag = "<readonly-block"

// xmlTagPattern matches any XML/HTML tag for tag stripping during orphan
// detection.
var xmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// whitespacePattern matches one or more whitespace characters (incl.
// newlines) for normalization to a single space.
var whitespacePattern = regexp.MustCompile(`\s+`)

// DriveListComments lists comments on a docx with smart defaults: filter to
// unresolved + non-orphan anchors, ordered by anchor position, including
// replies and reactions. See https://github.com/larksuite/cli/issues/1111.
var DriveListComments = common.Shortcut{
	Service:           "drive",
	Command:           "+list-comments",
	Description:       "List docx comments with smart defaults (unresolved + non-orphan + anchor-ordered + replies + reactions); wiki URLs are auto-unwrapped",
	Risk:              "read",
	Scopes:            []string{"docs:document.comment:read"},
	ConditionalScopes: []string{"wiki:node:retrieve"},
	AuthTypes:         []string{"user", "bot"},
	HasFormat:         true,
	Flags: []common.Flag{
		{
			Name:     "doc",
			Desc:     "document URL or token (docx URL, bare token + --type=docx, or wiki URL resolving to docx)",
			Required: true,
		},
		{
			Name: "type",
			Desc: "document type (required when --doc is a bare token; MVP supports docx only)",
			Enum: []string{"docx", "wiki"},
		},
		{
			Name: "include-orphaned",
			Type: "bool",
			Desc: "include comments whose anchor text was deleted/rewritten (default: hide, matching the Lark UI)",
		},
		{
			Name: "include-resolved",
			Type: "bool",
			Desc: "include resolved comments (default: hide, i.e. is_solved=false only)",
		},
		{
			Name: "no-reactions",
			Type: "bool",
			Desc: "do not include reaction details on replies (default: reactions are fetched)",
		},
		{
			Name:    "order",
			Desc:    "result order: anchor (by position in document, default) | created (by create_time)",
			Default: "anchor",
			Enum:    []string{"anchor", "created"},
		},
	},
	Validate: validateListComments,
	DryRun:   dryRunListComments,
	Execute:  executeListComments,
}

// validateListComments enforces docx-only scope for MVP and checks that --doc
// is either a recognized URL or a bare token paired with --type.
func validateListComments(_ context.Context, runtime *common.RuntimeContext) error {
	raw := strings.TrimSpace(runtime.Str("doc"))
	if raw == "" {
		return output.ErrValidation("--doc cannot be empty")
	}
	ref, ok := parseListCommentsDocRef(raw, runtime.Str("type"))
	if !ok {
		if strings.Contains(raw, "://") {
			return output.ErrValidation("unsupported --doc input %q: use a docx URL, a bare token with --type=docx, or a wiki URL", raw)
		}
		if strings.TrimSpace(runtime.Str("type")) == "" {
			return output.ErrValidation("--type is required when --doc is a bare token (allowed: docx, wiki)")
		}
		return output.ErrValidation("--type %q is not supported by drive +list-comments (MVP supports docx and wiki resolving to docx)", runtime.Str("type"))
	}
	if ref.Type != "docx" && ref.Type != "wiki" {
		return output.ErrValidation("--doc must resolve to docx (got %q); sheet/slides/file/doc are not supported by drive +list-comments yet", ref.Type)
	}
	if v := strings.TrimSpace(runtime.Str("order")); v != "" && v != "anchor" && v != "created" {
		return output.ErrValidation("--order must be one of: anchor, created (got %q)", v)
	}
	return nil
}

// parseListCommentsDocRef recognizes a docx or wiki URL, or a bare token
// combined with --type. Returns (ref, true) on success.
func parseListCommentsDocRef(raw, docType string) (common.ResourceRef, bool) {
	ref, ok := common.ParseResourceURL(raw)
	if ok {
		return ref, true
	}
	if strings.Contains(raw, "://") {
		return common.ResourceRef{}, false
	}
	t := strings.TrimSpace(docType)
	if t == "" {
		return common.ResourceRef{}, false
	}
	if t != "docx" && t != "wiki" {
		return common.ResourceRef{}, false
	}
	return common.ResourceRef{Type: t, Token: raw}, true
}

func dryRunListComments(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	raw := strings.TrimSpace(runtime.Str("doc"))
	ref, _ := parseListCommentsDocRef(raw, runtime.Str("type"))
	isWiki := ref.Type == "wiki"

	dry := common.NewDryRunAPI()
	stepCount := 0

	if isWiki {
		stepCount++
		dry.GET("/open-apis/wiki/v2/spaces/get_node").
			Desc(fmt.Sprintf("[%d] Resolve wiki node to underlying docx", stepCount)).
			Params(map[string]interface{}{"token": ref.Token})
	}

	listToken := ref.Token
	if isWiki {
		listToken = "<obj_token from previous step>"
	}
	stepCount++
	listParams := map[string]interface{}{
		"file_type": "docx",
		"page_size": listCommentsPageSize,
	}
	if !runtime.Bool("include-resolved") {
		listParams["is_solved"] = false
	}
	if !runtime.Bool("no-reactions") {
		listParams["need_reaction"] = true
	}
	dry.GET(fmt.Sprintf("/open-apis/drive/v1/files/%s/comments", listToken)).
		Desc(fmt.Sprintf("[%d] List comments (paginated until exhausted)", stepCount)).
		Params(listParams)

	stepCount++
	dry.POST(fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", listToken)).
		Desc(fmt.Sprintf("[%d] Fetch document XML for orphan detection", stepCount)).
		Body(map[string]interface{}{"format": "xml"})

	dry.Desc(fmt.Sprintf("%d-step orchestration: list comments + fetch document + cross-check anchors", stepCount))
	return dry
}

// commentItem holds a single comment card plus our derived anchor metadata.
type commentItem struct {
	Raw            map[string]interface{}
	CommentID      string
	Quote          string
	IsWhole        bool
	IsSolved       bool
	CreateTime     int64
	AnchorState    string // "valid" | "structural" | "orphaned"
	AnchorPosition int64  // byte offset within normalized doc text; -1 for orphan
}

func executeListComments(_ context.Context, runtime *common.RuntimeContext) error {
	raw := strings.TrimSpace(runtime.Str("doc"))
	ref, ok := parseListCommentsDocRef(raw, runtime.Str("type"))
	if !ok {
		// Validate should have caught this; keep guard for safety.
		return output.ErrValidation("unable to parse --doc input %q", raw)
	}

	// Step 1: Resolve wiki to docx token if needed.
	fileToken := ref.Token
	var wikiToken string
	if ref.Type == "wiki" {
		wikiToken = ref.Token
		fmt.Fprintf(runtime.IO().ErrOut, "Resolving wiki node: %s\n", common.MaskToken(wikiToken))
		data, err := runtime.CallAPI(
			"GET",
			"/open-apis/wiki/v2/spaces/get_node",
			map[string]interface{}{"token": wikiToken},
			nil,
		)
		if err != nil {
			return err
		}
		node := common.GetMap(data, "node")
		objType := common.GetString(node, "obj_type")
		objToken := common.GetString(node, "obj_token")
		if objType != "docx" {
			return output.ErrValidation("wiki resolved to %q; drive +list-comments only supports docx (MVP)", objType)
		}
		if objToken == "" {
			return output.Errorf(output.ExitAPI, "api_error", "wiki get_node returned empty obj_token")
		}
		fileToken = objToken
		fmt.Fprintf(runtime.IO().ErrOut, "Wiki resolved to docx: %s\n", common.MaskToken(fileToken))
	}

	// Step 2: Paginate through all comments.
	includeResolved := runtime.Bool("include-resolved")
	needReactions := !runtime.Bool("no-reactions")
	allRaw, err := listAllComments(runtime, fileToken, includeResolved, needReactions)
	if err != nil {
		return err
	}

	// Step 3: Fetch document XML for orphan detection.
	docXML, err := fetchDocxXML(runtime, fileToken)
	if err != nil {
		return err
	}
	normalized := normalizeDocContent(docXML)
	hasStructuralAnchor := strings.Contains(docXML, structuralAnchorTag)
	// Approximate position of the FIRST sticky-note anchor in normalized space.
	// All sticky-anchored comments share this position for sorting purposes.
	structuralPos := projectRawPosToNormalized(docXML, normalized, structuralAnchorTag)

	// Step 4: Build commentItem list with anchor_state + anchor_position.
	items := make([]commentItem, 0, len(allRaw))
	for _, r := range allRaw {
		items = append(items, buildCommentItem(r, normalized, hasStructuralAnchor, structuralPos))
	}

	// Step 5: Filter by include-orphaned.
	includeOrphaned := runtime.Bool("include-orphaned")
	filtered := items
	if !includeOrphaned {
		filtered = make([]commentItem, 0, len(items))
		for _, it := range items {
			if it.AnchorState == "orphaned" {
				continue
			}
			filtered = append(filtered, it)
		}
	}

	// Step 6: Sort.
	orderKey := strings.TrimSpace(runtime.Str("order"))
	if orderKey == "" {
		orderKey = "anchor"
	}
	sortCommentItems(filtered, orderKey)

	// Step 7: Build output envelope.
	outItems := make([]map[string]interface{}, 0, len(filtered))
	for _, it := range filtered {
		m := cloneCommentMap(it.Raw)
		m["anchor_state"] = it.AnchorState
		m["anchor_position"] = it.AnchorPosition
		outItems = append(outItems, m)
	}

	counts := countByAnchorState(items)
	result := map[string]interface{}{
		"items":      outItems,
		"file_token": fileToken,
		"counts":     counts,
	}
	if wikiToken != "" {
		result["wiki_token"] = wikiToken
	}

	fmt.Fprintf(runtime.IO().ErrOut,
		"Comments: %d total (valid=%d, structural=%d, orphaned=%d); returned %d\n",
		counts["total"], counts["valid"], counts["structural"], counts["orphaned"], len(filtered))

	runtime.OutFormatRaw(result, nil, func(w io.Writer) {
		for i, it := range filtered {
			fmt.Fprintf(w, "[%d] %s (%s, pos=%d) %s\n",
				i+1, it.CommentID, it.AnchorState, it.AnchorPosition, truncateQuote(it.Quote, 60))
		}
	})
	return nil
}

// listAllComments paginates through all comment cards on a docx, returning
// the raw maps from the API. Pagination is bounded by listCommentsMaxPages.
func listAllComments(runtime *common.RuntimeContext, fileToken string, includeResolved, needReactions bool) ([]map[string]interface{}, error) {
	encodedToken := validate.EncodePathSegment(fileToken)
	apiPath := fmt.Sprintf("/open-apis/drive/v1/files/%s/comments", encodedToken)
	var pageToken string
	var all []map[string]interface{}
	for page := 0; page < listCommentsMaxPages; page++ {
		params := map[string]interface{}{
			"file_type": "docx",
			"page_size": listCommentsPageSize,
		}
		if !includeResolved {
			params["is_solved"] = false
		}
		if needReactions {
			params["need_reaction"] = true
		}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		data, err := runtime.CallAPI("GET", apiPath, params, nil)
		if err != nil {
			return nil, err
		}
		if rawItems, ok := data["items"].([]interface{}); ok {
			for _, it := range rawItems {
				if m, ok := it.(map[string]interface{}); ok {
					all = append(all, m)
				}
			}
		}
		hasMore, _ := data["has_more"].(bool)
		if !hasMore {
			return all, nil
		}
		next, _ := data["page_token"].(string)
		if next == "" || next == pageToken {
			return all, nil
		}
		pageToken = next
	}
	return all, nil
}

// fetchDocxXML returns the document content as XML for orphan detection.
// `text` format was removed from the docs +fetch enum (see PR #1109), so we
// use `xml` and strip tags client-side. Markdown would also work but XML is
// preferred because it preserves structural elements like <readonly-block>.
func fetchDocxXML(runtime *common.RuntimeContext, fileToken string) (string, error) {
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch",
		validate.EncodePathSegment(fileToken))
	body := map[string]interface{}{"format": "xml"}
	data, err := runtime.CallAPI("POST", apiPath, nil, body)
	if err != nil {
		return "", err
	}
	doc := common.GetMap(data, "document")
	return common.GetString(doc, "content"), nil
}

// normalizeDocContent prepares a haystack for substring matching by stripping
// XML tags, decoding HTML entities, and removing ALL whitespace.
//
// Whitespace is removed (not just collapsed) so that text broken by inline
// tags matches correctly. Example:
//
//	"<p>由 <b>P2 / P3 事件聚类</b>触发：…</p>"
//
// becomes "由P2/P3事件聚类触发：…" — the quote "由 P2 / P3 事件聚类触发：" then
// matches after the same normalization is applied to the needle.
func normalizeDocContent(content string) string {
	stripped := xmlTagPattern.ReplaceAllString(content, "")
	decoded := html.UnescapeString(stripped)
	return whitespacePattern.ReplaceAllString(decoded, "")
}

// normalizeQuoteNeedle prepares the first line of a comment quote for
// substring matching against a normalized document. Whitespace and HTML
// entities are stripped to keep the same coordinate system as
// normalizeDocContent.
func normalizeQuoteNeedle(quote string) string {
	firstLine := strings.TrimSpace(strings.SplitN(quote, "\n", 2)[0])
	if firstLine == "" {
		return ""
	}
	decoded := html.UnescapeString(firstLine)
	return whitespacePattern.ReplaceAllString(decoded, "")
}

// projectRawPosToNormalized returns the position of `marker` in `raw`
// mapped onto the normalized coordinate system by re-normalizing the prefix
// that precedes the marker. Returns -1 if the marker is not found.
//
// This is exact rather than a linear projection: it spends one extra
// normalize pass on the prefix to get the true position of the marker in
// normalized space, which keeps structural anchors (sticky notes etc.)
// correctly ordered relative to text anchors.
func projectRawPosToNormalized(raw, _ string, marker string) int64 {
	idx := strings.Index(raw, marker)
	if idx < 0 {
		return -1
	}
	return int64(len(normalizeDocContent(raw[:idx])))
}

// buildCommentItem parses an API comment card and computes anchor_state +
// anchor_position. The position is a byte offset in normalized doc text.
//
// Known limitations:
//   - Short quote text that appears multiple times in the doc cannot be
//     uniquely localized; the first occurrence wins. This can yield
//     false-positive "valid" classification when the specific anchor
//     instance has been deleted but the text still appears elsewhere.
//   - Quotes broken by XML tags (e.g. "<b>...</b>" wrapping a substring)
//     are matched after tag stripping; partial wrapping is handled.
func buildCommentItem(raw map[string]interface{}, normalized string, hasStructuralAnchor bool, structuralPos int64) commentItem {
	quote, _ := raw["quote"].(string)
	isWhole, _ := raw["is_whole"].(bool)
	isSolved, _ := raw["is_solved"].(bool)
	commentID, _ := raw["comment_id"].(string)
	var createTime int64
	if v, ok := raw["create_time"].(float64); ok {
		createTime = int64(v)
	}

	item := commentItem{
		Raw:        raw,
		CommentID:  commentID,
		Quote:      quote,
		IsWhole:    isWhole,
		IsSolved:   isSolved,
		CreateTime: createTime,
	}

	switch {
	case isWhole:
		item.AnchorState = "valid"
		item.AnchorPosition = 0
	case quote == stickyNoteQuote:
		if hasStructuralAnchor {
			item.AnchorState = "structural"
			if structuralPos >= 0 {
				item.AnchorPosition = structuralPos
			} else {
				item.AnchorPosition = 0
			}
		} else {
			item.AnchorState = "orphaned"
			item.AnchorPosition = -1
		}
	default:
		needle := normalizeQuoteNeedle(quote)
		if needle == "" {
			item.AnchorState = "orphaned"
			item.AnchorPosition = -1
			break
		}
		pos := strings.Index(normalized, needle)
		if pos >= 0 {
			item.AnchorState = "valid"
			item.AnchorPosition = int64(pos)
		} else {
			item.AnchorState = "orphaned"
			item.AnchorPosition = -1
		}
	}
	return item
}

// sortCommentItems sorts items in-place. Orphans always sort to the end.
// Within the non-orphan group:
//   - order=anchor (default): by AnchorPosition ascending
//   - order=created: by CreateTime ascending
func sortCommentItems(items []commentItem, orderKey string) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		aOrphan := a.AnchorState == "orphaned"
		bOrphan := b.AnchorState == "orphaned"
		if aOrphan != bOrphan {
			return !aOrphan
		}
		if orderKey == "created" {
			return a.CreateTime < b.CreateTime
		}
		// anchor order
		if a.AnchorPosition != b.AnchorPosition {
			return a.AnchorPosition < b.AnchorPosition
		}
		// stable tiebreaker
		return a.CreateTime < b.CreateTime
	})
}

func countByAnchorState(items []commentItem) map[string]int {
	out := map[string]int{
		"total":      len(items),
		"valid":      0,
		"structural": 0,
		"orphaned":   0,
	}
	for _, it := range items {
		out[it.AnchorState]++
	}
	return out
}

func cloneCommentMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func truncateQuote(s string, n int) string {
	s = whitespacePattern.ReplaceAllString(s, " ")
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
