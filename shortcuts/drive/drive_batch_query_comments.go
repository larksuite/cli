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

// driveBatchQueryCommentsMaxIDs mirrors the server-side cap on comment_ids
// per batch_query call.
const driveBatchQueryCommentsMaxIDs = 100

var driveBatchQueryCommentsOp = driveCommentOp{
	Label: "comments batch query",
	Types: []string{"doc", "docx", "sheet", "file", "slides", "bitable", "apps"},
}

type driveBatchQueryCommentsSpec struct {
	Ref          driveCommentRef
	CommentIDs   []string
	NeedReaction bool
	NeedRelation bool
}

// RequestBody assembles the batch_query body for the resolved fileType.
// need_relation is absent from the platform metadata for this endpoint but
// honored live (same undocumented parameter +list-comments already uses);
// only docx returns relation data, so it is sent for docx targets only.
func (s driveBatchQueryCommentsSpec) RequestBody(fileType string) map[string]interface{} {
	body := map[string]interface{}{
		"comment_ids": s.CommentIDs,
	}
	if s.NeedReaction {
		body["need_reaction"] = true
	}
	if s.NeedRelation && fileType == "docx" {
		body["need_relation"] = true
	}
	return body
}

// DriveBatchQueryComments fetches comments by ID through the Drive comment
// batch_query API, while accepting Wiki URLs/tokens and resolving them to the
// underlying object.
var DriveBatchQueryComments = common.Shortcut{
	Service:           "drive",
	Command:           "+batch-query-comments",
	Description:       "Batch get comments by comment ID for doc/docx/sheet/file/slides/base(bitable)/apps, with URL parsing and Wiki token unwrapping",
	Risk:              "read",
	Scopes:            []string{"docs:document.comment:read"},
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: append(driveCommentTargetFlags(driveBatchQueryCommentsOp),
		common.Flag{Name: "comment-ids", Type: "string_slice", Desc: fmt.Sprintf("comment IDs to fetch (comma-separated or repeated flag, max %d)", driveBatchQueryCommentsMaxIDs), Required: true},
		common.Flag{Name: "need-reaction", Type: "bool", Desc: "include reaction data on comment cards"},
		common.Flag{Name: "need-relation", Type: "bool", Desc: "include docx comment relation data; ignored for non-docx targets"},
	),
	Tips: []string{
		"Comment IDs come from `drive +list-comments` (items[].comment_id).",
		"--comment-ids accepts comma-separated values and repeated flags, up to 100 IDs per call.",
		"--need-relation returns the docx comment anchor (items[].relation with the block position); see the lark-drive comment-location guide.",
		"Wiki URLs/tokens are resolved to the underlying document automatically.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readDriveBatchQueryCommentsSpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDriveBatchQueryCommentsSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildDriveBatchQueryCommentsDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveBatchQueryCommentsSpec(runtime)
		if err != nil {
			return err
		}

		target, err := resolveDriveCommentTarget(ctx, runtime, driveBatchQueryCommentsOp, spec.Ref)
		if err != nil {
			return err
		}

		path := fmt.Sprintf("/open-apis/drive/v1/files/%s/comments/batch_query", validate.EncodePathSegment(target.FileToken))
		data, err := runtime.CallAPITyped(
			"POST",
			path,
			map[string]interface{}{"file_type": target.FileType},
			spec.RequestBody(target.FileType),
		)
		if err != nil {
			return err
		}

		items := driveCommentItems(data)
		runtime.Out(driveCommentTargetOutput(target, map[string]interface{}{
			"items": items,
			"count": len(items),
		}), nil)
		return nil
	},
}

func readDriveBatchQueryCommentsSpec(runtime *common.RuntimeContext) (driveBatchQueryCommentsSpec, error) {
	ref, err := resolveDriveCommentInput(driveBatchQueryCommentsOp, runtime.Str("url"), runtime.Str("token"), runtime.Str("type"))
	if err != nil {
		return driveBatchQueryCommentsSpec{}, err
	}
	ids, err := normalizeDriveCommentIDs(runtime.StrSlice("comment-ids"))
	if err != nil {
		return driveBatchQueryCommentsSpec{}, err
	}
	return driveBatchQueryCommentsSpec{
		Ref:          ref,
		CommentIDs:   ids,
		NeedReaction: runtime.Bool("need-reaction"),
		NeedRelation: runtime.Bool("need-relation"),
	}, nil
}

func normalizeDriveCommentIDs(raw []string) ([]string, error) {
	ids := make([]string, 0, len(raw))
	for i, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--comment-ids element #%d is empty", i+1).WithParam("--comment-ids")
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--comment-ids must contain at least one comment ID").WithParam("--comment-ids")
	}
	if len(ids) > driveBatchQueryCommentsMaxIDs {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--comment-ids accepts at most %d comment IDs per call (got %d)", driveBatchQueryCommentsMaxIDs, len(ids)).WithParam("--comment-ids")
	}
	return ids, nil
}

func buildDriveBatchQueryCommentsDryRun(spec driveBatchQueryCommentsSpec) *common.DryRunAPI {
	if spec.Ref.Type == "wiki" {
		// The wiki obj_type is unknown until step 1 resolves, so RequestBody
		// cannot decide the docx-only need_relation gate here; surface it as a
		// placeholder the same way +list-comments does.
		body := spec.RequestBody("<obj_type from step 1>")
		if spec.NeedRelation {
			body["need_relation"] = "<sent only when obj_type is docx>"
		}
		return common.NewDryRunAPI().
			Desc("2-step orchestration: resolve wiki -> batch query comments").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to underlying document").
			Params(map[string]interface{}{"token": spec.Ref.Token}).
			POST("/open-apis/drive/v1/files/<obj_token from step 1>/comments/batch_query").
			Desc("[2] Batch query comments on resolved document").
			Params(map[string]interface{}{"file_type": "<obj_type from step 1>"}).
			Body(body)
	}

	return common.NewDryRunAPI().
		Desc("1-step request: batch query comments").
		POST("/open-apis/drive/v1/files/:file_token/comments/batch_query").
		Params(map[string]interface{}{"file_type": spec.Ref.Type}).
		Body(spec.RequestBody(spec.Ref.Type)).
		Set("file_token", spec.Ref.Token)
}
