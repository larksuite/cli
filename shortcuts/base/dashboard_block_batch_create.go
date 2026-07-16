// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

type dashboardBlockCreateSpec struct {
	Index int
	Body  map[string]interface{}
}

var BaseDashboardBlockBatchCreate = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-block-batch-create",
	Description: "Create multiple blocks in a dashboard sequentially",
	Risk:        "write",
	Scopes:      []string{"base:dashboard:create"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
		{Name: "blocks", Desc: "JSON array of block objects: [{\"name\":\"...\",\"type\":\"statistics\",\"data_config\":{...}}]. Use @file.json for long dashboards.", Required: true},
		{Name: "user-id-type", Desc: "user ID type for user fields in filters: open_id / union_id / user_id"},
		{Name: "no-validate", Type: "bool", Desc: "skip local data_config validation"},
	},
	Tips: []string{
		`lark-cli base +dashboard-block-batch-create --base-token <base_token> --dashboard-id <dashboard_id> --blocks '[{"name":"Order Count","type":"statistics","data_config":{"table_name":"Orders","count_all":true}},{"name":"Monthly Sales","type":"line","data_config":{"table_name":"Orders","series":[{"field_name":"Amount","rollup":"SUM"}],"group_by":[{"field_name":"Month","mode":"integrated","sort":{"type":"group","order":"asc"}}]}}]'`,
		"Use this when creating two or more blocks in the same dashboard; the CLI calls the platform sequentially for you.",
		"Before creating data-backed blocks, use +table-list and +field-list to confirm real table and field names.",
		"data_config uses table and field names, not table_id or field_id.",
		"Read dashboard-block-data-config.md as the SSOT for chart templates, filters, metric rules, and type-specific fields; do not invent data_config from natural language.",
		"The output is intentionally compact: created_count plus each created block's index, block_id, name, and type.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := parseDashboardBlockCreateSpecs(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		specs, _ := parseDashboardBlockCreateSpecs(runtime)
		blocks := make([]interface{}, 0, len(specs))
		for _, spec := range specs {
			blocks = append(blocks, spec.Body)
		}
		params := map[string]interface{}{}
		if uid := strings.TrimSpace(runtime.Str("user-id-type")); uid != "" {
			params["user_id_type"] = uid
		}
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks").
			Params(params).
			Body(map[string]interface{}{"blocks": blocks, "sequential": true}).
			Set("base_token", runtime.Str("base-token")).
			Set("dashboard_id", runtime.Str("dashboard-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardBlockBatchCreate(runtime)
	},
}

func parseDashboardBlockCreateSpecs(runtime *common.RuntimeContext) ([]dashboardBlockCreateSpec, error) {
	pc := newParseCtx(runtime)
	items, err := parseJSONArray(pc, runtime.Str("blocks"), "blocks")
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--blocks must contain at least one block").WithParam("--blocks")
	}
	specs := make([]dashboardBlockCreateSpec, 0, len(items))
	for index, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--blocks[%d] must be a JSON object", index).WithParam("--blocks")
		}
		body, err := buildDashboardBlockCreateBody(runtime, obj, index)
		if err != nil {
			return nil, err
		}
		specs = append(specs, dashboardBlockCreateSpec{Index: index, Body: body})
	}
	return specs, nil
}

func buildDashboardBlockCreateBody(runtime *common.RuntimeContext, obj map[string]interface{}, index int) (map[string]interface{}, error) {
	name, _ := obj["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--blocks[%d].name is required", index).WithParam("--blocks")
	}
	blockType, _ := obj["type"].(string)
	blockType = strings.TrimSpace(blockType)
	if blockType == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--blocks[%d].type is required", index).WithParam("--blocks")
	}
	body := map[string]interface{}{
		"name": name,
		"type": blockType,
	}
	if rawConfig, ok := obj["data-config"]; ok {
		obj["data_config"] = rawConfig
	}
	if rawConfig, ok := obj["data_config"]; ok {
		cfg, err := dashboardBlockDataConfigFromSpec(rawConfig, index)
		if err != nil {
			return nil, err
		}
		if !runtime.Bool("no-validate") {
			cfg = normalizeDataConfig(cfg)
			if problems := validateBlockDataConfig(blockType, cfg); len(problems) > 0 {
				return nil, formatDataConfigErrors(prefixDataConfigProblems(index, problems))
			}
		}
		body["data_config"] = cfg
	} else if strings.EqualFold(blockType, "text") && !runtime.Bool("no-validate") {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--blocks[%d].data_config.text is required for text blocks", index).WithParam("--blocks")
	}
	return body, nil
}

func dashboardBlockDataConfigFromSpec(raw interface{}, index int) (map[string]interface{}, error) {
	switch val := raw.(type) {
	case map[string]interface{}:
		return val, nil
	case string:
		var cfg map[string]interface{}
		if err := common.ParseJSON([]byte(strings.TrimSpace(val)), &cfg); err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--blocks[%d].data_config must be a JSON object or object-encoded string", index).WithParam("--blocks").WithCause(err)
		}
		if cfg == nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--blocks[%d].data_config must be a JSON object", index).WithParam("--blocks")
		}
		return cfg, nil
	default:
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--blocks[%d].data_config must be a JSON object", index).WithParam("--blocks")
	}
}

func prefixDataConfigProblems(index int, problems []string) []string {
	prefixed := make([]string, 0, len(problems))
	for _, problem := range problems {
		prefixed = append(prefixed, "blocks["+strconv.Itoa(index)+"].data_config: "+problem)
	}
	return prefixed
}

func executeDashboardBlockBatchCreate(runtime *common.RuntimeContext) error {
	specs, err := parseDashboardBlockCreateSpecs(runtime)
	if err != nil {
		return err
	}
	params := map[string]interface{}{}
	if userIDType := strings.TrimSpace(runtime.Str("user-id-type")); userIDType != "" {
		params["user_id_type"] = userIDType
	}
	created := make([]interface{}, 0, len(specs))
	for _, spec := range specs {
		data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "blocks"), params, spec.Body)
		if err != nil {
			return err
		}
		created = append(created, compactCreatedDashboardBlock(spec.Index, data))
	}
	runtime.Out(map[string]interface{}{
		"created":       true,
		"created_count": len(created),
		"dashboard_id":  runtime.Str("dashboard-id"),
		"blocks":        created,
	}, nil)
	return nil
}

func compactCreatedDashboardBlock(index int, data map[string]interface{}) map[string]interface{} {
	item := map[string]interface{}{"index": index}
	for _, key := range []string{"block_id", "id", "name", "type"} {
		if value, ok := data[key]; ok {
			item[key] = value
		}
	}
	return item
}
