// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewSetVisibleFields = common.Shortcut{
	Service:     "base",
	Command:     "+view-set-visible-fields",
	Description: "Set view visible fields",
	Risk:        "write",
	Scopes:      []string{"base:view:write_only"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		viewRefFlag(true),
		{Name: "json", Desc: `visible fields JSON object, e.g. {"visible_fields":["Name","Status"]}`, Required: true},
	},
	Tips: []string{
		"Supported view types: grid, kanban, gallery, calendar, gantt and form.",
		"For form views, use field IDs.",
		"Use a JSON object, not a bare array; primary field may be forced to the first position by the API.",
		"visible_fields controls both visibility and order; include every field that should remain visible.",
		"Entries must be unique (duplicates leave the final order undefined) and free of leading/trailing whitespace (values are sent as-is); both are rejected locally.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewVisibleFieldsJSON(runtime)
	},
	DryRun: dryRunViewSetVisibleFields,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetVisibleFields(runtime)
	},
}

// validateViewVisibleFieldsJSON 在通用的 view --json 校验之上，补 visible_fields 的
// 结构约束。不并入共用的 validateViewJSONObject：那个函数还被 set-group /
// set-sort / set-filter 等 5 条命令复用，它们的 --json 根本没有 visible_fields。
//
// visible_fields 同时决定可见性和顺序，且是整体覆盖：漏传的题目会被隐藏，
// 重复 ID 的排序结果由服务端决定，两者都不该等到线上才发现。
func validateViewVisibleFieldsJSON(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	obj, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return err
	}
	raw, ok := obj["visible_fields"]
	if !ok {
		return baseFlagErrorf(`--json must contain "visible_fields", e.g. {"visible_fields":["Name","Status"]}`)
	}
	fields, ok := raw.([]interface{})
	if !ok {
		return baseFlagErrorf("--json.visible_fields must be an array of field names or IDs, got %s", jsonTypeName(raw))
	}
	seen := make(map[string]int, len(fields))
	for i, item := range fields {
		name, ok := item.(string)
		if !ok {
			return baseFlagErrorf("--json.visible_fields[%d] must be a field name or ID string, got %s", i, jsonTypeName(item))
		}
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return baseFlagErrorf("--json.visible_fields[%d] must not be empty", i)
		}
		// 选择直接拒绝而不是替调用方 trim：这条命令是整体覆盖，值原样发给服务端，
		// 悄悄改写会让「我提交的数组」和「实际生效的数组」对不上，出问题更难查。
		// 文案只陈述本地要求，不承诺去掉空格一定能匹配上目标字段——字段名本身
		// 是否允许首尾空格由服务端决定，这里没有依据替它作答。
		if trimmed != name {
			return baseFlagErrorf("--json.visible_fields[%d] (%q) must not have leading or trailing whitespace; the value is sent to the server as-is", i, name)
		}
		if first, dup := seen[name]; dup {
			return baseFlagErrorf("--json.visible_fields[%d] duplicates item %d (%q); the array is the final order, list each field once", i, first, name)
		}
		seen[name] = i
	}
	return nil
}
