// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseBaseGet = common.Shortcut{
	Service:           "base",
	Command:           "+base-get",
	Description:       "Get a base resource",
	Risk:              "read",
	ConditionalScopes: []string{"wiki:node:retrieve"},
	Scopes:            []string{"base:app:read"},
	AuthTypes:         authTypes(),
	Flags:             []common.Flag{baseTokenFlag(true)},
	Tips: []string{
		"Accepts a Base token, a /base/ URL, or a /wiki/ URL (wiki links auto-resolve to the underlying bitable); workspace tokens are not accepted.",
	},
	DryRun: dryRunBaseGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseGet(runtime)
	},
}
