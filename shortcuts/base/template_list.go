// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseTemplateList = common.Shortcut{
	Service:     "base",
	Command:     "+template-list",
	Description: "List Base templates by category",
	Risk:        "read",
	Scopes:      []string{templateReadScope},
	AuthTypes:   authTypes(),
	Flags: append([]common.Flag{
		{Name: "category-key", Desc: "template category key; omit to list the recommended category"},
	}, templatePaginationFlags()...),
	Tips: []string{
		"Use --category-key with a key returned by +template-categories; omit it to read the recommended category.",
		"Returned template.token is the Base template token. To create from it, run +base-copy --base-token <token>.",
		`Example: lark-cli base +template-list --category-key office --limit 10 --as user`,
	},
	Validate: validateTemplatePagination,
	DryRun:   dryRunTemplateList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTemplateList(runtime)
	},
}

func dryRunTemplateList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/templates").
		Params(templateListParams(runtime))
}

func executeTemplateList(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", "templates"), templateListParams(runtime), nil)
	if err != nil {
		return err
	}
	runtime.Out(projectTemplateListResponse(data), nil)
	return nil
}

func templateListParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := templatePaginationParams(runtime)
	if categoryKey := strings.TrimSpace(runtime.Str("category-key")); categoryKey != "" {
		params["category_key"] = categoryKey
	}
	return params
}
