// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"

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

// v2UpdateFlags returns the flag definitions for the v2 (OpenAPI) update path.
func v2UpdateFlags() []common.Flag {
	return []common.Flag{
		{Name: "command", Desc: "operation: str_replace | block_delete | block_insert_after | block_copy_insert_after | block_replace | block_move_after | overwrite | append", Hidden: true, Enum: validCommandsV2Keys()},
		{Name: "doc-format", Desc: "content format (prefer XML)", Hidden: true, Default: "xml", Enum: []string{"xml", "markdown"}},
		{Name: "content", Desc: "new content (XML or Markdown)", Hidden: true, Input: []string{common.File, common.Stdin}},
		{Name: "pattern", Desc: "regex pattern for str_replace", Hidden: true},
		{Name: "block-id", Desc: "target block ID for block_* operations", Hidden: true},
		{Name: "src-block-ids", Desc: "source block IDs (comma-separated) for block_copy_insert_after / block_move_after", Hidden: true},
		{Name: "revision-id", Desc: "base revision (-1 = latest)", Hidden: true, Type: "int", Default: "-1"},
	}
}

func validCommandsV2Keys() []string {
	return []string{"str_replace", "block_delete", "block_insert_after", "block_copy_insert_after", "block_replace", "block_move_after", "overwrite", "append"}
}

func validateUpdateV2(_ context.Context, runtime *common.RuntimeContext) error {
	if _, err := parseDocumentRef(runtime.Str("doc")); err != nil {
		return common.FlagErrorf("invalid --doc: %v", err)
	}
	cmd := runtime.Str("command")
	if cmd == "" {
		return common.FlagErrorf("--command is required")
	}
	if !validCommandsV2[cmd] {
		return common.FlagErrorf("invalid --command %q, valid: str_replace | block_delete | block_insert_after | block_copy_insert_after | block_replace | block_move_after | overwrite | append", cmd)
	}
	content := runtime.Str("content")
	pattern := runtime.Str("pattern")
	blockID := runtime.Str("block-id")
	srcBlockIDs := runtime.Str("src-block-ids")

	switch cmd {
	case "str_replace":
		if pattern == "" {
			return common.FlagErrorf("--command str_replace requires --pattern")
		}
	case "block_delete":
		if blockID == "" {
			return common.FlagErrorf("--command block_delete requires --block-id")
		}
	case "block_insert_after":
		if blockID == "" {
			return common.FlagErrorf("--command block_insert_after requires --block-id")
		}
		if content == "" {
			return common.FlagErrorf("--command block_insert_after requires --content")
		}
	case "block_copy_insert_after":
		if blockID == "" {
			return common.FlagErrorf("--command block_copy_insert_after requires --block-id")
		}
		if srcBlockIDs == "" {
			return common.FlagErrorf("--command block_copy_insert_after requires --src-block-ids")
		}
	case "block_move_after":
		if blockID == "" {
			return common.FlagErrorf("--command block_move_after requires --block-id")
		}
		if srcBlockIDs == "" {
			return common.FlagErrorf("--command block_move_after requires --src-block-ids")
		}
		if content != "" {
			return common.FlagErrorf("--command block_move_after does not accept --content; use --src-block-ids")
		}
	case "block_replace":
		if blockID == "" {
			return common.FlagErrorf("--command block_replace requires --block-id")
		}
		if content == "" {
			return common.FlagErrorf("--command block_replace requires --content")
		}
	case "overwrite":
		if content == "" {
			return common.FlagErrorf("--command overwrite requires --content")
		}
	case "append":
		if content == "" {
			return common.FlagErrorf("--command append requires --content")
		}
	}
	return nil
}

func dryRunUpdateV2(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	// Validate has already accepted --doc; parseDocumentRef cannot fail here.
	ref, _ := parseDocumentRef(runtime.Str("doc"))
	body := buildUpdateBody(runtime)
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
	body := buildUpdateBody(runtime)

	data, err := doDocAPI(runtime, "PUT", apiPath, body)
	if err != nil {
		return err
	}

	runtime.OutRaw(data, nil)
	return nil
}

func buildUpdateBody(runtime *common.RuntimeContext) map[string]interface{} {
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
