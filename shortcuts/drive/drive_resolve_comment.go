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

type driveCommentSolvedSpec struct {
	Ref       driveCommentRef
	CommentID string
	Solved    bool
}

func (s driveCommentSolvedSpec) RequestBody() map[string]interface{} {
	return map[string]interface{}{
		"is_solved": s.Solved,
	}
}

// driveCommentSolvedConfig parameterizes the two solved-state shortcuts:
// they share one PATCH endpoint whose body is only {is_solved}, so the
// commands differ solely in direction and wording.
type driveCommentSolvedConfig struct {
	Command     string
	Description string
	Label       string // driveCommentOp label used in unsupported-type errors
	Action      string // echoed in output and dry-run descriptions
	Verb        string // progress-line verb
	Solved      bool
	Tip         string // direction-specific tip (counterpart pointer)
}

// DriveResolveComment marks a comment solved through the Drive comment patch
// API, while accepting Wiki URLs/tokens and resolving them to the underlying
// object. Reopening is the separate +restore-comment command.
var DriveResolveComment = newDriveCommentSolvedShortcut(driveCommentSolvedConfig{
	Command:     "+resolve-comment",
	Description: "Resolve (mark solved) a comment on doc/docx/sheet/file/slides/base(bitable)/apps, with URL parsing and Wiki token unwrapping",
	Label:       "comment resolve",
	Action:      "resolve",
	Verb:        "Resolving",
	Solved:      true,
	Tip:         "To reopen a solved comment, use `drive +restore-comment`.",
})

// DriveRestoreComment reopens a solved comment through the same Drive comment
// patch API (is_solved=false).
var DriveRestoreComment = newDriveCommentSolvedShortcut(driveCommentSolvedConfig{
	Command:     "+restore-comment",
	Description: "Restore (reopen) a solved comment on doc/docx/sheet/file/slides/base(bitable)/apps, with URL parsing and Wiki token unwrapping",
	Label:       "comment restore",
	Action:      "restore",
	Verb:        "Restoring",
	Solved:      false,
	Tip:         "To mark a comment solved, use `drive +resolve-comment`.",
})

func newDriveCommentSolvedShortcut(cfg driveCommentSolvedConfig) common.Shortcut {
	op := driveCommentOp{
		Label: cfg.Label,
		Types: []string{"doc", "docx", "sheet", "file", "slides", "bitable", "apps"},
	}
	readSpec := func(runtime *common.RuntimeContext) (driveCommentSolvedSpec, error) {
		ref, err := resolveDriveCommentInput(op, runtime.Str("url"), runtime.Str("token"), runtime.Str("type"))
		if err != nil {
			return driveCommentSolvedSpec{}, err
		}
		commentID := strings.TrimSpace(runtime.Str("comment-id"))
		if err := validateDriveCommentPathID(commentID, "--comment-id"); err != nil {
			return driveCommentSolvedSpec{}, err
		}
		return driveCommentSolvedSpec{Ref: ref, CommentID: commentID, Solved: cfg.Solved}, nil
	}

	return common.Shortcut{
		Service:           "drive",
		Command:           cfg.Command,
		Description:       cfg.Description,
		Risk:              "write",
		Scopes:            []string{"docs:document.comment:write_only"},
		ConditionalScopes: []string{"wiki:node:read"},
		AuthTypes:         []string{"user", "bot"},
		Flags: append(driveCommentTargetFlags(op),
			common.Flag{Name: "comment-id", Desc: fmt.Sprintf("comment ID to %s (from drive +list-comments)", cfg.Action), Required: true},
		),
		Tips: []string{
			"Comment IDs come from `drive +list-comments` (items[].comment_id).",
			cfg.Tip,
			"Back-to-back solved-state flips on the same comment can hit server rate limiting (HTTP 429); space out consecutive calls or retry after a short delay.",
			"Wiki URLs/tokens are resolved to the underlying document automatically.",
		},
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			_, err := readSpec(runtime)
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			spec, err := readSpec(runtime)
			if err != nil {
				return common.NewDryRunAPI().Set("error", err.Error())
			}
			return buildDriveCommentSolvedDryRun(cfg, spec)
		},
		Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
			spec, err := readSpec(runtime)
			if err != nil {
				return err
			}

			target, err := resolveDriveCommentTarget(ctx, runtime, op, spec.Ref)
			if err != nil {
				return err
			}

			path := fmt.Sprintf(
				"/open-apis/drive/v1/files/%s/comments/%s",
				validate.EncodePathSegment(target.FileToken),
				validate.EncodePathSegment(spec.CommentID),
			)
			if _, err := runtime.CallAPITyped(
				"PATCH",
				path,
				map[string]interface{}{"file_type": target.FileType},
				spec.RequestBody(),
			); err != nil {
				return err
			}

			runtime.Out(driveCommentTargetOutput(target, map[string]interface{}{
				"comment_id": spec.CommentID,
				"action":     cfg.Action,
				"is_solved":  spec.Solved,
				"updated":    true,
			}), nil)
			return nil
		},
	}
}

func buildDriveCommentSolvedDryRun(cfg driveCommentSolvedConfig, spec driveCommentSolvedSpec) *common.DryRunAPI {
	if spec.Ref.Type == "wiki" {
		return common.NewDryRunAPI().
			Desc(fmt.Sprintf("2-step orchestration: resolve wiki -> %s comment", cfg.Action)).
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to underlying document").
			Params(map[string]interface{}{"token": spec.Ref.Token}).
			PATCH("/open-apis/drive/v1/files/<obj_token from step 1>/comments/:comment_id").
			Desc(fmt.Sprintf("[2] %s comment (is_solved=%t) on resolved document", cfg.Verb, cfg.Solved)).
			Params(map[string]interface{}{"file_type": "<obj_type from step 1>"}).
			Body(spec.RequestBody()).
			Set("comment_id", spec.CommentID)
	}

	return common.NewDryRunAPI().
		Desc(fmt.Sprintf("1-step request: %s comment (is_solved=%t)", cfg.Action, cfg.Solved)).
		PATCH("/open-apis/drive/v1/files/:file_token/comments/:comment_id").
		Params(map[string]interface{}{"file_type": spec.Ref.Type}).
		Body(spec.RequestBody()).
		Set("file_token", spec.Ref.Token).
		Set("comment_id", spec.CommentID)
}
