// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewGetFilter = common.Shortcut{
	Service:           "base",
	Command:           "+view-get-filter",
	Description:       "Get view filter configuration",
	Risk:              "read",
	ConditionalScopes: []string{"wiki:node:retrieve"},
	Scopes:            []string{"base:view:read"},
	AuthTypes:         authTypes(),
	Flags:             []common.Flag{baseTokenFlag(true), tableRefFlag(true), viewRefFlag(true)},
	DryRun:            dryRunViewGetFilter,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewGetProperty(runtime, "filter", "filter")
	},
}
