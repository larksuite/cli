// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// driveCommentOp describes one comment-family shortcut for the shared
// --url/--token/--type input resolution. Label appears in error messages;
// Types lists the wire file_type values the underlying endpoint accepts.
// Wiki URLs/tokens are always accepted as input and unwrapped to the
// underlying document, which must then land in Types.
type driveCommentOp struct {
	Label string
	Types []string
}

func (op driveCommentOp) supports(fileType string) bool {
	return slices.Contains(op.Types, fileType)
}

// inputTypeList renders the values accepted as input (wire types plus wiki).
func (op driveCommentOp) inputTypeList() string {
	return strings.Join(op.flagEnum(), ", ")
}

// targetTypeList renders the wire types the endpoint accepts (wiki excluded).
func (op driveCommentOp) targetTypeList() string {
	return strings.Join(op.Types, ", ")
}

// flagEnum returns the Enum set for the --type flag: the endpoint's wire
// types plus wiki (resolved to a wire type before the API call) and the
// base product-name alias when bitable is supported (normalized to bitable).
func (op driveCommentOp) flagEnum() []string {
	enum := make([]string, 0, len(op.Types)+2)
	for _, t := range op.Types {
		enum = append(enum, t)
		if t == "bitable" {
			enum = append(enum, "base")
		}
	}
	return append(enum, "wiki")
}

// driveCommentRef is the parsed --url/--token/--type input before wiki unwrapping.
type driveCommentRef struct {
	Token      string
	Type       string
	SourceFlag string
}

// driveCommentTarget is the underlying document a comment API call targets.
type driveCommentTarget struct {
	FileToken string
	FileType  string
	WikiToken string // non-empty when the input was a wiki node
}

// driveCommentTargetFlags returns the shared --url/--token/--type flag trio
// used by the comment-family shortcuts that resolve a document target.
func driveCommentTargetFlags(op driveCommentOp) []common.Flag {
	return []common.Flag{
		{Name: "url", Desc: fmt.Sprintf("recommended: Lark/Feishu document URL (%s); Wiki URLs are unwrapped automatically", op.inputTypeList())},
		{Name: "token", Desc: "document token, Wiki token, or document URL; bare tokens require --type"},
		{Name: "type", Desc: "document type for bare --token; optional for URLs but must match the URL type when provided", Enum: op.flagEnum()},
	}
}

// resolveDriveCommentInput parses --url/--token/--type into a driveCommentRef,
// mirroring +list-comments input handling: --url and --token are mutually
// exclusive, URLs are parsed for type+token, bare tokens require --type, and
// wiki is always accepted for later unwrapping.
func resolveDriveCommentInput(op driveCommentOp, urlInput, tokenInput, explicitType string) (driveCommentRef, error) {
	urlInput = strings.TrimSpace(urlInput)
	tokenInput = strings.TrimSpace(tokenInput)
	if urlInput != "" && tokenInput != "" {
		return driveCommentRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--url and --token are mutually exclusive; pass one input only").WithParam("--url")
	}
	if urlInput == "" && tokenInput == "" {
		return driveCommentRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --url or --token").WithParam("--url")
	}

	raw := urlInput
	sourceFlag := "--url"
	if raw == "" {
		raw = tokenInput
		sourceFlag = "--token"
	}
	inputType := normalizeDriveCommentType(strings.ToLower(strings.TrimSpace(explicitType)))

	if ref, ok := common.ParseResourceURL(raw); ok {
		refType := normalizeDriveCommentType(ref.Type)
		if inputType != "" && inputType != refType {
			return driveCommentRef{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--type %q conflicts with URL path type %q; remove --type or use a matching value",
				inputType,
				refType,
			).WithParam("--type")
		}
		if refType != "wiki" && !op.supports(refType) {
			return driveCommentRef{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"unsupported %s resource type %q; %s supports %s",
				sourceFlag,
				refType,
				op.Label,
				op.inputTypeList(),
			).WithParam(sourceFlag)
		}
		return driveCommentRef{Token: ref.Token, Type: refType, SourceFlag: sourceFlag}, nil
	}

	if token, ok := parseDriveListCommentsAppsURL(raw); ok {
		const refType = "apps"
		if inputType != "" && inputType != refType {
			return driveCommentRef{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--type %q conflicts with URL path type %q; remove --type or use a matching value",
				inputType,
				refType,
			).WithParam("--type")
		}
		if !op.supports(refType) {
			return driveCommentRef{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"unsupported %s resource type %q; %s supports %s",
				sourceFlag,
				refType,
				op.Label,
				op.inputTypeList(),
			).WithParam(sourceFlag)
		}
		return driveCommentRef{Token: token, Type: refType, SourceFlag: sourceFlag}, nil
	}

	if strings.Contains(raw, "://") {
		return driveCommentRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported %s URL %q: use a recognized Lark document URL or pass a bare token with --type", sourceFlag, raw).WithParam(sourceFlag)
	}
	if strings.ContainsAny(raw, "/?#") {
		return driveCommentRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid bare token %q: remove path/query fragments or pass a recognized Lark document URL", raw).WithParam(sourceFlag)
	}
	if inputType == "" {
		return driveCommentRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--type is required when %s is a bare token (allowed: %s)", sourceFlag, op.inputTypeList()).WithParam("--type")
	}
	if inputType != "wiki" && !op.supports(inputType) {
		return driveCommentRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --type %q; allowed: %s", inputType, op.inputTypeList()).WithParam("--type")
	}
	return driveCommentRef{Token: raw, Type: inputType, SourceFlag: sourceFlag}, nil
}

