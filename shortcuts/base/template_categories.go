// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseTemplateCategories = common.Shortcut{
	Service:     "base",
	Command:     "+template-categories",
	Description: "List Base template center categories",
	Risk:        "read",
	Scopes:      []string{templateReadScope},
	AuthTypes:   authTypes(),
	Tips: []string{
		"Use this first when the user asks to browse template categories.",
		`Example: lark-cli base +template-categories --as user`,
	},
	DryRun: dryRunTemplateCategories,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTemplateCategories(runtime)
	},
}

func dryRunTemplateCategories(_ context.Context, _ *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/templates/category")
}

func executeTemplateCategories(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", "templates", "category"), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(projectTemplateCategoriesResponse(data), nil)
	return nil
}
