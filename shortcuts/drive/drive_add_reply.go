// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var driveAddReplyOp = driveCommentOp{
	Label: "comment reply",
	Types: []string{"doc", "docx", "sheet", "file", "slides", "bitable", "apps"},
}

type driveAddReplySpec struct {
	Ref           driveCommentRef
	CommentID     string
	ReplyElements []map[string]interface{} // simplified +add-comment element form, text already escaped
}

func (s driveAddReplySpec) RequestBody() map[string]interface{} {
	return map[string]interface{}{
		"content": map[string]interface{}{
			"elements": driveReplyV1Elements(s.ReplyElements),
		},
	}
}

// DriveAddReply replies to an existing comment through the Drive comment
// reply create API (POST .../comments/:comment_id/replies), while accepting
// Wiki URLs/tokens and resolving them to the underlying object.
//
// Note: the documented alternative — POST .../comments with comment_id in the
// body ("如填写，则视为回复已有评论") — does NOT reply on docx in practice; it
// silently creates a new standalone comment instead.
var DriveAddReply = common.Shortcut{
	Service:           "drive",
	Command:           "+add-reply",
	Description:       "Add a reply to an existing comment on doc/docx/sheet/file/slides/base(bitable)/apps, with URL parsing and Wiki token unwrapping",
	Risk:              "write",
	Scopes:            []string{"docs:document.comment:create"},
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: append(driveCommentTargetFlags(driveAddReplyOp),
		common.Flag{Name: "comment-id", Desc: "comment ID to reply to (from drive +list-comments)", Required: true},
		common.Flag{Name: "content", Desc: "reply_elements JSON string, same format as drive +add-comment", Required: true, Input: []string{common.File, common.Stdin}},
	),
	Tips: []string{
		"--content uses the same JSON as `drive +add-comment`: '[{\"type\":\"text\",\"text\":\"正文\"}]' (types: text, mention_user, link).",
		"Comment IDs come from `drive +list-comments` (items[].comment_id).",
		"Whole-document comments (is_whole=true) and solved comments (is_solved=true) do not accept replies; check the comment state via `drive +list-comments` first.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readDriveAddReplySpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDriveAddReplySpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildDriveAddReplyDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveAddReplySpec(runtime)
		if err != nil {
			return err
		}

		target, err := resolveDriveCommentTarget(ctx, runtime, driveAddReplyOp, spec.Ref)
		if err != nil {
			return err
		}

		path := fmt.Sprintf(
			"/open-apis/drive/v1/files/%s/comments/%s/replies",
			validate.EncodePathSegment(target.FileToken),
			validate.EncodePathSegment(spec.CommentID),
		)
		data, err := runtime.CallAPITyped(
			"POST",
			path,
			map[string]interface{}{"file_type": target.FileType},
			spec.RequestBody(),
		)
		if err != nil {
			return err
		}

		extra := map[string]interface{}{
			"comment_id": spec.CommentID,
			"created":    true,
		}
		if replyID := extractDriveCreatedReplyID(data); replyID != "" {
			extra["reply_id"] = replyID
		}
		runtime.Out(driveCommentTargetOutput(target, extra), nil)
		return nil
	},
}

func readDriveAddReplySpec(runtime *common.RuntimeContext) (driveAddReplySpec, error) {
	ref, err := resolveDriveCommentInput(driveAddReplyOp, runtime.Str("url"), runtime.Str("token"), runtime.Str("type"))
	if err != nil {
		return driveAddReplySpec{}, err
	}
	commentID := strings.TrimSpace(runtime.Str("comment-id"))
	if err := validateDriveCommentPathID(commentID, "--comment-id"); err != nil {
		return driveAddReplySpec{}, err
	}
	replyElements, err := parseCommentReplyElements(runtime.Str("content"))
	if err != nil {
		return driveAddReplySpec{}, err
	}
	// The reply-create endpoint documents a 100-element cap on content.elements;
	// reject over-cap input locally instead of surfacing the opaque [1069302].
	if len(replyElements) > maxCommentReplyElements {
		return driveAddReplySpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--content has %d elements; the reply endpoint caps content.elements at %d", len(replyElements), maxCommentReplyElements).
			WithParam("--content")
	}
	return driveAddReplySpec{
		Ref:           ref,
		CommentID:     commentID,
		ReplyElements: replyElements,
	}, nil
}

// driveReplyV1Elements converts the simplified +add-comment reply element form
// (text / mention_user / link) to the Drive v1 comment create wire form
// (text_run / person / docs_link).
func driveReplyV1Elements(replyElements []map[string]interface{}) []map[string]interface{} {
	elements := make([]map[string]interface{}, 0, len(replyElements))
	for _, element := range replyElements {
		switch common.GetString(element, "type") {
		case "text":
			elements = append(elements, map[string]interface{}{
				"type":     "text_run",
				"text_run": map[string]interface{}{"text": common.GetString(element, "text")},
			})
		case "mention_user":
			elements = append(elements, map[string]interface{}{
				"type":   "person",
				"person": map[string]interface{}{"user_id": common.GetString(element, "mention_user")},
			})
		case "link":
			elements = append(elements, map[string]interface{}{
				"type":      "docs_link",
				"docs_link": map[string]interface{}{"url": common.GetString(element, "link")},
			})
		}
	}
	return elements
}

// extractDriveCreatedReplyID pulls the created reply ID out of the reply
// create response, tolerating the shapes the API family uses: a top-level
// reply_id, a nested reply object, or a reply_list wrapper.
func extractDriveCreatedReplyID(data map[string]interface{}) string {
	if replyID := common.GetString(data, "reply_id"); replyID != "" {
		return replyID
	}
	if reply := common.GetMap(data, "reply"); reply != nil {
		if replyID := common.GetString(reply, "reply_id"); replyID != "" {
			return replyID
		}
	}
	replyList := common.GetMap(data, "reply_list")
	if replyList == nil {
		return ""
	}
	for _, item := range common.GetSlice(replyList, "replies") {
		reply, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if replyID := common.GetString(reply, "reply_id"); replyID != "" {
			return replyID
		}
	}
	return ""
}

func buildDriveAddReplyDryRun(spec driveAddReplySpec) *common.DryRunAPI {
	if spec.Ref.Type == "wiki" {
		return common.NewDryRunAPI().
			Desc("2-step orchestration: resolve wiki -> add reply to comment").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to underlying document").
			Params(map[string]interface{}{"token": spec.Ref.Token}).
			POST("/open-apis/drive/v1/files/<obj_token from step 1>/comments/:comment_id/replies").
			Desc("[2] Add reply to comment on resolved document").
			Params(map[string]interface{}{"file_type": "<obj_type from step 1>"}).
			Body(spec.RequestBody()).
			Set("comment_id", spec.CommentID)
	}

	return common.NewDryRunAPI().
		Desc("1-step request: add reply to comment").
		POST("/open-apis/drive/v1/files/:file_token/comments/:comment_id/replies").
		Params(map[string]interface{}{"file_type": spec.Ref.Type}).
		Body(spec.RequestBody()).
		Set("file_token", spec.Ref.Token).
		Set("comment_id", spec.CommentID)
}
