// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFormDetail = common.Shortcut{
	Service:     "base",
	Command:     "+form-detail",
	Description: "Get form detail by share token",
	Risk:        "read",
	Scopes:      []string{"base:form:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "share-token", Desc: "Form share token (share_token)", Required: true},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/tables/forms/detail").
			Body(map[string]interface{}{
				"share_token": runtime.Str("share-token"),
			})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := map[string]interface{}{
			"share_token": runtime.Str("share-token"),
		}

		data, err := baseV3Call(runtime, "POST",
			baseV3Path("bases", "tables", "forms", "detail"), nil, body)
		if err != nil {
			return err
		}

		runtime.Out(data, nil)
		return nil
	},
}
