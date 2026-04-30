// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const defaultLocateDocLimit = 10

// maxCommentTotalRunes is the cap on the combined character (rune) count
// across all `reply_elements[].text` fields in a single
// `drive +add-comment` request.
//
// The open-platform `/open-apis/drive/v1/files/{token}/new_comments`
// endpoint returns an opaque `[1069302] Invalid or missing parameters`
// when this is exceeded — no indication that length is the cause or
// which element is at fault.
//
// Empirically (probing the live API):
//
//   - 10000 runes in a single text element: OK (10000 ASCII / 30000
//     bytes for Chinese / 40000 bytes if all '<' — server counts the
//     raw rune count, not byte width and not the post-escape form)
//   - 10001 runes in a single text element: [1069302]
//   - 5000 + 5000 across two elements (total 10000): OK
//   - 5000 + 5001 across two elements (total 10001): [1069302]
//
// So the cap is applied to the *total* across all reply_elements, not
// per element. Splitting an over-the-cap message into multiple text
// elements does NOT help — the server enforces the same limit on the
// sum.
//
// The schema doc currently advertises a 1-1000 character limit, but
// the live API accepts up to 10000 runes; the schema is out of date.
// If this constant ever needs to track a server-side change, re-probe
// with `drive file.comments create_v2` against a fresh docx.
const maxCommentTotalRunes = 10000

type commentDocRef struct {
	Kind  string
	Token string
}

type resolvedCommentTarget struct {
	DocID      string
	FileToken  string
	FileType   string
	ResolvedBy string
	WikiToken  string
}

type locateDocBlock struct {
	BlockID     string
	RawMarkdown string
}

type locateDocMatch struct {
	AnchorBlockID string
	ParentBlockID string
	Blocks        []locateDocBlock
}

type locateDocResult struct {
	MatchCount int
	Matches    []locateDocMatch
}

type commentReplyElementInput struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	MentionUser string `json:"mention_user"`
	Link        string `json:"link"`
}

type commentMode string

const (
	commentModeLocal commentMode = "local"
	commentModeFull  commentMode = "full"
)

