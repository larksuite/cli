// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
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

// DriveListComments lists comments on a docx with smart defaults: filter to
// unresolved + non-orphan anchors, ordered by anchor position, including
// replies and reactions. See https://github.com/larksuite/cli/issues/1111.
var DriveListComments = common.Shortcut{
	Service:           "drive",
	Command:           "+list-comments",
	Description:       "List docx comments with smart defaults (unresolved + non-orphan + anchor-ordered + replies + reactions); wiki URLs are auto-unwrapped",
	Risk:              "read",
	Scopes:            []string{"docs:document.comment:read", "docx:document:readonly"},
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
		"file_type":     "docx",
		"page_size":     listCommentsPageSize,
		"need_relation": true,
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
		Desc(fmt.Sprintf("[%d] Fetch document XML with block IDs for anchor ordering", stepCount)).
		Body(buildListCommentsFetchBody())

	dry.Desc(fmt.Sprintf("%d-step orchestration: list comments with relation + fetch block IDs + order anchors", stepCount))
	return dry
}

// commentItem holds a single comment card plus our derived anchor metadata.
type commentItem struct {
	Raw              map[string]interface{}
	CommentID        string
	Quote            string
	IsWhole          bool
	IsSolved         bool
	CreateTime       int64
	AnchorState      string // "valid" | "structural" | "orphaned"
	AnchorPosition   int64  // block order in docs fetch XML; -1 when unknown/orphaned
	AnchorBlockID    string
	LocationAccuracy string // relation_exact | parent_resource_exact | weak_inferred | content_deleted
	ParentType       string
	ParentToken      string
	ContentDeleted   bool
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

	// Step 3: Fetch document XML with block IDs for stable anchor ordering.
	docXML, err := fetchDocxXML(runtime, fileToken)
	if err != nil {
		return err
	}
	blockIndex := buildDocBlockIndex(docXML)

	// Step 4: Build commentItem list with relation-derived location metadata.
	items := make([]commentItem, 0, len(allRaw))
	for _, r := range allRaw {
		items = append(items, buildCommentItem(r, blockIndex))
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
		m["location_accuracy"] = it.LocationAccuracy
		m["content_deleted"] = it.ContentDeleted
		if it.AnchorBlockID != "" {
			m["anchor_block_id"] = it.AnchorBlockID
		}
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
			"file_type":     "docx",
			"page_size":     listCommentsPageSize,
			"need_relation": true,
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

// fetchDocxXML returns document XML with block IDs exported for comment anchor
// ordering. The comment API supplies the anchor block ID via need_relation; the
// docs fetch call supplies the current document block order.
func fetchDocxXML(runtime *common.RuntimeContext, fileToken string) (string, error) {
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch",
		validate.EncodePathSegment(fileToken))
	body := buildListCommentsFetchBody()
	data, err := runtime.CallAPI("POST", apiPath, nil, body)
	if err != nil {
		return "", err
	}
	doc := common.GetMap(data, "document")
	return common.GetString(doc, "content"), nil
}

func buildListCommentsFetchBody() map[string]interface{} {
	return map[string]interface{}{
		"format": "xml",
		"export_option": map[string]interface{}{
			"export_block_id": true,
		},
	}
}

type docBlockIndex struct {
	orderByBlockID map[string]int64
	blockByToken   map[string]string
}

func buildDocBlockIndex(content string) docBlockIndex {
	idx := docBlockIndex{
		orderByBlockID: map[string]int64{},
		blockByToken:   map[string]string{},
	}
	decoder := xml.NewDecoder(strings.NewReader(content))
	var order int64
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		blockID := firstXMLAttr(start, "id", "block_id")
		if blockID == "" {
			continue
		}
		if _, exists := idx.orderByBlockID[blockID]; !exists {
			idx.orderByBlockID[blockID] = order
			order++
		}
		for _, attr := range start.Attr {
			if attr.Value == "" {
				continue
			}
			switch attr.Name.Local {
			case "token", "spreadsheet_token", "base_token", "app_token", "whiteboard_token":
				idx.blockByToken[attr.Value] = blockID
			}
		}
	}
	return idx
}

func firstXMLAttr(start xml.StartElement, names ...string) string {
	for _, want := range names {
		for _, attr := range start.Attr {
			if attr.Name.Local == want {
				return attr.Value
			}
		}
	}
	return ""
}

func (idx docBlockIndex) orderOfBlock(blockID string) int64 {
	if blockID == "" {
		return -1
	}
	order, ok := idx.orderByBlockID[blockID]
	if !ok {
		return -1
	}
	return order
}

func (idx docBlockIndex) lookupEmbeddedToken(parentToken string) (string, int64, bool) {
	for _, candidate := range parentTokenCandidates(parentToken) {
		if blockID := idx.blockByToken[candidate]; blockID != "" {
			return blockID, idx.orderOfBlock(blockID), true
		}
	}
	return "", -1, false
}

