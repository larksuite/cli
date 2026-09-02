// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var validCommandsV2 = map[string]bool{
	"str_replace":             true,
	"block_delete":            true,
	"block_insert_after":      true,
	"block_copy_insert_after": true,
	"block_replace":           true,
	"block_move_after":        true,
	"overwrite":               true,
	"append":                  true,
}

const docsReferenceMapFlagDesc = "Structured `reference_map` JSON object; must be used with `--content`. Prefer embedding structure directly in the document body for ordinary writes; use `--reference-map` primarily to preserve or replay an existing `document.reference_map`. Accepts inline JSON, `@reference-map.json` (relative path), or `-` to read from stdin."

const docsUpdateReferenceMapFlagDesc = docsReferenceMapFlagDesc

const docsUpdateBlockIDFlagDesc = "target block ID(s) for block operations (comma-separated for multi-block replace or batch delete); sentinel values are passed through to the service: -1 means document end where supported, 0 means document start where supported"

const docsUpdateCommandFlagDesc = "operation; str_replace does not support resource replacement; prefer block_replace when multiple blocks are involved; requirements: str_replace(--pattern), block_delete(--block-id or --start-block-id/--end-block-id), block_insert_after(--block-id,--content), block_replace(--content plus --block-id or --start-block-id/--end-block-id), block_copy_insert_after/block_move_after(--block-id,--src-block-ids), overwrite/append(--content)"

// v2UpdateFlags returns the flag definitions for the v2 (OpenAPI) update path.
func v2UpdateFlags() []common.Flag {
	return []common.Flag{
		{Name: "command", Desc: docsUpdateCommandFlagDesc, Enum: validCommandsV2Keys()},
		{Name: "doc-format", Desc: "content format for --content; xml is default for precise rich edits, markdown for user-provided Markdown or plain append/overwrite", Default: "xml", Enum: []string{"xml", "markdown"}},
		{Name: "content", Desc: docsUpdateContentFlagBase, Input: []string{common.File, common.Stdin}},
		{Name: "reference-map", Desc: docsUpdateReferenceMapFlagDesc, Input: []string{common.File, common.Stdin}},
		{Name: "pattern", Desc: "simple inline text matched by str_replace; use block_replace for paragraphs, multiline content, or multiple blocks"},
		{Name: "block-id", Desc: docsUpdateBlockIDFlagDesc},
		{Name: "start-block-id", Desc: "inclusive start block ID for a block_replace or block_delete sibling range; requires --end-block-id and cannot be combined with --block-id; 0 means document start"},
		{Name: "end-block-id", Desc: "inclusive end block ID for a block_replace or block_delete sibling range; requires --start-block-id and cannot be combined with --block-id; -1 means document end"},
		{Name: "src-block-ids", Desc: "comma-separated source block ids for block_copy_insert_after and block_move_after"},
		{Name: "revision-id", Desc: "base revision id; -1 means latest", Type: "int", Default: "-1"},
	}
}

func validCommandsV2Keys() []string {
	return []string{"str_replace", "block_delete", "block_insert_after", "block_copy_insert_after", "block_replace", "block_move_after", "overwrite", "append"}
}