var DriveAddComment = common.Shortcut{
	Service:     "drive",
	Command:     "+add-comment",
	Description: "Add a comment to doc/docx/sheet/slides, also supports wiki URL resolving to doc/docx/sheet/slides",
	Risk:        "write",
	Scopes: []string{
		"docx:document:readonly",
		"docs:document.comment:create",
		"docs:document.comment:write_only",
	},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "doc", Desc: "document URL/token, sheet/slides URL, or wiki URL that resolves to doc/docx/sheet/slides", Required: true},
		{Name: "type", Desc: "document type: doc, docx, sheet, slides (required when --doc is a bare token; auto-detected for URLs)", Enum: []string{"doc", "docx", "sheet", "slides"}},
		{Name: "content", Desc: "reply_elements JSON string", Required: true},
		{Name: "full-comment", Type: "bool", Desc: "create a full-document comment; also the default when no location is provided"},
		{Name: "selection-with-ellipsis", Desc: "target content locator (plain text or 'start...end')"},
		{Name: "block-id", Desc: "for docx: anchor block ID; for sheet: <sheetId>!<cell> (e.g. a281f9!D6); for slides: <slide-block-type>!<xml-id> (e.g. shape!bPq)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		docRef, err := parseCommentDocRef(runtime.Str("doc"), runtime.Str("type"))
		if err != nil {
			return err
		}

		if _, err := parseCommentReplyElements(runtime.Str("content")); err != nil {
			return err
		}

		// Sheet comment validation.
		if docRef.Kind == "sheet" {
			blockID := strings.TrimSpace(runtime.Str("block-id"))
			if blockID == "" {
				return output.ErrValidation("--block-id is required for sheet comments (format: <sheetId>!<cell>, e.g. a281f9!D6)")
			}
			if _, err := parseSheetCellRef(blockID); err != nil {
				return err
			}
			if runtime.Bool("full-comment") || strings.TrimSpace(runtime.Str("selection-with-ellipsis")) != "" {
				return output.ErrValidation("--full-comment and --selection-with-ellipsis are not applicable for sheet comments; use --block-id with <sheetId>!<cell> format")
			}
			return nil
		}
		if docRef.Kind == "slides" {
			if _, _, err := parseSlidesBlockRef(runtime.Str("block-id")); err != nil {
				return err
			}
			if runtime.Bool("full-comment") {
				return output.ErrValidation("--full-comment is not applicable for slide comments; use --block-id <slide-block-type>!<xml-id>")
			}
			if strings.TrimSpace(runtime.Str("selection-with-ellipsis")) != "" {
				return output.ErrValidation("--selection-with-ellipsis is not applicable for slide comments; use --block-id <slide-block-type>!<xml-id>")
			}
			return nil
		}

		selection := runtime.Str("selection-with-ellipsis")
		blockID := strings.TrimSpace(runtime.Str("block-id"))
		if strings.TrimSpace(selection) != "" && blockID != "" {
			return output.ErrValidation("--selection-with-ellipsis and --block-id are mutually exclusive")
		}
		if runtime.Bool("full-comment") && (strings.TrimSpace(selection) != "" || blockID != "") {
			return output.ErrValidation("--full-comment cannot be used with --selection-with-ellipsis or --block-id")
		}

		mode := resolveCommentMode(runtime.Bool("full-comment"), selection, blockID)
		if mode == commentModeLocal && docRef.Kind == "doc" {
			return output.ErrValidation("local comments only support docx, sheet, and slides; old doc format only supports full comments")
		}

		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		docRef, _ := parseCommentDocRef(runtime.Str("doc"), runtime.Str("type"))
		replyElements, _ := parseCommentReplyElements(runtime.Str("content"))
		selection := runtime.Str("selection-with-ellipsis")
		blockID := strings.TrimSpace(runtime.Str("block-id"))
		mode := resolveCommentMode(runtime.Bool("full-comment"), selection, blockID)

		// For wiki URLs, resolve the actual target type via API so dry-run
		// matches real execution behavior instead of guessing from --block-id.
		resolvedKind := docRef.Kind
		resolvedToken := docRef.Token
		isWiki := false
		if docRef.Kind == "wiki" {
			isWiki = true
			target, err := resolveCommentTarget(ctx, runtime, runtime.Str("doc"), mode)
			if err != nil {
				return common.NewDryRunAPI().Set("error", err.Error())
			}
			resolvedKind = target.FileType
			resolvedToken = target.FileToken
		}

		// Sheet comment dry-run.
		if resolvedKind == "sheet" {
			anchor, _ := parseSheetCellRef(blockID)
			if anchor == nil {
				anchor = &sheetAnchor{SheetID: "<sheetId>", Col: 0, Row: 0}
			}
			commentBody := buildCommentCreateV2Request("sheet", "", "", replyElements, anchor)
			desc := "1-step request: create sheet comment"
			if isWiki {
				desc = "2-step orchestration: resolve wiki -> create sheet comment"
			}
			return common.NewDryRunAPI().
				Desc(desc).
				POST("/open-apis/drive/v1/files/:file_token/new_comments").
				Body(commentBody).
				Set("file_token", resolvedToken)
		}
		if resolvedKind == "slides" {
			slideAnchorBlockID, slideBlockType, err := parseSlidesBlockRef(blockID)
			if err != nil {
				return common.NewDryRunAPI().Set("error", err.Error())
			}
			commentBody := buildCommentCreateV2Request("slides", slideAnchorBlockID, slideBlockType, replyElements, nil)
			desc := "1-step request: create slide block comment"
			if isWiki {
				desc = "2-step orchestration: resolve wiki -> create slide block comment"
			}
			return common.NewDryRunAPI().
				Desc(desc).
				POST("/open-apis/drive/v1/files/:file_token/new_comments").
				Body(commentBody).
				Set("file_token", resolvedToken)
		}

		// Doc/docx comment dry-run.
		createPath := "/open-apis/drive/v1/files/:file_token/new_comments"
		commentBody := buildCommentCreateV2Request(resolvedKind, "", "", replyElements, nil)
		if mode == commentModeLocal {
			commentBody = buildCommentCreateV2Request(resolvedKind, anchorBlockIDForDryRun(blockID), "", replyElements, nil)
		}

		mcpEndpoint := common.MCPEndpoint(runtime.Config.Brand)

		dry := common.NewDryRunAPI()
		switch {
		case mode == commentModeFull && isWiki:
			dry.Desc("2-step orchestration: resolve wiki -> create full comment")
		case mode == commentModeFull:
			dry.Desc("1-step request: create full comment")
		case isWiki && strings.TrimSpace(selection) != "":
			dry.Desc("3-step orchestration: resolve wiki -> locate block -> create local comment")
		case isWiki:
			dry.Desc("2-step orchestration: resolve wiki -> create local comment")
		case strings.TrimSpace(selection) != "":
			dry.Desc("2-step orchestration: locate block -> create local comment")
		default:
			dry.Desc("1-step request: create local comment with explicit block ID")
		}

		if mode == commentModeLocal && strings.TrimSpace(selection) != "" {
			step := "[1]"
			if isWiki {
				step = "[2]"
			}
			docID := resolvedToken
			if isWiki && resolvedToken == docRef.Token {
				docID = "<resolved_docx_token>"
			}
			mcpArgs := map[string]interface{}{
				"doc_id":                  docID,
				"limit":                   defaultLocateDocLimit,
				"selection_with_ellipsis": selection,
			}
			dry.POST(mcpEndpoint).
				Desc(step+" MCP tool: locate-doc").
				Body(map[string]interface{}{
					"method": "tools/call",
					"params": map[string]interface{}{
						"name":      "locate-doc",
						"arguments": mcpArgs,
					},
				}).
				Set("mcp_tool", "locate-doc").
				Set("args", mcpArgs)
		}

		step := "[1]"
		createDesc := "Create full comment"
		if mode == commentModeLocal {
			createDesc = "Create local comment"
			step = "[2]"
			if isWiki && strings.TrimSpace(selection) != "" {
				step = "[3]"
			} else if isWiki || strings.TrimSpace(selection) != "" {
				step = "[2]"
			} else {
				step = "[1]"
			}
		} else if isWiki {
			step = "[2]"
		}

		return dry.POST(createPath).
			Desc(step+" "+createDesc).
			Body(commentBody).
			Set("file_token", resolvedToken)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		// Sheet comment: direct URL or token fast path.
		docRef, _ := parseCommentDocRef(runtime.Str("doc"), runtime.Str("type"))
		if docRef.Kind == "sheet" {
			return executeSheetComment(runtime, docRef)
		}
		if docRef.Kind == "slides" {
			return executeSlidesComment(runtime, docRef)
		}

		selection := runtime.Str("selection-with-ellipsis")
		blockID := strings.TrimSpace(runtime.Str("block-id"))
		mode := resolveCommentMode(runtime.Bool("full-comment"), selection, blockID)

		target, err := resolveCommentTarget(ctx, runtime, runtime.Str("doc"), mode)
		if err != nil {
			return err
		}

		// Wiki resolved to sheet: redirect to sheet comment path.
		if target.FileType == "sheet" {
			return executeSheetComment(runtime, commentDocRef{Kind: "sheet", Token: target.FileToken})
		}
		if target.FileType == "slides" {
			return executeSlidesComment(runtime, commentDocRef{Kind: "slides", Token: target.FileToken})
		}

		replyElements, err := parseCommentReplyElements(runtime.Str("content"))
		if err != nil {
			return err
		}

		var locateResult locateDocResult
		selectedMatch := 0
		if mode == commentModeLocal && blockID == "" {
			_, locateResult, err = locateDocumentSelection(runtime, target, selection, defaultLocateDocLimit)
			if err != nil {
				return err
			}

			match, idx, err := selectLocateMatch(locateResult)
			if err != nil {
				return err
			}
			blockID = match.AnchorBlockID
			if strings.TrimSpace(blockID) == "" {
				return output.Errorf(output.ExitAPI, "api_error", "locate-doc response missing anchor_block_id")
			}
			selectedMatch = idx
			fmt.Fprintf(runtime.IO().ErrOut, "Locate-doc matched %d block(s); using match #%d (%s)\n", len(locateResult.Matches), idx, blockID)
		} else if mode == commentModeLocal {
			fmt.Fprintf(runtime.IO().ErrOut, "Using explicit block ID: %s\n", blockID)
		}

		requestPath := fmt.Sprintf("/open-apis/drive/v1/files/%s/new_comments", validate.EncodePathSegment(target.FileToken))
		requestBody := buildCommentCreateV2Request(target.FileType, "", "", replyElements, nil)
		if mode == commentModeLocal {
			requestBody = buildCommentCreateV2Request(target.FileType, blockID, "", replyElements, nil)
		}

		if mode == commentModeLocal {
			fmt.Fprintf(runtime.IO().ErrOut, "Creating local comment in %s\n", common.MaskToken(target.FileToken))
		} else {
			fmt.Fprintf(runtime.IO().ErrOut, "Creating full comment in %s\n", common.MaskToken(target.FileToken))
		}

		data, err := runtime.CallAPI(
			"POST",
			requestPath,
			nil,
			requestBody,
		)
		if err != nil {
			return err
		}

		out := map[string]interface{}{
			"comment_id":   data["comment_id"],
			"doc_id":       target.DocID,
			"file_token":   target.FileToken,
			"file_type":    target.FileType,
			"resolved_by":  target.ResolvedBy,
			"comment_mode": string(mode),
		}
		if createdAt := firstPresentValue(data, "created_at", "create_time"); createdAt != nil {
			out["created_at"] = createdAt
		}
		if target.WikiToken != "" {
			out["wiki_token"] = target.WikiToken
		}
		if mode == commentModeLocal {
			out["anchor_block_id"] = blockID
			out["selection_source"] = "block_id"
			if strings.TrimSpace(selection) != "" {
				out["selection_source"] = "locate-doc"
				out["selection_with_ellipsis"] = selection
				out["match_count"] = locateResult.MatchCount
				out["match_index"] = selectedMatch
			}
		} else if isWhole, ok := data["is_whole"]; ok {
			out["is_whole"] = isWhole
		}

		runtime.Out(out, nil)
		return nil
	},
}

