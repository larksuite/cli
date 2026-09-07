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

var driveListRepliesOp = driveCommentOp{
	Label: "replies list",
	Types: []string{"doc", "docx", "sheet", "file", "slides", "bitable", "apps"},
}

type driveListRepliesSpec struct {
	Ref          driveCommentRef
	CommentID    string
	PageSize     int
	PageToken    string
	NeedReaction bool
}

// DriveListReplies lists the replies of one comment through the Drive comment
// reply list API, while accepting Wiki URLs/tokens and resolving them to the
// underlying object.
var DriveListReplies = common.Shortcut{
	Service:           "drive",
	Command:           "+list-replies",
	Description:       "List replies of a comment on doc/docx/sheet/file/slides/base(bitable)/apps, with URL parsing and Wiki token unwrapping",
	Risk:              "read",
	Scopes:            []string{"docs:document.comment:read"},
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: append(driveCommentTargetFlags(driveListRepliesOp),
		common.Flag{Name: "comment-id", Desc: "comment ID whose replies to list (from drive +list-comments)", Required: true},
		common.Flag{Name: "page-size", Type: "int", Default: "50", Desc: "page size, 1-100"},
		common.Flag{Name: "page-token", Desc: "pagination token from previous response"},
		common.Flag{Name: "need-reaction", Type: "bool", Desc: "include reaction data on replies"},
	),
	Tips: []string{
		"Comment IDs come from `drive +list-comments` (items[].comment_id).",
		"The root reply (the comment body itself) is the earliest-created reply: it is items[0] of the FIRST page only (no --page-token); items[0] of later pages is a regular reply.",
		"Wiki URLs/tokens are resolved to the underlying document automatically.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readDriveListRepliesSpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDriveListRepliesSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildDriveListRepliesDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveListRepliesSpec(runtime)
		if err != nil {
			return err
		}

		target, err := resolveDriveCommentTarget(ctx, runtime, driveListRepliesOp, spec.Ref)
		if err != nil {
			return err
		}

		path := fmt.Sprintf(
			"/open-apis/drive/v1/files/%s/comments/%s/replies",
			validate.EncodePathSegment(target.FileToken),
			validate.EncodePathSegment(spec.CommentID),
		)
		data, err := runtime.CallAPITyped(
			"GET",
			path,
			buildDriveListRepliesParams(spec, target.FileType),
			nil,
		)
		if err != nil {
			return err
		}

		items := driveCommentItems(data)
		runtime.Out(driveCommentTargetOutput(target, map[string]interface{}{
			"comment_id": spec.CommentID,
			"items":      items,
			"has_more":   common.GetBool(data, "has_more"),
			"page_token": common.GetString(data, "page_token"),
			"count":      len(items),
		}), nil)
		return nil
	},
}

func readDriveListRepliesSpec(runtime *common.RuntimeContext) (driveListRepliesSpec, error) {
	ref, err := resolveDriveCommentInput(driveListRepliesOp, runtime.Str("url"), runtime.Str("token"), runtime.Str("type"))
	if err != nil {
		return driveListRepliesSpec{}, err
	}
	commentID := strings.TrimSpace(runtime.Str("comment-id"))
	if err := validateDriveCommentPathID(commentID, "--comment-id"); err != nil {
		return driveListRepliesSpec{}, err
	}
	pageSize := runtime.Int("page-size")
	if pageSize < 1 || pageSize > 100 {
		return driveListRepliesSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-size must be between 1 and 100").WithParam("--page-size")
	}
	return driveListRepliesSpec{
		Ref:          ref,
		CommentID:    commentID,
		PageSize:     pageSize,
		PageToken:    strings.TrimSpace(runtime.Str("page-token")),
		NeedReaction: runtime.Bool("need-reaction"),
	}, nil
}

func buildDriveListRepliesParams(spec driveListRepliesSpec, fileType string) map[string]interface{} {
	params := map[string]interface{}{
		"file_type": fileType,
		"page_size": spec.PageSize,
	}
	if spec.PageToken != "" {
		params["page_token"] = spec.PageToken
	}
	if spec.NeedReaction {
		params["need_reaction"] = true
	}
	return params
}

func buildDriveListRepliesDryRun(spec driveListRepliesSpec) *common.DryRunAPI {
	if spec.Ref.Type == "wiki" {
		return common.NewDryRunAPI().
			Desc("2-step orchestration: resolve wiki -> list comment replies").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to underlying document").
			Params(map[string]interface{}{"token": spec.Ref.Token}).
			GET("/open-apis/drive/v1/files/<obj_token from step 1>/comments/:comment_id/replies").
			Desc("[2] List replies of comment on resolved document").
			Params(buildDriveListRepliesParams(spec, "<obj_type from step 1>")).
			Set("comment_id", spec.CommentID)
	}

	return common.NewDryRunAPI().
		Desc("1-step request: list comment replies").
		GET("/open-apis/drive/v1/files/:file_token/comments/:comment_id/replies").
		Params(buildDriveListRepliesParams(spec, spec.Ref.Type)).
		Set("file_token", spec.Ref.Token).
		Set("comment_id", spec.CommentID)
}
