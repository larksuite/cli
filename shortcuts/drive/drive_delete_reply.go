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

var driveDeleteReplyOp = driveCommentOp{
	Label: "reply delete",
	Types: []string{"doc", "docx", "sheet", "file", "slides", "bitable", "apps"},
}

type driveDeleteReplySpec struct {
	Ref       driveCommentRef
	CommentID string
	ReplyID   string
}

// DriveDeleteReply deletes a reply of a comment through the Drive comment
// reply delete API, while accepting Wiki URLs/tokens and resolving them to
// the underlying object.
var DriveDeleteReply = common.Shortcut{
	Service:           "drive",
	Command:           "+delete-reply",
	Description:       "Delete a reply of a comment on doc/docx/sheet/file/slides/base(bitable)/apps, with URL parsing and Wiki token unwrapping",
	Risk:              "high-risk-write",
	Scopes:            []string{"docs:document.comment:write_only"},
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: append(driveCommentTargetFlags(driveDeleteReplyOp),
		common.Flag{Name: "comment-id", Desc: "comment ID the reply belongs to (from drive +list-comments)", Required: true},
		common.Flag{Name: "reply-id", Desc: "reply ID to delete (from drive +list-comments items[].reply_list.replies[].reply_id)", Required: true},
	),
	Tips: []string{
		"Reply IDs come from `drive +list-comments` (items[].reply_list.replies[].reply_id).",
		"Deletion is permanent; there is no undo or trash for comment replies.",
		"Wiki URLs/tokens are resolved to the underlying document automatically.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readDriveDeleteReplySpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDriveDeleteReplySpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildDriveDeleteReplyDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveDeleteReplySpec(runtime)
		if err != nil {
			return err
		}

		target, err := resolveDriveCommentTarget(ctx, runtime, driveDeleteReplyOp, spec.Ref)
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
			"DELETE",
			path,
			map[string]interface{}{"file_type": target.FileType},
			nil,
		); err != nil {
			return err
		}

		runtime.Out(driveCommentTargetOutput(target, map[string]interface{}{
			"comment_id": spec.CommentID,
			"reply_id":   spec.ReplyID,
			"deleted":    true,
		}), nil)
		return nil
	},
}

func readDriveDeleteReplySpec(runtime *common.RuntimeContext) (driveDeleteReplySpec, error) {
	ref, err := resolveDriveCommentInput(driveDeleteReplyOp, runtime.Str("url"), runtime.Str("token"), runtime.Str("type"))
	if err != nil {
		return driveDeleteReplySpec{}, err
	}
	commentID := strings.TrimSpace(runtime.Str("comment-id"))
	if err := validateDriveCommentPathID(commentID, "--comment-id"); err != nil {
		return driveDeleteReplySpec{}, err
	}
	replyID := strings.TrimSpace(runtime.Str("reply-id"))
	if err := validateDriveCommentPathID(replyID, "--reply-id"); err != nil {
		return driveDeleteReplySpec{}, err
	}
	return driveDeleteReplySpec{
		Ref:       ref,
		CommentID: commentID,
		ReplyID:   replyID,
	}, nil
}

func buildDriveDeleteReplyDryRun(spec driveDeleteReplySpec) *common.DryRunAPI {
	if spec.Ref.Type == "wiki" {
		return common.NewDryRunAPI().
			Desc("2-step orchestration: resolve wiki -> delete reply").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to underlying document").
			Params(map[string]interface{}{"token": spec.Ref.Token}).
			DELETE("/open-apis/drive/v1/files/<obj_token from step 1>/comments/:comment_id/replies/:reply_id").
			Desc("[2] Delete reply on resolved document").
			Params(map[string]interface{}{"file_type": "<obj_type from step 1>"}).
			Set("comment_id", spec.CommentID).
			Set("reply_id", spec.ReplyID)
	}

	return common.NewDryRunAPI().
		Desc("1-step request: delete reply").
		DELETE("/open-apis/drive/v1/files/:file_token/comments/:comment_id/replies/:reply_id").
		Params(map[string]interface{}{"file_type": spec.Ref.Type}).
		Set("file_token", spec.Ref.Token).
		Set("comment_id", spec.CommentID).
		Set("reply_id", spec.ReplyID)
}
