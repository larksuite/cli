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

const (
	driveListCommentsDefaultPageSize     = 50
	driveListCommentsDefaultSolvedStatus = "false"
	driveListCommentsDefaultScope        = "all"
)

var driveListCommentsTypes = []string{"doc", "docx", "sheet", "file", "slides", "bitable", "base", "wiki"}

type driveListCommentsRef struct {
	Token      string
	Type       string
	SourceFlag string
}

type driveListCommentsTarget struct {
	FileToken string
	FileType  string
}

type driveListCommentsSpec struct {
	Ref          driveListCommentsRef
	PageSize     int
	PageToken    string
	SolvedStatus string
	CommentScope string
	NeedReaction bool
	NeedRelation bool
}

// DriveListComments lists document comments through the Drive comments API,
// while accepting Wiki URLs/tokens and resolving them to the underlying object.
var DriveListComments = common.Shortcut{
	Service:           "drive",
	Command:           "+list-comments",
	Description:       "List comments for doc/docx/sheet/file/slides/base(bitable), with URL parsing and Wiki token unwrapping",
	Risk:              "read",
	Scopes:            []string{"docs:document.comment:read"},
	ConditionalScopes: []string{"wiki:node:retrieve"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "recommended: Lark/Feishu document URL (doc/docx/sheet/file/slides/base/bitable/wiki); Wiki URLs are unwrapped automatically"},
		{Name: "token", Desc: "document token, Wiki token, or document URL; bare tokens require --type"},
		{Name: "type", Desc: "document type for bare --token; optional for URLs but must match the URL type when provided", Enum: driveListCommentsTypes},
		{Name: "solved-status", Default: driveListCommentsDefaultSolvedStatus, Desc: "comment solved filter: false=unresolved, true=solved, all=all comments", Enum: []string{"false", "true", "all"}},
		{Name: "comment-scope", Default: driveListCommentsDefaultScope, Desc: "comment scope filter: all=all comments, whole=full-document comments, partial=local/selection comments", Enum: []string{"all", "whole", "partial"}},
		{Name: "need-reaction", Type: "bool", Desc: "include reaction data on comment cards"},
		{Name: "need-relation", Type: "bool", Desc: "include docx comment relation data; ignored for non-docx targets"},
		{Name: "page-size", Type: "int", Default: "50", Desc: "page size, 1-100"},
		{Name: "page-token", Desc: "pagination token from previous response"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveListCommentsSpec(runtime)
		if err != nil {
			return err
		}
		return validateDriveListCommentsSpec(spec)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDriveListCommentsSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		if err := validateDriveListCommentsSpec(spec); err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildDriveListCommentsDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveListCommentsSpec(runtime)
		if err != nil {
			return err
		}
		if err := validateDriveListCommentsSpec(spec); err != nil {
			return err
		}

		target, err := resolveDriveListCommentsTarget(ctx, runtime, spec.Ref)
		if err != nil {
			return err
		}
		params := buildDriveListCommentsParams(spec, target.FileType)
		path := fmt.Sprintf("/open-apis/drive/v1/files/%s/comments", validate.EncodePathSegment(target.FileToken))

		data, err := runtime.CallAPITyped("GET", path, params, nil)
		if err != nil {
			return err
		}
		runtime.Out(buildDriveListCommentsOutput(target, data), nil)
		return nil
	},
}

func readDriveListCommentsSpec(runtime *common.RuntimeContext) (driveListCommentsSpec, error) {
	ref, err := resolveDriveListCommentsInput(runtime.Str("url"), runtime.Str("token"), runtime.Str("type"))
	if err != nil {
		return driveListCommentsSpec{}, err
	}
	return driveListCommentsSpec{
		Ref:          ref,
		PageSize:     runtime.Int("page-size"),
		PageToken:    strings.TrimSpace(runtime.Str("page-token")),
		SolvedStatus: strings.TrimSpace(runtime.Str("solved-status")),
		CommentScope: strings.TrimSpace(runtime.Str("comment-scope")),
		NeedReaction: runtime.Bool("need-reaction"),
		NeedRelation: runtime.Bool("need-relation"),
	}, nil
}

