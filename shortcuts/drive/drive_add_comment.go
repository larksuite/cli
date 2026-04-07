// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const defaultLocateDocLimit = 10

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
	Description: "Add a full-document comment, or a local comment to selected docx text (also supports wiki URL resolving to doc/docx)",
	Risk:        "write",
	Scopes: []string{
		"docx:document:readonly",
		"docs:document.comment:create",
		"docs:document.comment:write_only",
	},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "doc", Desc: "document URL/token, or wiki URL that resolves to doc/docx", Required: true},
		{Name: "content", Desc: "reply_elements JSON string", Required: true},
		{Name: "full-comment", Type: "bool", Desc: "create a full-document comment; also the default when no location is provided"},
		{Name: "selection-with-ellipsis", Desc: "target content locator (plain text or 'start...end')"},
		{Name: "block-id", Desc: "anchor block ID (skip MCP locate-doc if already known)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		docRef, err := parseCommentDocRef(runtime.Str("doc"))
		if err != nil {
			return err
		}

		if _, err := parseCommentReplyElements(runtime.Str("content")); err != nil {
			return err
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
			return output.ErrValidation("local comments only support docx documents; use --full-comment or omit location flags for a whole-document comment")
		}

		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		docRef, _ := parseCommentDocRef(runtime.Str("doc"))
		replyElements, _ := parseCommentReplyElements(runtime.Str("content"))
		selection := runtime.Str("selection-with-ellipsis")
		blockID := strings.TrimSpace(runtime.Str("block-id"))
		mode := resolveCommentMode(runtime.Bool("full-comment"), selection, blockID)

		targetToken, targetFileType, resolvedBy := dryRunResolvedCommentTarget(docRef, mode)

		createPath := "/open-apis/drive/v1/files/:file_token/new_comments"
		commentBody := buildCommentCreateV2Request(targetFileType, "", replyElements)
		if mode == commentModeLocal {
			commentBody = buildCommentCreateV2Request(targetFileType, anchorBlockIDForDryRun(blockID), replyElements)
		}

		mcpEndpoint := common.MCPEndpoint(runtime.Config.Brand)

		dry := common.NewDryRunAPI()
		switch {
		case mode == commentModeFull && resolvedBy == "wiki":
			dry.Desc("2-step orchestration: resolve wiki -> create full comment")
		case mode == commentModeFull:
			dry.Desc("1-step request: create full comment")
		case resolvedBy == "wiki" && strings.TrimSpace(selection) != "":
			dry.Desc("3-step orchestration: resolve wiki -> locate block -> create local comment")
		case resolvedBy == "wiki":
			dry.Desc("2-step orchestration: resolve wiki -> create local comment")
		case strings.TrimSpace(selection) != "":
			dry.Desc("2-step orchestration: locate block -> create local comment")
		default:
			dry.Desc("1-step request: create local comment with explicit block ID")
		}

		if resolvedBy == "wiki" {
			dry.GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to target document").
				Params(map[string]interface{}{"token": docRef.Token})
		}

		if mode == commentModeLocal && strings.TrimSpace(selection) != "" {
			step := "[1]"
			if resolvedBy == "wiki" {
				step = "[2]"
			}
			mcpArgs := map[string]interface{}{
				"doc_id":                  dryRunLocateDocRef(docRef),
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
			if resolvedBy == "wiki" && strings.TrimSpace(selection) != "" {
				step = "[3]"
			} else if resolvedBy == "wiki" || strings.TrimSpace(selection) != "" {
				step = "[2]"
			} else {
				step = "[1]"
			}
		} else if resolvedBy == "wiki" {
			step = "[2]"
		}

		return dry.POST(createPath).
			Desc(step+" "+createDesc).
			Body(commentBody).
			Set("file_token", targetToken)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		selection := runtime.Str("selection-with-ellipsis")
		blockID := strings.TrimSpace(runtime.Str("block-id"))
		mode := resolveCommentMode(runtime.Bool("full-comment"), selection, blockID)

		target, err := resolveCommentTarget(ctx, runtime, runtime.Str("doc"), mode)
		if err != nil {
			return err
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
		requestBody := buildCommentCreateV2Request(target.FileType, "", replyElements)
		if mode == commentModeLocal {
			requestBody = buildCommentCreateV2Request(target.FileType, blockID, replyElements)
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

func parseCommentDocRef(input string) (commentDocRef, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return commentDocRef{}, output.ErrValidation("--doc cannot be empty")
	}

	if token, ok := extractURLToken(raw, "/wiki/"); ok {
		return commentDocRef{Kind: "wiki", Token: token}, nil
	}
	if token, ok := extractURLToken(raw, "/docx/"); ok {
		return commentDocRef{Kind: "docx", Token: token}, nil
	}
	if token, ok := extractURLToken(raw, "/doc/"); ok {
		return commentDocRef{Kind: "doc", Token: token}, nil
	}
	if strings.Contains(raw, "://") {
		return commentDocRef{}, output.ErrValidation("unsupported --doc input %q: use a doc/docx URL, a docx token, or a wiki URL that resolves to doc/docx", raw)
	}
	if strings.ContainsAny(raw, "/?#") {
		return commentDocRef{}, output.ErrValidation("unsupported --doc input %q: use a docx token or a wiki URL", raw)
	}

	return commentDocRef{Kind: "docx", Token: raw}, nil
}

func dryRunResolvedCommentTarget(docRef commentDocRef, mode commentMode) (token, fileType, resolvedBy string) {
	switch docRef.Kind {
	case "docx":
		return docRef.Token, "docx", "docx"
	case "doc":
		return docRef.Token, "doc", "doc"
	case "wiki":
		if mode == commentModeFull {
			return "<resolved_file_token>", "<resolved_file_type>", "wiki"
		}
		return "<resolved_docx_token>", "docx", "wiki"
	default:
		return "<resolved_docx_token>", "docx", "docx"
	}
}

func resolveCommentTarget(ctx context.Context, runtime *common.RuntimeContext, input string, mode commentMode) (resolvedCommentTarget, error) {
	docRef, err := parseCommentDocRef(input)
	if err != nil {
		return resolvedCommentTarget{}, err
	}

	if docRef.Kind == "docx" || docRef.Kind == "doc" {
		if mode == commentModeLocal && docRef.Kind != "docx" {
			return resolvedCommentTarget{}, output.ErrValidation("local comments only support docx documents")
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
	if mode == commentModeLocal && objType != "docx" {
		return resolvedCommentTarget{}, output.ErrValidation("wiki resolved to %q, but local comments currently only support docx documents", objType)
	}
	if mode == commentModeFull && objType != "docx" && objType != "doc" {
		return resolvedCommentTarget{}, output.ErrValidation("wiki resolved to %q, but full comments only support doc/docx documents", objType)
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
	for i, input := range inputs {
		index := i + 1
		elementType := strings.TrimSpace(input.Type)
		switch elementType {
		case "text":
			if strings.TrimSpace(input.Text) == "" {
				return nil, output.ErrValidation("--content element #%d type=text requires non-empty text", index)
			}
			if utf8.RuneCountInString(input.Text) > 1000 {
				return nil, output.ErrValidation("--content element #%d text exceeds 1000 characters", index)
			}
			replyElements = append(replyElements, map[string]interface{}{
				"type": "text",
				"text": input.Text,
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

func buildCommentCreateV2Request(fileType, blockID string, replyElements []map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"file_type":      fileType,
		"reply_elements": replyElements,
	}
	if strings.TrimSpace(blockID) != "" {
		body["anchor"] = map[string]interface{}{
			"block_id": blockID,
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

func dryRunLocateDocRef(docRef commentDocRef) string {
	if docRef.Kind == "wiki" {
		return "<resolved_docx_token>"
	}
	return docRef.Token
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