func resolveCommentMode(explicitFullComment bool, selection, blockID string) commentMode {
	if explicitFullComment {
		return commentModeFull
	}
	if strings.TrimSpace(selection) == "" && strings.TrimSpace(blockID) == "" {
		return commentModeFull
	}
	return commentModeLocal
}

func parseCommentDocRef(input, docType string) (commentDocRef, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return commentDocRef{}, output.ErrValidation("--doc cannot be empty")
	}

	if token, ok := extractURLToken(raw, "/wiki/"); ok {
		return commentDocRef{Kind: "wiki", Token: token}, nil
	}
	if token, ok := extractURLToken(raw, "/sheets/"); ok {
		return commentDocRef{Kind: "sheet", Token: token}, nil
	}
	if token, ok := extractURLToken(raw, "/slides/"); ok {
		return commentDocRef{Kind: "slides", Token: token}, nil
	}
	if token, ok := extractURLToken(raw, "/docx/"); ok {
		return commentDocRef{Kind: "docx", Token: token}, nil
	}
	if token, ok := extractURLToken(raw, "/doc/"); ok {
		return commentDocRef{Kind: "doc", Token: token}, nil
	}
	if strings.Contains(raw, "://") {
		return commentDocRef{}, output.ErrValidation("unsupported --doc input %q: use a doc/docx/sheet/slides URL, a token with --type, or a wiki URL that resolves to doc/docx/sheet/slides", raw)
	}
	if strings.ContainsAny(raw, "/?#") {
		return commentDocRef{}, output.ErrValidation("unsupported --doc input %q: use a token with --type, or a wiki URL", raw)
	}

	// Bare token: --type is required.
	docType = strings.TrimSpace(docType)
	if docType == "" {
		return commentDocRef{}, output.ErrValidation("--type is required when --doc is a bare token (allowed values: doc, docx, sheet, slides)")
	}
	return commentDocRef{Kind: docType, Token: raw}, nil
}