func validateDriveListCommentsSpec(spec driveListCommentsSpec) error {
	if spec.PageSize < 1 || spec.PageSize > 100 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-size must be between 1 and 100").WithParam("--page-size")
	}
	if _, ok := driveListCommentsSolvedStatusParam(spec.SolvedStatus); !ok {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --solved-status %q; allowed: false, true, all", spec.SolvedStatus).WithParam("--solved-status")
	}
	if _, ok := driveListCommentsScopeParam(spec.CommentScope); !ok {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --comment-scope %q; allowed: all, whole, partial", spec.CommentScope).WithParam("--comment-scope")
	}
	return nil
}

func resolveDriveListCommentsInput(urlInput, tokenInput, explicitType string) (driveListCommentsRef, error) {
	urlInput = strings.TrimSpace(urlInput)
	tokenInput = strings.TrimSpace(tokenInput)
	if urlInput != "" && tokenInput != "" {
		return driveListCommentsRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--url and --token are mutually exclusive; pass one input only").WithParam("--url")
	}
	if urlInput == "" && tokenInput == "" {
		return driveListCommentsRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --url or --token").WithParam("--url")
	}

	raw := urlInput
	sourceFlag := "--url"
	if raw == "" {
		raw = tokenInput
		sourceFlag = "--token"
	}
	inputType := normalizeDriveListCommentsType(strings.ToLower(strings.TrimSpace(explicitType)))

	if ref, ok := common.ParseResourceURL(raw); ok {
		refType := normalizeDriveListCommentsType(ref.Type)
		if inputType != "" && inputType != refType {
			return driveListCommentsRef{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--type %q conflicts with URL path type %q; remove --type or use a matching value",
				inputType,
				refType,
			).WithParam("--type")
		}
		if !driveListCommentsTypeSupported(refType) {
			return driveListCommentsRef{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"unsupported %s resource type %q; comments list supports doc, docx, sheet, file, slides, bitable/base, and wiki",
				sourceFlag,
				refType,
			).WithParam(sourceFlag)
		}
		return driveListCommentsRef{Token: ref.Token, Type: refType, SourceFlag: sourceFlag}, nil
	}

	if strings.Contains(raw, "://") {
		return driveListCommentsRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported %s URL %q: use a recognized Lark document URL or pass a bare token with --type", sourceFlag, raw).WithParam(sourceFlag)
	}
	if strings.ContainsAny(raw, "/?#") {
		return driveListCommentsRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid bare token %q: remove path/query fragments or pass a recognized Lark document URL", raw).WithParam(sourceFlag)
	}
	if inputType == "" {
		return driveListCommentsRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--type is required when %s is a bare token (allowed: doc, docx, sheet, file, slides, bitable, base, wiki)", sourceFlag).WithParam("--type")
	}
	if !driveListCommentsTypeSupported(inputType) {
		return driveListCommentsRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --type %q; allowed: doc, docx, sheet, file, slides, bitable, base, wiki", inputType).WithParam("--type")
	}
	return driveListCommentsRef{Token: raw, Type: inputType, SourceFlag: sourceFlag}, nil
}

func normalizeDriveListCommentsType(docType string) string {
	switch strings.TrimSpace(docType) {
	case "base":
		return "bitable"
	default:
		return strings.TrimSpace(docType)
	}
}

func driveListCommentsTypeSupported(docType string) bool {
	switch normalizeDriveListCommentsType(docType) {
	case "doc", "docx", "sheet", "file", "slides", "bitable", "wiki":
		return true
	default:
		return false
	}
}