func parentTokenCandidates(parentToken string) []string {
	parentToken = strings.TrimSpace(parentToken)
	if parentToken == "" {
		return nil
	}
	candidates := []string{parentToken}
	if idx := strings.LastIndex(parentToken, "_tbl"); idx > 0 {
		candidates = append(candidates, parentToken[:idx])
	}
	if idx := strings.LastIndex(parentToken, "_"); idx > 0 {
		candidates = append(candidates, parentToken[:idx])
	}
	if idx := strings.Index(parentToken, "_"); idx > 0 {
		candidates = append(candidates, parentToken[:idx])
	}
	return candidates
}

// extractCommentRelation parses the relation payload returned by
// need_relation=true. The nested relation.relation value is itself a JSON
// string, keyed by server object identity; positionInfo.blockID is the stable
// docx block anchor we need.
func extractCommentRelation(raw map[string]interface{}) (blockID string, contentDeleted bool, ok bool) {
	relation := common.GetMap(raw, "relation")
	if relation == nil {
		return "", false, false
	}
	contentDeleted = common.GetBool(relation, "content_deleted")
	relJSON := strings.TrimSpace(common.GetString(relation, "relation"))
	if relJSON == "" {
		return "", contentDeleted, false
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(relJSON), &parsed); err != nil {
		return "", contentDeleted, false
	}
	blockID = findRelationBlockID(parsed)
	return blockID, contentDeleted, blockID != ""
}

func findRelationBlockID(v interface{}) string {
	switch node := v.(type) {
	case map[string]interface{}:
		if pos, ok := node["positionInfo"].(map[string]interface{}); ok {
			if blockID, _ := pos["blockID"].(string); blockID != "" {
				return blockID
			}
			if blockID, _ := pos["block_id"].(string); blockID != "" {
				return blockID
			}
		}
		for _, child := range node {
			if blockID := findRelationBlockID(child); blockID != "" {
				return blockID
			}
		}
	case []interface{}:
		for _, child := range node {
			if blockID := findRelationBlockID(child); blockID != "" {
				return blockID
			}
		}
	}
	return ""
}

// buildCommentItem parses an API comment card and computes relation-derived
// anchor_state + anchor_position. Position is current document block order, not
// a byte offset, and -1 means the location is unknown or deleted.
func buildCommentItem(raw map[string]interface{}, idx docBlockIndex) commentItem {
	quote, _ := raw["quote"].(string)
	isWhole, _ := raw["is_whole"].(bool)
	isSolved, _ := raw["is_solved"].(bool)
	commentID, _ := raw["comment_id"].(string)
	parentType, _ := raw["parent_type"].(string)
	parentToken, _ := raw["parent_token"].(string)
	var createTime int64
	if v, ok := raw["create_time"].(float64); ok {
		createTime = int64(v)
	}

	item := commentItem{
		Raw:              raw,
		CommentID:        commentID,
		Quote:            quote,
		IsWhole:          isWhole,
		IsSolved:         isSolved,
		CreateTime:       createTime,
		AnchorPosition:   -1,
		LocationAccuracy: "weak_inferred",
		ParentType:       parentType,
		ParentToken:      parentToken,
	}

	blockID, deleted, hasRelation := extractCommentRelation(raw)
	item.ContentDeleted = deleted
	if deleted {
		item.AnchorState = "orphaned"
		item.LocationAccuracy = "content_deleted"
		item.AnchorBlockID = blockID
		return item
	}
	if hasRelation {
		item.AnchorBlockID = blockID
		if order := idx.orderOfBlock(blockID); order >= 0 {
			if parentType != "" {
				item.AnchorState = "structural"
				item.LocationAccuracy = "parent_resource_exact"
			} else {
				item.AnchorState = "valid"
				item.LocationAccuracy = "relation_exact"
			}
			item.AnchorPosition = order
			return item
		}
	}
	if parentType != "" && parentToken != "" {
		if blockID, order, ok := idx.lookupEmbeddedToken(parentToken); ok {
			item.AnchorState = "structural"
			item.AnchorBlockID = blockID
			item.AnchorPosition = order
			item.LocationAccuracy = "parent_resource_exact"
			return item
		}
	}
	if hasRelation {
		item.AnchorState = "valid"
		item.LocationAccuracy = "weak_inferred"
		return item
	}
	if isWhole {
		item.AnchorState = "valid"
		item.AnchorPosition = 0
		item.LocationAccuracy = "whole_document"
		return item
	}
	item.AnchorState = "valid"
	return item
}

// sortCommentItems sorts items in-place. Orphans always sort to the end.
// Within the non-orphan group:
//   - order=anchor (default): by AnchorPosition ascending, unknown locations after anchored comments
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
		aUnknown := a.AnchorPosition < 0
		bUnknown := b.AnchorPosition < 0
		if aUnknown != bUnknown {
			return !aUnknown
		}
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
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
