// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
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

// v2UpdateFlags returns the flag definitions for the v2 (OpenAPI) update path.
func v2UpdateFlags() []common.Flag {
	return []common.Flag{
		{Name: "command", Desc: "operation; requirements: str_replace(--pattern), block_delete(--block-id, comma-separated for batch), block_insert_after/block_replace(--block-id,--content), block_copy_insert_after/block_move_after(--block-id,--src-block-ids), overwrite/append(--content)", Enum: validCommandsV2Keys()},
		{Name: "doc-format", Desc: "content format for --content; xml is default for precise rich edits, markdown for user-provided Markdown or plain append/overwrite", Default: "xml", Enum: []string{"xml", "markdown"}},
		{Name: "content", Desc: "replacement or inserted content; XML by default or Markdown when --doc-format markdown; empty with str_replace deletes match. " + docsContentSkillHelp + "; use --help for the latest command flags", Input: []string{common.File, common.Stdin}},
		{Name: "reference-map", Desc: docsUpdateReferenceMapFlagDesc, Input: []string{common.File, common.Stdin}},
		{Name: "pattern", Desc: "str_replace match pattern; XML mode is inline text, Markdown mode can match multiline text"},
		{Name: "block-id", Desc: "target block ID(s) for block operations (comma-separated for batch delete); -1 means document end where supported"},
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
	if _, err := parseDocumentRef(runtime.Str("doc")); err != nil {
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
	blockID := runtime.Str("block-id")
	srcBlockIDs := runtime.Str("src-block-ids")

	switch cmd {
	case "str_replace":
		if pattern == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command str_replace requires --pattern").WithParam("--pattern")
		}
	case "block_delete":
		if blockID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command block_delete requires --block-id").WithParam("--block-id")
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
		if blockID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--command block_replace requires --block-id").WithParam("--block-id")
		}
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
		_, err := resolveDocsV2ContentReferenceMap(runtime)
		return err
	}
	return nil
}

func dryRunUpdateV2(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	// Validate has already accepted --doc; parseDocumentRef cannot fail here.
	ref, _ := parseDocumentRef(runtime.Str("doc"))
	body, err := buildUpdateBodyWithHTML5ReferenceMap(runtime)
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s", ref.Token)
	return common.NewDryRunAPI().
		PUT(apiPath).
		Desc("OpenAPI: update document").
		Body(body).
		Set("document_id", ref.Token)
}

func executeUpdateV2(_ context.Context, runtime *common.RuntimeContext) error {
	ref, _ := parseDocumentRef(runtime.Str("doc"))

	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s", ref.Token)
	body, err := buildUpdateBodyWithHTML5ReferenceMap(runtime)
	if err != nil {
		return err
	}

	data, err := doDocAPI(runtime, "PUT", apiPath, body)
	if err != nil {
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
	if blockID != "" {
		body["block_id"] = blockID
	}
	if v := runtime.Str("src-block-ids"); v != "" {
		body["src_block_ids"] = v
	}
	injectDocsScene(runtime, body)
	return body
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