func resolveDriveListCommentsTarget(ctx context.Context, runtime *common.RuntimeContext, ref driveListCommentsRef) (driveListCommentsTarget, error) {
	if ref.Type != "wiki" {
		return driveListCommentsTarget{FileToken: ref.Token, FileType: ref.Type}, nil
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Resolving wiki node: %s\n", common.MaskToken(ref.Token))
	data, err := runtime.CallAPITyped(
		"GET",
		"/open-apis/wiki/v2/spaces/get_node",
		map[string]interface{}{"token": ref.Token},
		nil,
	)
	if err != nil {
		return driveListCommentsTarget{}, err
	}

	node := common.GetMap(data, "node")
	objType := normalizeDriveListCommentsType(common.GetString(node, "obj_type"))
	objToken := common.GetString(node, "obj_token")
	if objType == "" || objToken == "" {
		return driveListCommentsTarget{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki get_node returned incomplete node data")
	}
	if !driveListCommentsTypeSupported(objType) || objType == "wiki" {
		return driveListCommentsTarget{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"wiki resolved to %q, but comments list only supports doc, docx, sheet, file, slides, and bitable",
			objType,
		).WithParam(ref.SourceFlag)
	}
	fmt.Fprintf(runtime.IO().ErrOut, "Resolved wiki to %s: %s\n", objType, common.MaskToken(objToken))
	return driveListCommentsTarget{FileToken: objToken, FileType: objType}, nil
}

func buildDriveListCommentsDryRun(spec driveListCommentsSpec) *common.DryRunAPI {
	if spec.Ref.Type == "wiki" {
		params := buildDriveListCommentsParams(spec, "<obj_type from step 1>")
		if spec.NeedRelation {
			params["need_relation"] = "<sent only when obj_type is docx>"
		}
		return common.NewDryRunAPI().
			Desc("2-step orchestration: resolve wiki -> list comments").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to underlying document").
			Params(map[string]interface{}{"token": spec.Ref.Token}).
			GET("/open-apis/drive/v1/files/<obj_token from step 1>/comments").
			Desc("[2] List comments on resolved document").
			Params(params)
	}

	return common.NewDryRunAPI().
		Desc("1-step request: list comments").
		GET("/open-apis/drive/v1/files/:file_token/comments").
		Params(buildDriveListCommentsParams(spec, spec.Ref.Type)).
		Set("file_token", spec.Ref.Token)
}

func buildDriveListCommentsParams(spec driveListCommentsSpec, fileType string) map[string]interface{} {
	params := map[string]interface{}{
		"file_type": fileType,
		"page_size": spec.PageSize,
	}
	if spec.PageToken != "" {
		params["page_token"] = spec.PageToken
	}
	if value, ok := driveListCommentsSolvedStatusParam(spec.SolvedStatus); ok && value != nil {
		params["is_solved"] = *value
	}
	if value, ok := driveListCommentsScopeParam(spec.CommentScope); ok && value != nil {
		params["is_whole"] = *value
	}
	if spec.NeedReaction {
		params["need_reaction"] = true
	}
	if spec.NeedRelation && fileType == "docx" {
		params["need_relation"] = true
	}
	return params
}

func driveListCommentsSolvedStatusParam(status string) (*bool, bool) {
	switch strings.TrimSpace(status) {
	case "false", "":
		value := false
		return &value, true
	case "true":
		value := true
		return &value, true
	case "all":
		return nil, true
	default:
		return nil, false
	}
}

func driveListCommentsScopeParam(scope string) (*bool, bool) {
	switch strings.TrimSpace(scope) {
	case "all", "":
		return nil, true
	case "whole":
		value := true
		return &value, true
	case "partial":
		value := false
		return &value, true
	default:
		return nil, false
	}
}

func buildDriveListCommentsOutput(target driveListCommentsTarget, data map[string]interface{}) map[string]interface{} {
	items := common.GetSlice(data, "items")
	return map[string]interface{}{
		"file_token": target.FileToken,
		"file_type":  target.FileType,
		"items":      items,
		"has_more":   common.GetBool(data, "has_more"),
		"page_token": common.GetString(data, "page_token"),
		"count":      len(items),
	}
}
