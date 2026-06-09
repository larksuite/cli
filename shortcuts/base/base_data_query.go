// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseDataQuery = common.Shortcut{
	Service:     "base",
	Command:     "+data-query",
	Description: "Query and analyze Base data with JSON DSL (aggregation, filter, sort)",
	Risk:        "read",
	Scopes:      []string{"base:table:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "dsl", Desc: "query JSON DSL; read lark-base-data-query-guide.md first, then lark-base-data-query.md for the full DSL SSOT", Required: true},
	},
	Tips: []string{
		"Use +data-query for server-side aggregation, grouping, filtering, sorting, and Top N queries.",
		"Read lark-base-data-query-guide.md for common fewshots; use lark-base-data-query.md only when the full DSL reference is needed.",
		"`dimensions` and `measures` cannot both be empty.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		var dsl map[string]interface{}
		dec := json.NewDecoder(bytes.NewReader([]byte(runtime.Str("dsl"))))
		dec.UseNumber()
		if err := dec.Decode(&dsl); err != nil {
			return baseFlagErrorf("--dsl invalid JSON: %v", err)
		}
		_, hasDim := dsl["dimensions"]
		_, hasMeas := dsl["measures"]
		if !hasDim && !hasMeas {
			return baseFlagErrorf("--dsl must contain at least one of 'dimensions' or 'measures'")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		var dsl map[string]interface{}
		dec := json.NewDecoder(bytes.NewReader([]byte(runtime.Str("dsl"))))
		dec.UseNumber()
		dec.Decode(&dsl)
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/data/query").
			Body(dsl).
			Set("base_token", runtime.Str("base-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")

		var dsl map[string]interface{}
		dec := json.NewDecoder(bytes.NewReader([]byte(runtime.Str("dsl"))))
		dec.UseNumber()
		dec.Decode(&dsl)

		data, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "data/query"), nil, dsl)
		if err != nil {
			return err
		}

		runtime.Out(data, nil)
		return nil
	},
}