func resolveCommentTarget(ctx context.Context, runtime *common.RuntimeContext, input string, mode commentMode) (resolvedCommentTarget, error) {
	docRef, err := parseCommentDocRef(input, runtime.Str("type"))
	if err != nil {
		return resolvedCommentTarget{}, err
	}

	if docRef.Kind == "docx" || docRef.Kind == "doc" || docRef.Kind == "sheet" || docRef.Kind == "slides" {
		if mode == commentModeLocal && docRef.Kind != "docx" && docRef.Kind != "sheet" && docRef.Kind != "slides" {
			return resolvedCommentTarget{}, output.ErrValidation("local comments only support docx, sheet, and slides; old doc format only supports full comments")
		}
		return resolvedCommentTarget{
			DocID:      docRef.Token,
			FileToken:  docRef.Token,
			FileType:   docRef.Kind,
			ResolvedBy: docRef.Kind,
		}, nil
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Resolving wiki node: %s\n", common.MaskToken(docRef.Token))
	data, err := runtime.CallAPI(
		"GET",
		"/open-apis/wiki/v2/spaces/get_node",
		map[string]interface{}{"token": docRef.Token},
		nil,
	)
	if err != nil {
		return resolvedCommentTarget{}, err
	}

	node := common.GetMap(data, "node")
	objType := common.GetString(node, "obj_type")
	objToken := common.GetString(node, "obj_token")
	if objType == "" || objToken == "" {
		return resolvedCommentTarget{}, output.Errorf(output.ExitAPI, "api_error", "wiki get_node returned incomplete node data")
	}
	if objType == "slides" && mode == commentModeFull {
		return resolvedCommentTarget{}, output.ErrValidation("wiki resolved to %q, but slide comments require --block-id <slide-block-type>!<xml-id>; --full-comment is not applicable", objType)
	}
	if objType == "slides" && strings.TrimSpace(runtime.Str("selection-with-ellipsis")) != "" {
		return resolvedCommentTarget{}, output.ErrValidation("wiki resolved to %q, but --selection-with-ellipsis is not applicable for slide comments; use --block-id <slide-block-type>!<xml-id>", objType)
	}
	if objType == "sheet" {
		// Sheet comments are handled via the sheet fast path in Execute.
		fmt.Fprintf(runtime.IO().ErrOut, "Resolved wiki to %s: %s\n", objType, common.MaskToken(objToken))
		return resolvedCommentTarget{
			DocID:      objToken,
			FileToken:  objToken,
			FileType:   "sheet",
			ResolvedBy: "wiki",
			WikiToken:  docRef.Token,
		}, nil
	}
	if objType == "slides" {
		fmt.Fprintf(runtime.IO().ErrOut, "Resolved wiki to %s: %s\n", objType, common.MaskToken(objToken))
		return resolvedCommentTarget{
			DocID:      objToken,
			FileToken:  objToken,
			FileType:   "slides",
			ResolvedBy: "wiki",
			WikiToken:  docRef.Token,
		}, nil
	}
	if mode == commentModeLocal && objType != "docx" {
		return resolvedCommentTarget{}, output.ErrValidation("wiki resolved to %q, but local comments only support docx, sheet, and slides; for sheet use --block-id <sheetId>!<cell>, for slides use --block-id <slide-block-type>!<xml-id>", objType)
	}
	if mode == commentModeFull && objType != "docx" && objType != "doc" {
		return resolvedCommentTarget{}, output.ErrValidation("wiki resolved to %q, but comments only support doc/docx/sheet/slides", objType)
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Resolved wiki to %s: %s\n", objType, common.MaskToken(objToken))
	return resolvedCommentTarget{
		DocID:      objToken,
		FileToken:  objToken,
		FileType:   objType,
		ResolvedBy: "wiki",
		WikiToken:  docRef.Token,
	}, nil
}

func locateDocumentSelection(runtime *common.RuntimeContext, target resolvedCommentTarget, selection string, limit int) (map[string]interface{}, locateDocResult, error) {
	args := map[string]interface{}{
		"doc_id":                  target.DocID,
		"limit":                   limit,
		"selection_with_ellipsis": selection,
	}

	result, err := common.CallMCPTool(runtime, "locate-doc", args)
	if err != nil {
		return nil, locateDocResult{}, err
	}

	return result, parseLocateDocResult(result), nil
}

func parseLocateDocResult(result map[string]interface{}) locateDocResult {
	rawMatches := common.GetSlice(result, "matches")
	locate := locateDocResult{
		MatchCount: int(common.GetFloat(result, "match_count")),
	}

	for _, item := range rawMatches {
		matchMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		match := locateDocMatch{
			AnchorBlockID: common.GetString(matchMap, "anchor_block_id"),
			ParentBlockID: common.GetString(matchMap, "parent_block_id"),
		}
		for _, blockItem := range common.GetSlice(matchMap, "blocks") {
			blockMap, ok := blockItem.(map[string]interface{})
			if !ok {
				continue
			}
			match.Blocks = append(match.Blocks, locateDocBlock{
				BlockID:     common.GetString(blockMap, "block_id"),
				RawMarkdown: common.GetString(blockMap, "raw_markdown"),
			})
		}
		if match.AnchorBlockID == "" && len(match.Blocks) > 0 {
			match.AnchorBlockID = match.Blocks[0].BlockID
		}
		locate.Matches = append(locate.Matches, match)
	}

	if locate.MatchCount == 0 {
		locate.MatchCount = len(locate.Matches)
	}
	return locate
}

func selectLocateMatch(result locateDocResult) (locateDocMatch, int, error) {
	if len(result.Matches) == 0 {
		return locateDocMatch{}, 0, output.ErrValidation("locate-doc did not find any matching block")
	}

	if len(result.Matches) > 1 {
		return locateDocMatch{}, 0, output.ErrWithHint(
			output.ExitValidation,
			"ambiguous_match",
			fmt.Sprintf("locate-doc matched %d blocks:\n%s", len(result.Matches), formatLocateCandidates(result.Matches)),
			"narrow --selection-with-ellipsis until only one block matches",
		)
	}

	return result.Matches[0], 1, nil
}

func formatLocateCandidates(matches []locateDocMatch) string {
	lines := make([]string, 0, len(matches))
	for i, match := range matches {
		lines = append(lines, fmt.Sprintf("%d. anchor_block_id=%s", i+1, match.AnchorBlockID))
	}
	return strings.Join(lines, "\n")
}

func summarizeLocateMatch(match locateDocMatch) string {
	if len(match.Blocks) == 0 {
		return ""
	}

	parts := make([]string, 0, len(match.Blocks))
	for _, block := range match.Blocks {
		snippet := strings.TrimSpace(block.RawMarkdown)
		if snippet == "" {
			continue
		}
		snippet = strings.ReplaceAll(snippet, "\n", " ")
		parts = append(parts, snippet)
	}
	return common.TruncateStr(strings.Join(parts, " | "), 120)
}

func parseCommentReplyElements(raw string) ([]map[string]interface{}, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, output.ErrValidation("--content cannot be empty")
	}

	var inputs []commentReplyElementInput
	if err := json.Unmarshal([]byte(raw), &inputs); err != nil {
		return nil, output.ErrValidation("--content is not valid JSON: %s\nexample: --content '[{\"type\":\"text\",\"text\":\"文本信息\"}]'", err)
	}
	if len(inputs) == 0 {
		return nil, output.ErrValidation("--content must contain at least one reply element")
	}

	replyElements := make([]map[string]interface{}, 0, len(inputs))
	totalRunes := 0
	for i, input := range inputs {
		index := i + 1
		elementType := strings.TrimSpace(input.Type)
		switch elementType {
		case "text":
			if strings.TrimSpace(input.Text) == "" {
				return nil, output.ErrValidation("--content element #%d type=text requires non-empty text", index)
			}
			// Measure the raw rune count of the user input — that is what
			// the server actually counts. byte width and post-escape form
			// don't matter (10000 '<' chars succeed even though they
			// expand to 40000 bytes when escaped, and 10000 Chinese chars
			// succeed even though they encode as 30000 UTF-8 bytes).
			runes := utf8.RuneCountInString(input.Text)
			totalRunes += runes
			if totalRunes > maxCommentTotalRunes {
				return nil, output.ErrWithHint(
					output.ExitValidation,
					"text_too_long",
					fmt.Sprintf("--content reply_elements text totals %d characters at element #%d (this element: %d); the server caps the combined length at %d characters across ALL reply_elements",
						totalRunes, index, runes, maxCommentTotalRunes),
					fmt.Sprintf("shorten the comment so the combined text across all reply_elements fits within %d characters. The server enforces this cap on the TOTAL — splitting one long element into multiple smaller text elements does NOT help (they all add up against the same %d-rune budget). Server returns an opaque [1069302] on overflow, so this check is pre-flight; no escape transform changes the count (server reads raw runes).", maxCommentTotalRunes, maxCommentTotalRunes),
				)
			}
			// Escape '<' and '>' so the rendered comment displays them as
			// literal characters instead of being interpreted as markup
			// by Lark's comment renderer. This is independent of the
			// length check — the server sees the escaped form, but
			// counts characters by the raw input length above.
			replyElements = append(replyElements, map[string]interface{}{
				"type": "text",
				"text": escapeCommentText(input.Text),
			})
		case "mention_user":
			mentionUser := firstNonEmptyString(input.MentionUser, input.Text)
			if mentionUser == "" {
				return nil, output.ErrValidation("--content element #%d type=mention_user requires text or mention_user", index)
			}
			replyElements = append(replyElements, map[string]interface{}{
				"type":         "mention_user",
				"mention_user": mentionUser,
			})
		case "link":
			link := firstNonEmptyString(input.Link, input.Text)
			if link == "" {
				return nil, output.ErrValidation("--content element #%d type=link requires text or link", index)
			}
			replyElements = append(replyElements, map[string]interface{}{
				"type": "link",
				"link": link,
			})
		default:
			return nil, output.ErrValidation("--content element #%d has unsupported type %q; allowed values: text, mention_user, link", index, input.Type)
		}
	}

	return replyElements, nil
}

func escapeCommentText(input string) string {
	replacer := strings.NewReplacer(
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(input)
}

type sheetAnchor struct {
	SheetID string
	Col     int
	Row     int
}

func buildCommentCreateV2Request(fileType, blockID, slideBlockType string, replyElements []map[string]interface{}, sheet *sheetAnchor) map[string]interface{} {
	body := map[string]interface{}{
		"file_type":      fileType,
		"reply_elements": replyElements,
	}
	if sheet != nil {
		body["anchor"] = map[string]interface{}{
			"block_id":  sheet.SheetID,
			"sheet_col": sheet.Col,
			"sheet_row": sheet.Row,
		}
	} else if strings.TrimSpace(blockID) != "" {
		body["anchor"] = map[string]interface{}{
			"block_id": blockID,
		}
		if strings.TrimSpace(slideBlockType) != "" {
			body["anchor"].(map[string]interface{})["slide_block_type"] = strings.TrimSpace(slideBlockType)
		}
	}
	return body
}

func anchorBlockIDForDryRun(blockID string) string {
	if strings.TrimSpace(blockID) != "" {
		return strings.TrimSpace(blockID)
	}
	return "<anchor_block_id>"
}

func parseSlidesBlockRef(blockID string) (string, string, error) {
	blockID = strings.TrimSpace(blockID)
	if blockID == "" {
		return "", "", output.ErrValidation("slide comments require --block-id in <slide-block-type>!<xml-id> format")
	}

	parts := strings.SplitN(blockID, "!", 2)
	if len(parts) != 2 {
		return "", "", output.ErrValidation("slide --block-id must be <slide-block-type>!<xml-id> (e.g. shape!bPq), got %q", blockID)
	}
	parsedType := strings.TrimSpace(parts[0])
	parsedID := strings.TrimSpace(parts[1])
	if parsedType == "" || parsedID == "" {
		return "", "", output.ErrValidation("slide --block-id must be <slide-block-type>!<xml-id> (e.g. shape!bPq), got %q", blockID)
	}
	return parsedID, parsedType, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstPresentValue(m map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := m[key]; ok && value != nil {
			return value
		}
	}
	return nil
}

// parseSheetCellRef parses "<sheetId>!<cell>" (e.g. "a281f9!D6") into a sheetAnchor.
// Column is converted from letter to 0-based index (A=0), row from 1-based to 0-based.
func parseSheetCellRef(input string) (*sheetAnchor, error) {
	parts := strings.SplitN(input, "!", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, output.ErrValidation("--block-id for sheet must be <sheetId>!<cell> (e.g. a281f9!D6), got %q", input)
	}
	sheetID := parts[0]
	cell := strings.TrimSpace(parts[1])

	// Parse cell reference like "D6" into col letter + row number.
	i := 0
	for i < len(cell) && ((cell[i] >= 'A' && cell[i] <= 'Z') || (cell[i] >= 'a' && cell[i] <= 'z')) {
		i++
	}
	if i == 0 || i >= len(cell) {
		return nil, output.ErrValidation("--block-id cell reference %q is invalid (expected e.g. D6)", cell)
	}
	colStr := strings.ToUpper(cell[:i])
	rowStr := cell[i:]

	// Column letter to 0-based index: A=0, B=1, ..., Z=25, AA=26.
	col := 0
	for _, ch := range colStr {
		col = col*26 + int(ch-'A'+1)
	}
	col-- // convert to 0-based

	row, err := strconv.Atoi(rowStr)
	if err != nil || row < 1 {
		return nil, output.ErrValidation("--block-id row %q is invalid (must be >= 1)", rowStr)
	}
	row-- // convert to 0-based

	return &sheetAnchor{SheetID: sheetID, Col: col, Row: row}, nil
}

func executeSheetComment(runtime *common.RuntimeContext, docRef commentDocRef) error {
	replyElements, err := parseCommentReplyElements(runtime.Str("content"))
	if err != nil {
		return err
	}

	blockID := strings.TrimSpace(runtime.Str("block-id"))
	if blockID == "" {
		return output.ErrValidation("--block-id is required for sheet comments (format: <sheetId>!<cell>, e.g. a281f9!D6)")
	}
	anchor, err := parseSheetCellRef(blockID)
	if err != nil {
		return err
	}

	requestPath := fmt.Sprintf("/open-apis/drive/v1/files/%s/new_comments", validate.EncodePathSegment(docRef.Token))
	requestBody := buildCommentCreateV2Request("sheet", "", "", replyElements, anchor)

	fmt.Fprintf(runtime.IO().ErrOut, "Creating sheet comment in %s (sheet=%s, col=%d, row=%d)\n",
		common.MaskToken(docRef.Token), anchor.SheetID, anchor.Col, anchor.Row)

	data, err := runtime.CallAPI("POST", requestPath, nil, requestBody)
	if err != nil {
		return err
	}

	out := map[string]interface{}{
		"comment_id":   data["comment_id"],
		"file_token":   docRef.Token,
		"file_type":    "sheet",
		"comment_mode": "sheet",
		"block_id":     blockID,
	}
	if createdAt := firstPresentValue(data, "created_at", "create_time"); createdAt != nil {
		out["created_at"] = createdAt
	}
	runtime.Out(out, nil)
	return nil
}

func executeSlidesComment(runtime *common.RuntimeContext, docRef commentDocRef) error {
	replyElements, err := parseCommentReplyElements(runtime.Str("content"))
	if err != nil {
		return err
	}

	blockID, slideBlockType, err := parseSlidesBlockRef(runtime.Str("block-id"))
	if err != nil {
		return err
	}

	requestPath := fmt.Sprintf("/open-apis/drive/v1/files/%s/new_comments", validate.EncodePathSegment(docRef.Token))
	requestBody := buildCommentCreateV2Request("slides", blockID, slideBlockType, replyElements, nil)

	fmt.Fprintf(runtime.IO().ErrOut, "Creating slide block comment in %s (block_id=%s, slide_block_type=%s)\n",
		common.MaskToken(docRef.Token), blockID, slideBlockType)

	data, err := runtime.CallAPI("POST", requestPath, nil, requestBody)
	if err != nil {
		return err
	}

	out := map[string]interface{}{
		"comment_id":       data["comment_id"],
		"file_token":       docRef.Token,
		"file_type":        "slides",
		"comment_mode":     "slide_block",
		"anchor_block_id":  blockID,
		"slide_block_type": slideBlockType,
	}
	if createdAt := firstPresentValue(data, "created_at", "create_time"); createdAt != nil {
		out["created_at"] = createdAt
	}
	runtime.Out(out, nil)
	return nil
}

func extractURLToken(raw, marker string) (string, bool) {
	idx := strings.Index(raw, marker)
	if idx < 0 {
		return "", false
	}
	token := raw[idx+len(marker):]
	if end := strings.IndexAny(token, "/?#"); end >= 0 {
		token = token[:end]
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	return token, true
}
