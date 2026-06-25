// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldSearchOptions = common.Shortcut{
	Service:     "base",
	Command:     "+field-search-options",
	Description: "Search select options of a field",
	Risk:        "read",
	Scopes:      []string{"base:field:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
		{Name: "keyword", Desc: "keyword for option query"},
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Type: "int", Default: "30", Desc: "pagination size, range 1-200"},
		pageSizeLimitAliasFlag(),
	},
	Tips: []string{
		`Example: lark-cli base +field-search-options --base-token <base_token> --table-id <table_id> --field-id "Status" --keyword "Do"`,
		"Use only for fields with options, such as select or multi-select fields.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateLimitPageSizeAlias(runtime); err != nil {
			return err
		}
		if _, err := common.ValidatePageSizeTyped(runtime, "limit", 30, 1, 200); err != nil {
			return err
		}
		if runtime.Changed("page-size") {
			if _, err := common.ValidatePageSizeTyped(runtime, "page-size", 30, 1, 200); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: dryRunFieldSearchOptions,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldSearchOptions(runtime)
	},
}