func validateUpdateV2(_ context.Context, runtime *common.RuntimeContext) error {
	if err := validateDocsV2Only(runtime, "+update", docsUpdateLegacyFlags()); err != nil {
		return err
	}
	docRef, err := parseDocumentRef(runtime.Str("doc"))
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --doc: %v", err).WithParam("--doc")
	}
	cmd := runtime.Str("command")
	if cmd == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command is required").WithParam("--command")
	}
	if !validCommandsV2[cmd] {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --command %q, valid: str_replace | block_delete | block_insert_after | block_copy_insert_after | block_replace | block_move_after | overwrite | append", cmd).WithParam("--command")
	}
	content := runtime.Str("content")
	if err := validateUpdateReferenceMap(runtime, cmd, content); err != nil {
		return err
	}
	pattern := runtime.Str("pattern")
	blockID := strings.TrimSpace(runtime.Str("block-id"))
	startBlockID := runtime.Str("start-block-id")
	endBlockID := runtime.Str("end-block-id")
	hasStartBlockID := strings.TrimSpace(startBlockID) != ""
	hasEndBlockID := strings.TrimSpace(endBlockID) != ""
	hasBlockRange := hasStartBlockID || hasEndBlockID
	srcBlockIDs := runtime.Str("src-block-ids")
	isBlockMutation := cmd == "block_replace" || cmd == "block_delete"
	if hasBlockRange && !isBlockMutation {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--start-block-id and --end-block-id are only supported with --command block_replace or block_delete").
			WithParams(blockRangeInvalidParams(startBlockID, endBlockID)...)
	}
	if isBlockMutation {
		if blockID != "" && hasBlockRange {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--command %s accepts either --block-id or --start-block-id with --end-block-id, not both", cmd).
				WithParams(
					errs.InvalidParam{Name: "--block-id", Reason: "remove --block-id when using a range"},
					errs.InvalidParam{Name: "--start-block-id", Reason: "use the range pair without --block-id"},
					errs.InvalidParam{Name: "--end-block-id", Reason: "use the range pair without --block-id"},
				)
		}
		if hasBlockRange && (!hasStartBlockID || !hasEndBlockID) {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--command %s requires --start-block-id and --end-block-id together", cmd).
				WithParams(
					errs.InvalidParam{Name: "--start-block-id", Reason: "provide the inclusive range start"},
					errs.InvalidParam{Name: "--end-block-id", Reason: "provide the inclusive range end"},
				)
		}
		if blockID == "" && !hasBlockRange {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--command %s requires --block-id or both --start-block-id and --end-block-id", cmd).WithParam("--block-id")
		}
		if strings.TrimSpace(startBlockID) == "-1" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--start-block-id cannot be -1; -1 is only valid as --end-block-id").WithParam("--start-block-id")
		}
		if strings.TrimSpace(endBlockID) == "0" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--end-block-id cannot be 0; 0 is only valid as --start-block-id").WithParam("--end-block-id")
		}
	}

	switch cmd {
	case "str_replace":
		if pattern == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command str_replace requires --pattern").WithParam("--pattern")
		}
	case "block_delete":
		if content != "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command block_delete does not accept --content").WithParam("--content")
		}
	case "block_insert_after":
		if blockID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command block_insert_after requires --block-id").WithParam("--block-id")
		}
		if content == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command block_insert_after requires --content").WithParam("--content")
		}
	case "block_copy_insert_after":
		if blockID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command block_copy_insert_after requires --block-id").WithParam("--block-id")
		}
		if srcBlockIDs == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command block_copy_insert_after requires --src-block-ids").WithParam("--src-block-ids")
		}
	case "block_move_after":
		if blockID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command block_move_after requires --block-id").WithParam("--block-id")
		}
		if srcBlockIDs == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command block_move_after requires --src-block-ids").WithParam("--src-block-ids")
		}
		if content != "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command block_move_after does not accept --content; use --src-block-ids").WithParam("--content")
		}
	case "block_replace":
		if content == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command block_replace requires --content").WithParam("--content")
		}
	case "overwrite":
		if content == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command overwrite requires --content").WithParam("--content")
		}
	case "append":
		if content == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command append requires --content").WithParam("--content")
		}
	}
	if content != "" {
		input, err := resolveDocsV2ContentReferenceMap(runtime)
		if err != nil {
			return err
		}
		if len(input.LocalResources) > 0 {
			if err := validateLocalDocResourceUpdateCommand(cmd, input.LocalResources); err != nil {
				return err
			}
			if docRef.Kind == "doc" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument,
					"local images and files require a docx token/URL or a wiki URL that resolves to docx").
					WithParam("--doc")
			}
			return runtime.EnsureScopes(docsUpdateLocalResourceScopesFor(docRef))
		}
	}
	return nil
}

func dryRunUpdateV2(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	// Validate has already accepted --doc; parseDocumentRef cannot fail here.
	ref, _ := parseDocumentRef(runtime.Str("doc"))
	body, resources, err := buildUpdateBodyWithPreparedInput(runtime)
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}
	documentID := ref.Token
	dry := common.NewDryRunAPI()
	if len(resources) > 0 && ref.Kind == "wiki" {
		documentID = "<resolved_docx_token>"
		dry.GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("Resolve wiki node to its docx document before writing local resources").
			Params(map[string]interface{}{"token": ref.Token})
	}
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s", validate.EncodePathSegment(documentID))
	dry.PUT(apiPath).
		Desc("OpenAPI: update document").
		Body(body).
		Set("document_id", documentID)
	dry = appendRemoteDocImageDownloadsDryRun(dry, resources)
	return appendLocalDocResourcesDryRun(dry, documentID, resources)
}