// normalizeDriveCommentType maps compatibility aliases to wire values
// (base → bitable) so type checks and error messages use one vocabulary.
func normalizeDriveCommentType(docType string) string {
	switch strings.TrimSpace(docType) {
	case "base":
		return "bitable"
	default:
		return strings.TrimSpace(docType)
	}
}

// resolveDriveCommentTarget unwraps wiki refs to the underlying document via
// wiki get_node and validates the resolved type against op.Types.
func resolveDriveCommentTarget(ctx context.Context, runtime *common.RuntimeContext, op driveCommentOp, ref driveCommentRef) (driveCommentTarget, error) {
	if ref.Type != "wiki" {
		return driveCommentTarget{FileToken: ref.Token, FileType: ref.Type}, nil
	}

	data, err := runtime.CallAPITyped(
		"GET",
		"/open-apis/wiki/v2/spaces/get_node",
		map[string]interface{}{"token": ref.Token},
		nil,
	)
	if err != nil {
		return driveCommentTarget{}, err
	}

	node := common.GetMap(data, "node")
	objType := normalizeDriveCommentType(common.GetString(node, "obj_type"))
	objToken := common.GetString(node, "obj_token")
	if objType == "" || objToken == "" {
		return driveCommentTarget{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki get_node returned incomplete node data")
	}
	if objType == "wiki" || !op.supports(objType) {
		return driveCommentTarget{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"wiki resolved to %q, but %s only supports %s",
			objType,
			op.Label,
			op.targetTypeList(),
		).WithParam(ref.SourceFlag)
	}
	return driveCommentTarget{FileToken: objToken, FileType: objType, WikiToken: ref.Token}, nil
}

// validateDriveCommentPathID validates a comment/reply identifier destined
// for a URL path segment.
func validateDriveCommentPathID(value, flagName string) error {
	if strings.TrimSpace(value) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s must not be empty", flagName).WithParam(flagName)
	}
	if err := validate.ResourceName(strings.TrimSpace(value), flagName); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam(flagName)
	}
	return nil
}

// driveCommentItems extracts data.items for output, normalizing a missing or
// null field to an empty slice: emitting the server's shape verbatim would
// surface "items": null, which breaks jq consumers iterating .data.items[].
func driveCommentItems(data map[string]interface{}) []interface{} {
	if items := common.GetSlice(data, "items"); items != nil {
		return items
	}
	return []interface{}{}
}

// driveCommentTargetOutput assembles the output fields shared by the
// comment-family shortcuts: the resolved target plus the wiki origin, if any.
func driveCommentTargetOutput(target driveCommentTarget, extra map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"file_token": target.FileToken,
		"file_type":  target.FileType,
	}
	if target.WikiToken != "" {
		out["wiki_token"] = target.WikiToken
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}
