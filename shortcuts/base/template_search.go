// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseTemplateSearch = common.Shortcut{
	Service:     "base",
	Command:     "+template-search",
	Description: "Search Base templates by keyword",
	Risk:        "read",
	Scopes:      []string{templateReadScope},
	AuthTypes:   authTypes(),
	Flags: append([]common.Flag{
		{Name: "keyword", Required: true, Desc: "template keyword; empty search is not supported"},
	}, templatePaginationFlags()...),
	Tips: []string{
		"Use this when the user wants to create a new Base and has no owned/recent Base anchor.",
		"Do not use drive +search for marketplace templates; drive search only finds user-accessible Drive/Wiki objects.",
		"Returned template.token is the Base template token. To create from it, run +base-copy --base-token <token>.",
		`Example: lark-cli base +template-search --keyword "project management" --limit 10 --as user`,
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("keyword")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "template keyword must not be blank").WithParam("--keyword")
		}
		return validateTemplatePagination(ctx, runtime)
	},
	DryRun: dryRunTemplateSearch,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTemplateSearch(runtime)
	},
}

func dryRunTemplateSearch(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/templates/search").
		Params(templateSearchParams(runtime))
}

func executeTemplateSearch(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", "templates", "search"), templateSearchParams(runtime), nil)
	if err != nil {
		return err
	}
	runtime.Out(projectTemplateListResponse(data), nil)
	return nil
}

func templateSearchParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := templatePaginationParams(runtime)
	params["keyword"] = strings.TrimSpace(runtime.Str("keyword"))
	return params
}
