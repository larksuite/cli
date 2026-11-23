// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseAppBlockUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+app-block-update",
	Description: "Update a block on a BaseApp page",
	Risk:        "write",
	Scopes:      []string{"base:appmode_block:update", "base:appmode_block:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		appTokenFlag(true),
		pageIDFlag(true),
		appBlockIDFlag(true),
		{Name: "name", Desc: "new block name"},
		{Name: "data-config", Desc: "data_config JSON object; read lark-base-app-block-data-config.md for the SSOT"},
		{Name: "no-validate", Type: "bool", Desc: "skip local data_config normalization; send data_config as-is"},
	},
	Tips: []string{
		`lark-cli base +app-block-update --app-token <app_token> --page-id <page_id> --block-id <block_id> --name "Monthly sales"`,
		`lark-cli base +app-block-update --app-token <app_token> --page-id <page_id> --block-id <block_id> --data-config '{"base_token":"basxxx","data_sources":[{"table_name":"Orders","count_all":true,"filter":{"conjunction":"and","conditions":[{"field_name":"Status","operator":"is","value":"Closed"}]}}]}'`,
		"Do not call this command for a component whose +app-block-list result has type=unsupported; the API will return an error.",
		"Read lark-base-app-block-data-config.md as the SSOT; do not invent data_config from natural language.",
		"Use +app-block-get first to inspect the current data_config before replacing nested values.",
		"The type and sub_type of an existing Block are immutable after creation and are not part of data_config; +app-block-update accepts only the name and data_config fields. If a user asks to change type/sub_type, read the current Block and always state this constraint in the final answer, even when it already matches and no write is needed; if it differs, it can only be fixed in the UI.",
		"Only explicitly provided data_config fields are sent; omitted fields stay unchanged. For charts, passing data_sources replaces the whole ordered array, and changing base_token requires sending the full data_sources.",
		"Widget layout, position, size and display settings are not part of the public create/update protocol.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		name := strings.TrimSpace(runtime.Str("name"))
		raw := strings.TrimSpace(runtime.Str("data-config"))
		if name == "" && raw == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--name 与 --data-config 至少提供一个").WithParam("--name")
		}
		if runtime.Bool("no-validate") {
			return nil
		}
		if raw == "" {
			return nil
		}
		pc := newParseCtx(runtime)
		cfg, err := parseJSONObject(pc, raw, "data-config")
		if err != nil {
			return err
		}
		if containsJSONNull(cfg) {
			return formatDataConfigErrors([]string{"Update 不接受 null 作为清空标记"})
		}
		if problems := validateAppBlockUpdateTopLevelFields(cfg); len(problems) > 0 {
			return formatDataConfigErrors(problems)
		}
		// update 不传 type，无法做强类型校验；按多数据源图表结构归一化
		// （data_sources[] 存在时逐项归一化，否则原样透传）。
		norm := normalizeAppChartDataConfig(cfg)
		if sources, exists := norm["data_sources"]; exists {
			items, ok := sources.([]interface{})
			if !ok || len(items) == 0 {
				return formatDataConfigErrors([]string{"data_sources 一旦传入，必须是至少包含一项的完整有序数组"})
			}
			var problems []string
			for i, rawSource := range items {
				source, ok := rawSource.(map[string]interface{})
				if !ok {
					problems = append(problems, fmt.Sprintf("data_sources[%d] 必须是对象", i))
					continue
				}
				for _, problem := range validateAppChartDataSourceConfig(source) {
					problems = append(problems, fmt.Sprintf("data_sources[%d]: %s", i, problem))
				}
			}
			if len(problems) > 0 {
				return formatDataConfigErrors(problems)
			}
		}
		b, _ := json.Marshal(norm)
		_ = runtime.Cmd.Flags().Set("data-config", string(b))
		return nil
	},
	DryRun: dryRunAppBlockUpdate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeAppBlockUpdate(runtime)
	},
}

func validateAppBlockUpdateTopLevelFields(cfg map[string]interface{}) []string {
	allowed := map[string]bool{
		// Chart.
		"base_token": true, "data_sources": true, "data_source_mode": true, "sort": true,
		// List.
		"table_name": true, "filter": true, "sort_by": true, "columns": true,
		"group_by": true, "fields": true, "card_config": true, "detail_config": true,
		// Text.
		"text": true,
	}
	var problems []string
	for key := range cfg {
		if !allowed[key] {
			problems = append(problems, fmt.Sprintf("data_config 不支持字段 %s", key))
		}
	}
	return problems
}

func containsJSONNull(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case map[string]interface{}:
		for _, item := range typed {
			if containsJSONNull(item) {
				return true
			}
		}
	case []interface{}:
		for _, item := range typed {
			if containsJSONNull(item) {
				return true
			}
		}
	}
	return false
}
