// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRecordHistoryList = common.Shortcut{
	Service:     "base",
	Command:     "+record-history-list",
	Description: "List record change history",
	Risk:        "read",
	Scopes:      []string{"base:history:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		recordRefFlag(true),
		{Name: "max-version", Type: "int", Desc: "max version for next page"},
		{Name: "page-size", Type: "int", Default: "30", Desc: "pagination size"},
	},
	DryRun: dryRunRecordHistoryList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		params := map[string]interface{}{
			"table_id":  baseTableID(runtime),
			"record_id": runtime.Str("record-id"),
			"page_size": runtime.Int("page-size"),
		}
		if value := runtime.Int("max-version"); value > 0 {
			params["max_version"] = value
		}
		data, err := baseV3Call(runtime, "GET", baseV3Path("bases", runtime.Str("base-token"), "record_history"), params, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
