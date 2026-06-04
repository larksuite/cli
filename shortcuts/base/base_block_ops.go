// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var baseBlockTypeEnums = []string{"folder", "table", "docx", "dashboard", "workflow"}

func baseBlockIDFlag(required bool) common.Flag {
	return common.Flag{Name: "block-id", Desc: "block id", Required: required}
}

func dryRunBaseBlockList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/blocks/list").
		Body(buildBaseBlockListBody(runtime)).
		Set("base_token", runtime.Str("base-token"))
}

func dryRunBaseBlockCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/blocks").
		Body(buildBaseBlockCreateBody(runtime)).
		Set("base_token", runtime.Str("base-token"))
}

func dryRunBaseBlockMove(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/blocks/:block_id/move").
		Body(buildBaseBlockMoveBody(runtime)).
		Set("base_token", runtime.Str("base-token")).
		Set("block_id", runtime.Str("block-id"))
}

func dryRunBaseBlockRename(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/blocks/:block_id/rename").
		Body(map[string]interface{}{"name": strings.TrimSpace(runtime.Str("name"))}).
		Set("base_token", runtime.Str("base-token")).
		Set("block_id", runtime.Str("block-id"))
}

func dryRunBaseBlockDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		DELETE("/open-apis/base/v3/bases/:base_token/blocks/:block_id").
		Set("base_token", runtime.Str("base-token")).
		Set("block_id", runtime.Str("block-id"))
}

func validateBaseBlockCreate(runtime *common.RuntimeContext) error {
	if strings.TrimSpace(runtime.Str("name")) == "" {
		return common.FlagErrorf("--name must not be blank")
	}
	if strings.TrimSpace(runtime.Str("type")) == "" {
		return common.FlagErrorf("--type must not be blank")
	}
	return nil
}

func validateBaseBlockMove(runtime *common.RuntimeContext) error {
	if strings.TrimSpace(runtime.Str("before-id")) != "" && strings.TrimSpace(runtime.Str("after-id")) != "" {
		return common.FlagErrorf("--before-id and --after-id are mutually exclusive")
	}
	return nil
}

func validateBaseBlockRename(runtime *common.RuntimeContext) error {
	if strings.TrimSpace(runtime.Str("name")) == "" {
		return common.FlagErrorf("--name must not be blank")
	}
	return nil
}

func executeBaseBlockList(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "blocks", "list"), nil, buildBaseBlockListBody(runtime))
	if err != nil {
		return err
	}
	filterBaseBlockListData(data, strings.TrimSpace(runtime.Str("type")))
	runtime.Out(data, nil)
	return nil
}

func executeBaseBlockCreate(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "blocks"), nil, buildBaseBlockCreateBody(runtime))
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"block": data, "created": true}, nil)
	return nil
}

func executeBaseBlockMove(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "blocks", runtime.Str("block-id"), "move"), nil, buildBaseBlockMoveBody(runtime))
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"block": data, "moved": true}, nil)
	return nil
}

func executeBaseBlockRename(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "blocks", runtime.Str("block-id"), "rename"), nil, map[string]interface{}{
		"name": strings.TrimSpace(runtime.Str("name")),
	})
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"block": data, "renamed": true}, nil)
	return nil
}

func executeBaseBlockDelete(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "DELETE", baseV3Path("bases", runtime.Str("base-token"), "blocks", runtime.Str("block-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"block": data, "deleted": true}, nil)
	return nil
}

func buildBaseBlockListBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}
	if parentID := strings.TrimSpace(runtime.Str("parent-id")); parentID != "" {
		body["parent_id"] = parentID
	}
	return body
}

func filterBaseBlockListData(data map[string]interface{}, blockType string) {
	if blockType == "" {
		return
	}
	blocks, ok := data["blocks"].([]interface{})
	if !ok {
		return
	}
	filtered := make([]interface{}, 0, len(blocks))
	for _, block := range blocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok || blockMap["type"] != blockType {
			continue
		}
		filtered = append(filtered, block)
	}
	data["blocks"] = filtered
	data["total"] = len(filtered)
}

func buildBaseBlockCreateBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"type": strings.TrimSpace(runtime.Str("type")),
		"name": strings.TrimSpace(runtime.Str("name")),
	}
	if parentID := strings.TrimSpace(runtime.Str("parent-id")); parentID != "" {
		body["parent_id"] = parentID
	}
	return body
}

func buildBaseBlockMoveBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{"parent_id": nil}
	if parentID := strings.TrimSpace(runtime.Str("parent-id")); parentID != "" {
		body["parent_id"] = parentID
	}
	if beforeID := strings.TrimSpace(runtime.Str("before-id")); beforeID != "" {
		body["before_id"] = beforeID
	}
	if afterID := strings.TrimSpace(runtime.Str("after-id")); afterID != "" {
		body["after_id"] = afterID
	}
	return body
}
