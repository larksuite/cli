// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var driveUpdateReplyOp = driveCommentOp{
	Label: "reply update",
	Types: []string{"doc", "docx", "sheet", "file", "slides", "bitable", "apps"},
}

type driveUpdateReplySpec struct {
	Ref           driveCommentRef
	CommentID     string
	ReplyID       string
	ReplyElements []map[string]interface{} // simplified +add-comment element form, text already escaped
}

func (s driveUpdateReplySpec) RequestBody() map[string]interface{} {
	return map[string]interface{}{
		"content": map[string]interface{}{
			"elements": driveReplyV1Elements(s.ReplyElements),
		},
	}
}

// DriveUpdateReply replaces the content of an existing comment reply through
// the Drive comment reply update API (PUT .../comments/:comment_id/replies/:reply_id),
// while accepting Wiki URLs/tokens and resolving them to the underlying object.
var DriveUpdateReply = common.Shortcut{
	Service:           "drive",
	Command:           "+update-reply",
	Description:       "Update the content of a comment reply on doc/docx/sheet/file/slides/base(bitable)/apps, with URL parsing and Wiki token unwrapping",
	Risk:              "write",
	Scopes:            []string{"docs:document.comment:write_only"},
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: append(driveCommentTargetFlags(driveUpdateReplyOp),
		common.Flag{Name: "comment-id", Desc: "comment ID that owns the reply (from drive +list-comments)", Required: true},
		common.Flag{Name: "reply-id", Desc: "reply ID to update (from drive +list-replies)", Required: true},
		common.Flag{Name: "content", Desc: "reply_elements JSON string, same format as drive +add-comment", Required: true, Input: []string{common.File, common.Stdin}},
	),
	Tips: []string{
		"--content uses the same JSON as `drive +add-comment`: '[{\"type\":\"text\",\"text\":\"正文\"}]' (types: text, mention_user, link).",
		"The update replaces the whole reply content; there is no partial edit.",
		"Reply IDs come from `drive +list-replies` (items[].reply_id); updating a comment's root reply rewrites the comment body itself.",
		"Only the identity that created a reply can update it; other identities get API error 1069303 (forbidden). Use the same --as identity that created the reply.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readDriveUpdateReplySpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDriveUpdateReplySpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildDriveUpdateReplyDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveUpdateReplySpec(runtime)
		if err != nil {
			return err
		}

		target, err := resolveDriveCommentTarget(ctx, runtime, driveUpdateReplyOp, spec.Ref)
		if err != nil {
			return err
		}

		path := fmt.Sprintf(
			"/open-apis/drive/v1/files/%s/comments/%s/replies/%s",
			validate.EncodePathSegment(target.FileToken),
			validate.EncodePathSegment(spec.CommentID),
			validate.EncodePathSegment(spec.ReplyID),
		)
		if _, err := runtime.CallAPITyped(
			"PUT",
			path,
			map[string]interface{}{"file_type": target.FileType},
			spec.RequestBody(),
		); err != nil {
			return err
		}

		runtime.Out(driveCommentTargetOutput(target, map[string]interface{}{
			"comment_id": spec.CommentID,
			"reply_id":   spec.ReplyID,
			"updated":    true,
		}), nil)
		return nil
	},
}

func readDriveUpdateReplySpec(runtime *common.RuntimeContext) (driveUpdateReplySpec, error) {
	ref, err := resolveDriveCommentInput(driveUpdateReplyOp, runtime.Str("url"), runtime.Str("token"), runtime.Str("type"))
	if err != nil {
		return driveUpdateReplySpec{}, err
	}
	commentID := strings.TrimSpace(runtime.Str("comment-id"))
	if err := validateDriveCommentPathID(commentID, "--comment-id"); err != nil {
		return driveUpdateReplySpec{}, err
	}
	replyID := strings.TrimSpace(runtime.Str("reply-id"))
	if err := validateDriveCommentPathID(replyID, "--reply-id"); err != nil {
		return driveUpdateReplySpec{}, err
	}
	replyElements, err := parseCommentReplyElements(runtime.Str("content"))
	if err != nil {
		return driveUpdateReplySpec{}, err
	}
	return driveUpdateReplySpec{
		Ref:           ref,
		CommentID:     commentID,
		ReplyID:       replyID,
		ReplyElements: replyElements,
	}, nil
}

func buildDriveUpdateReplyDryRun(spec driveUpdateReplySpec) *common.DryRunAPI {
	if spec.Ref.Type == "wiki" {
		return common.NewDryRunAPI().
			Desc("2-step orchestration: resolve wiki -> update comment reply").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to underlying document").
			Params(map[string]interface{}{"token": spec.Ref.Token}).
			PUT("/open-apis/drive/v1/files/<obj_token from step 1>/comments/:comment_id/replies/:reply_id").
			Desc("[2] Update reply content on resolved document").
			Params(map[string]interface{}{"file_type": "<obj_type from step 1>"}).
			Body(spec.RequestBody()).
			Set("comment_id", spec.CommentID).
			Set("reply_id", spec.ReplyID)
	}

	return common.NewDryRunAPI().
		Desc("1-step request: update comment reply").
		PUT("/open-apis/drive/v1/files/:file_token/comments/:comment_id/replies/:reply_id").
		Params(map[string]interface{}{"file_type": spec.Ref.Type}).
		Body(spec.RequestBody()).
		Set("file_token", spec.Ref.Token).
		Set("comment_id", spec.CommentID).
		Set("reply_id", spec.ReplyID)
}