func executeUpdateV2(_ context.Context, runtime *common.RuntimeContext) error {
	ref, _ := parseDocumentRef(runtime.Str("doc"))

	body, resources, err := buildUpdateBodyWithPreparedInput(runtime)
	if err != nil {
		return err
	}
	if err := validateRemoteDocImageSources(runtime.Ctx(), resources); err != nil {
		return err
	}
	documentID := ref.Token
	if len(resources) > 0 {
		documentID, err = resolveDocxDocumentID(runtime, runtime.Str("doc"))
		if err != nil {
			return err
		}
	}
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s", validate.EncodePathSegment(documentID))

	data, err := doDocAPI(runtime, "PUT", apiPath, body)
	if err != nil {
		return err
	}
	if docsAPIOperationFailed(data) {
		return runtime.OutPartialFailure(data, nil)
	}

	if err := finalizeLocalDocResources(runtime, documentID, data, resources); err != nil {
		return err
	}
	runtime.OutRaw(data, nil)
	return nil
}

func buildUpdateBody(runtime *common.RuntimeContext) map[string]interface{} {
	body, _ := buildUpdateBodyWithReferenceMap(runtime)
	return body
}

func buildUpdateBodyWithReferenceMap(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	body := buildUpdateBodyBase(runtime)
	if !runtime.Changed("reference-map") {
		return body, nil
	}
	refMap, err := parseUpdateReferenceMap(runtime.Str("reference-map"))
	if err != nil {
		return body, err
	}
	body["reference_map"] = refMap
	return body, nil
}

func buildUpdateBodyBase(runtime *common.RuntimeContext) map[string]interface{} {
	cmd := runtime.Str("command")

	// append is a shorthand for block_insert_after with block_id "-1" (end of document)
	blockID := runtime.Str("block-id")
	if cmd == "append" {
		cmd = "block_insert_after"
		blockID = "-1"
	}

	body := map[string]interface{}{
		"format":  runtime.Str("doc-format"),
		"command": cmd,
	}
	if v := runtime.Int("revision-id"); v != 0 {
		body["revision_id"] = v
	}
	if v := runtime.Str("content"); v != "" {
		body["content"] = v
	}
	if v := runtime.Str("pattern"); v != "" {
		body["pattern"] = v
	}
	if strings.TrimSpace(blockID) != "" {
		body["block_id"] = blockID
	}
	if v := runtime.Str("start-block-id"); strings.TrimSpace(v) != "" {
		body["start_block_id"] = v
	}
	if v := runtime.Str("end-block-id"); strings.TrimSpace(v) != "" {
		body["end_block_id"] = v
	}
	if v := runtime.Str("src-block-ids"); v != "" {
		body["src_block_ids"] = v
	}
	injectDocsScene(runtime, body)
	return body
}

func blockRangeInvalidParams(startBlockID, endBlockID string) []errs.InvalidParam {
	params := make([]errs.InvalidParam, 0, 2)
	if strings.TrimSpace(startBlockID) != "" {
		params = append(params, errs.InvalidParam{Name: "--start-block-id", Reason: "only valid with --command block_replace or block_delete"})
	}
	if strings.TrimSpace(endBlockID) != "" {
		params = append(params, errs.InvalidParam{Name: "--end-block-id", Reason: "only valid with --command block_replace or block_delete"})
	}
	return params
}

func validateUpdateReferenceMap(runtime *common.RuntimeContext, command string, content string) error {
	if !runtime.Changed("reference-map") {
		return nil
	}
	if !updateCommandAcceptsReferenceMap(command) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--reference-map is only supported with update commands that send --content").WithParam("--reference-map")
	}
	if content == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--reference-map requires --content that uses matching sidecar refs").WithParam("--reference-map")
	}
	_, err := parseUpdateReferenceMap(runtime.Str("reference-map"))
	return err
}

func updateCommandAcceptsReferenceMap(command string) bool {
	switch command {
	case "str_replace", "block_insert_after", "block_replace", "overwrite", "append":
		return true
	default:
		return false
	}
}

func parseUpdateReferenceMap(raw string) (map[string]interface{}, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--reference-map must be a non-empty JSON object").WithParam("--reference-map")
	}
	var refMap map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &refMap); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--reference-map must be a valid JSON object: %v", err).WithParam("--reference-map").WithCause(err)
	}
	if refMap == nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--reference-map must be a JSON object, got null").WithParam("--reference-map")
	}
	return refMap, nil
}
